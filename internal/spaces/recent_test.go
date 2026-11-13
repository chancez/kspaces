package spaces

import (
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/kitty"
)

// The session to go back to is the one focused most recently other than the focused window's, docks left
// out, the first by name of equal times, in a space or not.
func TestRecentSession(t *testing.T) {
	t.Setenv(kitty.FromEnv, "")
	dock := map[string]string{"kspaces": "1"}
	// Each tab has a dock, focused last of all, in notes, which has no other window. The focused window is
	// the active one of the active tab.
	tab := func(focused int, times map[int]float64, id int, windows ...kitty.Window) kitty.Tab {
		tb := kitty.Tab{ID: id}
		for _, w := range windows {
			w.LastFocusedAt = times[w.ID]
			w.IsActive = w.ID == focused
			tb.IsActive = tb.IsActive || w.IsActive
			tb.Windows = append(tb.Windows, w)
		}
		tb.Windows = append(tb.Windows, kitty.Window{ID: id*10 + 9, SessionName: "notes", LastFocusedAt: 100, UserVars: dock})
		return tb
	}
	state := func(focused int, times map[int]float64) kitty.State {
		data := []kitty.OSWindow{{IsFocused: true, Tabs: []kitty.Tab{
			tab(focused, times, 1, kitty.Window{ID: 10, SessionName: "gr"}),
			tab(focused, times, 2, kitty.Window{ID: 20, SessionName: "gr@dev"}, kitty.Window{ID: 21, SessionName: "gr@dev"}),
			tab(focused, times, 3, kitty.Window{ID: 30, SessionName: "default"}),
			tab(focused, times, 4, kitty.Window{ID: 40}),
		}}}
		return kitty.StateOf(data, kitty.FocusedWindow)
	}
	choose := func(st kitty.State) []any {
		s, ok := recentSession(st)
		return []any{s, ok}
	}
	alone := kitty.StateOf([]kitty.OSWindow{{IsFocused: true, Tabs: []kitty.Tab{
		tab(10, map[int]float64{10: 9}, 1, kitty.Window{ID: 10, SessionName: "gr"}),
	}}}, kitty.FocusedWindow)
	got := [][]any{
		// From gr, gr@dev's latest window is the most recent.
		choose(state(10, map[int]float64{10: 9, 20: 3, 21: 7, 30: 5, 40: 8})),
		// default, in no space, is.
		choose(state(10, map[int]float64{10: 9, 20: 3, 21: 4, 30: 5})),
		// Equal times, as from a kitty that gives none: the first by name.
		choose(state(10, nil)),
		// From default, in no space.
		choose(state(30, map[int]float64{10: 4, 20: 3, 30: 9})),
		// From a tab in no session, any session counts.
		choose(state(40, map[int]float64{10: 4, 20: 3, 30: 5, 40: 9})),
		// gr alone has windows: there is no other session to go to.
		choose(alone),
	}
	want := [][]any{
		{"gr@dev", true}, {"default", true}, {"default", true}, {"gr", true}, {"default", true}, {"", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}
