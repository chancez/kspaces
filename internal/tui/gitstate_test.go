package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
		"/src/gr/.worktrees/dev":     {Status: &gitstate.Status{Changes: 3, Upstream: "origin/pr/me/dev", Ahead: 2, Behind: 1}, PR: &gitstate.PR{Number: 123, State: "open"}},
		"/src/gr/.worktrees/fix":     {Status: &gitstate.Status{Upstream: "origin/fix", Gone: true}, PR: &gitstate.PR{Number: 98, State: "merged"}},
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
