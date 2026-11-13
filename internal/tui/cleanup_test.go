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

	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/spaces"
)

// cleanupFixture is gr with three linked worktrees that look done with: dev's PR merged, fix's upstream
// gone with two files not committed, and old merged but locked.
func cleanupFixture() (*spaces.Space, []spaces.Cleanup) {
	space := &spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{
		{Path: "/src/gr", Branch: "main", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/me/dev", Session: "gr@dev"},
		{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"},
		{Path: "/src/gr/.worktrees/old", Branch: "old", Session: "gr@old", Locked: true},
	}}
	return space, []spaces.Cleanup{
		{Space: "gr", Worktree: space.Worktrees[1], Reasons: []string{"PR #12 merged"}},
		{Space: "gr", Worktree: space.Worktrees[2], Reasons: []string{"upstream gone"}, Changes: 2},
		{Space: "gr", Worktree: space.Worktrees[3], Reasons: []string{"merged into main"}, Refused: errors.New("locked")},
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

// Also delete their branches, off at first, puts each worktree's branch on a line of its own: merged, its PR
// merged, kept, or its commits not merged, said lost once the worktree is ticked. y deletes those that can
// go, forcing git for the commits not merged, which the form said are lost or merged by the PR.
func TestCleanupFormBranches(t *testing.T) {
	launched := fakeKitten(t)
	space, items := cleanupFixture()
	items[0].Branch = spaces.Branch{Name: "pr/me/dev", Merge: git.Merge{Unmerged: 3}, PR: 12}
	items[1].Branch = spaces.Branch{Name: "fix", Merge: git.Merge{Unmerged: 2}}
	done := spaces.Worktree{Path: "/src/gr/.worktrees/done", Branch: "done", Session: "gr@done"}
	trunk := spaces.Worktree{Path: "/src/gr/.worktrees/trunk", Branch: "trunk", Session: "gr@trunk"}
	items = append(items,
		spaces.Cleanup{Space: "gr", Worktree: done, Reasons: []string{"merged into main"}, Branch: spaces.Branch{Name: "done", Merge: git.Merge{Into: "main"}}},
		spaces.Cleanup{Space: "gr", Worktree: trunk, Reasons: []string{"upstream gone"}, Branch: spaces.Branch{Name: "trunk", Kept: "it is the default branch"}})
	f := newCleanupForm(space, items)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	want := []string{
		"",
		"  Clean up worktrees of gr",
		"",
		"  [x] dev    PR #12 merged",
		"  [ ] fix    upstream gone; 2 files not committed",
		"  [-] old    merged into main; locked",
		"  [x] done   merged into main",
		"  [x] trunk  upstream gone",
		"",
		"  [ ] Also delete their branches d",
		"  Each closes its session and is removed; its branch is kept.",
		"",
		"   Remove 3 worktrees y    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("off:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("d"))
	want = []string{
		"",
		"  Clean up worktrees of gr",
		"",
		"  [x] dev    PR #12 merged",
		"      branch: merged by PR #12, nothing lost",
		"  [ ] fix    upstream gone; 2 files not committed",
		"      branch: 2 commits not merged",
		"  [-] old    merged into main; locked",
		"  [x] done   merged into main",
		"      branch: merged into main, nothing lost",
		"  [x] trunk  upstream gone",
		"      branch: kept, as it is the default branch",
		"",
		"  [x] Also delete their branches d",
		"  Each closes its session and is removed, and its branch deleted.",
		"",
		"   Remove 3 worktrees y    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("on:\n%q\nwant\n%q", got, want)
	}
	// A click on fix's branch line ticks fix, as one on its row does.
	f.View()
	clickOn(t, f, "2 commits not merged")
	want[5] = "  [x] fix    upstream gone; 2 files not committed, lost"
	want[6] = "      branch: 2 commits not merged, lost"
	want[16] = "   Remove 4 worktrees y    Cancel n"
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("fix ticked:\n%q\nwant\n%q", got, want)
	}

	_, cmd := f.Update(keyMsg("y"))
	if msg := cmd(); msg != (cleanedMsg{}) {
		t.Errorf("removing said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes",
		"--delete-branch-path", "/src/gr/.worktrees/dev", "--force-branch-path", "/src/gr/.worktrees/dev",
		"--force-path", "/src/gr/.worktrees/fix", "--delete-branch-path", "/src/gr/.worktrees/fix", "--force-branch-path", "/src/gr/.worktrees/fix",
		"--delete-branch-path", "/src/gr/.worktrees/done",
		"--", "/src/gr/.worktrees/dev", "/src/gr/.worktrees/fix", "/src/gr/.worktrees/done", "/src/gr/.worktrees/trunk"}
	if got := launched(); !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("launched\n%q\nwant\n%q", got, wantArgs)
	}

	// Off again, nothing but the worktrees goes.
	f = newCleanupForm(space, items)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	f.Update(keyMsg("d"))
	f.View()
	clickOn(t, f, "Also delete their branches")
	if _, cmd := f.Update(keyMsg("y")); cmd() != (cleanedMsg{}) {
		t.Error("removing failed")
	}
	wantArgs = []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes",
		"--", "/src/gr/.worktrees/dev", "/src/gr/.worktrees/done", "/src/gr/.worktrees/trunk"}
	if got := launched(); !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("launched with the box clicked off\n%q\nwant\n%q", got, wantArgs)
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
		menuRule("Session"), " Open", " Save as layout", " Close session",
		menuRule("Space"), " New worktree", " Fold worktrees", " Clean up worktrees", " Select", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+12]; !reflect.DeepEqual(got, want) {
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

// Missing worktrees follow the linked ones: one with only a saved file ticked, one with windows open not,
// ticking it saying they are closed. y ends their sessions in the same background process, after the
// worktrees.
func TestCleanupFormMissing(t *testing.T) {
	launched := fakeKitten(t)
	space, items := cleanupFixture()
	items = append(items[:1],
		spaces.Cleanup{Space: "gr", Worktree: spaces.Worktree{Session: "gr@gone"}, Reasons: []string{"directory gone"}, Missing: true},
		spaces.Cleanup{Space: "gr", Worktree: spaces.Worktree{Session: "gr@open"}, Reasons: []string{"directory gone"}, Missing: true, Windows: 2})
	f := newCleanupForm(space, items)
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	want := []string{
		"",
		"  Clean up worktrees of gr",
		"",
		"  [x] dev   PR #12 merged",
		"  [x] gone  directory gone",
		"  [ ] open  directory gone; 2 windows open",
		"",
		"  Each closes its session and is removed; its branch is kept.",
		"  A missing one has its windows closed and its saved file deleted.",
		"",
		"   Remove 2 worktrees y    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("j"))
	f.Update(keyMsg("j"))
	f.Update(keyMsg("space"))
	want[5] = "  [x] open  directory gone; 2 windows closed"
	want[10] = "   Remove 3 worktrees y    Cancel n"
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("open ticked:\n%q\nwant\n%q", got, want)
	}
	_, cmd := f.Update(keyMsg("y"))
	if msg := cmd(); msg != (cleanedMsg{}) {
		t.Errorf("removing said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes",
		"--forget-session", "gr@gone", "--forget-session", "gr@open", "--", "/src/gr/.worktrees/dev"}
	if got := launched(); !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("launched\n%q\nwant\n%q", got, wantArgs)
	}

	// With only the missing one with no windows ticked, the note says what goes of it.
	f = newCleanupForm(space, items[1:2])
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if got, want := cleanupScreen(f)[5:7], []string{"  Each closes its session and is removed; its branch is kept.", "  A missing one has its windows closed and its saved file deleted."}; !reflect.DeepEqual(got, want) {
		t.Errorf("notes %q, want %q", got, want)
	}
}

// Sessions given are listed by their sessions when they span spaces: a linked worktree that does not look
// done with has no reason, and a main worktree and a session in no space are refused. Remove worktrees
// offers deleting the branches as Clean up does.
func TestCleanupFormSessions(t *testing.T) {
	launched := fakeKitten(t)
	items := []spaces.Cleanup{
		{Space: "gr", Worktree: spaces.Worktree{Path: "/src/gr/.worktrees/dev", Branch: "dev", Session: "gr@dev"},
			Branch: spaces.Branch{Name: "dev", Merge: git.Merge{Unmerged: 1}}},
		{Space: "kspaces", Worktree: spaces.Worktree{Path: "/src/kspaces/.worktrees/fix", Branch: "fix", Session: "kspaces@fix"},
			Reasons: []string{"upstream gone"}, Changes: 1, Branch: spaces.Branch{Name: "fix", Merge: git.Merge{Into: "main"}}},
		{Space: "kspaces", Worktree: spaces.Worktree{Session: "kspaces@gone"}, Reasons: []string{"directory gone"}, Missing: true},
		{Space: "gr", Worktree: spaces.Worktree{Path: "/src/gr", Branch: "main", Session: "gr"},
			Refused: errors.New("/src/gr is the main worktree of gr, which cannot be removed")},
		{Worktree: spaces.Worktree{Session: "kitty"}, Refused: errors.New("in no space, not a worktree")},
	}
	f := newCleanupForm(nil, items)
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	f.Update(keyMsg("d"))
	want := []string{
		"",
		"  Remove worktrees",
		"",
		"  [x] gr@dev",
		"      branch: 1 commit not merged, lost",
		"  [ ] kspaces@fix   upstream gone; 1 file not committed",
		"      branch: merged into main, nothing lost",
		"  [x] kspaces@gone  directory gone",
		"  [-] gr            /src/gr is the main worktree of gr, which cannot be removed",
		"  [-] kitty         in no space, not a worktree",
		"",
		"  [x] Also delete their branches d",
		"  Each closes its session and is removed, and its branch deleted.",
		"  A missing one has its windows closed and its saved file deleted.",
		"",
		"   Remove 2 worktrees y    Cancel n",
		"",
	}
	if got := cleanupScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	_, cmd := f.Update(keyMsg("y"))
	if msg := cmd(); msg != (cleanedMsg{}) {
		t.Errorf("removing said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes",
		"--forget-session", "kspaces@gone", "--delete-branch-path", "/src/gr/.worktrees/dev", "--force-branch-path", "/src/gr/.worktrees/dev",
		"--", "/src/gr/.worktrees/dev"}
	if got := launched(); !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("launched\n%q\nwant\n%q", got, wantArgs)
	}
}
