package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/kitty"
)

// newPickEnv is a picker 40 cells wide and 26 high showing fixture, the preview below the list.
func newPickEnv() *picker {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 26})
	m.Update(fixture(time.Now()))
	return m
}

// pickMenuRule is a menu's rule in the 40 cell picker, as menuRule is in the sidebar.
func pickMenuRule(head string) string {
	if head == "" {
		return strings.Repeat("\u2500", 40)
	}
	return "\u2500 " + head + " " + strings.Repeat("\u2500", 40-3-len(head))
}

func pressCode(m tea.Model, code rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return cmd
}

// tab opens the menu of the entry under the cursor below its row, over the preview, with what its row in
// the sidebar offers but what only the sidebar does, folding and selecting. Typing does nothing while it
// is open, esc closes it and leaves the picker open, and a form chosen opens over the window the picker
// is for, closing the picker.
func TestPickMenu(t *testing.T) {
	m := newPickEnv()
	launched := fakeKitten(t)
	down(m, 7)
	pressCode(m, tea.KeyTab)
	want := []string{
		"   gr / fix  fix",
		pickMenuRule("Session"), " Open",
		pickMenuRule("Worktree"), " Remove worktree",
		pickMenuRule("Space"), " New worktree", " New worktree from PR", " New worktree from issue",
		pickMenuRule(""),
		"branch   fix",
	}
	if got := pickerScreen(m)[9:20]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	typeInto(m, "zz")
	if got := m.input.Value(); got != "" {
		t.Errorf("typed %q with the menu open", got)
	}
	if cmd := pressCode(m, tea.KeyEscape); cmd != nil || m.menu != nil {
		t.Errorf("esc: menu open %v, cmd %v", m.menu != nil, cmd)
	}
	if got := pickerScreen(m)[10]; got != "   kitty  master" {
		t.Errorf("after esc, row 10 %q, want the list back", got)
	}

	pressCode(m, tea.KeyTab)
	pressCode(m, tea.KeyDown)
	cmd := pressCode(m, tea.KeyEnter)
	if cmd == nil || m.menu != nil {
		t.Fatalf("enter on Remove worktree: menu open %v, cmd %v", m.menu != nil, cmd)
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Errorf("opening the form said %v, want the picker to quit", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "remove", "/src/gr/.worktrees/fix"}; !reflect.DeepEqual(got, want) {
		t.Errorf("launched %q, want %q", got, want)
	}
}

// A status entry's menu opens it or replies to it. Near the bottom the menu goes above the entry's row.
func TestPickMenuStatus(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	m.Update(fixture(time.Now()))
	pressCode(m, tea.KeyTab)
	want := []string{blocked + "gr@dev  approve", pickMenuRule(""), " Open", " Reply", pickMenuRule("")}
	if got := pickerScreen(m)[2:7]; !reflect.DeepEqual(got, want) {
		t.Errorf("status menu:\n%q\nwant\n%q", got, want)
	}
}

// A form from the picker is for the window it covers: an overlay picker is the most recently focused
// window of its tab, so Home, and the form goes over the window of its session focused before it.
func TestFormHome(t *testing.T) {
	st := kitty.State{Home: 30, Current: "gr", Screens: map[string]int{"gr": 11}}
	got := []int{formHome(st, 30), formHome(st, 0), formHome(kitty.State{Home: 30}, 30)}
	if want := []int{11, 30, 30}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
