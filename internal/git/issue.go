package git

import (
	"os/exec"
	"strconv"
	"strings"
)

// issueKey is the git config key that records the issue a branch is for. Being the branch's, git renames
// it with the branch and drops it when the branch is deleted.
func issueKey(branch string) string { return "branch." + branch + ".kspaces-issue" }

// SetBranchIssue records issue, by number, as the one the local branch is for.
func SetBranchIssue(root, branch string, issue int) error {
	return gitDo(root, "config", issueKey(branch), strconv.Itoa(issue))
}

// BranchIssue is the issue SetBranchIssue recorded for the local branch, 0 for none.
func BranchIssue(root, branch string) int {
	out, err := exec.Command("git", "-C", root, "config", "--get", issueKey(branch)).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
