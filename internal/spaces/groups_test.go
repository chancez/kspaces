package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/status"
)

// groupFixture is a session in no space and spaces in config.toml's order: gr and api in work, kitty in
// personal, docs in play and lab in art, neither of which groups lists, and web and notes in none.
func groupFixture() ([]Space, []Space, Order) {
	others := []Space{{Name: "default", Worktrees: []Worktree{{Session: "default"}}}}
	space := func(name, group string, linked ...string) Space {
		s := Space{Name: name, Root: "/src/" + name, Group: group, Worktrees: []Worktree{{Path: "/src/" + name, Session: name}}}
		for _, dir := range linked {
			s.Worktrees = append(s.Worktrees, Worktree{Path: "/src/" + name + "/.worktrees/" + dir, Session: name + "@" + dir})
		}
		return s
	}
	spaces := []Space{
		space("gr", "work", "dev"),
		space("web", ""),
		space("kitty", "personal"),
		space("api", "work"),
		space("docs", "play", "x"),
		space("notes", ""),
		space("lab", "art"),
	}
	return others, spaces, Order{Groups: []string{"personal", "work"}}
}

// Spaces in no group come first, under no header, then each group under its header in groups' order, the
// groups it does not list after, by name. A group's first space is right under its header.
func TestGroupRows(t *testing.T) {
	for _, c := range []struct {
		name           string
		folded, hidden map[string]bool
		mode           SortMode
		focused        map[string]float64
		want           []string
	}{
		{"manual", nil, nil, SortManual, nil, []string{
			"default", "", "web", "", "notes",
			"", "[personal]", "kitty",
			"", "[work]", "gr", "gr@dev", "", "api",
			"", "[art]", "lab",
			"", "[play]", "docs", "docs@x",
		}},
		// A folded group keeps its header alone; a folded space in a group folds as ever.
		{"folded", map[string]bool{GroupKey("work"): true, "/src/docs": true}, nil, SortManual, nil, []string{
			"default", "", "web", "", "notes",
			"", "[personal]", "kitty",
			"", "[work] (folded)",
			"", "[art]", "lab",
			"", "[play]", "docs (folded)",
		}},
		// A hidden group goes, header and all, folded or not; the empty name hides nothing.
		{"hidden", map[string]bool{GroupKey("work"): true}, map[string]bool{"work": true, "art": true, "": true}, SortManual, nil, []string{
			"default", "", "web", "", "notes",
			"", "[personal]", "kitty",
			"", "[play]", "docs", "docs@x",
		}},
		// The mode sorts within each group, and the groups keep their order: kitty, focused last, stays in
		// personal, api comes before gr in work, and notes before web.
		{"activity", nil, nil, SortActivity, map[string]float64{"kitty": 50, "api": 40, "gr@dev": 30, "notes": 20, "web": 10}, []string{
			"default", "", "notes", "", "web",
			"", "[personal]", "kitty",
			"", "[work]", "api", "", "gr", "gr@dev",
			"", "[art]", "lab",
			"", "[play]", "docs", "docs@x",
		}},
	} {
		others, spaces, order := groupFixture()
		order.Mode, order.Focused = c.mode, c.focused
		if got := rowNames(BuildRows(others, spaces, c.folded, c.hidden, order)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}

// A header has no space, and points at its group, which has its spaces in the rows' order.
func TestGroupHeaderRows(t *testing.T) {
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Group: "work", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}, {Path: "/src/gr/dev", Session: "gr@dev"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Path: "/src/kitty", Session: "kitty"}}},
		{Name: "api", Root: "/src/api", Group: "work", Worktrees: []Worktree{{Path: "/src/api", Session: "api"}}},
	}
	gr, kitty, api := &spaces[0], &spaces[1], &spaces[2]
	work := &Group{Name: "work", Spaces: []*Space{gr, api}}
	want := []Row{
		{Space: kitty, Worktree: &kitty.Worktrees[0], Main: true},
		{},
		{Group: work},
		{Space: gr, Worktree: &gr.Worktrees[0], Main: true},
		{Space: gr, Worktree: &gr.Worktrees[1]},
		{},
		{Space: api, Worktree: &api.Worktrees[0], Main: true},
	}
	if got := BuildRows(nil, spaces, nil, nil, Order{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	want = []Row{
		{Space: kitty, Worktree: &kitty.Worktrees[0], Main: true},
		{},
		{Group: work, Folded: true},
	}
	if got := BuildRows(nil, spaces, map[string]bool{GroupKey("work"): true}, nil, Order{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("folded: got %#v\nwant %#v", got, want)
	}
	var blank []bool
	for _, r := range want {
		blank = append(blank, r.Blank())
	}
	if !reflect.DeepEqual(blank, []bool{false, true, false}) {
		t.Errorf("blank %v: only the row between kitty and the header is", blank)
	}
}

// The attention filter shows a group's header above what stirs in it, and a folded group's header alone
// when anything of its spaces does.
func TestGroupAttentionRows(t *testing.T) {
	others, spaces, order := groupFixture()
	statuses := []status.Status{statusOf("gr@dev", "blocked"), statusOf("kitty", "idle"), statusOf("docs@x", "working"), statusOf("web", "done")}
	rows := BuildRows(others, spaces, map[string]bool{GroupKey("play"): true}, nil, order)
	got := rowNames(AttentionRows(rows, statuses))
	want := []string{"web", "", "[work]", "gr", "gr@dev", "", "[play] (folded)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// With only an idle session in a folded group, its header goes too.
	if got := AttentionRows(rows, []status.Status{statusOf("docs", "idle")}); got != nil {
		t.Fatalf("got %q, want none", rowNames(got))
	}
}

// Statuses of a hidden group's spaces' sessions, the space's own and SPACE@DIR, are left out; a session
// in no space and the other spaces' stay.
func TestShownStatuses(t *testing.T) {
	cfg := config.Config{
		Spaces: []string{"/src/gr", "/src/kitty", "/src/api"},
		Space:  map[string]config.SpaceSettings{"gr": {Group: "work"}, "api": {Group: "personal"}},
	}
	statuses := []status.Status{
		statusOf("gr", "done"), statusOf("gr@dev", "blocked"), statusOf("grx", "error"), statusOf("kitty", "done"),
		statusOf("api@v2", "done"), statusOf("default", "blocked"), statusOf("", "error"),
	}
	got := Shown(cfg, map[string]bool{"work": true, "personal": false}, statuses)
	want := []status.Status{statusOf("grx", "error"), statusOf("kitty", "done"), statusOf("api@v2", "done"), statusOf("default", "blocked"), statusOf("", "error")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if got := Shown(cfg, nil, statuses); !reflect.DeepEqual(got, statuses) {
		t.Fatalf("nothing hidden: %v", got)
	}
}

// The hidden groups and the folds, groups' among the spaces', are saved a line each, in order, and read
// back.
func TestGroupState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := SaveHidden(map[string]bool{"work": true, "art": true, "play": false}); err != nil {
		t.Fatal(err)
	}
	if err := SaveFolded(map[string]bool{"/src/gr": true, GroupKey("work"): true, "/src/kitty": false}); err != nil {
		t.Fatal(err)
	}
	type state struct {
		Hidden, Folded         string
		LoadHidden, LoadFolded map[string]bool
	}
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, "kspaces", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	got := state{read("hidden"), read("folded"), LoadHidden(), LoadFolded()}
	want := state{
		Hidden: "art\nwork\n", Folded: "/src/gr\ngroup:work\n",
		LoadHidden: map[string]bool{"art": true, "work": true},
		LoadFolded: map[string]bool{"/src/gr": true, "group:work": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}
