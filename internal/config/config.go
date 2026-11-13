// Package config reads kspaces' config files.
package config

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is what ~/.config/kspaces/config.toml says, with the other config files merged in, see readFiles:
//
//	spaces = ["~/projects/gr", "~/projects/kitty"]
//	groups = ["work", "personal"]
//	launch = ["launch"]
//	sessions = "~/.local/share/kitty/sessions"
//
//	[worktrees]
//	dir = ".worktrees"
//	branch_prefix = "pr/me"
//
//	[sidebar]
//	width = 28
//	popup = "overlay"
//
//	[kitty]
//	auto_install = true
//	save_sessions = true
//
//	[[commands]]
//	name = "gr"
//	run = ["gr"]
type Config struct {
	// Roots of the repositories shown as spaces, in the order they are listed. The file can list each as
	// a path or as an inline table with a path, see spaceEntries, which Parse reads.
	Spaces []string `toml:"-"`
	// The kitty action every window kspaces opens for a session goes through, such as a worktree's first
	// tab, with kspaces' own options appended: --type, --cwd, --add-to-session and the like, then any
	// command. A kitten that passes them on to launch can stand in for it.
	Launch []string `toml:"launch"`
	// Where kitty session files are saved, named SESSION.kitty-session. A worktree whose session has no
	// windows but has a file here is restored from it rather than opened with Launch. Empty to always use
	// Launch.
	SessionsDir string    `toml:"sessions"`
	Worktrees   Worktrees `toml:"worktrees"`
	Sidebar     Sidebar   `toml:"sidebar"`
	Kitty       Kitty     `toml:"kitty"`
	// Commands to run in a worktree, see Command.
	Commands []Command `toml:"commands"`
	// Layouts to open in a session, see Layout.
	Layouts []Layout `toml:"layouts"`
	// The layout a worktree's session opens with afresh, when the space says none; empty for a shell.
	DefaultLayout string `toml:"layout"`
	// What each space, by name, says of itself.
	Space map[string]SpaceSettings `toml:"space"`
	// The order of the groups spaces are in, see SpaceSettings.Group. A group it does not list comes
	// after those it does, by name.
	Groups []string `toml:"groups"`
}

// Sidebar is how the sidebar's dock is laid out, and where its forms and the picker open. kitty sizes the
// dock, so kitty.conf's watcher reads Width rather than kspaces; kspaces checks it with the rest.
type Sidebar struct {
	// The dock's width in columns.
	Width int `toml:"width"`
	// Where the forms and the picker open: PopupOverlay or PopupPanel.
	Popup string `toml:"popup"`
}

// The places a form or the picker can open: an overlay over the window it is for, or a kitty panel, an
// OS window of its own of a fixed size, centered on the screen.
const (
	PopupOverlay = "overlay"
	PopupPanel   = "panel"
)

// Kitty is what `kspaces kitty install` puts in kitty's config directory.
type Kitty struct {
	// AutoInstall makes the sidebar rewrite the installed files when they are not what this kspaces would
	// install, rather than only saying so.
	AutoInstall bool `toml:"auto_install"`
	// SaveSessions installs the session saver, which saves each kitty session to its own file in kitty's
	// sessions directory and restores them at startup.
	SaveSessions bool `toml:"save_sessions"`
}

// The sidebar's width when none is set, and the range a width must be in to leave room for names and for
// the window beside it.
const (
	DefaultSidebarWidth = 28
	MinSidebarWidth     = 12
	MaxSidebarWidth     = 120
)

// Worktrees are the defaults of the new worktree form.
type Worktrees struct {
	// Where new worktrees go, relative to their space's root.
	Dir string `toml:"dir"`
	// What new branch names start with, followed by a /, such as `pr/me`. Empty for nothing.
	BranchPrefix string `toml:"branch_prefix"`
}

var defaultLaunch = []string{"launch"}

const defaultWorktreeDir = ".worktrees"

func filePath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kspaces", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "kspaces", "config.toml"), nil
}

// Load reads the config files, see readFiles, merged as parse merges them. Without any, the error is
// config.toml's, fs.ErrNotExist.
func Load() (Config, error) {
	files, err := readFiles()
	if err != nil {
		return Config{}, err
	}
	if files.missing != nil && len(files.paths) == 1 {
		return Config{}, fmt.Errorf("reading the config: %w", files.missing)
	}
	return parse(files.paths, files.texts)
}

// Parse reads the config, filling in defaults and expanding a leading ~/ in paths. A key it does not
// know is an error, so a typo is not silently ignored.
func Parse(data string) (Config, error) {
	return parse([]string{""}, []string{data})
}

