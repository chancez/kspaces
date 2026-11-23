package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The remove form offers deleting the branch, off at first, but not a detached HEAD, the default branch or
// a branch another worktree has checked out. It says whether deleting loses commits; d or a click ticks
// it, and y deletes the branch after the worktree, forcing git only for the commits it said are lost.
// worktrees.delete_branch, the global one or the space file's, ticks it at first for a branch that loses
// nothing, merged or by its PR, and leaves one that would lose commits off.
func TestRemoveFormDeleteBranch(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	// The main worktree is on dev, so main, the default branch, can be a linked worktree's.
	r.Git("checkout", "-q", "-b", "dev")
	wt := func(name string) string { return filepath.Join(r.Dir, ".worktrees", name) }
	trunk := wt("main")
	r.Git("worktree", "add", "-q", trunk, "main")
	r.Worktree("merged", wt("merged")).Commit("merged")
	unmerged := r.Worktree("unmerged", wt("unmerged"))
	unmerged.Commit("one")
	unmerged.Commit("two")
	r.Worktree("squashed", wt("squashed")).Commit("squashed")
	r.Git("-C", trunk, "merge", "-q", "--no-edit", "merged")
	r.Git("worktree", "add", "-q", "--detach", wt("detached"))
	r.Worktree("twice", wt("twice"))
	r.Git("worktree", "add", "-q", "-f", wt("twice2"), "twice")
	space := spaces.LoadSpace(r.Dir)
	formWith := func(cfg config.Config, space spaces.Space, name string, pr *gh.PR) *removeWorktreeForm {
		for _, w := range space.Worktrees {
			if w.Path == wt(name) {
				f, err := newRemoveForm(cfg, space, w, kitty.State{}, pr, false)
				if err != nil {
					t.Fatal(err)
				}
				f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
				return f
			}
		}
		t.Fatalf("no worktree %s", name)
		return nil
	}
	form := func(name string, pr *gh.PR) *removeWorktreeForm { return formWith(config.Config{}, space, name, pr) }
	screen := func(f *removeWorktreeForm) []string {
		var out []string
		for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
		return out
	}
	without := func(name, branch string) []string {
		return []string{
			"",
			"  Remove worktree " + name + " from " + space.Name,
			"",
			"  directory  .worktrees/" + name,
			"  branch     " + branch,
			"  session    no windows open",
			"  changes    none",
			"",
			"   Remove y    Cancel n",
			"",
		}
	}
	with := func(name, branch, box string) []string {
		lines := without(name, branch)
		return append(lines[:8], append([]string{box, ""}, lines[8:]...)...)
	}
	type check struct {
		Name   string
		Screen []string
	}
	var got, want []check
	for _, tc := range []struct{ name, branch string }{
		{"detached", "none, HEAD is detached kept"},
		{"main", "main kept"},
		{"twice", "twice kept"},
	} {
		f := form(tc.name, nil)
		f.Update(keyMsg("d"))
		got = append(got, check{tc.name, screen(f)})
		want = append(want, check{tc.name, without(tc.name, tc.branch)})
	}

	merged := form("merged", nil)
	got = append(got, check{"merged, off", screen(merged)})
	want = append(want, check{"merged, off", with("merged", "merged kept", "  \u2610 Delete branch merged d  merged into main, nothing lost")})
	merged.Update(keyMsg("d"))
	got = append(got, check{"merged, on", screen(merged)})
	want = append(want, check{"merged, on", with("merged", "merged deleted", "  \u2611 Delete branch merged d  merged into main, nothing lost")})

	lost := form("unmerged", nil)
	got = append(got, check{"unmerged, off", screen(lost)})
	want = append(want, check{"unmerged, off", with("unmerged", "unmerged kept", "  \u2610 Delete branch unmerged d  2 commits not merged, lost")})
	clickOn(t, lost, "Delete branch")
	got = append(got, check{"unmerged, clicked", screen(lost)})
	want = append(want, check{"unmerged, clicked", with("unmerged", "unmerged deleted", "  \u2611 Delete branch unmerged d  2 commits not merged, lost")})

	squashed := form("squashed", &gh.PR{Number: 12, State: "merged"})
	squashed.Update(keyMsg("d"))
	got = append(got, check{"squashed, on", screen(squashed)})
	want = append(want, check{"squashed, on", with("squashed", "squashed deleted", "  \u2611 Delete branch squashed d  merged by PR #12, nothing lost")})

	on := config.Config{Worktrees: config.Worktrees{DeleteBranch: true}}
	for _, tc := range []struct{ name, branch string }{
		{"detached", "none, HEAD is detached kept"},
		{"main", "main kept"},
		{"twice", "twice kept"},
	} {
		got = append(got, check{tc.name + ", option on", screen(formWith(on, space, tc.name, nil))})
		want = append(want, check{tc.name + ", option on", without(tc.name, tc.branch)})
	}
	byOption := formWith(on, space, "merged", nil)
	got = append(got, check{"merged, option on", screen(byOption)})
	want = append(want, check{"merged, option on", with("merged", "merged deleted", "  \u2611 Delete branch merged d  merged into main, nothing lost")})
	byOption.Update(keyMsg("d"))
	got = append(got, check{"merged, option on, d", screen(byOption)})
	want = append(want, check{"merged, option on, d", with("merged", "merged kept", "  \u2610 Delete branch merged d  merged into main, nothing lost")})
	got = append(got, check{"squashed, option on", screen(formWith(on, space, "squashed", &gh.PR{Number: 12, State: "merged"}))})
	want = append(want, check{"squashed, option on", with("squashed", "squashed deleted", "  \u2611 Delete branch squashed d  merged by PR #12, nothing lost")})
	got = append(got, check{"unmerged, option on", screen(formWith(on, space, "unmerged", nil))})
	want = append(want, check{"unmerged, option on", with("unmerged", "unmerged kept", "  \u2610 Delete branch unmerged d  2 commits not merged, lost")})
	// The space's file says so in place of the global config, either way.
	ownOn, ownOff := space, space
	ownOn.Config, ownOff.Config = &on, &config.Config{}
	got = append(got, check{"merged, space file on", screen(formWith(config.Config{}, ownOn, "merged", nil))})
	want = append(want, check{"merged, space file on", with("merged", "merged deleted", "  \u2611 Delete branch merged d  merged into main, nothing lost")})
	got = append(got, check{"merged, space file off", screen(formWith(on, ownOff, "merged", nil))})
	want = append(want, check{"merged, space file off", with("merged", "merged kept", "  \u2610 Delete branch merged d  merged into main, nothing lost")})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	onMain := form("main", nil)
	launched := fakeKitten(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var calls, wantCalls [][]string
	// enter removes as y does.
	for i, f := range []*removeWorktreeForm{onMain, merged, lost, squashed} {
		_, cmd := f.Update(keyMsg([]string{"y", "enter"}[i%2]))
		if msg := cmd(); msg != (removedMsg{}) {
			t.Errorf("removing %s said %v", f.wt.Path, msg)
		}
		calls = append(calls, launched())
	}
	launch := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "remove", "--yes"}
	for _, tail := range [][]string{
		{trunk},
		{"--delete-branch", wt("merged")},
		{"--delete-branch", "--force-branch", wt("unmerged")},
		{"--delete-branch", "--force-branch", wt("squashed")},
	} {
		wantCalls = append(wantCalls, append(append([]string{}, launch...), tail...))
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Errorf("launched\n%q\nwant\n%q", calls, wantCalls)
	}
}

