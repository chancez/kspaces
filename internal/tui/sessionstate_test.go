package tui

import (
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// sessionsFixture is the fixture with provider sessions no window shows: in gr's fix, which has no
// windows, one running and one shelved; in kitty, with none, one shelved; in docs, folded, one that exited.
func sessionsFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Config = &config.Config{Launch: []string{"launch"}, Provider: []string{"prov"}}
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapOpen: true}
	snap.Detached = map[string]spaces.Detached{
		"gr": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{
			{ID: "a1", Name: "build", Space: "gr", State: "running", Tags: map[string]string{"worktree": "fix"}},
			{ID: "b2", Name: "@b2", Space: "gr", State: "shelved", Tags: map[string]string{"worktree": "fix"}},
		}}}},
		"kitty": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "c3", Name: "edit", Space: "kitty", State: "shelved"}}}}},
		"docs":  {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "d4", Name: "notes", Space: "docs", State: "exited"}}}}},
	}
	return snap
}

// A worktree's provider sessions are no rows of their own: one with no windows whose sessions still run has
// the running marker and is not faint, as its work goes on; one whose sessions are all shelved has the
// suspended marker; one with only ended sessions is closed. enter opens it as any, and has the sessions no
// window shows then opened in tabs of it.
func TestSessionStates(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	e.m.Update(sessionsFixture())
	e.m.Update(tea.FocusMsg{})
	want := []string{
		spacesTitle,
		"",
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev",
		"  " + runningIcon + "fix fix",
		"",
		"  \uf04c  kitty master",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n%q\nwant\n%q", got, want)
	}
	e.m.render()
	first := func(session string, r rune) cells.Cell {
		row := cells.Parse(scanned(e.m.renderRow(e.row(session))))[0]
		return row[slices.IndexFunc(row, func(c cells.Cell) bool { return c.R == r })]
	}
	snap := sessionsFixture()
	plain := cells.Parse(e.m.th.Render(look.Plain, look.Worktree(snap.Spaces[0].Worktrees[2])))[0][0]
	faint := cells.Parse(e.m.th.Render(look.Plain, look.DimIf(true, look.Space(snap.Spaces[1]))...))[0][0]
	if got, want := []cells.Cell{first("gr@fix", 'f'), first("kitty", 'k')}, []cells.Cell{plain, faint}; !reflect.DeepEqual(got, want) {
		t.Errorf("names drawn %v, want fix's plain and kitty's faint %v", got, want)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e.m.cursor = e.row("gr@fix")
	cmd := e.press("enter")
	if cmd == nil {
		t.Fatal("enter on fix ran nothing")
	}
	cmd()
	adopt := []string{"@", "launch", "--type=background", "--allow-remote-control", exe, "adopt", "--", "gr@fix"}
	if got := calls(); !slices.ContainsFunc(got, func(c []string) bool { return slices.Equal(c, adopt) }) {
		t.Errorf("kitten ran\n%q\nwant among them\n%q", got, adopt)
	}
}
