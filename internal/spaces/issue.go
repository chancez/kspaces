package spaces

import (
	"fmt"

	"github.com/chancez/kspaces/internal/git"
)

// CreateIssueWorktree adds a worktree at dir to the space on branch, as CreateWorktree does, and records
// issue, by number, as the one it is for, so {issue} is known in it from then on, see WorktreeIssue.
func CreateIssueWorktree(space Space, issue int, branch, dir string) (Worktree, error) {
	wt, err := CreateWorktree(space, branch, dir)
	if err != nil {
		return Worktree{}, err
	}
	if err := git.SetBranchIssue(space.Root, wt.Branch, issue); err != nil {
		return Worktree{}, fmt.Errorf("recording issue #%d for %s: %w", issue, wt.Branch, err)
	}
	return wt, nil
}
