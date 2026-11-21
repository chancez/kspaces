package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/chancez/kspaces/internal/state"
)

// Dragging the rule moves it as far as each list keeps a row, and letting go keeps the STATUS list's
// height for the next sidebar.
func TestDragSplit(t *testing.T) {
	e := newSidebarEnv(t)
	if got, want := e.m.layout(), (layout{spaces: 27, status: 8, statusTop: 31}); got != want {
		t.Fatalf("default layout = %+v, want %+v", got, want)
	}
	x, y := e.zone(splitZone)
	e.m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	steps := []struct {
		y    int
		want layout
	}{
		{27, layout{spaces: 25, status: 10, statusTop: 29}},
		{100, layout{spaces: 34, status: 1, statusTop: 38}},
		{0, layout{spaces: 1, status: 34, statusTop: 5}},
		{27, layout{spaces: 25, status: 10, statusTop: 29}},
	}
	for _, s := range steps {
		e.m.Update(tea.MouseMotionMsg{X: x, Y: s.y, Button: tea.MouseLeft})
		if got := e.m.layout(); got != s.want {
			t.Errorf("dragged to %d: layout = %+v, want %+v", s.y, got, s.want)
		}
	}
	e.m.Update(tea.MouseReleaseMsg{X: x, Y: 27, Button: tea.MouseLeft})
	if got := state.Load().Sidebar.StatusRows; got != 10 {
		t.Fatalf("saved status rows = %d, want 10", got)
	}
	if e.screen()[27] != strings.Repeat("\u2500", 30) {
		t.Errorf("no rule at 27:\n%q", e.screen())
	}

	// A new sidebar opens at it, and moving without the button no longer drags.
	next := newSidebar(0)
	next.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	if got, want := next.layout(), (layout{spaces: 25, status: 10, statusTop: 29}); got != want {
		t.Errorf("next sidebar's layout = %+v, want %+v", got, want)
	}
	e.m.Update(tea.MouseMotionMsg{X: x, Y: 10})
	if got, want := e.m.layout(), (layout{spaces: 25, status: 10, statusTop: 29}); got != want {
		t.Errorf("layout after letting go = %+v, want %+v", got, want)
	}
}
