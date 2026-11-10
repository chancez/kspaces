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
//	new_tab = ["launch", "--type=tab"]
//	sessions = "~/.local/share/kitty/sessions"
//
//	[worktrees]
//	dir = ".worktrees"
//	branch_prefix = "pr/me"
//
//	[sidebar]
//	width = 28
//
//	[kitty]
//	auto_install = true
//	save_sessions = true
type Config struct {
	// Roots of the repositories shown as spaces, in the order they are listed.
	Spaces []string `toml:"spaces"`
	// The kitty action that opens the first tab of a worktree's session. kspaces appends --cwd and
	// --add-to-session to it.
	NewTab []string `toml:"new_tab"`
	// Where kitty session files are saved, named SESSION.kitty-session. A worktree whose session has no
	// windows but has a file here is restored from it rather than opened with NewTab. Empty to always use
	// NewTab.
	SessionsDir string    `toml:"sessions"`
	Worktrees   Worktrees `toml:"worktrees"`
	Sidebar     Sidebar   `toml:"sidebar"`
	Kitty       Kitty     `toml:"kitty"`
}

// Sidebar is how the sidebar's dock is laid out. kitty sizes the dock, so kitty.conf's watcher reads this
// rather than kspaces; kspaces checks it with the rest.
type Sidebar struct {
	// The dock's width in columns, the right border included.
	Width int `toml:"width"`
}

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

var defaultNewTab = []string{"launch", "--type=tab"}

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
	cfg := Config{NewTab: defaultNewTab, Worktrees: Worktrees{Dir: defaultWorktreeDir}, Sidebar: Sidebar{Width: DefaultSidebarWidth}}
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
	if len(cfg.NewTab) == 0 {
		return Config{}, fmt.Errorf("new_tab cannot be empty")
	}
	if w := cfg.Sidebar.Width; w < MinSidebarWidth || w > MaxSidebarWidth {
		return Config{}, fmt.Errorf("sidebar.width is %d, it must be from %d to %d", w, MinSidebarWidth, MaxSidebarWidth)
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
	return cfg, nil
}
