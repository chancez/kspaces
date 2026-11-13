package spaces

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/kitty"
)

// A linked worktree looks done with when its PR is merged, its upstream is gone, or its branch is merged
// into the main worktree's after having moved; not when it was only just made. Its changes are counted,
// and what deleting its branch loses, and a locked one is offered refused.
func TestCleanups(t *testing.T) {
	remote := gittest.Bare(t)
	r := gittest.New(t)
	root := r.Dir
	r.Commit("first")
	r.Git("remote", "add", "origin", remote.Dir)
	wt := func(name string) string { return filepath.Join(root, ".worktrees", name) }
	wts := map[string]*gittest.Repo{}
	for _, name := range []string{"merged", "fresh", "gone", "pr", "open", "locked"} {
		wts[name] = r.Worktree(name, wt(name))
	}
	for _, name := range []string{"merged", "gone", "pr", "open", "locked"} {
		wts[name].Commit(name)
	}
	r.Git("merge", "-q", "--no-edit", "merged", "locked")
	r.Git("worktree", "lock", wt("locked"))
	// gone was pushed and tracked, then its branch deleted on the remote.
	wts["gone"].Git("push", "-q", "-u", "origin", "gone")
	r.Git("push", "-q", "origin", "--delete", "gone")
	r.Git("fetch", "-q", "--prune")
	wts["gone"].Write("notes", "x")
	r.Git("worktree", "add", "-q", "--detach", wt("detached"))

	space := LoadSpace(root)
	n := space.Name
	space.Missing = []Worktree{{Session: n + "@lost"}, {Session: n + "@saved"}}
	windows := map[string]int{n + "@lost": 2, n + "@gone": 1}
	prs := map[string]*gh.PR{"pr": {Number: 12, State: "merged"}, "open": {Number: 13, State: "open"}, "merged": {Number: 9, State: "closed"}}
	pr := func(wt Worktree) *gh.PR { return prs[wt.Branch] }
	got, err := Cleanups(space, windows, pr)
	if err != nil {
		t.Fatal(err)
	}
	byBranch := map[string]Worktree{}
	for _, w := range space.Worktrees {
		byBranch[w.Branch] = w
	}
	// In the order git lists them, then the missing ones with their windows.
	gone := Cleanup{Space: n, Worktree: byBranch["gone"], Reasons: []string{"upstream gone"}, Changes: 1, Branch: Branch{Name: "gone", Merge: git.Merge{Unmerged: 1}}}
	lost := Cleanup{Space: n, Worktree: Worktree{Session: n + "@lost"}, Reasons: []string{"directory gone"}, Missing: true, Windows: 2}
	want := []Cleanup{
		gone,
		{Space: n, Worktree: byBranch["locked"], Reasons: []string{"merged into main"}, Refused: errors.New(wt("locked") + " is locked; git worktree unlock it first")},
		{Space: n, Worktree: byBranch["merged"], Reasons: []string{"merged into main"}, Branch: Branch{Name: "merged", Merge: git.Merge{Into: "main"}}},
		// Its PR is merged, as by a squash, though its commit is not.
		{Space: n, Worktree: byBranch["pr"], Reasons: []string{"PR #12 merged"}, Branch: Branch{Name: "pr", Merge: git.Merge{Unmerged: 1}, PR: 12}},
		lost,
		{Space: n, Worktree: Worktree{Session: n + "@saved"}, Reasons: []string{"directory gone"}, Missing: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}

	// Sessions given are listed in their order whether they look done with or not: the main worktree and
	// a session in no space refused.
	snap := Snapshot{Spaces: []Space{space}, Kitty: kitty.State{Windows: windows}}
	got = SessionCleanups(snap, []string{n + "@lost", n + "@fresh", n, "kitty", n + "@gone"}, pr)
	want = []Cleanup{
		lost,
		{Space: n, Worktree: byBranch["fresh"], Branch: Branch{Name: "fresh", Merge: git.Merge{Into: "main"}}},
		{Space: n, Worktree: space.Worktrees[0], Refused: errors.New(space.Worktrees[0].Path + " is the main worktree of " + n + ", which cannot be removed")},
		{Worktree: Worktree{Session: "kitty"}, Refused: errors.New("in no space, not a worktree")},
		gone,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sessions: got\n%#v\nwant\n%#v", got, want)
	}

	// A space without linked worktrees has nothing to clean up, and a broken one says why.
	if got, err := Cleanups(Space{Name: "solo", Root: root, Worktrees: space.Worktrees[:1]}, nil, nil); got != nil || err != nil {
		t.Errorf("no linked worktrees: %v, %v", got, err)
	}
	broken := errors.New("not a git repository")
	if _, err := Cleanups(Space{Err: broken}, nil, nil); err != broken {
		t.Errorf("broken space: %v", err)
	}
}