// With a provider that can end them, the remove form says how many of its sessions removing ends, and what
// a busy one runs; left and right choose to shelve them or leave them running instead, which the removal
// in the background is told.
func TestRemoveFormSessions(t *testing.T) {
	calls := fakeKittenCalls(t)
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr"}, {Path: "/src/gr/.worktrees/fix", Session: "gr@fix"}}}
	f := &removeWorktreeForm{th: look.New(true), space: space, wt: space.Worktrees[1], windows: 2,
		sessions: []spaces.ProviderSession{
			{ID: "a", Name: "build", State: "running", Busy: true, Command: "make watch"},
			{ID: "b", Name: "old", State: "exited"},
		},
		fate: fateChoice{fates: []spaces.Fate{spaces.FateEnd, spaces.FateShelve, spaces.FateLeave}}}
	f.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	screen := func(choice, says string) []string {
		return []string{"", "  Remove worktree fix from gr", "",
			"  directory  .worktrees/fix", "  branch     none, HEAD is detached kept", "  session    2 windows closed", "  changes    none", "",
			"  sessions   < " + choice + " >  left/right change", "             " + says, "",
			"   Remove y    Cancel n", ""}
	}
	got := [][]string{formScreen(f)}
	f.Update(keyMsg("right"))
	got = append(got, formScreen(f))
	f.Update(keyMsg("l"))
	got = append(got, formScreen(f))
	want := [][]string{
		screen("end", "Ends 2 sessions: build running make watch"),
		screen("shelve", "Shelves 2 sessions, to bring back later: build running make watch"),
		screen("leave running", "Leaves 2 sessions running"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("h"))
	_, remove := f.Update(keyMsg("y"))
	remove()
	var args []string
	for _, c := range calls() {
		args = c[len(c)-6:]
	}
	if want := []string{"remove", "--yes", "--sessions", "shelve", "/src/gr/.worktrees/fix"}; !reflect.DeepEqual(args[1:], want) {
		t.Errorf("background ran %q, want %q", args, want)
	}
}
