package spaces

import (
	"fmt"

	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gitstate"
)

// Branch is what deleting a linked worktree's branch along with it would lose, for the forms that offer
// it and the removal that does it.
type Branch struct {
	Name string
	// Kept is why the branch cannot go with the worktree, "" when it can.
	Kept string
	git.Merge
	// PR is the number of the branch's merged PR when its commits are not merged, as a squash leaves them:
	// the PR's changes are in, so nothing is lost, though git has to be forced.
	PR int
}

// Offered reports whether the branch can be deleted with the worktree.
func (b Branch) Offered() bool { return b.Name != "" && b.Kept == "" }

// Lost is how many commits deleting the branch loses: 0 when it is merged, or its PR is.
func (b Branch) Lost() int {
	if b.PR > 0 {
		return 0
	}
	return b.Unmerged
}

// BranchOf is wt's branch, a linked worktree of space, as deleting it would find it. base is the default
// branch, which is kept, as is a branch another worktree has checked out or one whose commits cannot be
// counted. pr is the branch's PR, nil for none or not known.
func BranchOf(space Space, base string, wt Worktree, pr *gitstate.PR) Branch {
	b := Branch{Name: wt.Branch}
	if b.Name == "" {
		b.Kept = "HEAD is detached"
		return b
	}
	if b.Name == base {
		b.Kept = "it is the default branch"
		return b
	}
	for _, other := range space.Worktrees {
		if other.Path != wt.Path && other.Branch == b.Name {
			b.Kept = "it is checked out in " + other.Path
			return b
		}
	}
	m, err := git.MergeOf(space.Root, b.Name, base)
	if err != nil {
		b.Kept = fmt.Sprintf("its commits could not be counted: %v", err)
		return b
	}
	b.Merge = m
	if b.Unmerged > 0 && pr != nil && pr.State == "merged" {
		b.PR = pr.Number
	}
	return b
}

// RemoveOptions are what removing a linked worktree may lose.
type RemoveOptions struct {
	// Force removes the worktree even with changes, losing them.
	Force bool
	// DeleteBranch deletes the worktree's branch after removing it, when the branch is merged, or with
	// ForceBranch even when it is not, losing its commits.
	DeleteBranch, ForceBranch bool
}

// BranchError is a branch that could not be deleted after its worktree was removed.
type BranchError struct {
	Branch string
	Err    error
}

func (e *BranchError) Error() string {
	return fmt.Sprintf("the worktree was removed, but not its branch %s: %v", e.Branch, e.Err)
}

func (e *BranchError) Unwrap() error { return e.Err }

// branchToDelete is wt's branch, a linked worktree of space, when opts deletes it, or why it cannot go,
// worked out before the worktree is removed so a refusal keeps both.
func branchToDelete(space Space, wt Worktree, opts RemoveOptions) (Branch, error) {
	b := BranchOf(space, git.DefaultBranch(space.Root), wt, nil)
	switch {
	case b.Kept != "":
		return b, fmt.Errorf("not deleting the branch of %s, as %s; the worktree is kept", wt.Path, b.Kept)
	case b.Unmerged > 0 && !opts.ForceBranch:
		return b, fmt.Errorf("%s has %s not merged, which deleting it would lose; the worktree is kept", b.Name, plural(b.Unmerged, "commit"))
	}
	return b, nil
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
