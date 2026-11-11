package spaces

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// NewSpaceRoot is the root to add as a space for path, as typed: the top of the work tree it is in, with
// symlinks resolved, as git reports it. The error says why it cannot be added, for the form to show before
// anything is written: there is no such directory, it is in no work tree, or the work tree already is
// one of roots, the configured spaces.
func NewSpaceRoot(roots []string, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("name a repository to add")
	}
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	top, err := git.TopLevel(abs)
	if err != nil {
		return "", fmt.Errorf("%s is not in a git repository", path)
	}
	for _, r := range roots {
		// The config may name a root through a symlink, and git resolves them.
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			r = resolved
		}
		if r == top {
			return "", fmt.Errorf("%s is already a space", config.Tilde(top))
		}
	}
	return top, nil
}

// AskAddSpace opens the form that adds a space, in an overlay as NewWorktree's, with the repository of the
// window the sidebar is beside filled in.
func AskAddSpace(st kitty.State) error {
	args := []string{"add"}
	if st.HomeDir != "" {
		if top, err := git.TopLevel(st.HomeDir); err == nil {
			args = append(args, top)
		}
	}
	return kitty.LaunchForm(st, args...)
}

// expandHome is path with a leading ~/ for the home directory.
func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}
