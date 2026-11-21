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
	got := rowNames(AttentionRows(BuildRows(others, spaces, folded, nil, Order{}), statuses))
	want := []string{"scratch", "", "gr", "gr@fix", "", "kitty", "", "docs (folded)", "", "api", "api@old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// Nothing needs me: no rows at all.
	if got := AttentionRows(BuildRows(others, spaces, folded, nil, Order{}), []status.Status{statusOf("gr", "idle")}); got != nil {
		t.Fatalf("got %q, want none", rowNames(got))
	}
}

func TestOpenRows(t *testing.T) {
	others := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Session: "gr"}, {Session: "gr@dev"}, {Session: "gr@fix"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Session: "kitty"}}},
		{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Session: "docs"}, {Session: "docs@notes"}}},
		{Name: "web", Root: "/src/web", Worktrees: []Worktree{{Session: "web"}, {Session: "web@ui"}}},
		{Name: "api", Root: "/src/api", Worktrees: []Worktree{{Session: "api"}},
			Missing: []Worktree{{Session: "api@old"}}},
		{Name: "lost", Root: "/src/lost"},
	}
	open := map[string]int{
		"scratch": 1,
		// Only a linked worktree open: the space's row shows above it.
		"gr@fix": 2,
		"kitty":  3,
		// Hidden in a folded space, so the space's row shows.
		"docs@notes": 4,
		"api@old":    5,
	}
	folded := map[string]bool{"/src/docs": true, "/src/web": true}
	got := rowNames(OpenRows(BuildRows(others, spaces, folded, nil, Order{}), open))
	want := []string{"scratch", "", "gr", "gr@fix", "", "kitty", "", "docs (folded)", "", "api", "api@old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := OpenRows(BuildRows(others, spaces, folded, nil, Order{}), nil); got != nil {
		t.Fatalf("got %q, want none", rowNames(got))
	}
}

func TestAwakeRows(t *testing.T) {
	others := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Session: "gr"}, {Session: "gr@dev"}, {Session: "gr@fix"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Session: "kitty"}}},
		{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Session: "docs"}, {Session: "docs@notes"}}},
		{Name: "web", Root: "/src/web", Worktrees: []Worktree{{Session: "web"}, {Session: "web@ui"}}},
		{Name: "api", Root: "/src/api", Worktrees: []Worktree{{Session: "api"}},
			Missing: []Worktree{{Session: "api@old"}}},
		{Name: "lost", Root: "/src/lost"},
	}
	suspended := map[string]bool{
		"scratch": true,
		// The space's own session suspended: its row shows above the linked worktrees that are not.
		"gr":     true,
		"gr@dev": true,
		// A space with only suspended sessions goes.
		"kitty": true,
		// A folded space with one session not suspended shows.
		"docs@notes": true,
		// A folded space with every session suspended goes.
		"web":     true,
		"web@ui":  true,
		"api@old": true,
	}
	folded := map[string]bool{"/src/docs": true, "/src/web": true}
	got := rowNames(AwakeRows(BuildRows(others, spaces, folded, nil, Order{}), suspended))
	want := []string{"default", "", "gr", "gr@fix", "", "docs (folded)", "", "api", "", "lost"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	all := BuildRows(others, spaces, folded, nil, Order{})
	if got := AwakeRows(all, nil); !reflect.DeepEqual(rowNames(got), rowNames(all)) {
		t.Fatalf("got %q\nwant %q", rowNames(got), rowNames(all))
	}
}
