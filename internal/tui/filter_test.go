package tui

import (
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// filterPrompt is the filter's line under the SPACES title, before the query.
const filterPrompt = "\uf002  "

// spacesScreen is the title, the filter's line and the spaces list's rows, in the 40 rows newSidebarEnv
// draws, down to the last one not blank.
func (e *sidebarEnv) spacesScreen() []string {
	l := e.m.layout()
	rows := e.screen()[:headerRows+l.spaces]
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// / opens the filter under the title, and the list narrows as I type: a space by its name or a worktree's
// name or branch, with only the worktrees that match under it, and folds ignored so docs' notes shows.
// The cursor stays on its row while it shows.
func TestFilter(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("/")
	want := []string{
		spacesTitle,
		filterPrompt + "filter",
		"  " + termIcon + "default",
		"",
		"  " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"  " + repoIcon + "docs main",
		"  \uf110  notes notes",
		"",
		"  \uf00d  broken",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("filter opened:\n%q\nwant\n%q", got, want)
	}

	e.press("d", "e")
	want = []string{
		spacesTitle,
		filterPrompt + "de",
		"  " + termIcon + "default",
		"",
		"  " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("filtered by de:\n%q\nwant\n%q", got, want)
	}
	if got, want := e.highlighted(), []string{filterPrompt + "de", here + blocked + "dev pr/me/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q, want the filter and the cursor on dev", got)
	}

	// Case does not matter, and a branch matches: NOTES is docs' notes worktree's.
	e.press("backspace", "backspace", "N", "O", "T")
	want = []string{
		spacesTitle,
		filterPrompt + "NOT",
		"  " + repoIcon + "docs main",
		"  \uf110  notes notes",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("filtered by NOT:\n%q\nwant\n%q", got, want)
	}
}

// A query nothing matches says so and leaves the cursor on no row; one that matches again puts it on the
// first row shown.
func TestFilterNoMatches(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("/", "q", "z")
	want := []string{spacesTitle, filterPrompt + "qz", "No matches"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("no matches:\n%q\nwant\n%q", got, want)
	}
	if got, want := e.highlighted(), []string{filterPrompt + "qz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q with no matches", got)
	}
	// enter opens nothing, and the arrows find nothing to move to.
	e.press("down")
	if cmd := e.press("enter"); cmd != nil || e.m.filtering {
		t.Errorf("enter with no matches: ran %v, still filtering %v", cmd != nil, e.m.filtering)
	}
	e.press("/", "q", "z", "backspace")
	want = []string{spacesTitle, filterPrompt + "q", "No matches"}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("q:\n%q\nwant\n%q", got, want)
	}
	e.press("backspace", "k")
	want = []string{
		spacesTitle,
		filterPrompt + "k",
		"  " + repoIcon + "kitty master",
		"",
		"  \uf00d  broken",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("k:\n%q\nwant\n%q", got, want)
	}
	if got, want := e.highlighted(), []string{filterPrompt + "k", "  " + repoIcon + "kitty master"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q, want the cursor on the first row shown", got)
	}
}

// esc clears the filter, leaving the cursor on its row, and the folds are back; enter opens the cursor's
// row and clears it too.
func TestFilterClear(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	unfiltered := e.spacesScreen()
	e.press("/", "n", "o", "t", "e", "s")
	if cmd := e.press("esc"); cmd != nil {
		t.Error("esc left the sidebar rather than clearing the filter")
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) {
		t.Errorf("after esc:\n%q\nwant\n%q", got, unfiltered)
	}
	// The cursor was on notes, which docs, folded, holds again.
	if got, want := e.highlighted(), []string{"\u25b6 \uf110  docs main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after esc", got)
	}

	e.press("/", "f", "i", "down")
	if got, want := e.highlighted(), []string{filterPrompt + "fi", "  " + branchIcon + "fix fix"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("highlighted %q filtering by fi", got)
	}
	if cmd := e.press("enter"); cmd == nil {
		t.Error("enter opened nothing")
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, unfiltered) || e.m.cursor != e.row("gr@fix") {
		t.Errorf("after enter, cursor %d:\n%q\nwant\n%q", e.m.cursor, got, unfiltered)
	}
}

// The cursor moves with the arrows over the rows shown, as j and k type; a row the query hides moves it
// to the first row shown. The scroll indicators count the rows the filter shows.
func TestFilterCursor(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.press("/", "down")
	if got, want := e.highlighted(), []string{filterPrompt + "filter", "  " + branchIcon + "fix fix"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after down", got)
	}
	e.press("d")
	if got, want := e.highlighted(), []string{filterPrompt + "d", "  " + termIcon + "default"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after fix was filtered out", got)
	}
	e.press("down", "down", "down", "down", "down")
	if got, want := e.highlighted(), []string{filterPrompt + "d", "  " + repoIcon + "docs main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("highlighted %q after moving past the end", got)
	}

	// Short, the list scrolls to docs, with notes below it, which docs folds without the filter.
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	e.press("backspace")
	want := []string{
		spacesTitle,
		filterPrompt + "filter",
		"\u25b4 5 more",
		"",
		"  " + repoIcon + "docs main",
		"\u25be 2 more",
	}
	if got := e.screen()[:headerRows+e.m.layout().spaces]; !reflect.DeepEqual(got, want) {
		t.Errorf("short sidebar with an empty query:\n%q\nwant\n%q", got, want)
	}
}

// Keys sent without focus filter too, and the filter goes with remote navigation, at esc or ten seconds
// after the last key.
func TestFilterRemote(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newSidebarEnv(t)
		e.press("/", "k", "i")
		want := []string{spacesTitle, filterPrompt + "ki", "  " + repoIcon + "kitty master"}
		if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
			t.Errorf("remote filter:\n%q\nwant\n%q", got, want)
		}
		time.Sleep(11 * time.Second)
		e.tick()
		if e.m.filtering || e.has(filterPrompt+"ki") {
			t.Errorf("filter left 11s after the last key:\n%s", strings.Join(e.screen(), "\n"))
		}
		e.press("/", "k", "esc")
		if e.m.filtering || e.m.remote {
			t.Errorf("esc: filtering %v, remote %v", e.m.filtering, e.m.remote)
		}
	})
}
