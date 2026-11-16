package spaces

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
)

// A worktree created for an issue knows it from then on, as git config records it with its branch; the
// branch deleted, a new one of its name is for no issue.
func TestCreateIssueWorktree(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	space := LoadSpace(r.Dir)
	dir := filepath.Join(r.Dir, ".worktrees", "42-fix")
	wt, err := CreateIssueWorktree(space, 42, "pr/me/42-fix", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := []Links{WorktreeLinks(space, wt)}
	r.Git("worktree", "remove", dir)
	r.Git("branch", "-q", "-D", "pr/me/42-fix")
	again, err := CreateWorktree(space, "pr/me/42-fix", dir)
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, WorktreeLinks(space, again))
	if want := []Links{{Issue: 42}, {}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
