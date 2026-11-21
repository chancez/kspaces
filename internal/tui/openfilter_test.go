package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/state"
	"github.com/chancez/kspaces/internal/tui/look"
)

// openScreen is the fixture's spaces list with the open filter on: default, gr and dev, which have
// windows, and docs, folded, for its notes; fix and kitty, with none, go.
var openScreen = []string{
	filteredTitle("open"),
	"",
	"  \uf120  default",
	"",
	"\u25bc " + repoIcon + "gr main",
	here + blocked + "dev pr/me/dev",
	"",
	"\u25b6 \uf110  docs main",
}

// O shows only the sessions with windows, under their spaces, names the filter on the title and lights
// the funnel, and O again shows the whole list. The cursor stays on its row throughout.
func TestOpenFilter(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	unfiltered := e.spacesScreen()
	e.press("O")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, openScreen) {
		t.Errorf("open filter:\n%q\nwant\n%q", got, openScreen)
	}
	if got, on := e.titleRow(), e.m.th.Render(look.Plain, look.AttentionButton(true, false)); !strings.Contains(got, on) {
		t.Errorf("funnel not lit with the open filter on: %q", got)
	}
	if got, want := e.highlighted(), []string{here + blocked + "dev pr/me/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q, want the cursor still on dev", got)
	}
	e.press("O")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) {
		t.Errorf("open filter off:\n%q\nwant\n%q", got, unfiltered)
	}
}

// The list's menu's Only open sessions turns the open filter on and off, marked while it is on.
func TestOpenMenu(t *testing.T) {
	e := newSidebarEnv(t)
	unfiltered := e.spacesScreen()
	e.click(sortZone, tea.MouseRight)
	e.click(fmt.Sprintf(menuZone, 4), tea.MouseLeft)
	if got := e.spacesScreen(); e.m.menu != nil || !reflect.DeepEqual(got, openScreen) {
		t.Errorf("chose the filter, menu open %v:\n%q\nwant\n%q", e.m.menu != nil, got, openScreen)
	}
	e.click(sortZone, tea.MouseRight)
	want := []string{
		menuRule("Sort"), "\uf00c Manual", "  Activity", "  Status",
		menuRule("Filter"), "  Only what is not idle", "\uf00c Only open sessions", "  Hide suspended sessions",
		menuRule("Rows"), "  Select", menuRule(""),
	}
	if got := e.screen()[1:12]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu with the filter on:\n%q\nwant\n%q", got, want)
	}
	e.click(fmt.Sprintf(menuZone, 4), tea.MouseLeft)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) {
		t.Errorf("chose the filter again:\n%q\nwant\n%q", got, unfiltered)
	}
}

// With no session open the list says so; with the attention filter on too, that nothing matches.
func TestOpenFilterNothing(t *testing.T) {
	e := newSidebarEnv(t)
	e.press("O")
	snap := fixture(time.Now())
	snap.Kitty.Sessions = nil
	e.m.Update(snap)
	want := []string{filteredTitle("open"), "", "Nothing is open"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("nothing open:\n%q\nwant\n%q", got, want)
	}
	e.press("f")
	want = []string{filteredTitle("active, open"), "", "No matches"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("nothing open, attention filter on:\n%q\nwant\n%q", got, want)
	}
}

// The filters on are saved as they change, and a new sidebar, as after a restart, starts with them.
func TestFiltersSaved(t *testing.T) {
	e := newSidebarEnv(t)
	e.press("O", "Z")
	if got, want := spaces.LoadFilters(), (state.Filters{Open: true, Awake: true}); got != want {
		t.Fatalf("saved %+v, want %+v", got, want)
	}
	again := &sidebarEnv{t: t, m: newSidebar(0)}
	again.m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	again.m.Update(fixture(time.Now()))
	if got, want := again.spacesScreen(), e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("new sidebar:\n%q\nwant\n%q", got, want)
	}
	e.press("O", "Z")
	if got := spaces.LoadFilters(); got != (state.Filters{}) {
		t.Errorf("saved %+v after turning them off", got)
	}
}
