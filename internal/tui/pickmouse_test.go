package tui

import (
	"os"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A right click on an entry moves the cursor to it and opens its menu, as tab does; the mouse lights
// the item under it, and a left click runs it. A click off the menu closes it.
func TestPickMenuMouse(t *testing.T) {
	m := newPickEnv()
	launched := fakeKitten(t)
	m.render()
	// gr / fix is the list's eighth entry, under the query and its rule.
	m.Update(tea.MouseClickMsg{X: 5, Y: 9, Button: tea.MouseRight})
	m.render()
	if m.cursor != 7 || m.menu == nil {
		t.Fatalf("right click: cursor %d, menu open %v", m.cursor, m.menu != nil)
	}
	// Remove worktree is on the fifth line under the row, item 1.
	m.Update(tea.MouseMotionMsg{X: 5, Y: 13})
	if m.menu.sel != 1 {
		t.Errorf("mouse on Remove worktree lit item %d", m.menu.sel)
	}
	m.Update(tea.MouseClickMsg{X: 39, Y: 22, Button: tea.MouseLeft})
	if m.menu != nil {
		t.Error("a click off the menu left it open")
	}

	m.Update(tea.MouseClickMsg{X: 5, Y: 9, Button: tea.MouseRight})
	m.render()
	_, cmd := m.Update(tea.MouseClickMsg{X: 5, Y: 13, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on Remove worktree ran nothing")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Errorf("opening the form said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "remove", "/src/gr/.worktrees/fix"}; !reflect.DeepEqual(got, want) {
		t.Errorf("launched %q, want %q", got, want)
	}
}

// A left click on an entry switches to it, as enter does, beside the preview too; the wheel moves the
// cursor and closes the menu.
func TestPickMouse(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	m.Update(fixture(time.Now()))
	m.render()
	_, cmd := m.Update(tea.MouseClickMsg{X: 5, Y: 10, Button: tea.MouseLeft})
	if m.cursor != 8 || cmd == nil || !m.choosing {
		t.Errorf("left click on kitty: cursor %d, choosing %v, cmd %v", m.cursor, m.choosing, cmd != nil)
	}
	// Off the list, beside it in the preview, a click does nothing.
	m.choosing = false
	if _, cmd := m.Update(tea.MouseClickMsg{X: 60, Y: 4, Button: tea.MouseLeft}); cmd != nil || m.cursor != 8 {
		t.Errorf("click on the preview: cursor %d, cmd %v", m.cursor, cmd != nil)
	}

	pressCode(m, tea.KeyTab)
	m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if m.cursor != 9 || m.menu != nil {
		t.Errorf("wheel down: cursor %d, menu open %v", m.cursor, m.menu != nil)
	}
	m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	if m.cursor != 8 {
		t.Errorf("wheel up: cursor %d", m.cursor)
	}
}

// In a panel the picker fills the frame inside its border and padding, so the mouse it hears is from
// inside them.
func TestPopupMouseInFrame(t *testing.T) {
	p := newPopup(newPicker(0), 11)
	got := []tea.Msg{
		p.inPanel(tea.MouseClickMsg{X: 5, Y: 4, Button: tea.MouseLeft}),
		p.inPanel(tea.MouseWheelMsg{X: 2, Y: 1, Button: tea.MouseWheelDown}),
		p.inPanel(tea.KeyPressMsg{Code: 'a'}),
	}
	want := []tea.Msg{
		tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft},
		tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelDown},
		tea.KeyPressMsg{Code: 'a'},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
