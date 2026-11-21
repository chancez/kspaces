// Package state is what kspaces keeps across restarts that is not its config: one JSON file,
// $XDG_STATE_HOME/kspaces/state.json, beside the log.
package state

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Version is the file's format. A file of another version is read as far as its fields allow.
const Version = 1

// State is the file's content.
type State struct {
	Version int     `json:"version"`
	Sidebar Sidebar `json:"sidebar"`
	// Trust is the sha256 of the content trusted of each space file, by the file's absolute path.
	Trust map[string]string `json:"trust,omitempty"`
}

// Sidebar is what the sidebar keeps.
type Sidebar struct {
	// Sort is the sort mode, empty for manual.
	Sort string `json:"sort,omitempty"`
	// Folded are the roots of the spaces whose linked worktrees are hidden, and the folded groups' keys.
	Folded []string `json:"folded,omitempty"`
	// Hidden are the groups left out of the sidebar.
	Hidden []string `json:"hidden,omitempty"`
	// Order is each space's manual order of its linked worktrees, by root: their directory names.
	Order map[string][]string `json:"order,omitempty"`
	// StatusRows is the STATUS list's height as dragging the rule left it, 0 for the default.
	StatusRows int `json:"statusRows,omitempty"`
}

// Dir is kspaces' state directory, $XDG_STATE_HOME/kspaces, else ~/.local/state/kspaces.
func Dir() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "kspaces"), nil
}

// fileName is the file's name in Dir.
const fileName = "state.json"

// Path is the file's path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads the file. A missing one is what the files kept before it say, see legacy, and a broken one
// is empty: what it kept is lost to the reader, but Update refuses to write over it.
func Load() State {
	s, _ := read()
	return s
}

// Update applies edit to the file as it is now and writes it back whole, by a rename, so a reader sees
// it before or after and never half of it. It is read again rather than taken from a Load before, so
// what another process wrote since is kept.
func Update(edit func(*State)) error {
	s, err := read()
	if err != nil {
		return err
	}
	edit(&s)
	s.Version = Version
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// read is the file, legacy's when it is missing, and an error when it cannot be read or parsed.
func read() (State, error) {
	path, err := Path()
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return legacy(filepath.Dir(path)), nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// legacy is the state as the files in dir before state.json kept it, a file each. They are left in
// place, for a kspaces from before it, and no longer read once state.json is written.
func legacy(dir string) State {
	file := func(name string) []byte {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		return data
	}
	lines := func(name string) []string {
		var ans []string
		sc := bufio.NewScanner(bytes.NewReader(file(name)))
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				ans = append(ans, line)
			}
		}
		return ans
	}
	s := State{Sidebar: Sidebar{
		Sort:   strings.TrimSpace(string(file("sort"))),
		Folded: lines("folded"),
		Hidden: lines("hidden"),
	}}
	if data := file("order"); len(data) > 0 {
		_ = json.Unmarshal(data, &s.Sidebar.Order)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(file("split")))); err == nil && n > 0 {
		s.Sidebar.StatusRows = n
	}
	// A line each, as sha256sum writes them: the sum, two spaces and the path.
	for _, line := range lines("trust") {
		if sum, path, ok := strings.Cut(line, "  "); ok {
			if s.Trust == nil {
				s.Trust = map[string]string{}
			}
			s.Trust[path] = sum
		}
	}
	return s
}
