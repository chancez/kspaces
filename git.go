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
