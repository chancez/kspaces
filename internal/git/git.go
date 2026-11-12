// Package git shells out to git for a repository's worktrees and branches.
package git

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

// Merge is how much of a branch its default branch and upstream have, which says what deleting it loses.
type Merge struct {
	// Unmerged is how many of the branch's commits neither the default branch, local or its base remote's,
	// nor the branch's upstream has.
	Unmerged int
	// Into is what has all the branch's commits when Unmerged is 0: the default branch, else the upstream.
	// One of them does, as its tip is in one.
	Into string
}

// MergeOf is how much of branch, in the repository at root, base, the default branch, and branch's
// upstream have. An upstream that is gone counts as none.
func MergeOf(root, branch, base string) (Merge, error) {
	var defaults []string
	if base != "" {
		refs := []string{"refs/heads/" + base}
		if remote := baseRemote(root); remote != "" {
			refs = append(refs, "refs/remotes/"+remote+"/"+base)
		}
		for _, ref := range refs {
			if refExists(root, ref) {
				defaults = append(defaults, ref)
			}
		}
	}
	upstream, err := line("-C", root, "for-each-ref", "--format=%(upstream)", "refs/heads/"+branch)
	if err != nil {
		return Merge{}, err
	}
	var upstreams []string
	if upstream != "" && refExists(root, upstream) {
		upstreams = []string{upstream}
	}
	count := func(not ...string) (int, error) {
		args := append([]string{"-C", root, "rev-list", "--count", "refs/heads/" + branch, "--not"}, not...)
		out, err := line(append(args, "--")...)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(out)
	}
	n, err := count(append(defaults, upstreams...)...)
	if err != nil || n > 0 {
		return Merge{Unmerged: n}, err
	}
	if len(defaults) > 0 {
		if n, err := count(defaults...); err != nil || n == 0 {
			return Merge{Into: base}, err
		}
	}
	return Merge{Into: strings.TrimPrefix(upstream, "refs/remotes/")}, nil
}

// DeleteBranch deletes the local branch named branch of the repository at root. git refuses one not
// merged into its upstream, or HEAD without one, unless force, which loses its commits.
func DeleteBranch(root, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	// The hint to force is for git's own command line; the error goes to a kitty error window.
	if out, err := exec.Command("git", "-C", root, "-c", "advice.forceDeleteBranch=false", "branch", flag, branch).CombinedOutput(); err != nil {
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

// DefaultBranch is the branch changes in the repository at root are based on, as gr works it out: the base
// remote's HEAD, or its branch named by init.defaultBranch, main or master, else a local branch by one of
// those names, named without the remote. "" when there is none. Only the base remote counts: a fork's
// remote may point its HEAD elsewhere, and an origin/HEAD left by a remote since removed names nothing.
func DefaultBranch(root string) string {
	names := []string{"main", "master"}
	if name, _ := line("-C", root, "config", "init.defaultBranch"); name != "" && !slices.Contains(names, name) {
		names = append([]string{name}, names...)
	}
	if remote := baseRemote(root); remote != "" {
		if ref, err := line("-C", root, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
			if name, ok := strings.CutPrefix(ref, remote+"/"); ok {
				return name
			}
		}
		for _, name := range names {
			if refExists(root, "refs/remotes/"+remote+"/"+name) {
				return name
			}
		}
	}
	for _, name := range names {
		if refExists(root, "refs/heads/"+name) {
			return name
		}
	}
	return ""
}

// baseRemote is the remote whose default branch changes are based on, or "" when none can be picked, as
// gr picks it: checkout.defaultRemote, then clone.defaultRemoteName, then origin, upstream, the only
// remote, or the one remote that is not remote.pushDefault.
func baseRemote(root string) string {
	out, _ := line("-C", root, "remote")
	remotes := strings.Fields(out)
	if len(remotes) == 0 {
		return ""
	}
	var candidates []string
	for _, key := range []string{"checkout.defaultRemote", "clone.defaultRemoteName"} {
		if name, _ := line("-C", root, "config", key); name != "" {
			candidates = append(candidates, name)
		}
	}
	for _, name := range append(candidates, "origin", "upstream") {
		if slices.Contains(remotes, name) {
			return name
		}
	}
	if len(remotes) == 1 {
		return remotes[0]
	}
	push, _ := line("-C", root, "config", "remote.pushDefault")
	if others := slices.DeleteFunc(slices.Clone(remotes), func(name string) bool { return name == push }); len(others) == 1 {
		return others[0]
	}
	return ""
}

func refExists(root, ref string) bool {
	_, err := output("-C", root, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// line is output trimmed of the newline git ends it with.
func line(args ...string) (string, error) {
	out, err := output(args...)
	return strings.TrimSpace(out), err
}
