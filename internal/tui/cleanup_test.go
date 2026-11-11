package tui

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/spaces"
)

// cleanupFixture is gr with three linked worktrees that look done with: dev's PR merged, fix's upstream
// gone with two files not committed, and old merged but locked.
func cleanupFixture() (spaces.Space, []spaces.Cleanup) {
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{
		{Path: "/src/gr", Branch: "main", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/me/dev", Session: "gr@dev"},
		{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"},
		{Path: "/src/gr/.worktrees/old", Branch: "old", Session: "gr@old", Locked: true},
	}}
	return space, []spaces.Cleanup{
		{Worktree: space.Worktrees[1], Reasons: []string{"PR #12 merged"}},
		{Worktree: space.Worktrees[2], Reasons: []string{"upstream gone"}, Changes: 2},
		{Worktree: space.Worktrees[3], Reasons: []string{"merged into main"}, Refused: errors.New("locked")},
	}
}

func cleanupScreen(f *cleanupForm) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// The clean up form ticks the worktrees that lose nothing; space ticks the cursor's, saying what is lost,
// but not one git would refuse. y removes the ticked ones in one background process, forcing git for those
// with changes.
func TestCleanupForm(t *testing.T) {
	launched := fakeKitten(t)
	f := newCleanupForm(cleanupFixture())
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	want := []string{
		"",
		"  Clean up worktrees of gr",
		"",
		"  [x] dev  PR #12 merged",
		"  [ ] fix  upstream gone; 2 files not committed",
		"  [-] old  merged into main; locked",
		"",
		"  Each closes its session and is removed; its branch is kept.",
		"",
		"   Remove 1 worktree y    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	press := func(keys ...string) {
		for _, k := range keys {
			f.Update(keyMsg(k))
		}
	}
	press("j", "space", "j", "space")
	want[4] = "  [x] fix  upstream gone; 2 files not committed, lost"
	want[9] = "   Remove 2 worktrees y    Cancel n"
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("after ticking fix and old:\n%q\nwant\n%q", got, want)
	}
	// The cursor's bar is on old, the last.
	if f.cursor != 2 {
		t.Errorf("cursor %d", f.cursor)
	}

	_, cmd := f.Update(keyMsg("y"))
	if cmd == nil || !f.busy {
		t.Fatalf("y: cmd %v, busy %v", cmd != nil, f.busy)
	}
	if msg := cmd(); msg != (cleanedMsg{}) {
		t.Errorf("removing said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes",
		"--force-path", "/src/gr/.worktrees/fix", "--", "/src/gr/.worktrees/dev", "/src/gr/.worktrees/fix"}
	if got := launched(); !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("launched %q, want %q", got, wantArgs)
	}
}

// With nothing ticked there is nothing to remove, and with nothing that looks done with the form says so.
func TestCleanupFormEmpty(t *testing.T) {
	space, items := cleanupFixture()
	f := newCleanupForm(space, items[:1])
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("space"))
	if _, cmd := f.Update(keyMsg("y")); cmd != nil || f.busy {
		t.Errorf("y with nothing ticked: cmd %v, busy %v", cmd != nil, f.busy)
	}
	if got := cleanupScreen(f)[7]; got != "   Remove 0 worktrees    Cancel n" {
		t.Errorf("buttons %q", got)
	}

	f = newCleanupForm(space, nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	want := []string{
		"",
		"  Clean up worktrees of gr",
		"",
		"  No linked worktree has its PR merged, its upstream gone or its",
		"  branch merged into main.",
		"",
		"   Remove 0 worktrees    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("empty form:\n%q\nwant\n%q", got, want)
	}
}

// p on a space's row, and its menu, open the clean up form for the space; a row of no space or a space
// without linked worktrees has neither.
func TestCleanupKey(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.row("gr")
	cmd := e.press("p")
	if cmd == nil {
		t.Fatal("p on gr ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("p said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "cleanup", "/src/gr"}; !reflect.DeepEqual(got, want) {
		t.Errorf("p launched %q, want %q", got, want)
	}
	e.press("m")
	_, y := e.zone(fmt.Sprintf(entryZone, e.row("gr")))
	want := []string{
		menuRule("Session"), " Open", " Close session",
		menuRule("Space"), " New worktree", " Fold worktrees", " Clean up worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+10]; !reflect.DeepEqual(got, want) {
		t.Errorf("gr's menu:\n%q\nwant\n%q", got, want)
	}
	e.press("esc")
	for _, session := range []string{"gr@dev", "kitty", "default"} {
		e.m.cursor = e.row(session)
		if cmd := e.press("p"); cmd != nil {
			t.Errorf("p on %s ran something", session)
		}
	}
}
