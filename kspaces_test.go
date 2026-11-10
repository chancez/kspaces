package main

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	porcelain := `worktree /src/gr
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /src/gr/.worktrees/dev
HEAD 2222222222222222222222222222222222222222
branch refs/heads/pr/chancez/dev

worktree /src/gr/.worktrees/gone
HEAD 3333333333333333333333333333333333333333
branch refs/heads/old
prunable gitdir file points to non-existent location

worktree /src/gr/.worktrees/bisect
HEAD 4444444444444444444444444444444444444444
detached
`
	got := parseWorktrees("gr", porcelain)
	want := []Worktree{
		{Path: "/src/gr", Branch: "main", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/chancez/dev", Session: "gr@dev"},
		{Path: "/src/gr/.worktrees/bisect", Branch: "", Session: "gr@bisect"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// A bare repository has no files to open, so its first checked out worktree is the space's main one.
func TestParseWorktreesBare(t *testing.T) {
	porcelain := `worktree /src/gr.git
bare

worktree /src/gr/main
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main
`
	got := parseWorktrees("gr", porcelain)
	want := []Worktree{{Path: "/src/gr/main", Branch: "main", Session: "gr"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestKittyStateFrom(t *testing.T) {
	dock := map[string]string{dockVar: "1"}
	data := []lsOSWindow{{Tabs: []lsTab{
		{ID: 1, Windows: []lsWindow{
			{ID: 10, SessionName: "gr", LastFocusedAt: 5},
			{ID: 11, SessionName: "gr", LastFocusedAt: 9},
			{ID: 12, UserVars: dock},
		}},
		{ID: 2, Windows: []lsWindow{
			{ID: 20, SessionName: "gr", LastFocusedAt: 7},
			{ID: 21, SessionName: "kitty@docks", LastFocusedAt: 1},
			// A dock never counts as a session's window, even if it was saved with one.
			{ID: 22, SessionName: "kitty@docks", LastFocusedAt: 99, UserVars: dock},
		}},
		{ID: 3, Windows: []lsWindow{{ID: 30}}},
	}}}

	got := kittyStateFrom(data, 12)
	// Home is the tab's most recently focused window that is not a dock.
	want := KittyState{Sessions: map[string]int{"gr": 11, "kitty@docks": 21}, Current: "gr", Home: 11}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// A sidebar in a tab with no session highlights nothing.
	got = kittyStateFrom(data, 30)
	want.Current, want.Home = "", 30
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestParseConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	in := `# spaces
space ~/projects/gr
space /src/kitty

new_tab kitten cm_launch.py --type=tab
sessions ~/.local/share/kitty/sessions
`
	got, err := parseConfig(bufio.NewScanner(strings.NewReader(in)))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Spaces:      []string{filepath.Join(home, "projects/gr"), "/src/kitty"},
		NewTab:      []string{"kitten", "cm_launch.py", "--type=tab"},
		SessionsDir: filepath.Join(home, ".local/share/kitty/sessions"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	if _, err := parseConfig(bufio.NewScanner(strings.NewReader("spaces ~/x\n"))); err == nil {
		t.Fatal("an unknown key was accepted")
	}
}

func TestOtherSessions(t *testing.T) {
	spaces := []Space{{Name: "gr", Worktrees: []Worktree{
		{Path: "/src/gr", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Session: "gr@dev"},
	}}}
	open := map[string]int{"gr": 1, "default": 2, "scratch": 3}
	saved := []string{"default", "gr@dev", "old"}

	got := otherSessions(spaces, open, saved)
	want := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "old", Worktrees: []Worktree{{Session: "old"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestStatuses(t *testing.T) {
	data := []lsOSWindow{{Tabs: []lsTab{
		{ID: 1, Windows: []lsWindow{
			// Several records: the most urgent one stands for the window.
			{ID: 10, SessionName: "gr", ProgramStatus: []programRecord{
				{ID: "build", State: "working"},
				{ID: "deploy", State: "blocked", App: "claude", Title: "approve"},
			}},
			{ID: 11, SessionName: "gr", ProgramStatus: []programRecord{{State: "idle"}}},
		}},
		{ID: 2, Windows: []lsWindow{
			{ID: 20, SessionName: "kitty", ProgramStatus: []programRecord{{State: "done", App: "make"}}},
			{ID: 21, Title: "scratch", ProgramStatus: []programRecord{{State: "error", Msg: "exit 1"}}},
			{ID: 22, SessionName: "kitty", UserVars: map[string]string{dockVar: "1"}, ProgramStatus: []programRecord{{State: "blocked"}}},
		}},
	}}}

	got := kittyStateFrom(data, 0).Statuses
	want := []Status{
		{Window: 10, Session: "gr", programRecord: programRecord{ID: "deploy", State: "blocked", App: "claude", Title: "approve"}},
		{Window: 21, WindowTitle: "scratch", programRecord: programRecord{State: "error", Msg: "exit 1"}},
		{Window: 20, Session: "kitty", programRecord: programRecord{State: "done", App: "make"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	gotStates := []string{sessionStatus(got, "gr"), sessionStatus(got, "kitty"), sessionStatus(got, "docs")}
	if wantStates := []string{"blocked", "done", ""}; !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("session states: got %q, want %q", gotStates, wantStates)
	}
}
