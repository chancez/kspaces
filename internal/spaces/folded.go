package spaces

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// statePath is the file name in kspaces' state directory, $XDG_STATE_HOME/kspaces, where what every
// sidebar shares is kept: there is one sidebar per tab, and a change in one should show in all of them,
// which read it on their next refresh.
func statePath(name string) (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "kspaces", name), nil
}

// readState is the state file name, nil when it is missing or unreadable.
func readState(name string) []byte {
	path, err := statePath(name)
	if err != nil {
		return nil
	}
	data, _ := os.ReadFile(path)
	return data
}

// writeState replaces the state file name with data, so a sidebar reading it never sees half of it.
func writeState(name string, data []byte) error {
	path, err := statePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// The spaces whose linked worktrees are hidden in the sidebar, by root, and the groups whose spaces are,
// by GroupKey, a line each.
const foldedFile = "folded"

// GroupKey is how the folded file names the group called name. A root is a path, absolute once ~/ is
// expanded, as kitty opens sessions in it from anywhere, so it never starts with group:.
func GroupKey(name string) string {
	return "group:" + name
}

// LoadFolded reads the folded spaces and groups. A missing file folds none.
func LoadFolded() map[string]bool {
	return readLines(foldedFile)
}

// SaveFolded writes the folded spaces and groups.
func SaveFolded(folded map[string]bool) error {
	return writeLines(foldedFile, folded)
}

// The groups left out of the sidebar, by name, a line each. Shared, as the folds are, so hiding a group
// hides it in every tab's sidebar, and it stays hidden after a restart.
const hiddenFile = "hidden"

// LoadHidden reads the hidden groups. A missing file hides none.
func LoadHidden() map[string]bool {
	return readLines(hiddenFile)
}

// SaveHidden writes the hidden groups.
func SaveHidden(hidden map[string]bool) error {
	return writeLines(hiddenFile, hidden)
}

// readLines is the set of lines of the state file name.
func readLines(name string) map[string]bool {
	ans := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(readState(name)))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			ans[line] = true
		}
	}
	return ans
}

// writeLines replaces the state file name with the lines set in lines, sorted, so it reads the same
// whatever order the map has.
func writeLines(name string, lines map[string]bool) error {
	var b strings.Builder
	for _, line := range slices.Sorted(maps.Keys(lines)) {
		if lines[line] {
			b.WriteString(line + "\n")
		}
	}
	return writeState(name, []byte(b.String()))
}
