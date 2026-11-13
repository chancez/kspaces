package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// attentionScreen is the fixture's spaces list with the attention filter on: dev, blocked, under gr, and
// docs, folded, for its working notes; default, idle, goes, as does the error of a window in no session.
var attentionScreen = []string{
	spacesTitle,
	"",
	"\u25bc " + repoIcon + "gr main",
	"  " + blocked + "dev pr/me/dev",
	"",
	"\u25b6 \uf110  docs main",
}

// titleRow is the first line of the frame, styles and all.
func (e *sidebarEnv) titleRow() string {
	return strings.Split(e.m.render(), "\n")[0]
}

// f shows only the sessions whose programs are not idle, under their spaces, and lights the attention
// filter's button. The cursor stays on its row, and f again shows the whole list with it still there.
func TestAttentionFilter(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	unfiltered := e.spacesScreen()
	on := e.m.th.Render(look.Plain, look.AttentionButton(true, false))
	if got := e.titleRow(); strings.Contains(got, on) {
		t.Errorf("button lit with the filter off: %q", got)
	}
	e.press("f")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, attentionScreen) {
		t.Errorf("attention filter:\n%q\nwant\n%q", got, attentionScreen)
	}
	if got := e.titleRow(); !strings.Contains(got, on) {
		t.Errorf("button not lit with the filter on: %q", got)
	}
	if got, want := e.highlighted(), []string{"  " + blocked + "dev pr/me/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q, want the cursor still on dev", got)
	}
	// Rows do not move while some are hidden, as with the text filter: dev would go below fix.
	e.press("J")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, attentionScreen) {
		t.Errorf("after J:\n%q\nwant\n%q", got, attentionScreen)
	}
	e.press("f")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) {
		t.Errorf("attention filter off:\n%q\nwant\n%q", got, unfiltered)
	}
	if got, want := e.highlighted(), []string{"  " + blocked + "dev pr/me/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after f again", got)
	}

	// From a row it hides, the cursor goes to the first row shown.
	e.m.cursor = e.row("kitty")
	e.press("f")
	if got, want := e.highlighted(), []string{"\u25bc " + repoIcon + "gr main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after f from kitty", got)
	}
}

// The attention filter's button, left of the sort button, is lit while the mouse is on it, and a click
// on it does what f does. The narrowest sidebar keeps the whole title, with no cell before the buttons.
func TestAttentionButton(t *testing.T) {
	e := newSidebarEnv(t)
	unfiltered := e.spacesScreen()
	hot := e.m.th.Render(look.Plain, look.AttentionButton(false, true))
	e.hover(attentionZone)
	if got := e.titleRow(); !strings.Contains(got, hot) {
		t.Errorf("button under the mouse not lit: %q", got)
	}
	e.m.Update(mouseLeftMsg{})
	if got := e.titleRow(); strings.Contains(got, hot) {
		t.Errorf("button lit with the mouse gone: %q", got)
	}

	e.click(attentionZone, tea.MouseLeft)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, attentionScreen) || !e.m.attending {
		t.Errorf("clicked on, attending %v:\n%q\nwant\n%q", e.m.attending, got, attentionScreen)
	}
	if got, on := e.titleRow(), e.m.th.Render(look.Plain, look.AttentionButton(true, false)); !strings.Contains(got, on) {
		t.Errorf("button not lit with the filter on: %q", got)
	}
	e.click(attentionZone, tea.MouseLeft)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) || e.m.attending {
		t.Errorf("clicked off, attending %v:\n%q\nwant\n%q", e.m.attending, got, unfiltered)
	}

	e.m.Update(tea.WindowSizeMsg{Width: 12, Height: 40})
	if got, want := e.screen()[0], "SPACES\uf0b0 \uf0c9 \uf067"; got != want {
		t.Errorf("narrowest title %q, want %q", got, want)
	}
	e.click(attentionZone, tea.MouseLeft)
	if got, want := e.screen()[0], "SPACES\uf0b0 \uf0c9 \uf067"; got != want || !e.m.attending {
		t.Errorf("narrowest title clicked on, attending %v: %q, want %q", e.m.attending, got, want)
	}
}

