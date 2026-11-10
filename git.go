package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree is one checkout of a space's repository.
type Worktree struct {
	Path string
	// Branch without refs/heads/, or empty when HEAD is detached.
	Branch string
	// Session is the name of the kitty session that holds this worktree's tabs.
	Session string
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

func loadSpace(root string) Space {
	s := Space{Name: filepath.Base(root), Root: root}
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		s.Err = err
		return s
	}
	s.Worktrees = parseWorktrees(s.Name, string(out))
	return s
}

// parseWorktrees reads `git worktree list --porcelain`. Bare and prunable worktrees are skipped: the first
// has no files to open a tab in, the second no longer exists on disk.
func parseWorktrees(space, porcelain string) []Worktree {
	var (
		ans  []Worktree
		cur  Worktree
		skip bool
	)
	flush := func() {
		if cur.Path != "" && !skip {
			cur.Session = sessionName(space, len(ans) == 0, cur.Path)
			ans = append(ans, cur)
		}
		cur, skip = Worktree{}, false
	}
	sc := bufio.NewScanner(strings.NewReader(porcelain))
	for sc.Scan() {
		key, value, _ := strings.Cut(sc.Text(), " ")
		switch key {
		case "":
			flush()
		case "worktree":
			cur.Path = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare", "prunable":
			skip = true
		}
	}
	flush()
	return ans
}

// newWorktreePlan is where a worktree named name goes in a space and which branch it checks out: a
// directory under the space's WorktreeDir, with any / in the name turned into -, on BranchPrefix + name.
func newWorktreePlan(cfg Config, root, name string) (dir, branch string) {
	return filepath.Join(root, cfg.WorktreeDir, strings.ReplaceAll(name, "/", "-")), cfg.BranchPrefix + name
}

// createWorktree adds a worktree named name to the space at root. It checks out the branch when it exists
// and otherwise creates it from what the space's main worktree has checked out.
func createWorktree(cfg Config, space Space, name string) (Worktree, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t") {
		return Worktree{}, fmt.Errorf("a worktree name cannot be empty or contain spaces")
	}
	dir, branch := newWorktreePlan(cfg, space.Root, name)
	args := []string{"-C", space.Root, "worktree", "add", dir, branch}
	if !branchExists(space.Root, branch) {
		args = []string{"-C", space.Root, "worktree", "add", "-b", branch, dir}
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return Worktree{}, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	// git reports worktree paths with symlinks resolved, and sessions are matched by those paths.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return Worktree{Path: dir, Branch: branch, Session: sessionName(space.Name, false, dir)}, nil
}
