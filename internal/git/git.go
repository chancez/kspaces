// Package git shells out to git for a repository's worktrees and branches.
package git

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree is one checkout of a repository.
type Worktree struct {
	Path string
	// Branch without refs/heads/, or empty when HEAD is detached.
	Branch string
	// Locked is set with git worktree lock, and git will not remove a locked worktree.
	Locked bool
}

// ListWorktrees lists the worktrees of the repository at root, the main one first. The error is git's
// own message, for example when root is not a repository.
func ListWorktrees(root string) ([]Worktree, error) {
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return ParseWorktrees(string(out)), nil
}

// ParseWorktrees reads `git worktree list --porcelain`. Bare and prunable worktrees are skipped: the first
// has no files to open a tab in, the second no longer exists on disk.
func ParseWorktrees(porcelain string) []Worktree {
	var (
		ans  []Worktree
		cur  Worktree
		skip bool
	)
	flush := func() {
		if cur.Path != "" && !skip {
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
		case "locked":
			cur.Locked = true
		case "bare", "prunable":
			skip = true
		}
	}
	flush()
	return ans
}

// AddWorktree adds a worktree at dir to the repository at root, on branch, and returns its path as git
// reports it. It checks out the branch when it exists and otherwise creates it from what the main
// worktree has checked out.
func AddWorktree(root, branch, dir string) (string, error) {
	branch = strings.TrimSpace(branch)
	switch {
	case branch == "" || strings.HasSuffix(branch, "/"):
		return "", fmt.Errorf("the branch needs a name")
	case strings.ContainsAny(branch, " \t"):
		return "", fmt.Errorf("a branch name cannot contain spaces")
	}
	args := []string{"-C", root, "worktree", "add", dir, branch}
	if !BranchExists(root, branch) {
		args = []string{"-C", root, "worktree", "add", "-b", branch, dir}
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	// git reports worktree paths with symlinks resolved, and sessions are matched by those paths.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir, nil
}

// BranchExists reports whether the repository at root has a local branch named branch.
func BranchExists(root, branch string) bool {
	return exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// TopLevel is the top of the work tree dir is in, with symlinks resolved, as git reports it. The error is
// git's own message, for example when dir is in no repository.
func TopLevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Changes is how many files git status reports in the worktree at dir: changed, staged or untracked.
// Removing a worktree with any loses them.
func Changes(dir string) (int, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return 0, fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return 0, err
	}
	return strings.Count(string(out), "\n"), nil
}

// RemoveWorktree removes the linked worktree at dir from the repository at root, its directory included.
// git refuses one with changes unless force, which loses them. The branch is kept.
func RemoveWorktree(root, dir string, force bool) error {
	args := []string{"-C", root, "worktree", "remove", dir}
	if force {
		args = append(args, "--force")
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// output runs git with args and returns what it printed, or git's own message as the error.
func output(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// GoneUpstreams are the local branches of the repository at root whose upstream is set but no longer
// exists, as after its branch was deleted when merged and the remote pruned.
func GoneUpstreams(root string) (map[string]bool, error) {
	out, err := output("-C", root, "for-each-ref", "--format=%(refname:short) %(upstream:track)", "refs/heads")
	if err != nil {
		return nil, err
	}
	ans := map[string]bool{}
	for line := range strings.Lines(out) {
		if branch, ok := strings.CutSuffix(strings.TrimSpace(line), " [gone]"); ok {
			ans[branch] = true
		}
	}
	return ans, nil
}

// MergedInto are the local branches of the repository at root that base has all the commits of.
func MergedInto(root, base string) (map[string]bool, error) {
	out, err := output("-C", root, "branch", "--format=%(refname:short)", "--merged", base)
	if err != nil {
		return nil, err
	}
	ans := map[string]bool{}
	for line := range strings.Lines(out) {
		if b := strings.TrimSpace(line); b != "" {
			ans[b] = true
		}
	}
	return ans, nil
}

// HasOwnHistory reports whether branch has moved since it was created, by its reflog: committed to,
// reset or pulled. A branch that has not is merged into what it was made from without having been worked
// on.
func HasOwnHistory(root, branch string) bool {
	out, err := output("-C", root, "reflog", "show", "--format=%H", "refs/heads/"+branch, "--")
	return err == nil && strings.Count(out, "\n") > 1
}
