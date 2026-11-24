package spaces

import (
	"errors"
	"fmt"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// Cleanup is a worktree to offer removing: a linked one that looks done with, a missing one, or, in a
// list of sessions given, any session, refused when it is no linked or missing worktree.
type Cleanup struct {
	// Space names the space it is a worktree of, empty for a session in no space.
	Space    string
	Worktree Worktree
	// Why it looks done with: its PR merged, its upstream gone, or its branch merged into the main
	// worktree's; or, for a missing one, its directory gone.
	Reasons []string
	// Changes is how many files git status reports in it, which removing it loses.
	Changes int
	// Refused is why it cannot be removed, as CanRemove says, or why its changes could not be counted.
	Refused error
	// Branch is its branch as deleting it with the worktree would find it, unless Refused.
	Branch Branch
	// Missing is a missing worktree, see Space.Missing: removing it ends its session with EndSession, as
	// there is no directory left for git to remove.
	Missing bool
	// Windows is how many windows a missing worktree's session has open, which removing it closes.
	Windows int
}

// doneWith says why the linked worktrees of a space look done with, from what git says of all its
// branches at once.
type doneWith struct {
	space        Space
	gone, merged map[string]bool
	base, def    string
}

func newDoneWith(space Space) (*doneWith, error) {
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
	return &doneWith{space: space, gone: gone, merged: merged, base: base, def: git.DefaultBranch(space.Root)}, nil
}

// cleanup is wt, a linked worktree, with why it looks done with, none when it does not, its changes and
// its branch. p is its branch's PR, nil for none or not known.
func (d *doneWith) cleanup(wt Worktree, p *gh.PR) Cleanup {
	var reasons []string
	if p != nil && p.State == "merged" {
		reasons = append(reasons, fmt.Sprintf("PR #%d merged", p.Number))
	}
	if wt.Branch != "" && d.gone[wt.Branch] {
		reasons = append(reasons, "upstream gone")
	}
	if wt.Branch != "" && d.merged[wt.Branch] && git.HasOwnHistory(d.space.Root, wt.Branch) {
		reasons = append(reasons, "merged into "+d.base)
	}
	c := Cleanup{Space: d.space.Name, Worktree: wt, Reasons: reasons, Refused: CanRemove(d.space, wt)}
	if c.Refused == nil {
		var err error
		if c.Changes, err = git.Changes(wt.Path); err != nil {
			c.Refused = fmt.Errorf("checking %s for changes: %w", wt.Path, err)
		}
	}
	if c.Refused == nil {
		c.Branch = BranchOf(d.space, d.def, wt, p)
	}
	return c
}

// missingCleanup is wt, a missing worktree of space, whose session has windows[wt.Session] windows open.
func missingCleanup(space Space, wt Worktree, windows map[string]int) Cleanup {
	return Cleanup{Space: space.Name, Worktree: wt, Reasons: []string{"directory gone"}, Missing: true, Windows: windows[wt.Session]}
}

// Cleanups are the linked worktrees of space that look done with, then its missing ones, which the
// caller places in it, see placeSessions; windows counts each session's windows. pr is the PR of a
// worktree's branch, nil for none or not known. A branch merged into the main worktree's counts only once
// it has moved since it was made: a new one, not worked on yet, is merged too.
func Cleanups(space Space, windows map[string]int, pr func(Worktree) *gh.PR) ([]Cleanup, error) {
	if space.Err != nil {
		return nil, space.Err
	}
	var ans []Cleanup
	if len(space.Worktrees) > 1 {
		d, err := newDoneWith(space)
		if err != nil {
			return nil, err
		}
		for _, wt := range space.Worktrees[1:] {
			// A detached HEAD has no branch to say it is done.
			if wt.Branch == "" {
				continue
			}
			if c := d.cleanup(wt, pr(wt)); len(c.Reasons) > 0 {
				ans = append(ans, c)
			}
		}
	}
	for _, wt := range space.Missing {
		ans = append(ans, missingCleanup(space, wt, windows))
	}
	return ans, nil
}

// SessionCleanups are sessions, by name, as snap has them, in their order: a linked worktree with why it
// looks done with, if it does, a missing one, and the rest refused, a main worktree as CanRemove says.
// pr is as for Cleanups.
func SessionCleanups(snap Snapshot, sessions []string, pr func(Worktree) *gh.PR) []Cleanup {
	dones := map[string]*doneWith{}
	doneOf := func(s Space) (*doneWith, error) {
		if d, ok := dones[s.Root]; ok {
			return d, nil
		}
		d, err := newDoneWith(s)
		if err == nil {
			dones[s.Root] = d
		}
		return d, err
	}
	find := func(name string) Cleanup {
		for _, s := range snap.Spaces {
			for i, wt := range s.Worktrees {
				if wt.Session != name {
					continue
				}
				if i == 0 {
					return Cleanup{Space: s.Name, Worktree: wt, Refused: CanRemove(s, wt)}
				}
				d, err := doneOf(s)
				if err != nil {
					return Cleanup{Space: s.Name, Worktree: wt, Refused: err}
				}
				return d.cleanup(wt, pr(wt))
			}
			for _, wt := range s.Missing {
				if wt.Session == name {
					return missingCleanup(s, wt, snap.Kitty.Windows)
				}
			}
		}
		return Cleanup{Worktree: Worktree{Session: name}, Refused: errors.New("in no space, not a worktree")}
	}
	ans := make([]Cleanup, len(sessions))
	for i, name := range sessions {
		ans[i] = find(name)
	}
	return ans
}

// AskCleanup opens the form that offers removing the worktrees of the space at root that look done with,
// and its missing ones, in a popup as NewWorktree's.
func AskCleanup(cfg config.Config, st kitty.State, root string) error {
	return launchForm(cfg, st, listPanel, "cleanup", root)
}

// AskRemoveSessions opens the form that offers removing the worktrees of sessions, as the clean up form
// does, in a popup as AskCleanup's.
func AskRemoveSessions(cfg config.Config, st kitty.State, sessions []string) error {
	args := []string{"cleanup"}
	for _, s := range sessions {
		args = append(args, "--session", s)
	}
	return launchForm(cfg, st, listPanel, args...)
}

// RemoveAllInBackground runs `kspaces remove --yes` for the worktrees at paths, one after another, each as
// opts has it, and then ends sessions, the missing worktrees', with EndSession, as a kitty background
// process.
func RemoveAllInBackground(paths []string, opts map[string]RemoveOptions, sessions []string) error {
	args := []string{"remove", "--yes"}
	for _, s := range sessions {
		args = append(args, "--forget-session", s)
	}
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
