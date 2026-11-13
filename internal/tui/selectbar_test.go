package tui

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// bar is the select mode's bar and what is under it, the STATUS rule and title.
func (e *sidebarEnv) bar() []string {
	at := headerRows + e.m.layout().spaces
	return e.screen()[at : at+e.m.barHeight()+2]
}

// buttons are the bar's rows that can be clicked, by title, sorted.
func (e *sidebarEnv) buttons() []string {
	var out []string
	for _, b := range e.m.bar {
		out = append(out, b.Title)
	}
	slices.Sort(out)
	return out
}

// barRowY is the screen row of the bar's row of the action called name, greyed or not.
func (e *sidebarEnv) barRowY(name string) int {
	e.t.Helper()
	at := headerRows + e.m.layout().spaces
	for y, l := range e.screen()[at:] {
		if strings.HasPrefix(l, " "+name) {
			return at + y
		}
	}
	e.t.Fatalf("no row %q in the bar: %q", name, e.bar())
	return 0
}

// clickButton clicks the bar's row of the action titled title.
func (e *sidebarEnv) clickButton(title string) tea.Cmd {
	e.t.Helper()
	for id, b := range e.m.bar {
		if b.Title == title {
			return e.click(id, tea.MouseLeft)
		}
	}
	e.t.Fatalf("no row %q in the bar: %q", title, e.bar())
	return nil
}

// barTitle is the bar's first line, width cells wide: a rule with count set into it.
func barTitle(count string, width int) string {
	return "\u2500 " + count + " " + strings.Repeat("\u2500", width-3-len(count))
}

// barItem is a row of the bar width cells wide: the action's name, then count and key right-aligned in
// columns countW and keyW wide, none when 0, two cells apart, and a cell after.
func barItem(name, count, key string, width, countW, keyW int) string {
	right := ""
	if countW > 0 {
		right = fmt.Sprintf("%*s", countW, count)
	}
	if keyW > 0 {
		if right != "" {
			right += "  "
		}
		right += fmt.Sprintf("%*s", keyW, key)
	}
	return strings.TrimRight(" "+name+strings.Repeat(" ", width-1-len(name)-len(right)-1)+right, " ")
}

