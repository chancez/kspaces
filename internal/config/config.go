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
}

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
	cfg := Config{NewTab: defaultNewTab, Worktrees: Worktrees{Dir: defaultWorktreeDir}}
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
