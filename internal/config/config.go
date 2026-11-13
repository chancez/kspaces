// Package config reads kspaces' config file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is what ~/.config/kspaces/config.toml says:
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
	// Roots of the repositories shown as spaces, in the order they are listed.
	Spaces []string `toml:"spaces"`
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
	// The dock's width in columns, the right border included.
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

// Load reads $XDG_CONFIG_HOME/kspaces/config.toml, by default ~/.config/kspaces/config.toml.
func Load() (Config, error) {
	path, err := filePath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading the config: %w", err)
	}
	cfg, err := Parse(string(data))
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse reads the config, filling in defaults and expanding a leading ~/ in paths. A key it does not
// know is an error, so a typo is not silently ignored.
func Parse(data string) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("finding the home directory: %w", err)
	}
	cfg := Config{Launch: defaultLaunch, Worktrees: Worktrees{Dir: defaultWorktreeDir}, Sidebar: Sidebar{Width: DefaultSidebarWidth, Popup: PopupOverlay}}
	md, err := toml.Decode(data, &cfg)
	if err != nil {
		return Config{}, err
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = k.String()
		}
		return Config{}, fmt.Errorf("unknown keys: %s", strings.Join(names, ", "))
	}
	// The toml package skips a value of the wrong type for a map rather than failing, and `space` is also
	// what a typo of `spaces` looks like.
	if typ := md.Type("space"); typ != "" && typ != "Hash" {
		return Config{}, fmt.Errorf("space must be [space.NAME] tables; the list of spaces is spaces")
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
	expand := func(p string) string {
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			return filepath.Join(home, rest)
		}
		return p
	}
	for i, s := range cfg.Spaces {
		cfg.Spaces[i] = expand(s)
	}
	cfg.SessionsDir = expand(cfg.SessionsDir)
	if err := checkLayouts(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