// listMenu is the spaces list's menu, opened from the title, with the attention filter marked or not.
func listMenu(attending bool) []string {
	mark := "  "
	if attending {
		mark = "\uf00c "
	}
	return []string{
		menuRule("Sort"), "\uf00c Manual", "  Activity", "  Status",
		menuRule("Filter"), mark + "Only what is not idle", menuRule(""),
	}
}

// A right click on the title, the sort button or an empty row of the list opens the list's menu, whose
// Filter item turns the attention filter on and off, marked while it is on.
func TestAttentionMenu(t *testing.T) {
	e := newSidebarEnv(t)
	unfiltered := e.spacesScreen()
	e.clickAt(3, 0, tea.MouseRight)
	if got, want := e.screen()[:8], append([]string{spacesTitle}, listMenu(false)...); !reflect.DeepEqual(got, want) {
		t.Errorf("menu on the title:\n%q\nwant\n%q", got, want)
	}
	e.click(fmt.Sprintf(menuZone, 3), tea.MouseLeft)
	if got := e.spacesScreen(); e.m.menu != nil || !reflect.DeepEqual(got, attentionScreen) {
		t.Errorf("chose the filter, menu open %v:\n%q\nwant\n%q", e.m.menu != nil, got, attentionScreen)
	}

	e.click(sortZone, tea.MouseRight)
	if got, want := e.screen()[:8], append([]string{spacesTitle}, listMenu(true)...); !reflect.DeepEqual(got, want) {
		t.Errorf("menu on the sort button:\n%q\nwant\n%q", got, want)
	}
	e.click(fmt.Sprintf(menuZone, 3), tea.MouseLeft)
	if got := e.spacesScreen(); e.m.menu != nil || !reflect.DeepEqual(got, unfiltered) {
		t.Errorf("chose the filter again, menu open %v:\n%q\nwant\n%q", e.m.menu != nil, got, unfiltered)
	}

	// Row 3 is the blank between default and gr; the menu opens below it.
	e.clickAt(3, 3, tea.MouseRight)
	want := append(append([]string{}, unfiltered[:4]...), listMenu(false)...)
	if got := e.screen()[:11]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu on an empty row:\n%q\nwant\n%q", got, want)
	}
}

// The attention filter and the text filter both apply, the text filter ignoring folds as ever: no shows
// docs' notes, which is working, and k only kitty, which is not.
func TestAttentionFilterQuery(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("f", "/", "n", "o")
	want := []string{
		spacesTitle,
		filterPrompt + "no",
		"  " + repoIcon + "docs main",
		"  \uf110  notes notes",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("attention filter and no:\n%q\nwant\n%q", got, want)
	}
	e.press("backspace", "backspace", "k")
	want = []string{spacesTitle, filterPrompt + "k", "No matches"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("attention filter and k:\n%q\nwant\n%q", got, want)
	}
	// esc ends the text filter, not the attention filter.
	e.press("esc")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, attentionScreen) {
		t.Errorf("after esc:\n%q\nwant\n%q", got, attentionScreen)
	}
}

// With every program idle the list says nothing needs me, and the cursor goes to the status list.
func TestAttentionFilterNothing(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("f")
	snap := fixture(time.Now())
	snap.Kitty.Statuses = []status.Status{{Window: 23, Session: "default", Record: status.Record{State: "idle", App: "claude"}}}
	e.m.Update(snap)
	want := []string{spacesTitle, "", "Nothing needs you"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("nothing needs me:\n%q\nwant\n%q", got, want)
	}
	if e.m.cursor != 0 || len(e.m.rows) != 0 {
		t.Errorf("cursor %d with rows %d, want the first status entry:\n%s", e.m.cursor, len(e.m.rows), strings.Join(e.screen(), "\n"))
	}
}
