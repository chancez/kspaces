package git

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/logs"
)

// branchKey is the git config key, kspaces-NAME, that records something kspaces knows of a local branch.
// Being the branch's, git renames it with the branch and drops it when the branch is deleted.
func branchKey(branch, name string) string { return "branch." + branch + ".kspaces-" + name }

// branchValue is what branchKey(branch, name) is set to, empty for nothing.
func branchValue(root, branch, name string) string {
	out, err := logs.Query(exec.Command("git", "-C", root, "config", "--get", branchKey(branch, name)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SetBranchIssue records issue, by number, as the one the local branch is for.
func SetBranchIssue(root, branch string, issue int) error {
	return gitDo(root, "config", branchKey(branch, "issue"), strconv.Itoa(issue))
}

// BranchIssue is the issue SetBranchIssue recorded for the local branch, 0 for none.
func BranchIssue(root, branch string) int {
	n, err := strconv.Atoi(branchValue(root, branch, "issue"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// SetBranchBase records base as the branch the local branch was created from.
func SetBranchBase(root, branch, base string) error {
	return gitDo(root, "config", branchKey(branch, "base"), base)
}

// BranchBase is the base SetBranchBase recorded for the local branch, empty for none.
func BranchBase(root, branch string) string { return branchValue(root, branch, "base") }

// LocalBranches are the local branches of the repository at root, the last committed to first.
func LocalBranches(root string) ([]string, error) {
	out, err := output("-C", root, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}
