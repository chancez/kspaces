package config

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// SpaceFileName is a space's own config, in its main worktree: the layouts and commands of one repository,
// kept with it rather than in the global config, which is often in dotfiles. Every linked worktree of the
// space uses the main worktree's.
const SpaceFileName = ".kspaces.toml"

// SpaceFile is what a space's .kspaces.toml says, as ParseSpaceFile reads it:
//
//	layout = "agent"
//	worktree_layout = "agent"
//	pr_layout = "review"
//
//	[worktrees]
//	branch_prefix = "pr/me"
//
//	[[commands]]
//	name = "test"
//	run = ["make", "test"]
//
//	[[layouts]]
//	name = "agent"
//	tabs = [{ panes = [{ command = "test" }] }]
type SpaceFile struct {
	Commands []Command `toml:"commands"`
	Layouts  []Layout  `toml:"layouts"`
	// The space's defaults, which take the place of its [space.NAME]'s.
	Layout         string `toml:"layout"`
	WorktreeLayout string `toml:"worktree_layout"`
	PRLayout       string `toml:"pr_layout"`
	// Nil for a setting the file leaves to the global config.
	Dir          *string `toml:"-"`
	BranchPrefix *string `toml:"-"`
}

// spaceFileWorktrees is [worktrees] as a space file has it, telling a setting left out from one set empty.
type spaceFileWorktrees struct {
	Dir          *string `toml:"dir"`
	BranchPrefix *string `toml:"branch_prefix"`
}

// globalOnly are the global config's keys a space file cannot have: they are about every space, or name
// spaces, which a space's own file has no say in. group is a space table's, there for a file that sets it
// at its top, as it sets its layouts.
var globalOnly = []string{"spaces", "launch", "sessions", "sidebar", "kitty", "space", "groups", "group"}

// ParseSpaceFile reads a space file, its commands checked as the global config's are. Its layouts are
// checked when merged, by Config.WithSpace, as they can run the global config's commands. A key it does
// not know is an error, and so is one only the global config has.
func ParseSpaceFile(data string) (SpaceFile, error) {
	var raw struct {
		SpaceFile
		Worktrees spaceFileWorktrees `toml:"worktrees"`
	}
	md, err := toml.Decode(data, &raw)
	if err != nil {
		return SpaceFile{}, err
	}
	var global, unknown []string
	for _, k := range md.Undecoded() {
		switch {
		case slices.Contains(globalOnly, k[0]):
			if !slices.Contains(global, k[0]) {
				global = append(global, k[0])
			}
		default:
			unknown = append(unknown, k.String())
		}
	}
	switch {
	case slices.Contains(global, "space"):
		return SpaceFile{}, fmt.Errorf("space tables are for the global config; a space's file sets its own layout, worktree_layout and pr_layout at the top")
	case slices.Contains(global, "group"):
		return SpaceFile{}, fmt.Errorf("a space's group is set in the global config, as group in its [space.NAME]")
	case len(global) > 0:
		return SpaceFile{}, fmt.Errorf("only the global config can have %s", strings.Join(global, ", "))
	case len(unknown) > 0:
		return SpaceFile{}, fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
	}
	f := raw.SpaceFile
	f.Dir, f.BranchPrefix = raw.Worktrees.Dir, raw.Worktrees.BranchPrefix
	if err := checkCommands(f.Commands); err != nil {
		return SpaceFile{}, err
	}
	seen := map[string]bool{}
	for i, l := range f.Layouts {
		if l.Name == "" {
			return SpaceFile{}, fmt.Errorf("layouts: layout %d has no name", i+1)
		}
		if seen[l.Name] {
			return SpaceFile{}, fmt.Errorf("layouts: %s is defined twice", l.Name)
		}
		seen[l.Name] = true
	}
	return f, nil
}

// WithSpace is cfg as the space called name sees it with f, its file, merged in: f's commands and layouts
// added to cfg's, one called as one of cfg's taking its place; f's defaults in place of the space's
// [space.NAME] ones; and f's worktree settings in place of cfg's. The layouts are checked against the
// merged commands, and the defaults against the merged layouts, so f can use what cfg has.
func (cfg Config) WithSpace(name string, f SpaceFile) (Config, error) {
	out := cfg
	out.Commands = slices.Clone(cfg.Commands)
	for _, c := range f.Commands {
		if i := slices.IndexFunc(out.Commands, func(x Command) bool { return x.Name == c.Name }); i >= 0 {
			out.Commands[i] = c
		} else {
			out.Commands = append(out.Commands, c)
		}
	}
	// Each layout is checked again, as a command it runs may be the space's now, and checking fills in its
	// panes: copies, so cfg's are left as they were.
	out.Layouts = nil
	for _, l := range slices.Concat(cfg.Layouts, f.Layouts) {
		l.Tabs = slices.Clone(l.Tabs)
		for t := range l.Tabs {
			l.Tabs[t].Panes = slices.Clone(l.Tabs[t].Panes)
		}
		if i := slices.IndexFunc(out.Layouts, func(x Layout) bool { return x.Name == l.Name }); i >= 0 {
			out.Layouts[i] = l
		} else {
			out.Layouts = append(out.Layouts, l)
		}
	}
	if f.Layout != "" || f.WorktreeLayout != "" || f.PRLayout != "" {
		s := cfg.Space[name]
		out.Space = maps.Clone(cfg.Space)
		if out.Space == nil {
			out.Space = map[string]SpaceSettings{}
		}
		// The file's linked default falls back on its own main one before the space table's. The group is
		// the table's alone.
		out.Space[name] = SpaceSettings{
			Layout:         cmp.Or(f.Layout, s.Layout),
			WorktreeLayout: cmp.Or(f.WorktreeLayout, f.Layout, s.WorktreeLayout),
			PRLayout:       cmp.Or(f.PRLayout, s.PRLayout),
			Group:          s.Group,
		}
	}
	if f.Dir != nil {
		out.Worktrees.Dir = *f.Dir
	}
	if f.BranchPrefix != nil {
		out.Worktrees.BranchPrefix = *f.BranchPrefix
	}
	// The defaults before the layouts, which would blame the space table the file's take the place of.
	for _, d := range []struct{ key, layout string }{{"layout", f.Layout}, {"worktree_layout", f.WorktreeLayout}, {"pr_layout", f.PRLayout}} {
		if d.layout != "" && !slices.ContainsFunc(out.Layouts, func(l Layout) bool { return l.Name == d.layout }) {
			return Config{}, fmt.Errorf("%s is %q, no layout has that name", d.key, d.layout)
		}
	}
	if err := checkLayouts(&out); err != nil {
		return Config{}, err
	}
	return out, nil
}
