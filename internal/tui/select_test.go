package tui

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A row's box in the select mode, with the space after it.
const (
	unticked = "\u2610 "
	ticked   = "\u2611 "
	partly   = "\u229f "
)

// selectScreen is the fixture's spaces list in the select mode, each row with its box, or the blank of
// broken, which has nothing to tick.
func selectScreen(def, gr, dev, fix, kitty, docs string) []string {
	return []string{
		spacesTitle, "",
		"  " + def + termIcon + "default", "",
		"\u25bc " + gr + repoIcon + "gr main",
		here + dev + blocked + "dev pr/me/dev",
		"  " + fix + branchIcon + "fix fix", "",
		"  " + kitty + repoIcon + "kitty master", "",
		"\u25b6 " + docs + "\uf110  docs main", "",
		"    \uf00d  broken",
	}
}

// tickedSessions are the sessions ticked, sorted.
func (e *sidebarEnv) tickedSessions() []string {
	var out []string
	for s := range e.m.ticked {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// menuItem clicks the item of the open menu titled title.
func (e *sidebarEnv) menuItem(title string) tea.Cmd {
	e.t.Helper()
	for i, it := range e.m.menu.items {
		if it.Title == title {
			return e.click(fmt.Sprintf(menuZone, i), tea.MouseLeft)
		}
	}
	e.t.Fatalf("no menu item %q", title)
	return nil
}

// rowY is the screen row of the spaces row of session.
func (e *sidebarEnv) rowY(session string) int {
	e.t.Helper()
	_, y := e.zone(fmt.Sprintf(entryZone, e.row(session)))
	return y
}

// press, drag and release are the left button going down on the screen row y, the mouse moving to y with
// it down, and letting go there, with mods held.
func (e *sidebarEnv) pressAt(y int, mod tea.KeyMod) {
	e.m.Update(tea.MouseClickMsg{X: 8, Y: y, Button: tea.MouseLeft, Mod: mod})
	e.m.render()
}

func (e *sidebarEnv) dragAt(y int) {
	e.m.Update(tea.MouseMotionMsg{X: 8, Y: y, Button: tea.MouseLeft})
	e.m.render()
}

func (e *sidebarEnv) releaseAt(y int) tea.Cmd {
	_, cmd := e.m.Update(tea.MouseReleaseMsg{X: 8, Y: y, Button: tea.MouseLeft})
	e.m.render()
	return cmd
}

// The select mode comes on with v, with Select in a row's menu, which ticks the row, or in the list's
// menu, and with alt and a click on a row, which ticks it. Every row that can be acted on gets a box; a
// space's row is partly ticked while only some of its worktrees are.
func TestSelectEnter(t *testing.T) {
	none := selectScreen(unticked, unticked, unticked, unticked, unticked, unticked)
	for name, tc := range map[string]struct {
		enter func(e *sidebarEnv)
		want  []string
	}{
		"v": {func(e *sidebarEnv) { e.press("v") }, none},
		"row menu": {func(e *sidebarEnv) {
			e.click(fmt.Sprintf(entryZone, e.row("gr@dev")), tea.MouseRight)
			e.menuItem("Select")
		}, selectScreen(unticked, partly, ticked, unticked, unticked, unticked)},
		"list menu": {func(e *sidebarEnv) {
			e.clickAt(3, 0, tea.MouseRight)
			e.menuItem("Select")
		}, none},
		"alt click": {func(e *sidebarEnv) {
			y := e.rowY("gr@fix")
			e.pressAt(y, tea.ModAlt)
			e.releaseAt(y)
		}, selectScreen(unticked, partly, unticked, ticked, unticked, unticked)},
	} {
		t.Run(name, func(t *testing.T) {
			e := newSidebarEnv(t)
			tc.enter(e)
			if got := e.spacesScreen(); !e.m.selecting || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selecting %v:\n%q\nwant\n%q", e.m.selecting, got, tc.want)
			}
		})
	}
}

// In the select mode a click on a row ticks it, and again unticks it, rather than opening it; outside it a
// click opens the row. A click on a caret still folds.
func TestSelectClick(t *testing.T) {
	e := newSidebarEnv(t)
	if cmd := e.click(fmt.Sprintf(entryZone, e.row("gr@dev")), tea.MouseLeft); cmd == nil {
		t.Fatal("a click outside the mode opened nothing")
	}
	e.press("v")
	dev := fmt.Sprintf(entryZone, e.row("gr@dev"))
	if cmd := e.click(dev, tea.MouseLeft); cmd != nil {
		t.Errorf("a click in the mode ran %v", cmd())
	}
	if got, want := e.spacesScreen(), selectScreen(unticked, partly, ticked, unticked, unticked, unticked); !reflect.DeepEqual(got, want) {
		t.Errorf("dev clicked:\n%q\nwant\n%q", got, want)
	}
	e.click(dev, tea.MouseLeft)
	if got, want := e.spacesScreen(), selectScreen(unticked, unticked, unticked, unticked, unticked, unticked); !reflect.DeepEqual(got, want) {
		t.Errorf("dev clicked again:\n%q\nwant\n%q", got, want)
	}
	e.click(fmt.Sprintf(caretZone, e.row("gr")), tea.MouseLeft)
	want := []string{
		spacesTitle, "",
		"  " + unticked + termIcon + "default", "",
		"\u25b6 " + unticked + blocked + "gr main", "",
		"  " + unticked + repoIcon + "kitty master", "",
		"\u25b6 " + unticked + "\uf110  docs main", "",
		"    \uf00d  broken",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("gr's caret clicked:\n%q\nwant\n%q", got, want)
	}
}

// Pressing on a row and dragging gives every row the mouse passes what the first changed to, the rows
// between two reports too, down and up; nothing moves. alt and a drag outside the mode turns it on and
// paints.
func TestSelectPaint(t *testing.T) {
	e := newSidebarEnv(t)
	e.press("v")
	dev, kitty := e.rowY("gr@dev"), e.rowY("kitty")
	e.pressAt(dev, 0)
	e.dragAt(kitty)
	if cmd := e.releaseAt(kitty); cmd != nil {
		t.Errorf("letting go ran %v", cmd())
	}
	if got, want := e.spacesScreen(), selectScreen(unticked, partly, ticked, ticked, ticked, unticked); !reflect.DeepEqual(got, want) {
		t.Errorf("painted down:\n%q\nwant\n%q", got, want)
	}
	if got := stateFile(t, "order"); got != "" {
		t.Errorf("painting moved rows: %q", got)
	}
	e.pressAt(kitty, 0)
	e.dragAt(e.rowY("gr@fix"))
	e.dragAt(e.rowY("default"))
	e.releaseAt(e.rowY("default"))
	if got, want := e.spacesScreen(), selectScreen(unticked, unticked, unticked, unticked, unticked, unticked); !reflect.DeepEqual(got, want) {
		t.Errorf("painted up from kitty, ticked, so unticking:\n%q\nwant\n%q", got, want)
	}

	e = newSidebarEnv(t)
	e.pressAt(e.rowY("gr@dev"), tea.ModAlt)
	e.dragAt(e.rowY("gr@fix"))
	e.releaseAt(e.rowY("gr@fix"))
	if got, want := e.spacesScreen(), selectScreen(unticked, partly, ticked, ticked, unticked, unticked); !e.m.selecting || !reflect.DeepEqual(got, want) {
		t.Errorf("alt and a drag, selecting %v:\n%q\nwant\n%q", e.m.selecting, got, want)
	}
}

// space ticks the cursor's row, A every row, folded worktrees too, and esc leaves the mode, letting go of
// the ticks.
func TestSelectKeys(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.row("gr@dev")
	e.press("v", "space", "j", "space")
	if got, want := e.spacesScreen(), selectScreen(unticked, partly, ticked, ticked, unticked, unticked); !reflect.DeepEqual(got, want) {
		t.Errorf("dev and fix ticked:\n%q\nwant\n%q", got, want)
	}
	e.press("A")
	if got, want := e.spacesScreen(), selectScreen(ticked, ticked, ticked, ticked, ticked, ticked); !reflect.DeepEqual(got, want) {
		t.Errorf("all ticked:\n%q\nwant\n%q", got, want)
	}
	if got, want := e.tickedSessions(), []string{"default", "docs", "docs@notes", "gr", "gr@dev", "gr@fix", "kitty"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ticked %q, want %q", got, want)
	}
	e.press("esc")
	if got := e.spacesScreen(); e.m.selecting || e.m.ticked != nil || !reflect.DeepEqual(got, plainScreen()) {
		t.Errorf("esc, selecting %v, ticked %v:\n%q\nwant\n%q", e.m.selecting, e.m.ticked, got, plainScreen())
	}
}

// In the select mode no row gets the cursor's bar: a click ticks its row's box and leaves the cursor where
// it was, and the cursor, which space ticks, is the one box lit, reversed.
func TestSelectCursor(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.row("gr@fix")
	e.press("v")
	e.clickRow(e.row("kitty"))
	if e.m.cursor != e.row("gr@fix") || !e.m.ticked["kitty"] {
		t.Errorf("clicking kitty moved the cursor to %d, ticked %q", e.m.cursor, e.tickedSessions())
	}
	frame := cells.Parse(e.m.render())
	lit := cells.Parse(e.m.th.Render(look.Plain, look.Box(look.Unticked, false)[0].With(look.Lit)))[0][0]
	bar := cells.Parse(e.m.th.Render(look.Cursor, look.Text("x")))[0][0].Style
	for _, session := range []string{"gr@fix", "kitty"} {
		row := frame[e.rowY(session)]
		for x, c := range row {
			switch {
			case session == "gr@fix" && x == 2:
				if c != lit {
					t.Errorf("fix's box drawn %v, want %v", c, lit)
				}
			case c.Style == lit.Style || c.Style == bar:
				t.Errorf("%s's cell %d %q drawn as the cursor: %v", session, x, c.R, c.Style)
			}
		}
	}
}

// plainScreen is the fixture's spaces list outside the select mode.
func plainScreen() []string {
	return []string{
		spacesTitle, "",
		"  " + termIcon + "default", "",
		"\u25bc " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix", "",
		"  " + repoIcon + "kitty master", "",
		"\u25b6 \uf110  docs main", "",
		"  \uf00d  broken",
	}
}

// clickRow clicks row i past its caret and box, on its name.
func (e *sidebarEnv) clickRow(i int) tea.Cmd {
	e.t.Helper()
	_, y := e.zone(fmt.Sprintf(entryZone, i))
	return e.clickAt(8, y, tea.MouseLeft)
}

// A space's row stands for its worktrees: ticking it ticks them all, shown or folded, and unticking it
// unticks them; with only some ticked it is partly ticked.
func TestSelectSpaceRow(t *testing.T) {
	e := newSidebarEnv(t)
	e.press("v")
	e.clickRow(e.row("docs"))
	if got, want := e.tickedSessions(), []string{"docs", "docs@notes"}; !reflect.DeepEqual(got, want) {
		t.Errorf("folded docs ticked %q, want %q", got, want)
	}
	e.click(fmt.Sprintf(caretZone, e.row("docs")), tea.MouseLeft)
	e.clickRow(e.row("docs@notes"))
	want := selectScreen(unticked, unticked, unticked, unticked, unticked, "")
	want = append(want[:10], "\u25bc "+partly+repoIcon+"docs main", "  "+unticked+"\uf110  notes notes", "", "    \uf00d  broken")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("docs unfolded, notes unticked:\n%q\nwant\n%q", got, want)
	}
	e.clickRow(e.row("gr"))
	if got, want := e.tickedSessions(), []string{"docs", "gr", "gr@dev", "gr@fix"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gr ticked %q, want %q", got, want)
	}
	e.clickRow(e.row("gr"))
	if got, want := e.tickedSessions(), []string{"docs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gr unticked %q, want %q", got, want)
	}
}

