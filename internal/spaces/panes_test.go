package spaces

import (
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/kitty"
)

// Every window but kspaces' own, the most recently focused first, with what runs in it: for a provider's
// client, what the provider says.
func TestPanesOf(t *testing.T) {
	tabs := []kitty.FullTab{
		{Title: "edit", Windows: []kitty.FullWindow{
			{ID: 1, Title: "nvim", SessionName: "gr", Cwd: "/src/gr", LastFocusedAt: 5, LastReportedCmdline: "nvim main.go"},
			{ID: 2, SessionName: "gr", Cwd: "/src/gr", LastFocusedAt: 9, Cmdline: []string{"mux", "attach", "busy"}},
			{ID: 3, SessionName: "gr", LastFocusedAt: 10, UserVars: map[string]string{"kspaces_picker": "1"}},
		}},
		{Title: "zsh", Windows: []kitty.FullWindow{
			{ID: 4, Title: "~", Cwd: "/home", LastFocusedAt: 7, AtPrompt: true},
		}},
	}
	shows := map[int]ProviderSession{2: {Name: "busy", Command: "gr --watch", Cwd: "/src/gr/web"}}
	got := panesOf(tabs, shows)
	want := []Pane{
		{Window: 2, Session: "gr", Tab: "edit", Program: "gr --watch", Cwd: "/src/gr/web", FocusedAt: 9},
		{Window: 4, Title: "~", Tab: "zsh", Cwd: "/home", FocusedAt: 7},
		{Window: 1, Session: "gr", Title: "nvim", Tab: "edit", Program: "nvim main.go", Cwd: "/src/gr", FocusedAt: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
