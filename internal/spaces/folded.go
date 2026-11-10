package spaces

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The spaces whose linked worktrees are hidden in the sidebar, by root. Kept in a file rather than in each
// sidebar, as there is one sidebar per tab and folding a space in one should fold it in all of them; the
// others read it on their next refresh.
func foldedPath() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "kspaces", "folded"), nil
}

// LoadFolded reads the folded spaces. A missing file folds none.
func LoadFolded() map[string]bool {
	ans := map[string]bool{}
	path, err := foldedPath()
	if err != nil {
		return ans
	}
	f, err := os.Open(path)
	if err != nil {
		return ans
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			ans[line] = true
		}
	}
	return ans
}

// SaveFolded writes the folded spaces, replacing the file so a sidebar reading it never sees half of it.
func SaveFolded(folded map[string]bool) error {
	path, err := foldedPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	for root, ok := range folded {
		if ok {
			b.WriteString(root + "\n")
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