// A group's header stands for its spaces' worktrees, partly ticked while only some are. Hiding the group
// unticks them, so nothing acts on rows out of sight.
func TestSelectGroupHeader(t *testing.T) {
	e := newGroupsEnv(t)
	e.press("v")
	e.clickRow(e.headerRow("work"))
	e.clickRow(e.row("gr@dev"))
	e.clickRow(e.row("kitty"))
	want := []string{
		spacesTitle, "",
		"  " + unticked + termIcon + "default", "",
		"    \uf00d  broken", "",
		partly + headerLine(false, "WORK", 28),
		"\u25bc " + partly + repoIcon + "gr main",
		here + unticked + blocked + "dev pr/me/dev",
		"  " + ticked + branchIcon + "fix fix", "",
		"\u25b6 " + ticked + "\uf110  docs main", "",
		ticked + headerLine(false, "PERSONAL", 28),
		"  " + ticked + repoIcon + "kitty master",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("work ticked, dev unticked, kitty ticked:\n%q\nwant\n%q", got, want)
	}
	e.m.toggleHidden("work")
	if got, want := e.tickedSessions(), []string{"kitty"}; !reflect.DeepEqual(got, want) {
		t.Errorf("work hidden, ticked %q, want %q", got, want)
	}
	// Shown again, its rows come back unticked.
	e.m.toggleHidden("work")
	if got := e.tickedSessions(); !reflect.DeepEqual(got, []string{"kitty"}) {
		t.Errorf("work shown, ticked %q", got)
	}
}
