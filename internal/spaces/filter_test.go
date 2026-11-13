package spaces

import (
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/status"
)

func statusOf(session, state string) status.Status {
	return status.Status{Session: session, Record: status.Record{State: state}}
}

func TestAttentionRows(t *testing.T) {
	others := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Session: "gr"}, {Session: "gr@dev"}, {Session: "gr@fix"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Session: "kitty"}}},
		{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Session: "docs"}, {Session: "docs@notes"}}},
		{Name: "web", Root: "/src/web", Worktrees: []Worktree{{Session: "web"}, {Session: "web@ui"}}},
		{Name: "api", Root: "/src/api", Worktrees: []Worktree{{Session: "api"}, {Session: "api@v2"}},
			Missing: []Worktree{{Session: "api@old"}}},
		{Name: "lost", Root: "/src/lost"},
	}
	statuses := []status.Status{
		statusOf("scratch", "done"),
		statusOf("default", "idle"),
		statusOf("gr@fix", "blocked"),
		statusOf("gr", "idle"),
		statusOf("kitty", "working"),
		// Hidden in a folded space, so the space's row shows.
		statusOf("docs@notes", "error"),
		// A folded space with only idle sessions stays hidden.
		statusOf("web@ui", "idle"),
		statusOf("api@old", "done"),
	}
	folded := map[string]bool{"/src/docs": true, "/src/web": true}
	got := rowNames(AttentionRows(BuildRows(others, spaces, folded, Order{}), statuses))
	want := []string{"scratch", "", "gr", "gr@fix", "", "kitty", "", "docs (folded)", "", "api", "api@old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// Nothing needs me: no rows at all.
	if got := AttentionRows(BuildRows(others, spaces, folded, Order{}), []status.Status{statusOf("gr", "idle")}); got != nil {
		t.Fatalf("got %q, want none", rowNames(got))
	}
}
