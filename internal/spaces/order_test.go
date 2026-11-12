package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/status"
)

// orderFixture is two sessions in no space and five spaces: gr with three linked worktrees and a missing
// one, docs with one, and three with none. Some sessions have been focused and some report a status.
func orderFixture() ([]Space, []Space, Order) {
	others := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	wt := func(space, dir string) Worktree {
		return Worktree{Path: "/src/" + space + "/.worktrees/" + dir, Session: space + "@" + dir}
	}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}, wt("gr", "a"), wt("gr", "b"), wt("gr", "c")}, Missing: []Worktree{{Session: "gr@gone"}}},
		{Name: "web", Root: "/src/web", Worktrees: []Worktree{{Path: "/src/web", Session: "web"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Path: "/src/kitty", Session: "kitty"}}},
		{Name: "api", Root: "/src/api", Worktrees: []Worktree{{Path: "/src/api", Session: "api"}}},
		{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Path: "/src/docs", Session: "docs"}, wt("docs", "x")}},
	}
	order := Order{
		Worktrees: map[string][]string{"/src/gr": {"c", "a"}},
		Focused:   map[string]float64{"gr": 5, "gr@a": 20, "gr@b": 50, "gr@gone": 70, "kitty": 40, "docs@x": 60, "scratch": 10},
		Statuses: []status.Status{
			{Session: "gr@c", Record: status.Record{State: "blocked"}},
			{Session: "kitty", Record: status.Record{State: "working"}},
			{Session: "docs@x", Record: status.Record{State: "working"}},
			{Session: "web", Record: status.Record{State: "done"}},
			{Session: "default", Record: status.Record{State: "idle"}},
		},
	}
	return others, spaces, order
}

// Each mode orders the spaces, the linked worktrees within each and the sessions in no space, keeping a
// space's main worktree first and its missing ones last, and the sessions in no space above the spaces.
func TestOrderModes(t *testing.T) {
	for _, c := range []struct {
		mode SortMode
		held map[string]int
		want []string
	}{
		// config.toml's order, gr's worktrees as moved, b after them in git's order.
		{SortManual, nil, []string{"default", "scratch", "", "gr", "gr@c", "gr@a", "gr@b", "gr@gone", "", "web", "", "kitty", "", "api", "", "docs", "docs@x"}},
		{"", nil, []string{"default", "scratch", "", "gr", "gr@c", "gr@a", "gr@b", "gr@gone", "", "web", "", "kitty", "", "api", "", "docs", "docs@x"}},
		// gr by its missing worktree's focus; web and api never focused, last in manual order, as is gr@c.
		{SortActivity, nil, []string{"scratch", "default", "", "gr", "gr@b", "gr@a", "gr@c", "gr@gone", "", "docs", "docs@x", "", "kitty", "", "web", "", "api"}},
		// Blocked, done, then the two working by activity; idle above none.
		{SortStatus, nil, []string{"default", "scratch", "", "gr", "gr@c", "gr@b", "gr@a", "gr@gone", "", "web", "", "docs", "docs@x", "", "kitty", "", "api"}},
		// What is held stays ahead, in its place; the rest follow in the mode's order.
		{SortActivity, map[string]int{"/src/api": 0, "/src/gr": 1, "gr@a": 0, "default": 0}, []string{"default", "scratch", "", "api", "", "gr", "gr@a", "gr@b", "gr@c", "gr@gone", "", "docs", "docs@x", "", "kitty", "", "web"}},
	} {
		others, spaces, order := orderFixture()
		order.Mode, order.Held = c.mode, c.held
		if got := rowNames(BuildRows(others, spaces, nil, order)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q held %v:\ngot  %q\nwant %q", c.mode, c.held, got, c.want)
		}
		// The snapshot keeps its own order.
		if again, _, _ := orderFixture(); !reflect.DeepEqual(others, again) {
			t.Errorf("%q: others changed to %v", c.mode, others)
		}
		if _, again, _ := orderFixture(); !reflect.DeepEqual(spaces, again) {
			t.Errorf("%q: spaces changed to %v", c.mode, spaces)
		}
	}
}

// The sort mode and the worktrees' order are saved in the state directory, which every sidebar's View
// reads.
func TestOrderState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	cfg := config.Config{}
	view := func() Order {
		o := Shared{Config: &cfg}.View(0).Order
		return Order{Mode: o.Mode, Worktrees: o.Worktrees}
	}
	if got, want := view(), (Order{Mode: SortManual, Worktrees: map[string][]string{}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("with nothing saved: %#v, want %#v", got, want)
	}
	if err := SaveSort(SortStatus); err != nil {
		t.Fatal(err)
	}
	if err := SaveOrder(map[string][]string{"/src/gr": {"c", "a"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := view(), (Order{Mode: SortStatus, Worktrees: map[string][]string{"/src/gr": {"c", "a"}}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved: %#v, want %#v", got, want)
	}
	files := map[string]string{}
	for _, name := range []string{"sort", "order"} {
		data, err := os.ReadFile(filepath.Join(dir, "kspaces", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	if want := map[string]string{"sort": "status\n", "order": "{\n  \"/src/gr\": [\n    \"c\",\n    \"a\"\n  ]\n}\n"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files %q, want %q", files, want)
	}
	// What no kspaces wrote reads as nothing saved.
	for name, junk := range map[string]string{"sort": "sideways\n", "order": "[1]"} {
		if err := os.WriteFile(filepath.Join(dir, "kspaces", name), []byte(junk), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := view(), (Order{Mode: SortManual, Worktrees: map[string][]string{}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("broken files: %#v, want %#v", got, want)
	}
	// The modes go round.
	if got := []SortMode{SortManual.Next(), SortActivity.Next(), SortStatus.Next(), SortMode("").Next()}; !reflect.DeepEqual(got, []SortMode{SortActivity, SortStatus, SortManual, SortActivity}) {
		t.Fatalf("next %q", got)
	}
}
