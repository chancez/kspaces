package kitty

import (
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/status"
)

func TestStateFrom(t *testing.T) {
	dock := map[string]string{dockVar: "1"}
	data := []OSWindow{{Tabs: []Tab{
		{ID: 1, Windows: []Window{
			{ID: 10, SessionName: "gr", LastFocusedAt: 5, Cwd: "/src/gr/docs"},
			{ID: 11, SessionName: "gr", LastFocusedAt: 9, Cwd: "/src/gr"},
			{ID: 12, UserVars: dock, Cwd: "/"},
		}},
		{ID: 2, Windows: []Window{
			{ID: 20, SessionName: "gr", LastFocusedAt: 7},
			{ID: 21, SessionName: "kitty@docks", LastFocusedAt: 1},
			// A dock never counts as a session's window, even if it was saved with one.
			{ID: 22, SessionName: "kitty@docks", LastFocusedAt: 99, UserVars: dock},
		}},
		{ID: 3, Windows: []Window{{ID: 30, Cwd: "/tmp"}}},
	}}}

	got := stateFrom(data, 12)
	// Home is the tab's most recently focused window that is not a dock, and HomeDir its directory.
	// Windows counts each session's windows, docks left out.
	want := State{
		Sessions: map[string]int{"gr": 11, "kitty@docks": 21}, Windows: map[string]int{"gr": 3, "kitty@docks": 1},
		Current: "gr", Home: 11, HomeDir: "/src/gr",
		Screens: map[string]int{"gr": 11, "kitty@docks": 21}, Own: map[int]bool{12: true, 22: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// A sidebar in a tab with no session highlights nothing.
	got = stateFrom(data, 30)
	want.Current, want.Home, want.HomeDir = "", 30, "/tmp"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// A session's screen is its most recently focused window, as goto_session picks it, leaving out the
// picker and kspaces' docks and forms; without focus times, the active window of its active tab.
func TestScreens(t *testing.T) {
	picker := map[string]string{"kspaces_picker": "1"}
	data := []OSWindow{{Tabs: []Tab{
		{ID: 1, IsActive: true, Windows: []Window{
			{ID: 10, SessionName: "gr", LastFocusedAt: 5},
			{ID: 11, SessionName: "gr", LastFocusedAt: 9},
			{ID: 12, SessionName: "gr", LastFocusedAt: 20, UserVars: map[string]string{dockVar: "1"}},
			{ID: 13, SessionName: "gr", LastFocusedAt: 30, UserVars: map[string]string{"kspaces_form": "1"}},
			// The picker's overlay, which has focus.
			{ID: 14, SessionName: "gr", LastFocusedAt: 40, IsActive: true, UserVars: picker},
		}},
		// An older kitty gives no times.
		{ID: 2, Windows: []Window{{ID: 20, SessionName: "kitty"}, {ID: 21, SessionName: "kitty", IsActive: true}}},
		{ID: 3, IsActive: true, Windows: []Window{{ID: 30, SessionName: "kitty"}, {ID: 31, SessionName: "kitty", IsActive: true}}},
		{ID: 4, Windows: []Window{{ID: 40, SessionName: "docs"}, {ID: 41, SessionName: "docs"}}},
		// Only the picker: nothing to show.
		{ID: 5, IsActive: true, Windows: []Window{{ID: 50, SessionName: "notes", LastFocusedAt: 3, IsActive: true}}},
	}}}

	got := stateFrom(data, 50)
	want := State{
		Sessions: map[string]int{"gr": 14, "kitty": 20, "docs": 40, "notes": 50},
		Windows:  map[string]int{"gr": 4, "kitty": 4, "docs": 2, "notes": 1},
		Current:  "notes", Home: 50,
		Screens: map[string]int{"gr": 11, "kitty": 31, "docs": 40},
		Own:     map[int]bool{12: true, 13: true, 14: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestStatuses(t *testing.T) {
	at := func(sec int64) time.Time { return time.Unix(sec, 0) }
	data := []OSWindow{{Tabs: []Tab{
		{ID: 1, Windows: []Window{
			// Several records: the most urgent one stands for the window. A noted time counts only for the
			// state it was noted for: build has changed to working since.
			{
				ID: 10, SessionName: "gr",
				UserVars: map[string]string{status.AtVar: `{"pid": 1, "records": {"deploy": ["blocked", 100], "build": ["blocked", 50]}}`},
				ProgramStatus: []status.Record{
					{ID: "build", State: "working"},
					{ID: "deploy", State: "blocked", App: "claude", Title: "approve"},
				},
			},
			// Idle is listed: the program is waiting for its next task.
			{ID: 11, SessionName: "gr", ProgramStatus: []status.Record{{State: "idle", App: "claude"}}},
			// A broken var only loses the times.
			{ID: 12, SessionName: "gr", UserVars: map[string]string{status.AtVar: "{"}, ProgramStatus: []status.Record{{State: "done", App: "go"}}},
		}},
		{ID: 2, Windows: []Window{
			{ID: 20, SessionName: "kitty", UserVars: map[string]string{status.AtVar: `{"records": {"": ["done", 300]}}`}, ProgramStatus: []status.Record{{State: "done", App: "make"}}},
			{ID: 21, Title: "scratch", ProgramStatus: []status.Record{{State: "error", Msg: "exit 1"}}},
			{ID: 22, SessionName: "kitty", UserVars: map[string]string{dockVar: "1"}, ProgramStatus: []status.Record{{State: "blocked"}}},
		}},
	}}}

	got := stateFrom(data, 0).Statuses
	// Grouped by state, most urgent first, and within a state the most recent change first: the done
	// record noted at 300 comes before the one with no time.
	want := []status.Status{
		{
			Window: 10, Session: "gr",
			Record: status.Record{ID: "deploy", State: "blocked", App: "claude", Title: "approve", Since: at(100)},
			Others: []status.Record{{ID: "build", State: "working"}},
		},
		{Window: 21, WindowTitle: "scratch", Record: status.Record{State: "error", Msg: "exit 1"}},
		{Window: 20, Session: "kitty", Record: status.Record{State: "done", App: "make", Since: at(300)}},
		{Window: 12, Session: "gr", Record: status.Record{State: "done", App: "go"}},
		{Window: 11, Session: "gr", Record: status.Record{State: "idle", App: "claude"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	gotStates := []string{status.OfSession(got, "gr"), status.OfSession(got, "kitty"), status.OfSession(got, "docs")}
	if wantStates := []string{"blocked", "done", ""}; !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("session states: got %q, want %q", gotStates, wantStates)
	}
}

func TestKeyboardWindow(t *testing.T) {
	tabs := func(active int) []Tab {
		return []Tab{
			{ID: 1, IsActive: active == 1, Windows: []Window{{ID: 10, IsActive: true}, {ID: 11}}},
			{ID: 2, IsActive: active == 2, Windows: []Window{{ID: 20}, {ID: 21, IsActive: true}}},
		}
	}
	unfocused := []OSWindow{{Tabs: tabs(1)}, {LastFocused: true, Tabs: tabs(2)}}
	focused := []OSWindow{{IsFocused: true, Tabs: tabs(1)}, {LastFocused: true, Tabs: tabs(2)}}
	got := []int{keyboardWindow(unfocused), keyboardWindow(focused), keyboardWindow(nil)}
	// Without a focused OS window, the one focused last stands in.
	if want := []int{21, 10, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