// The bar, at the bottom of the spaces list, is titled by a rule with how many rows are ticked. Under it is
// a row for each action, as a menu's items: its name, how many ticked rows it applies to, - for none, when
// it is greyed, and its key, the counts and keys each in a column of their own, Cancel last: Remove for a
// linked or missing worktree, Close for a session with windows, Forget for one with only a saved file, and
// Run, with commands configured, for a worktree. A narrow sidebar drops the counts' column, then the
// keys'. The hint line says what the greyed ones have nothing to do to, and with the mouse on a row why it
// is greyed, or what it does, a row that does something lit.
func TestSelectBar(t *testing.T) {
	e := newSidebarEnv(t)
	e.press("v")
	rule := strings.Repeat("\u2500", 30)
	item := func(name, count, key string) string { return barItem(name, count, key, 30, 1, 3) }
	check := func(name string, bar, buttons []string) {
		t.Helper()
		if got := e.bar(); !reflect.DeepEqual(got, bar) {
			t.Errorf("%s, bar:\n%q\nwant\n%q", name, got, bar)
		}
		if got := e.buttons(); !reflect.DeepEqual(got, buttons) {
			t.Errorf("%s, rows that can be clicked %q, want %q", name, got, buttons)
		}
	}
	check("nothing ticked", []string{barTitle("0 selected", 30), item("Remove", "-", "x"), item("Close", "-", "c"),
		item("Forget", "-", "F"), item("Cancel", "", "esc"), "Click or drag over rows to ti\u2026", rule, "STATUS"}, []string{"Cancel"})
	e.clickRow(e.row("gr@fix"))
	check("fix ticked, no windows, no saved file", []string{barTitle("1 selected", 30), item("Remove", "1", "x"), item("Close", "-", "c"),
		item("Forget", "-", "F"), item("Cancel", "", "esc"), "Nothing to close or forget", rule, "STATUS"}, []string{"Cancel", "Remove"})
	e.clickRow(e.row("gr@dev"))
	ticked := []string{barTitle("2 selected", 30), item("Remove", "2", "x"), item("Close", "1", "c"),
		item("Forget", "-", "F"), item("Cancel", "", "esc"), "Nothing to forget", rule, "STATUS"}
	check("dev ticked, with windows", ticked, []string{"Cancel", "Close", "Remove"})

	// The greyed row is muted, the others in the text's color.
	frame := cells.Parse(e.m.render())
	th := e.m.th
	for name, want := range map[string]look.Span{"Remove": look.Text("Remove"), "Forget": look.Muted("Forget")} {
		if got, want := frame[e.barRowY(name)][1:7], cells.Parse(th.Render(look.Plain, want))[0]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s drawn %v, want %v", name, got, want)
		}
	}

	// Pointing at a greyed row says why and lights nothing; at one that does something, says what, and
	// lights it, across the whole row.
	e.m.Update(tea.MouseMotionMsg{X: 25, Y: e.barRowY("Forget")})
	if got, lit := e.bar()[5], e.highlighted(); got != "None has only a saved file" || lit != nil {
		t.Errorf("on Forget, the hint says %q, lit %q", got, lit)
	}
	e.m.Update(tea.MouseMotionMsg{X: 25, Y: e.barRowY("Remove")})
	if got, lit := e.bar()[5], e.highlighted(); got != "Removes 2 worktrees, asking f\u2026" || !reflect.DeepEqual(lit, []string{ticked[1]}) {
		t.Errorf("on Remove, the hint says %q, lit %q", got, lit)
	}

	// A click anywhere on a greyed row does nothing; on Cancel, at its far end, leaves the mode.
	if cmd := e.clickAt(25, e.barRowY("Forget"), tea.MouseLeft); cmd != nil || !e.m.selecting {
		t.Errorf("a click on Forget ran %v, selecting %v", cmd != nil, e.m.selecting)
	}
	e.clickAt(27, e.barRowY("Cancel"), tea.MouseLeft)
	if e.m.selecting {
		t.Error("a click on Cancel left the mode on")
	}
	e.press("v")
	e.clickRow(e.row("gr@fix"))
	e.clickRow(e.row("gr@dev"))
	e.m.Update(tea.MouseMotionMsg{X: 0, Y: 0})

	e.m.Update(tea.WindowSizeMsg{Width: 14, Height: 40})
	e.m.render()
	narrow := func(name, key string) string { return barItem(name, "", key, 14, 0, 3) }
	check("narrow, the counts left out", []string{barTitle("2", 14), narrow("Remove", "x"), narrow("Close", "c"), narrow("Forget", "F"),
		narrow("Cancel", "esc"), "Nothing to fo\u2026", strings.Repeat("\u2500", 14), "STATUS"}, []string{"Cancel", "Close", "Remove"})
	e.m.Update(tea.WindowSizeMsg{Width: 12, Height: 40})
	e.m.render()
	check("narrowest, the keys left out too", []string{barTitle("2", 12), " Remove", " Close", " Forget", " Cancel",
		"Nothing to \u2026", strings.Repeat("\u2500", 12), "STATUS"}, []string{"Cancel", "Close", "Remove"})

	snap := fixture(time.Now())
	cfg := testCommands(t)
	snap.Config = &cfg
	e.m.Update(snap)
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	e.m.render()
	check("with commands", []string{barTitle("2 selected", 30), item("Remove", "2", "x"), item("Close", "1", "c"), item("Forget", "-", "F"),
		item("Run", "2", "R"), item("Cancel", "", "esc"), "Nothing to forget", rule, "STATUS"}, []string{"Cancel", "Close", "Remove", "Run"})
}

