package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// An error of several lines, as gh's often are, is one line on the last line: its line breaks would
// push the frame past the sidebar's height.
func TestErrorOnOneLine(t *testing.T) {
	e := newSidebarEnv(t)
	before := e.screen()
	e.m.Update(spaces.Snapshot{Err: errors.New("no git remotes\n  add one\tfirst")})
	want := append(slices.Clone(before[:len(before)-1]), "no git remotes add one first")
	if got := e.screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("screen:\n%q\nwant\n%q", got, want)
	}
}

// The mouse on the last line shows the error in full above it, wrapped, when the line cuts it: an
// action's as a load's, over what is there, kept while the mouse moves onto it, gone when it moves off
// or leaves the sidebar. An error that fits shows nothing more. One too long for the rows above keeps
// to the rows below the title.
func TestErrorPopup(t *testing.T) {
	e := newSidebarEnv(t)
	before := e.screen()
	above := func(rows ...string) []string {
		out := slices.Clone(before)
		copy(out[len(out)-len(rows):], rows)
		return out
	}
	rule := strings.Repeat("\u2500", 29)
	long := "To get started with GitHub CLI, please run:  gh auth login\nAlternatively, populate the GH_TOKEN environment variable."
	cut := "To get started with GitHub..."

	show := func(msgs ...tea.Msg) {
		for _, msg := range msgs {
			e.m.Update(msg)
		}
		e.m.render()
	}
	var got [][]string
	show(failedMsg{errors.New("no pull request found")})
	e.hover(errorZone)
	got = append(got, e.screen())
	show(spaces.Snapshot{Err: errors.New(long)}, failedMsg{nil})
	e.hover(errorZone)
	got = append(got, e.screen())
	// Onto the popup, and off it onto a row.
	_, y := e.zone(errorZone)
	show(tea.MouseMotionMsg{X: 3, Y: y - 2})
	got = append(got, e.screen())
	e.hover(fmt.Sprintf(entryZone, e.row("gr@dev")))
	got = append(got, e.screen())
	e.hover(errorZone)
	show(mouseLeftMsg{})
	got = append(got, e.screen())
	want := [][]string{
		append(slices.Clone(before[:len(before)-1]), "no pull request found"),
		above(rule, "To get started with GitHub", "CLI, please run: gh auth", "login", "Alternatively, populate the", "GH_TOKEN environment", "variable.", cut),
		above(rule, "To get started with GitHub", "CLI, please run: gh auth", "login", "Alternatively, populate the", "GH_TOKEN environment", "variable.", cut),
		append(slices.Clone(before[:len(before)-1]), cut),
		append(slices.Clone(before[:len(before)-1]), cut),
	}
	for i := range max(len(got), len(want)) {
		if i >= len(got) || i >= len(want) || !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("screen %d:\n%s\nwant\n%s", i, strings.Join(got[min(i, len(got)-1)], "\n"), strings.Join(want[min(i, len(want)-1)], "\n"))
		}
	}

	show(tea.WindowSizeMsg{Width: 30, Height: 7})
	e.hover(errorZone)
	if got, want := e.screen(), []string{
		spacesTitle, "",
		rule, "To get started with GitHub", "CLI, please run: gh auth", "login", cut,
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("in a short sidebar:\n%q\nwant\n%q", got, want)
	}
}

// A click on the last line's error, or on its popup, or Show error in the menu a right click on it opens,
// shows the error in full in a form, through a file the form reads, and the error stays on the line.
func TestShowErrorLaunch(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	text := "To get started with GitHub CLI, please run:  gh auth login\nAlternatively, populate the GH_TOKEN environment variable."
	e.m.Update(failedMsg{errors.New(text)})
	e.m.render()
	before := e.screen()
	// opened runs cmd, and is how kitty was asked to launch the form, with the directory of the form's file
	// and what it says in place of its name. It deletes the file, as the form does.
	opened := func(cmd tea.Cmd) []string {
		t.Helper()
		if cmd == nil {
			t.Fatal("ran nothing")
		}
		if msg := cmd(); msg != nil {
			t.Fatalf("said %v", msg)
		}
		args := launched()
		path := args[len(args)-1]
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return append(args[:len(args)-1], filepath.Dir(path), string(data))
	}

	var got [][]string
	got = append(got, opened(e.click(errorZone, tea.MouseLeft)))
	e.hover(errorZone)
	_, y := e.zone(errorZone)
	got = append(got, opened(e.clickAt(3, y-2, tea.MouseLeft)))
	e.click(errorZone, tea.MouseRight)
	menu := e.screen()
	got = append(got, opened(e.click(fmt.Sprintf(menuZone, 0), tea.MouseLeft)))
	launch := []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "error", tmp, text}
	if want := [][]string{launch, launch, launch}; !reflect.DeepEqual(got, want) {
		t.Errorf("launched:\n%q\nwant\n%q", got, want)
	}
	wantMenu := slices.Clone(before)
	copy(wantMenu[len(wantMenu)-4:], []string{menuRule(""), " Show error", menuRule("")})
	if !reflect.DeepEqual(menu, wantMenu) {
		t.Errorf("menu:\n%s\nwant\n%s", strings.Join(menu, "\n"), strings.Join(wantMenu, "\n"))
	}
	if got := e.screen(); !reflect.DeepEqual(got, before) {
		t.Errorf("after showing the error:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(before, "\n"))
	}
}

// The form shows the error wrapped, scrolls it when it is longer than the form fits, copies it on y, and
// closes on esc, q or enter.
func TestErrorForm(t *testing.T) {
	text := "gh: could not resolve to a Repository with the name o/r, as no remote points to it\n" +
		"line 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8"
	f := newErrorForm(text)
	f.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	rows := []string{"gh: could not resolve to a", "Repository with the name o/r, as no", "remote points to it",
		"line 2", "line 3", "line 4", "line 5", "line 6", "line 7", "line 8"}
	// at is the form showing the 4 rows from the n-th, with what the footer says after its buttons.
	at := func(n int, after string) []string {
		out := []string{"", "  Error", ""}
		for _, r := range rows[n-1 : n+3] {
			out = append(out, "  "+r)
		}
		return append(out, "", fmt.Sprintf("   Copy y    Close esc   rows %d-%d of 10", n, n+3)+after, "")
	}
	keys := []tea.KeyPressMsg{
		{Code: tea.KeyDown}, {Code: tea.KeyPgDown}, keyMsg("j"), keyMsg("space"),
		keyMsg("k"), {Code: tea.KeyPgUp}, {Code: tea.KeyUp},
	}
	var got [][]string
	got = append(got, formScreen(f))
	for _, k := range keys {
		f.Update(k)
		got = append(got, formScreen(f))
	}
	_, cmd := f.Update(keyMsg("y"))
	if cmd == nil || !reflect.DeepEqual(cmd(), tea.SetClipboard(text)()) {
		t.Error("y did not copy the error")
	}
	got = append(got, formScreen(f))
	want := [][]string{at(1, ""), at(2, ""), at(6, ""), at(7, ""), at(7, ""), at(6, ""), at(2, ""), at(1, ""), at(1, "  copied")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("screens:\n%q\nwant\n%q", got, want)
	}
	for _, k := range []string{"esc", "q", "enter"} {
		if _, cmd := f.Update(keyMsg(k)); cmd == nil || !reflect.DeepEqual(cmd(), tea.Quit()) {
			t.Errorf("%s did not close the form", k)
		}
	}
}