// parse reads the config the texts of the files at paths make, in order, see merge. Each error names the
// file it is in: one file's own the file, and one of them merged the directory, or config.toml when it is
// the only file. A path "" names nothing, as for Parse.
func parse(paths, texts []string) (Config, error) {
	in := func(path string, err error) error {
		if path == "" {
			return err
		}
		return fmt.Errorf("%s: %w", path, err)
	}
	parts := make([]decoded, len(texts))
	for i, text := range texts {
		p, err := decode(text)
		if err != nil {
			return Config{}, in(paths[i], err)
		}
		parts[i] = p
	}
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = cmp.Or(filepath.Base(path), "config.toml")
	}
	cfg, err := merge(names, parts)
	if err != nil {
		if len(paths) > 1 && paths[0] != "" {
			return Config{}, in(filepath.Dir(paths[0]), err)
		}
		return Config{}, in(paths[0], err)
	}
	return cfg, nil
}

// decoded is one config file as decode reads it, before it is merged with the others: what it says, with
// none of the defaults, and which keys it sets.
type decoded struct {
	cfg Config
	md  toml.MetaData
	// The [space.NAME] keys the file sets, by the space's name, its spaces entries' included.
	spaceKeys map[string][]string
}

// decode reads one config file, with what can be checked of it alone: its keys, its spaces entries, and
// a key a space's entry and its [space.NAME] table both set. A leading ~/ in a path is expanded.
func decode(data string) (decoded, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return decoded{}, fmt.Errorf("finding the home directory: %w", err)
	}
	var file struct {
		Config
		Spaces spaceEntries `toml:"spaces"`
	}
	md, err := toml.Decode(data, &file)
	if err != nil {
		return decoded{}, err
	}
	var names []string
	for _, k := range md.Undecoded() {
		// The entries of spaces are read by spaceEntries, which says what is wrong with their keys.
		if len(k) > 1 && k[0] == "spaces" {
			continue
		}
		names = append(names, k.String())
	}
	if len(names) > 0 {
		return decoded{}, fmt.Errorf("unknown keys: %s", strings.Join(names, ", "))
	}
	// The toml package skips a value of the wrong type for a map rather than failing, and `space` is also
	// what a typo of `spaces` looks like.
	if typ := md.Type("space"); typ != "" && typ != "Hash" {
		return decoded{}, fmt.Errorf("space must be [space.NAME] tables; the list of spaces is spaces")
	}
	cfg := file.Config
	expand := func(p string) string {
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			return filepath.Join(home, rest)
		}
		return p
	}
	keys := map[string][]string{}
	for name, s := range cfg.Space {
		v := reflect.ValueOf(s)
		for i := range v.NumField() {
			if key := v.Type().Field(i).Tag.Get("toml"); md.IsDefined("space", name, key) {
				keys[name] = append(keys[name], key)
			}
		}
	}
	for _, e := range file.Spaces {
		cfg.Spaces = append(cfg.Spaces, expand(e.path))
		if len(e.keys) > 0 {
			name := filepath.Base(e.path)
			keys[name] = append(keys[name], e.keys...)
		}
	}
	if err := mergeSpaceEntries(&cfg, file.Spaces, func(name, key string) bool { return md.IsDefined("space", name, key) }); err != nil {
		return decoded{}, err
	}
	cfg.SessionsDir = expand(cfg.SessionsDir)
	return decoded{cfg: cfg, md: md, spaceKeys: keys}, nil
}

