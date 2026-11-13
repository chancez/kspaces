package spaces

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gittest"
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
	prs := map[string]*gh.PR{"pr": {Number: 12, State: "merged"}, "open": {Number: 13, State: "open"}, "merged": {Number: 9, State: "closed"}}
	got, err := Cleanups(space, func(wt Worktree) *gh.PR { return prs[wt.Branch] })
	if err != nil {
		t.Fatal(err)
	}
	byBranch := map[string]Worktree{}
	for _, w := range space.Worktrees {
		byBranch[w.Branch] = w
	}
	// In the order git lists them.
	want := []Cleanup{
		{Worktree: byBranch["gone"], Reasons: []string{"upstream gone"}, Changes: 1, Branch: Branch{Name: "gone", Merge: git.Merge{Unmerged: 1}}},
		{Worktree: byBranch["locked"], Reasons: []string{"merged into main"}, Refused: errors.New(wt("locked") + " is locked; git worktree unlock it first")},
		{Worktree: byBranch["merged"], Reasons: []string{"merged into main"}, Branch: Branch{Name: "merged", Merge: git.Merge{Into: "main"}}},
		// Its PR is merged, as by a squash, though its commit is not.
		{Worktree: byBranch["pr"], Reasons: []string{"PR #12 merged"}, Branch: Branch{Name: "pr", Merge: git.Merge{Unmerged: 1}, PR: 12}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}

	// A space without linked worktrees has nothing to clean up, and a broken one says why.
	if got, err := Cleanups(Space{Name: "solo", Root: root, Worktrees: space.Worktrees[:1]}, nil); got != nil || err != nil {
		t.Errorf("no linked worktrees: %v, %v", got, err)
	}
	broken := errors.New("not a git repository")
	if _, err := Cleanups(Space{Err: broken}, nil); err != broken {
		t.Errorf("broken space: %v", err)
	}
}
