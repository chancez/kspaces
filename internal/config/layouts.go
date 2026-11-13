package config

import (
	"cmp"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// Layout is tabs to open in a worktree's session, each split into panes with a program in each:
//
//	[[layouts]]
//	name = "agent"
//	[[layouts.tabs]]
//	title = "{branch}"
//	panes = [
//	  { command = "claude" },
//	  { command = "review", split = "right", size = 35 },
//	  { split = "below", of = 1, size = 30, cwd = "{worktree}/docs" },
//	]
type Layout struct {
	Name string      `toml:"name"`
	Tabs []LayoutTab `toml:"tabs"`
	// What its panes' commands ask for, merged by name in the order they come, filled in by Parse: the
	// same name in two commands is one input, so one value fills both.
	Inputs []Input `toml:"-"`
}

// LayoutTab is a tab of a layout, in kitty's splits layout unless it says another.
type LayoutTab struct {
	// The tab's title, a template; empty leaves it to kitty, the active window's title.
	Title string `toml:"title"`
	// The kitty layout the tab is set to, one of KittyLayouts; empty for splits. Only splits places panes
	// by split, of and size: in any other the panes go in order and the layout arranges them.
	Layout string `toml:"layout"`
	// The first is the tab's first window; in splits each after it splits one before it.
	Panes []Pane `toml:"panes"`
}

// Pane is a window of a layout's tab, running a configured command, a command line, or neither, the
// shell.
type Pane struct {
	// The name of a configured command, which brings its env and inputs.
	Command string `toml:"command"`
	// A command line, a template as a command's.
	Run []string `toml:"run"`
	// The pane's working directory, a template; the worktree's, {worktree}, when empty.
	Cwd string `toml:"cwd"`
	// Which side of pane Of the pane goes on, one of Splits. Not for the first pane.
	Split string `toml:"split"`
	// The pane split, from 1, the one before by default.
	Of int `toml:"of"`
	// The pane's share of what pane Of had, in percent; 50 by default.
	Size int `toml:"size"`
	// Focus ends the build with this pane focused, rather than the first.
	Focus bool `toml:"focus"`
}

// Splits are the sides a pane can be split off on. kitty's splits layout puts a new window after the one
// it splits, so to the right or below; a pane on the left or above is the earlier pane instead.
var Splits = []string{"right", "below"}

// SplitsLayout is the kitty layout a tab is in when it names none, the one that splits a given window on
// a given axis at a given size.
const SplitsLayout = "splits"

// KittyLayouts are kitty's layouts, which a tab can name. kitty only switches a tab to one that is in
// enabled_layouts.
var KittyLayouts = []string{"fat", "grid", "horizontal", SplitsLayout, "stack", "tall", "vertical"}

// SpaceSettings are what [space.NAME] says of the space NAME.
type SpaceSettings struct {
	// The layout the space's main worktree opens with afresh.
	Layout string `toml:"layout"`
	// The layout its linked worktrees open with afresh, Layout when empty.
	WorktreeLayout string `toml:"worktree_layout"`
	// The layout the new worktree form opens a worktree for a PR with, see Config.PRLayoutFor.
	PRLayout string `toml:"pr_layout"`
	// The group the sidebar lists the space under, such as work; empty for none. Only the global config
	// sets it: a group is about how every space is shown, which one space's own file has no say in.
	Group string `toml:"group"`
}

// checkLayouts fills in the layouts' defaults and inputs and says what is wrong with the first that has a
// problem, naming it, then with the defaults that name them. The commands must be checked first.
func checkLayouts(cfg *Config) error {
	seen := map[string]bool{}
	for i := range cfg.Layouts {
		l := &cfg.Layouts[i]
		if l.Name == "" {
			return fmt.Errorf("layouts: layout %d has no name", i+1)
		}
		if seen[l.Name] {
			return fmt.Errorf("layouts: %s is defined twice", l.Name)
		}
		seen[l.Name] = true
		if err := cfg.checkLayout(l); err != nil {
			return fmt.Errorf("layouts: %s: %w", l.Name, err)
		}
	}
	known := func(where, name string) error {
		if name != "" && !seen[name] {
			return fmt.Errorf("%s is %q, no layout has that name", where, name)
		}
		return nil
	}
	if err := known("layout", cfg.DefaultLayout); err != nil {
		return err
	}
	if err := known("pr_layout", cfg.DefaultPRLayout); err != nil {
		return err
	}
	names := map[string]bool{}
	for _, root := range cfg.Spaces {
		names[filepath.Base(root)] = true
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Space)) {
		if !names[name] {
			return fmt.Errorf("space.%s: no space is named %s", name, name)
		}
		s := cfg.Space[name]
		if err := known("space."+name+".layout", s.Layout); err != nil {
			return err
		}
		if err := known("space."+name+".worktree_layout", s.WorktreeLayout); err != nil {
			return err
		}
		if err := known("space."+name+".pr_layout", s.PRLayout); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *Config) checkLayout(l *Layout) error {
	if len(l.Tabs) == 0 {
		return fmt.Errorf("it has no tabs")
	}
	// The inputs first, as any template may use them.
	l.Inputs = nil
	from := map[string]string{}
	for _, tab := range l.Tabs {
		for _, p := range tab.Panes {
			cmd, ok := cfg.Command(p.Command)
			if !ok {
				continue
			}
			for _, in := range cmd.Inputs {
				j := slices.IndexFunc(l.Inputs, func(x Input) bool { return x.Name == in.Name })
				switch {
				case j < 0:
					l.Inputs = append(l.Inputs, in)
					from[in.Name] = cmd.Name
				case !reflect.DeepEqual(l.Inputs[j], in):
					return fmt.Errorf("input %s is defined differently by %s and %s", in.Name, from[in.Name], cmd.Name)
				}
			}
		}
	}
	known := map[string]bool{}
	for _, b := range Builtins {
		known[b] = true
	}
	for _, in := range l.Inputs {
		known[in.Name] = true
	}
	for t := range l.Tabs {
		tab := &l.Tabs[t]
		if len(tab.Panes) == 0 {
			return fmt.Errorf("tab %d has no panes", t+1)
		}
		if err := checkTemplate(fmt.Sprintf("tab %d title", t+1), tab.Title, known); err != nil {
			return err
		}
		if tab.Layout != "" && !slices.Contains(KittyLayouts, tab.Layout) {
			return fmt.Errorf("tab %d layout is %q, it must be one of kitty's %s, and in its enabled_layouts", t+1, tab.Layout, strings.Join(KittyLayouts, ", "))
		}
		focus := 0
		for i := range tab.Panes {
			p := &tab.Panes[i]
			if err := checkPane(p, i, tab.KittyLayout(), known, cfg); err != nil {
				return fmt.Errorf("tab %d pane %d: %w", t+1, i+1, err)
			}
			if p.Focus {
				focus++
			}
		}
		if focus > 1 {
			return fmt.Errorf("tab %d has %d panes with focus, it can have one", t+1, focus)
		}
	}
	return nil
}

// checkPane checks p, pane i from 0 of its tab in kitty's layout, and fills in where it splits.
func checkPane(p *Pane, i int, layout string, known map[string]bool, cfg *Config) error {
	switch {
	case p.Command != "" && p.Run != nil:
		return fmt.Errorf("it has both command and run, it can have one")
	case p.Command != "":
		if _, ok := cfg.Command(p.Command); !ok {
			return fmt.Errorf("command is %q, no command has that name", p.Command)
		}
	case p.Run != nil:
		if len(p.Run) == 0 {
			return fmt.Errorf("run cannot be empty")
		}
		if err := checkRun(p.Run, known); err != nil {
			return err
		}
	}
	if err := checkTemplate("cwd", p.Cwd, known); err != nil {
		return err
	}
	placed := p.Split != "" || p.Of != 0 || p.Size != 0
	switch {
	case i == 0 && placed:
		return fmt.Errorf("the first pane is the tab's first window, it takes no split, of or size")
	case layout != SplitsLayout && placed:
		return fmt.Errorf("the tab's layout is %s, which places its panes in order: split, of and size are only for splits", layout)
	case i == 0 || layout != SplitsLayout:
		return nil
	}
	if !slices.Contains(Splits, p.Split) {
		return fmt.Errorf("split is %q, it must be one of %s; a pane left of or above another goes before it", p.Split, strings.Join(Splits, ", "))
	}
	if p.Of == 0 {
		p.Of = i
	}
	if p.Of < 1 || p.Of > i {
		return fmt.Errorf("of is %d, it must be a pane before this one, from 1 to %d", p.Of, i)
	}
	if p.Size == 0 {
		p.Size = 50
	}
	if p.Size < 1 || p.Size > 99 {
		return fmt.Errorf("size is %d, it must be from 1 to 99", p.Size)
	}
	return nil
}

// Layout is the configured layout called name.
func (cfg Config) Layout(name string) (Layout, bool) {
	i := slices.IndexFunc(cfg.Layouts, func(l Layout) bool { return l.Name == name })
	if i < 0 {
		return Layout{}, false
	}
	return cfg.Layouts[i], true
}

// LayoutFor is the name of the layout a worktree of the space called space opens with afresh, a linked
// one if linked, or empty for a shell: a linked worktree's is the space's worktree_layout, else its layout;
// the main worktree's its layout; else the global layout.
func (cfg Config) LayoutFor(space string, linked bool) string {
	s := cfg.Space[space]
	switch {
	case linked && s.WorktreeLayout != "":
		return s.WorktreeLayout
	case s.Layout != "":
		return s.Layout
	}
	return cfg.DefaultLayout
}

// PRLayoutFor is the name of the layout the new worktree form opens a worktree of the space called space
// with when it checks out a PR, or empty for a shell: the space's pr_layout, else the global pr_layout,
// else a linked worktree's, see LayoutFor.
func (cfg Config) PRLayoutFor(space string) string {
	return cmp.Or(cfg.Space[space].PRLayout, cfg.DefaultPRLayout, cfg.LayoutFor(space, true))
}

// Values are the layout's inputs as given, by name, the default for any not given. An input it does not
// have, or a required one left empty, is an error.
func (l Layout) Values(given map[string]string) (map[string]string, error) {
	return inputValues(l.Name, l.Inputs, given)
}

// PaneCommand is what pane p of l runs: its configured command, or its run as a command with l's inputs.
// False for the shell.
func (cfg Config) PaneCommand(l Layout, p Pane) (Command, bool) {
	if p.Command != "" {
		return cfg.Command(p.Command)
	}
	if p.Run != nil {
		return Command{Name: l.Name, Run: p.Run, Inputs: l.Inputs}, true
	}
	return Command{}, false
}

// TitleWith is the tab's title with vars substituted, as Command.Argv does. Its newlines, as an input's
// value can have, become spaces, as a tab's title is one line.
func (t LayoutTab) TitleWith(vars map[string]string) string {
	return oneLine.Replace(expand(t.Title, vars))
}

var oneLine = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// KittyLayout is the kitty layout the tab is set to.
func (t LayoutTab) KittyLayout() string {
	if t.Layout == "" {
		return SplitsLayout
	}
	return t.Layout
}

// CwdWith is the pane's working directory with vars substituted, the worktree's when it names none.
func (p Pane) CwdWith(vars map[string]string) string {
	if p.Cwd == "" {
		return vars["worktree"]
	}
	return expand(p.Cwd, vars)
}