// merge is the config the decoded files make, called names, in order, with the defaults for what none
// sets, checked as a whole. Their spaces, groups, commands and layouts are listed one file's after
// another's, and their [space.NAME] tables merged by key. Every other setting, and each key of a space,
// is one file's to set: set in two, or a command, layout or group in two, it is an error naming both,
// rather than one silently winning.
func merge(names []string, parts []decoded) (Config, error) {
	cfg := Config{Launch: defaultLaunch, Worktrees: Worktrees{Dir: defaultWorktreeDir}, Sidebar: Sidebar{Width: DefaultSidebarWidth, Popup: PopupOverlay}}
	// The file that set each setting, a space's by "space.NAME.KEY", and each command, layout and group.
	setBy := map[string]int{}
	// once records file i setting key, what is called name, or says it is done twice: done is how, as in
	// "is set", and do as in "set it".
	once := func(key, name string, i int, done, do string) error {
		if j, ok := setBy[key]; ok && j != i {
			return fmt.Errorf("%s is %s both in %s and in %s; %s it in one", name, done, names[j], names[i], do)
		}
		setBy[key] = i
		return nil
	}
	for i, p := range parts {
		cfg.Spaces = append(cfg.Spaces, p.cfg.Spaces...)
		for _, c := range p.cfg.Commands {
			if err := once("commands."+c.Name, c.Name, i, "defined", "define"); err != nil {
				return Config{}, fmt.Errorf("commands: %w", err)
			}
		}
		for _, l := range p.cfg.Layouts {
			if err := once("layouts."+l.Name, l.Name, i, "defined", "define"); err != nil {
				return Config{}, fmt.Errorf("layouts: %w", err)
			}
		}
		for _, g := range p.cfg.Groups {
			if err := once("groups."+g, g, i, "listed", "list"); err != nil {
				return Config{}, fmt.Errorf("groups: %w", err)
			}
		}
		cfg.Commands = append(cfg.Commands, p.cfg.Commands...)
		cfg.Layouts = append(cfg.Layouts, p.cfg.Layouts...)
		cfg.Groups = append(cfg.Groups, p.cfg.Groups...)
		err := mergeSettings(reflect.ValueOf(&cfg).Elem(), reflect.ValueOf(p.cfg), nil, p.md, func(key string) error {
			return once(key, key, i, "set", "set")
		})
		if err != nil {
			return Config{}, err
		}
		for _, name := range slices.Sorted(maps.Keys(p.cfg.Space)) {
			s, from := cfg.Space[name], p.cfg.Space[name]
			for _, key := range p.spaceKeys[name] {
				if err := once("space."+name+"."+key, name+"'s "+key, i, "set", "set"); err != nil {
					return Config{}, err
				}
				dst, _ := spaceField(&s, key)
				src, _ := spaceField(&from, key)
				dst.Set(src)
			}
			if cfg.Space == nil {
				cfg.Space = map[string]SpaceSettings{}
			}
			cfg.Space[name] = s
		}
	}
	if len(cfg.Launch) == 0 {
		return Config{}, fmt.Errorf("launch cannot be empty")
	}
	if w := cfg.Sidebar.Width; w < MinSidebarWidth || w > MaxSidebarWidth {
		return Config{}, fmt.Errorf("sidebar.width is %d, it must be from %d to %d", w, MinSidebarWidth, MaxSidebarWidth)
	}
	if p := cfg.Sidebar.Popup; p != PopupOverlay && p != PopupPanel {
		return Config{}, fmt.Errorf("sidebar.popup is %q, it must be %q or %q", p, PopupOverlay, PopupPanel)
	}
	if err := checkCommands(cfg.Commands); err != nil {
		return Config{}, err
	}
	if err := checkGroups(cfg.Groups); err != nil {
		return Config{}, err
	}
	if err := checkLayouts(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// listed are Config's keys merge lists one file's after another's, or merges by key, rather than sets.
var listed = []string{"commands", "layouts", "groups", "space"}

// mergeSettings sets each setting of dst, a Config or a table of it at key, that md says src sets, after
// set says it can be: the keys of tables, such as sidebar.width, each a setting.
func mergeSettings(dst, src reflect.Value, key []string, md toml.MetaData, set func(key string) error) error {
	for i := range dst.NumField() {
		f := dst.Type().Field(i)
		tag := f.Tag.Get("toml")
		if tag == "" || tag == "-" || len(key) == 0 && slices.Contains(listed, tag) {
			continue
		}
		k := append(slices.Clone(key), tag)
		if f.Type.Kind() == reflect.Struct {
			if err := mergeSettings(dst.Field(i), src.Field(i), k, md, set); err != nil {
				return err
			}
			continue
		}
		if !md.IsDefined(k...) {
			continue
		}
		if err := set(strings.Join(k, ".")); err != nil {
			return err
		}
		dst.Field(i).Set(src.Field(i))
	}
	return nil
}

// checkGroups says what is wrong with the order of groups: a name empty, or listed twice.
func checkGroups(groups []string) error {
	seen := map[string]bool{}
	for i, g := range groups {
		if g == "" {
			return fmt.Errorf("groups: group %d has no name", i+1)
		}
		if seen[g] {
			return fmt.Errorf("groups: %s is listed twice", g)
		}
		seen[g] = true
	}
	return nil
}

// Tilde is path with the home directory written as ~/, the way the config can say it, or path as it is
// outside the home directory.
func Tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, strings.TrimSuffix(home, "/")+"/"); ok && rest != "" {
		return "~/" + rest
	}
	return path
}
