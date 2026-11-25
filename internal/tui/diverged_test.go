package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// divergedFixture is hostsFixture with gr@dev's layout diverged from server's, and docs@notes', folded into
// docs, from this machine's.
func divergedFixture() spaces.Snapshot {
	snap := hostsFixture()
	snap.Diverged = []spaces.Divergence{
		{Session: "docs@notes", LocalTabs: 1, RemoteTabs: 2},
		{Session: "gr@dev", Host: "server", LocalTabs: 5, RemoteTabs: 2},
	}
	return snap
}

// A worktree whose layout diverged is marked at its row's end, after its git state, as is a folded space
// whose worktree's did. On the hosts tab it is marked under the host it diverged from alone.
func TestDivergedRows(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(divergedFixture())
	want := []string{
		tabsTitle,
		"",
		"  " + termIcon + "default",
		"  \u25b6 local 1, server 1",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  \u25b6 server 1",
		here + blocked + "dev pr/me/dev  ~ diverged",
		"  " + branchIcon + "fix fix",
		"  \u25b6 2 detached",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main      ~ diverged",
		"",
		"  \uf00d  broken",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("spaces tab:\n%q\nwant\n%q", got, want)
	}

	e.m.Update(tea.FocusMsg{})
	e.press("tab")
	want = []string{
		tabsTitle,
		"",
		"\u25bc local " + strings.Repeat("\u2500", 22),
		"  " + termIcon + "default",
		"  \u25b6 1 detached",
		"",
		"\u25bc " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev",
		"  " + branchIcon + "fix fix",
		"  \u25b6 2 detached",
		"",
		"  " + repoIcon + "kitty master",
		"",
		"\u25b6 \uf110  docs main      ~ diverged",
		"",
		"  \uf00d  broken",
		"",
		"\u25bc server " + strings.Repeat("\u2500", 21),
		"  " + termIcon + "default",
		"  \u25b6 1 detached",
		"",
		"\u25bc " + repoIcon + "gr main",
		here + blocked + "dev pr/me/dev  ~ diverged",
		"  " + branchIcon + "far",
		"  \u25b6 1 detached",
	}
	if got := e.spacesScreen(); !reflect.DeepEqual(got, want) {
		t.Errorf("hosts tab:\n%q\nwant\n%q", got, want)
	}
}
