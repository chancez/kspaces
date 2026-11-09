package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is what ~/.config/kspaces/config says.
type Config struct {
	// Roots of the repositories shown as spaces, in the order they are listed.
	Spaces []string
	// The kitty action that opens the first tab of a worktree's session, split into words. kspaces appends
	// --cwd and --add-to-session to it.
	NewTab []string
}

var defaultNewTab = []string{"launch", "--type=tab"}

func configPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kspaces", "config"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "kspaces", "config"), nil
}

func loadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading the config: %w", err)
	}
	defer f.Close()
	cfg, err := parseConfig(bufio.NewScanner(f))
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// parseConfig reads lines of the form `KEY VALUE`. Blank lines and lines starting with # are ignored.
//
//	space ~/projects/gr
//	new_tab kitten cm_launch.py --type=tab
func parseConfig(sc *bufio.Scanner) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("finding the home directory: %w", err)
	}
	cfg := Config{NewTab: defaultNewTab}
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		value = strings.TrimSpace(value)
		if value == "" {
			return Config{}, fmt.Errorf("line %d: %q has no value", n, key)
		}
		switch key {
		case "space":
			if rest, ok := strings.CutPrefix(value, "~/"); ok {
				value = filepath.Join(home, rest)
			}
			cfg.Spaces = append(cfg.Spaces, value)
		case "new_tab":
			cfg.NewTab = strings.Fields(value)
		default:
			return Config{}, fmt.Errorf("line %d: unknown key %q", n, key)
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
