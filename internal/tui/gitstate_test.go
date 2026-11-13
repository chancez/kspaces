package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/spaces"
)

// gitFixture is fixture with git states: gr with a change; gr@dev with changes, ahead and behind its
// upstream, and an open PR; gr@fix with its upstream gone and its PR merged; kitty clean; docs@notes,
// folded into docs, with changes the folded row does not show.
func gitFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Git = map[string]gitstate.State{
		"/src/gr":                    {Status: &gitstate.Status{Changes: 1, Upstream: "origin/main"}},
		"/src/gr/.worktrees/dev":     {Status: &gitstate.Status{Changes: 3, Upstream: "origin/pr/me/dev", Ahead: 2, Behind: 1}, PR: &gh.PR{Number: 123, State: "open"}},
		"/src/gr/.worktrees/fix":     {Status: &gitstate.Status{Upstream: "origin/fix", Gone: true}, PR: &gh.PR{Number: 98, State: "merged"}},
		"/src/kitty":                 {Status: &gitstate.Status{Upstream: "origin/master"}},
		"/src/docs/.worktrees/notes": {Status: &gitstate.Status{Changes: 5}},
	}
	return snap
}

// A worktree's git state is at the right end of its row, and the branch gives way to it. In a narrow
// sidebar it gives way in turn: ahead and behind first, then the PR, and the changes last.
func TestGitStateOnRows(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(gitFixture())
	rows := func() []string { return e.screen()[2:13] }
	pad := func(n int) string { return strings.Repeat(" ", n) }
	want := []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main" + pad(15) + "\u25cf1",
		"  " + blocked + "dev pr/m... \u25cf3 \u21912\u21931 #123",
		"  " + branchIcon + "fix fix" + pad(14) + "#98",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows at 30:\n%q\nwant\n%q", got, want)
	}

	e.m.Update(tea.WindowSizeMsg{Width: 14, Height: 40})
	want = []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr ma \u25cf1",
		"  " + blocked + "dev   \u25cf3",
		"  " + branchIcon + "fix  #98",
		"",
		"  " + repoIcon + "kitty ma",
		"",
		"\u25b6 \uf110  docs mai",
		"",
		"  \uf00d  broken",
	}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows at 14:\n%q\nwant\n%q", got, want)
	}
}

// A worktree's name gives way to its changes and its PR, not to ahead and behind, down to a few cells.
func TestLongNameGivesWayToGitState(t *testing.T) {
	e := newSidebarEnv(t)
	snap := gitFixture()
	long := "/src/gr/.worktrees/intent_sync_api_updates"
	snap.Spaces[0].Worktrees = append(snap.Spaces[0].Worktrees, spaces.Worktree{Path: long, Branch: "intent_sync_api_updates", Session: "gr@intent_sync_api_updates"})
	snap.Git[long] = gitstate.State{Status: &gitstate.Status{Changes: 2, Ahead: 4}, PR: &gh.PR{Number: 11243, State: "open"}}
	e.m.Update(snap)
	row := func() string { return e.screen()[7] }
	for _, tc := range []struct {
		width int
		want  string
	}{
		{44, "  " + branchIcon + "intent_sync_api_updates   \u25cf2 \u21914 #11243"},
		{30, "  " + branchIcon + "intent_sync... \u25cf2 #11243"},
		{16, "  " + branchIcon + "inte... \u25cf2"},
	} {
		e.m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 40})
		if got := row(); got != tc.want {
			t.Errorf("row at %d is %q, want %q", tc.width, got, tc.want)
		}
	}
}

// The picker's preview of a worktree says what git and gh say of it, once they have.
func TestPickerPreviewGitState(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 26})
	snap := gitFixture()
	snap.Git["/src/kitty"] = gitstate.State{Status: &gitstate.Status{Upstream: "origin/master"}, PRAt: time.Now()}
	m.Update(snap)
	text := func() []string {
		var out []string
		for _, l := range m.preview(40, 20) {
			out = append(out, m.th.TextOf(l...))
		}
		return out
	}
	var got [][]string
	for _, n := range []int{6, 1, 1} {
		down(m, n)
		got = append(got, text())
	}
	want := [][]string{
		{
			"gr / dev", "",
			"space    gr",
			"path     .worktrees/dev",
			"branch   pr/me/dev",
			"changes  \u25cf3 not committed",
			"upstream origin/pr/me/dev \u21912\u21931",
			"pr       #123 open",
			"session  gr@dev",
			"windows  2 open",
			"saved    no sessions directory",
			"",
			blocked + "claude \u00b7 approve                +1 5m",
		},
		{
			"gr / fix", "",
			"space    gr",
			"path     .worktrees/fix",
			"branch   fix",
			"changes  none",
			"upstream origin/fix, gone",
			"pr       #98 merged",
			"session  gr@fix",
			"windows  none open",
			"saved    no sessions directory",
		},
		{
			"kitty", "",
			"space    kitty",
			"path     /src/kitty",
			"branch   master",
			"changes  none",
			"upstream origin/master, up to date",
			"pr       none",
			"session  kitty",
			"windows  none open",
			"saved    no sessions directory",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("previews:\n%q\nwant\n%q", got, want)
	}
}

// fakeGh puts a gh on PATH that records the directory it ran in and its arguments, a line each, and
// fails saying so when fail is set. It returns them as the last call had them.
func fakeGh(t *testing.T, fail bool) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n{ pwd -P; for a; do printf '%s\\n' \"$a\"; done; } > " + log + "\n"
	if fail {
		script += "echo 'no pull request found' >&2\nexit 1\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// A worktree whose branch has a PR offers to open it, in its menu and on b, with gh browse run in the
// worktree; one without a PR offers neither. gh failing shows on the last line.
func TestOpenPR(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(tea.FocusMsg{})
	// gh runs in the worktree, so dev's is a real directory.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snap := gitFixture()
	snap.Spaces[0].Worktrees[1].Path = dir
	snap.Git[dir] = snap.Git["/src/gr/.worktrees/dev"]
	e.m.Update(snap)
	e.m.render()

	dev := e.row("gr@dev")
	e.click(fmt.Sprintf(entryZone, dev), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, dev))
	want := []string{
		menuRule("Session"), " Open", " Save as layout", " Close session",
		menuRule("Worktree"), " Open PR #123", " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+13]; !reflect.DeepEqual(got, want) {
		t.Errorf("dev's menu:\n%q\nwant\n%q", got, want)
	}
	e.press("esc")
	kitty := e.row("kitty")
	e.click(fmt.Sprintf(entryZone, kitty), tea.MouseRight)
	_, y = e.zone(fmt.Sprintf(entryZone, kitty))
	want = []string{menuRule("Session"), " Open", menuRule("Space"), " New worktree", " Add space", menuRule("")}
	if got := e.screen()[y+1 : y+7]; !reflect.DeepEqual(got, want) {
		t.Errorf("kitty's menu:\n%q\nwant\n%q", got, want)
	}
	e.press("esc")
	if cmd := e.press("b"); cmd != nil {
		t.Error("b on kitty, with no PR, ran something")
	}

	calls := fakeGh(t, false)
	e.m.cursor = dev
	cmd := e.press("b")
	if cmd == nil {
		t.Fatal("b on dev ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("opening the PR said %v", msg)
	}
	if got, want := calls(), []string{dir, "browse", "123"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q, want %q", got, want)
	}

	fakeGh(t, true)
	e.m.Update(e.press("b")())
	lines := e.screen()
	if got := lines[len(lines)-1]; got != "no pull request found" {
		t.Errorf("last line %q, want gh's error", got)
	}
}
