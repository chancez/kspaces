// Package spaces is kspaces' domain: repositories ("spaces"), their worktrees and the kitty sessions that
// hold them, the order the sidebar lists them in, and switching kitty between them.
package spaces

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// Worktree is one checkout of a space's repository.
type Worktree struct {
	Path string
	// Branch without refs/heads/, or empty when HEAD is detached.
	Branch string
	// Session is the name of the kitty session that holds this worktree's tabs.
	Session string
	// Locked is set with git worktree lock, which keeps git from removing it.
	Locked bool
}

// Space is a repository and its worktrees. The first worktree is the main one.
type Space struct {
	Name      string
	Root      string
	Worktrees []Worktree
	// Err is why the worktrees could not be listed, for example because Root is not a repository.
	Err error
}

// sessionName names the kitty session of a worktree. The main worktree's session is named after the space,
// the others after the space and the worktree's directory, so two repositories that both have a worktree
// called `main` do not share a session.
func sessionName(space string, main bool, path string) string {
	if main {
		return space
	}
	return space + "@" + filepath.Base(path)
}

// LoadSpace lists the worktrees of the space at root.
func LoadSpace(root string) Space {
	s := Space{Name: filepath.Base(root), Root: root}
	wts, err := git.ListWorktrees(root)
	if err != nil {
		s.Err = err
		return s
	}
	s.Worktrees = fromGit(s.Name, wts)
	return s
}

// fromGit names the sessions of a space's worktrees, as git lists them, the main one first.
func fromGit(space string, wts []git.Worktree) []Worktree {
	var ans []Worktree
	for i, wt := range wts {
		ans = append(ans, Worktree{Path: wt.Path, Branch: wt.Branch, Session: sessionName(space, i == 0, wt.Path), Locked: wt.Locked})
	}
	return ans
}

// HasBranch reports whether the space's repository has a local branch named branch.
func (s Space) HasBranch(branch string) bool {
	return git.BranchExists(s.Root, branch)
}

// BranchStart is what a new branch's name starts with: the configured prefix and a /, or nothing.
func BranchStart(cfg config.Config) string {
	if p := strings.Trim(cfg.Worktrees.BranchPrefix, "/"); p != "" {
		return p + "/"
	}
	return ""
}

// WorktreeDir is where the worktree of branch goes in the space at root. A given dir is used as is when
// absolute, ~ expanded, and relative to root otherwise. Without one it is WorktreeDir, under root, and the
// branch without the configured prefix, any / turned into -: `pr/me/fix/x` goes in `.worktrees/fix-x`.
func WorktreeDir(cfg config.Config, root, branch, dir string) string {
	if dir = expandHome(strings.TrimSpace(dir)); dir != "" {
		if filepath.IsAbs(dir) {
			return filepath.Clean(dir)
		}
		return filepath.Join(root, dir)
	}
	name := strings.TrimPrefix(branch, BranchStart(cfg))
	return filepath.Join(root, cfg.Worktrees.Dir, strings.ReplaceAll(name, "/", "-"))
}

// CreateWorktree adds a worktree at dir to the space, on branch. It checks out the branch when it exists
// and otherwise creates it from what the space's main worktree has checked out.
func CreateWorktree(space Space, branch, dir string) (Worktree, error) {
	branch = strings.TrimSpace(branch)
	path, err := git.AddWorktree(space.Root, branch, dir)
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{Path: path, Branch: branch, Session: sessionName(space.Name, false, path)}, nil
}

// Find is the space given by root or name, or, with an empty query, the space of the session of the
// window that has the keyboard.
func Find(cfg config.Config, query string) (Space, error) {
	var current string
	if query == "" {
		st, err := kitty.LoadState(kitty.FocusedWindow)
		if err != nil {
			return Space{}, err
		}
		current = st.Current
	}
	abs, _ := filepath.Abs(query)
	for _, root := range cfg.Spaces {
		s := LoadSpace(root)
		switch {
		case query != "" && (root == abs || s.Name == query):
			return s, s.Err
		case query == "":
			for _, wt := range s.Worktrees {
				if wt.Session == current {
					return s, nil
				}
			}
		}
	}
	if query == "" {
		return Space{}, fmt.Errorf("the current window is not in a space")
	}
	return Space{}, fmt.Errorf("%s is not a configured space", query)
}
