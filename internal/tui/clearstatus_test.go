package tui

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/status"
)

// x on a status entry with a done or failed record, or a click on its button, clears those records
// without focusing its window; on its hover popup the button clears the entry the popup shows. An entry
// with neither, working, blocked or idle, has no button and x does nothing.
func TestClearStatus(t *testing.T) {
	e := newSidebarEnv(t)
	e.splitStatus(15)
	calls := fakeKittenCalls(t)
	snap := fixture(time.Now())
	snap.Kitty.Statuses[0].Others = append(snap.Kitty.Statuses[0].Others, status.Record{ID: "lint", State: "done"})
	e.m.Update(snap)
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.entry(21)
	e.m.render()
	cmd := e.press("x")
	if cmd == nil {
		t.Fatal("x ran nothing")
	}
	cmd()
	e.click(fmt.Sprintf(clearZone, e.entry(21)-len(e.m.rows)), tea.MouseLeft)()
	e.hover(fmt.Sprintf(entryZone, e.entry(20)))
	e.click(fmt.Sprintf(clearZone, e.entry(20)-len(e.m.rows)), tea.MouseLeft)()
	clear := []string{"@", "action", "kspaces_clear_status", "--state", "done,error"}
	want := [][]string{append(clear, "21"), append(clear, "21"), append(clear, "20")}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("cleared\n%q\nwant\n%q", got, want)
	}

	e.m.cursor = e.entry(22)
	e.m.render()
	if cmd := e.press("x"); cmd != nil {
		t.Errorf("x on a working entry ran %v", cmd())
	}
	if _, ok := e.m.zones[fmt.Sprintf(clearZone, e.entry(22)-len(e.m.rows))]; ok {
		t.Error("the working entry has a clear button")
	}
}

// X, or the STATUS title's button, clears the done and failed records of the windows that have one,
// wherever the cursor is; with none, the button is gone and X does nothing.
func TestClearFinished(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	snap := fixture(time.Now())
	snap.Kitty.Statuses[3].Others = []status.Record{{ID: "lint", State: "done"}}
	e.m.Update(snap)
	e.m.Update(tea.FocusMsg{})
	cmd := e.press("X")
	if cmd == nil {
		t.Fatal("X ran nothing")
	}
	cmd()
	e.click(clearDoneZone, tea.MouseLeft)()
	clear := []string{"@", "action", "kspaces_clear_status", "--state", "done,error", "21", "23"}
	if got, want := calls(), [][]string{clear, clear}; !reflect.DeepEqual(got, want) {
		t.Errorf("cleared\n%q\nwant\n%q", got, want)
	}

	snap.Kitty.Statuses = []status.Status{snap.Kitty.Statuses[0]}
	e.m.Update(snap)
	if cmd := e.press("X"); cmd != nil {
		t.Errorf("X with nothing finished ran %v", cmd())
	}
	if _, ok := e.m.zones[clearDoneZone]; ok {
		t.Error("the STATUS title kept its clear button with nothing finished")
	}
}
