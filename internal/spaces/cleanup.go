package spaces

import (
	"fmt"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// Cleanup is a linked worktree that looks done with, to offer removing.
type Cleanup struct {
	Worktree Worktree
	// Why it looks done with: its PR merged, its upstream gone, or its branch merged into the main
	// worktree's.
	Reasons []string
	// Changes is how many files git status reports in it, which removing it loses.
	Changes int
	// Refused is why it cannot be removed, as CanRemove says, or why its changes could not be counted.
	Refused error
	// Branch is its branch as deleting it with the worktree would find it, unless Refused.
	Branch Branch
}

// Cleanups are the linked worktrees of space that look done with. pr is the PR of a worktree's branch,
// nil for none or not known. A branch merged into the main worktree's counts only once it has moved
// since it was made: a new one, not worked on yet, is merged too.
func Cleanups(space Space, pr func(Worktree) *gh.PR) ([]Cleanup, error) {
	if space.Err != nil {
		return nil, space.Err
	}
	if len(space.Worktrees) < 2 {
		return nil, nil
	}
	gone, err := git.GoneUpstreams(space.Root)
	if err != nil {
		return nil, err
	}
	base, merged := space.Worktrees[0].Branch, map[string]bool{}
	if base != "" {
		if merged, err = git.MergedInto(space.Root, base); err != nil {
			return nil, err
		}
	}
	def := git.DefaultBranch(space.Root)
	var ans []Cleanup
	for _, wt := range space.Worktrees[1:] {
		// A detached HEAD has no branch to say it is done.
		if wt.Branch == "" {
			continue
		}
		var reasons []string
		p := pr(wt)
		if p != nil && p.State == "merged" {
			reasons = append(reasons, fmt.Sprintf("PR #%d merged", p.Number))
		}
		if gone[wt.Branch] {
			reasons = append(reasons, "upstream gone")
		}
		if merged[wt.Branch] && git.HasOwnHistory(space.Root, wt.Branch) {
			reasons = append(reasons, "merged into "+base)
		}
		if len(reasons) == 0 {
			continue
		}
		c := Cleanup{Worktree: wt, Reasons: reasons, Refused: CanRemove(space, wt)}
		if c.Refused == nil {
			if c.Changes, err = git.Changes(wt.Path); err != nil {
				c.Refused = fmt.Errorf("checking %s for changes: %w", wt.Path, err)
			}
		}
		if c.Refused == nil {
			c.Branch = BranchOf(space, def, wt, p)
		}
		ans = append(ans, c)
	}
	return ans, nil
}

// AskCleanup opens the form that offers removing the worktrees of the space at root that look done with,
// in a popup as NewWorktree's.
func AskCleanup(cfg config.Config, st kitty.State, root string) error {
	return launchForm(cfg, st, listPanel, "cleanup", root)
}

// RemoveAllInBackground runs `kspaces remove --yes` for the worktrees at paths, one after another, as a
// kitty background process, each as opts has it.
func RemoveAllInBackground(paths []string, opts map[string]RemoveOptions) error {
	args := []string{"remove", "--yes"}
	for _, p := range paths {
		o := opts[p]
		if o.Force {
			args = append(args, "--force-path", p)
		}
		if o.DeleteBranch {
			args = append(args, "--delete-branch-path", p)
		}
		if o.DeleteBranch && o.ForceBranch {
			args = append(args, "--force-branch-path", p)
		}
	}
	return kitty.LaunchBackground(append(append(args, "--"), paths...)...)
}
