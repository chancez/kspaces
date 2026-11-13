package tui

import (
	"errors"
	"fmt"
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
