package tui

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
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

// enter on a diverged row opens nothing and says how to resolve it. Its menu offers the three
// resolutions last, each side's tabs and age, with no keys, so neither a key pressed by habit nor enter on
// the item the menu opens on resolves it; each, moved to and picked, asks the layout pass to resolve the
// divergence so.
func TestDivergedMenu(t *testing.T) {
	var asked []string
	resolveLayout = func(session, host string, r spaces.Resolution) error {
		asked = append(asked, session+" "+host+" "+string(r))
		return nil
	}
	t.Cleanup(func() { resolveLayout = spaces.ResolveLayout })
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	snap := divergedFixture()
	snap.Diverged[1].Saved = time.Now().Add(-3*time.Hour - time.Minute)
	e.m.Update(snap)
	e.m.cursor = e.row("gr@dev")
	if cmd := e.press("enter"); cmd != nil {
		cmd()
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("enter on a diverged row ran %q", got)
	}
	if got, want := fmt.Sprint(e.m.failed), "gr@dev diverged from server: pick a layout from the menu (m)"; got != want {
		t.Errorf("last line %q, want %q", got, want)
	}

	e.m.Update(tea.WindowSizeMsg{Width: 44, Height: 40})
	e.press("m")
	_, y := e.zone(fmt.Sprintf(entryZone, e.m.cursor))
	want := []string{
		"\u2500 Layout " + strings.Repeat("\u2500", 35),
		" Use this machine's: 5 tabs now",
		" Use server's: 2 tabs 3h",
		" Keep both",
		strings.Repeat("\u2500", 44),
	}
	if got := e.screen()[y+14 : y+19]; !reflect.DeepEqual(got, want) {
		t.Errorf("the menu's end:\n%q\nwant\n%q", got, want)
	}
	// No key resolves, in the menu or on the row, nor enter on the item the menu opens on.
	for _, k := range []string{"L", "D", "B", "m", "L", "D", "B", "esc", "m", "enter"} {
		if cmd := e.press(k); cmd != nil {
			cmd()
		}
	}
	if len(asked) != 0 {
		t.Fatalf("keys asked %q", asked)
	}
	// Each is picked by moving to it and pressing enter, or by a click.
	for _, title := range []string{"Use this machine's:", "Use server's:"} {
		e.press("m")
		i := slices.IndexFunc(e.m.menu.items, func(it interact.Bound[*sidebar]) bool { return strings.HasPrefix(it.Title, title) })
		if i < 0 {
			t.Fatalf("no menu item %q", title)
		}
		for range i {
			e.press("j")
		}
		if cmd := e.press("enter"); cmd != nil {
			cmd()
		}
	}
	e.press("m")
	if cmd := e.menuItem("Keep both"); cmd != nil {
		cmd()
	}
	if want := []string{"gr@dev server local", "gr@dev server remote", "gr@dev server both"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("asked %q, want %q", asked, want)
	}
}
