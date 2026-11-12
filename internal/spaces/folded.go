package spaces

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

// The spaces whose linked worktrees are hidden in the sidebar, by root, a line each.
const foldedFile = "folded"

// LoadFolded reads the folded spaces. A missing file folds none.
func LoadFolded() map[string]bool {
	ans := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(readState(foldedFile)))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			ans[line] = true
		}
	}
	return ans
}

// SaveFolded writes the folded spaces.
func SaveFolded(folded map[string]bool) error {
	var b strings.Builder
	for root, ok := range folded {
		if ok {
			b.WriteString(root + "\n")
		}
	}
	return writeState(foldedFile, []byte(b.String()))
}