// Each action opens its form for what it applies to, in the list's order: Remove the clean up form with
// every row ticked, which says why it leaves the main worktree and a session in no space; Close the close
// form with the sessions that have windows; Forget the forget form with those that have only a saved
// file; and Run the run form in the worktrees, the first one's first command chosen. A key does as its
// button does, and one that applies to nothing ticked says so rather than reaching the cursor's row.
func TestSelectActions(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	form := func(args ...string) []string {
		return append([]string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:10", exe}, args...)
	}
	e := newMissingEnv(t, t.TempDir())
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 40})
	launched := fakeKitten(t)
	e.press("v")
	// gr's row ticks all of gr: its main worktree, dev, and the missing gr@open and gr@saved.
	for _, s := range []string{"default", "gr"} {
		e.clickRow(e.row(s))
	}
	run := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("ran nothing")
		}
		if msg := cmd(); msg != nil {
			t.Errorf("said %v", msg)
		}
	}
	run(e.clickButton("Remove"))
	if got, want := launched(), form("cleanup", "--session", "default", "--session", "gr", "--session", "gr@dev", "--session", "gr@open", "--session", "gr@saved"); !reflect.DeepEqual(got, want) {
		t.Errorf("Remove launched\n%q\nwant\n%q", got, want)
	}
	run(e.press("c"))
	if got, want := launched(), form("close", "default", "gr", "gr@open"); !reflect.DeepEqual(got, want) {
		t.Errorf("c launched\n%q\nwant\n%q", got, want)
	}
	run(e.clickButton("Forget"))
	if got, want := launched(), form("forget", "gr@saved"); !reflect.DeepEqual(got, want) {
		t.Errorf("Forget launched\n%q\nwant\n%q", got, want)
	}

	// Nothing ticked is a worktree to run in but gr and dev, nothing to forget once only dev is ticked.
	snap := missingFixture("")
	cfg := testCommands(t)
	cfg.SessionsDir = ""
	snap.Config = &cfg
	e.m.Update(snap)
	e.m.render()
	run(e.press("R"))
	if got, want := launched(), form("run", "claude", "/src/gr", "/src/gr/.worktrees/dev"); !reflect.DeepEqual(got, want) {
		t.Errorf("R launched\n%q\nwant\n%q", got, want)
	}
	e.press("esc", "v")
	if cmd := e.press("x"); cmd != nil || e.m.failed == nil || e.m.failed.Error() != "Nothing is ticked" {
		t.Errorf("x with nothing ticked: cmd %v, failed %v", cmd != nil, e.m.failed)
	}
	e.clickRow(e.row("gr@dev"))
	if cmd := e.press("F"); cmd != nil || e.m.failed == nil || e.m.failed.Error() != noneToForget {
		t.Errorf("F with only dev ticked: cmd %v, failed %v", cmd != nil, e.m.failed)
	}
	if !e.m.selecting {
		t.Error("the mode ended")
	}
}

// The mode ends once the rows show the action confirmed, as the form runs in a popup of its own: here the
// worktree removed. Cancelled, the rows stay as they were, and so do the ticks.
func TestSelectEndsWhenDone(t *testing.T) {
	e := newSidebarEnv(t)
	fakeKitten(t)
	e.press("v")
	e.clickRow(e.row("gr@fix"))
	e.press("x")()
	e.m.Update(fixture(time.Now()))
	if !e.m.selecting || !reflect.DeepEqual(e.tickedSessions(), []string{"gr@fix"}) {
		t.Errorf("cancelled, selecting %v, ticked %q", e.m.selecting, e.tickedSessions())
	}
	snap := fixture(time.Now())
	snap.Spaces[0].Worktrees = snap.Spaces[0].Worktrees[:2]
	e.m.Update(snap)
	if e.m.selecting || e.m.ticked != nil {
		t.Errorf("fix removed, selecting %v, ticked %q", e.m.selecting, e.tickedSessions())
	}
}
