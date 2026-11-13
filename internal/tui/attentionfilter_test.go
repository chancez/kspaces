package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/status"
)

// attentionTitle is the SPACES title while the attention filter is on.
const attentionTitle = "SPACES !" + "                  " + "\uf0c9 \uf067"

// f shows only the sessions whose programs are not idle, under their spaces: dev, blocked, and docs,
// folded, for its working notes; default, idle, goes, as does the error of a window in no session. The
// cursor stays on its row, and f again shows the whole list with it still there.
func TestAttentionFilter(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	unfiltered := e.spacesScreen()
	e.press("f")
	want := []string{
		attentionTitle,
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + blocked + "dev pr/me/dev",
		"",
		"\u25b6 \uf110  docs main",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("attention filter:\n%q\nwant\n%q", got, want)
	}
	if got, want := e.highlighted(), []string{"  " + blocked + "dev pr/me/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q, want the cursor still on dev", got)
	}
	// Rows do not move while some are hidden, as with the text filter: dev would go below fix.
	e.press("J")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("after J:\n%q\nwant\n%q", got, want)
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

// The attention filter and the text filter both apply, the text filter ignoring folds as ever: no shows
// docs' notes, which is working, and k only kitty, which is not.
func TestAttentionFilterQuery(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("f", "/", "n", "o")
	want := []string{
		attentionTitle,
		filterPrompt + "no",
		"  " + repoIcon + "docs main",
		"  \uf110  notes notes",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("attention filter and no:\n%q\nwant\n%q", got, want)
	}
	e.press("backspace", "backspace", "k")
	want = []string{attentionTitle, filterPrompt + "k", "No matches"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("attention filter and k:\n%q\nwant\n%q", got, want)
	}
	// esc ends the text filter, not the attention filter.
	e.press("esc")
	want = []string{
		attentionTitle,
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + blocked + "dev pr/me/dev",
		"",
		"\u25b6 \uf110  docs main",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("after esc:\n%q\nwant\n%q", got, want)
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
	want := []string{attentionTitle, "", "Nothing needs you"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("nothing needs me:\n%q\nwant\n%q", got, want)
	}
	if e.m.cursor != 0 || len(e.m.rows) != 0 {
		t.Errorf("cursor %d with rows %d, want the first status entry:\n%s", e.m.cursor, len(e.m.rows), strings.Join(e.screen(), "\n"))
	}
}
