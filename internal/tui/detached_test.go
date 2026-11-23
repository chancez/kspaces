package tui

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// detachedFixture is the fixture with provider sessions no window shows: two in gr, one in docs, which is folded,
// and one in default.
func detachedFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Config = &config.Config{Launch: []string{"launch"}, Provider: []string{"prov"}}
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapOpen: true}
	snap.Detached = map[string]spaces.Detached{
		"gr": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{
			{ID: "a1", Name: "build", Space: "gr", State: "running", Busy: true, Command: "make watch", Tags: map[string]string{"worktree": "fix"}},
			{ID: "b2", Name: "@b2", Space: "gr", State: "shelved", Title: "claude"},
		}}}},
		"docs":    {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "c3", Name: "notes", Space: "docs", State: "exited"}}}}},
		"default": {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{{ID: "d4", Name: "scratch", State: "running", Status: "blocked"}}}}},
	}
	return snap
}

// A space's provider sessions no window shows are a row after its own, of how many, but a folded space's;
// space shows them under it and hides them again. enter on one opens it in a tab of the sidebar's own
// session, which is the space's, running what the provider says opens it.
func TestDetachedRows(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	opens := fakeProvider(t, map[string]string{"open": `{"argv": ["mux", "attach", "b2"]}`}, "")
	e.m.Update(detachedFixture())
	e.m.Update(tea.FocusMsg{})
	head := "  \u25b6 2 detached"
	list := func(gr ...string) []string {
		return append(append([]string{
			spacesTitle,
			"",
			"  " + termIcon + "default",
			"  \u25b6 1 detached",
			"",
			"\u25bc " + repoIcon + "gr main",
			here + blocked + "dev pr/me/dev",
			"  " + branchIcon + "fix fix",
		}, gr...),
			"",
			"  "+repoIcon+"kitty master",
			"",
			"\u25b6 \uf110  docs main",
			"",
			"  \uf00d  broken",
		)
	}
	if got, want := e.spacesScreen(), list(head); !reflect.DeepEqual(got, want) {
		t.Errorf("collapsed:\n%q\nwant\n%q", got, want)
	}
	e.press("j", "j", "space")
	expanded := list("  \u25bc 2 detached", "    "+termIcon+"build busy make watch", "    "+termIcon+"@b2 shelved claude")
	if got := e.spacesScreen(); !reflect.DeepEqual(got, expanded) {
		t.Errorf("expanded:\n%q\nwant\n%q", got, expanded)
	}
	cmd := e.press("j", "j", "enter")
	if cmd == nil {
		t.Fatal("enter ran nothing")
	}
	cmd()
	want := [][]string{
		{"@", "focus-window", "--match", "id:11"},
		{"@", "action", "goto_session", "gr@dev.kitty-session"},
		{"@", "action", "--match=id:11", "launch", "--type=tab", "--cwd=/src/gr/.worktrees/dev", "--add-to-session=gr@dev",
			"--var=kspaces_space=gr", "--var=kspaces_worktree=dev", "mux", "attach", "b2"},
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("enter ran\n%q\nwant\n%q", got, want)
	}
	if got, want := opens(), []string{`open {"session":"b2"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked the provider %q, want %q", got, want)
	}

	// A click on the head hides them again, as space does.
	e.click(fmt.Sprintf(entryZone, slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.Detached != nil && r.Detached.Of.Name == "gr" })), tea.MouseLeft)
	if got, want := e.spacesScreen(), list(head); !reflect.DeepEqual(got, want) {
		t.Errorf("clicked:\n%q\nwant\n%q", got, want)
	}
	// What the filters look for is not there.
	e.press("O")
	for _, r := range e.m.rows {
		if r.Detached != nil {
			t.Errorf("a detached row under the open filter: %#v", r.Detached)
		}
	}
}

// formsOpened are the kspaces commands calls opened forms with, each its arguments after the program.
func formsOpened(t *testing.T, calls [][]string) [][]string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, c := range calls {
		if i := slices.Index(c, exe); i >= 0 {
			out = append(out, c[i+1:])
		}
	}
	return out
}

// On a detached session's row x kills a running one and removes an ended one, as F does, each confirmed in
// a form; z shelves a running one at once; M moves one, in a form; p on the head cleans up the space's. A
// provider without a verb has no key for it.
func TestDetachedActions(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	asked := fakeProvider(t, nil, "")
	snap := detachedFixture()
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapOpen: true, spaces.CapKill: true, spaces.CapShelve: true,
		spaces.CapRemove: true, spaces.CapMove: true}
	e.m.Update(snap)
	e.m.Update(tea.FocusMsg{})
	run := func(keys ...string) {
		for _, k := range keys {
			if cmd := e.press(k); cmd != nil {
				cmd()
			}
		}
	}
	// gr@dev, fix, the head, which opens; build, running; @b2, shelved.
	run("j", "j", "space", "j", "x", "z", "M", "j", "x", "F", "z", "k", "k", "p")
	want := [][]string{
		{"sessions", "kill", "--", "a1"},
		{"sessions", "move", "--", "a1"},
		{"sessions", "remove", "--", "b2"},
		{"sessions", "remove", "--", "b2"},
		{"sessions", "cleanup", "gr"},
	}
	if got := formsOpened(t, calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("forms\n%q\nwant\n%q", got, want)
	}
	if got, want := asked(), []string{`shelve {"session":"a1"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}

	snap.Caps = spaces.Capabilities{spaces.CapList: true}
	e.m.Update(snap)
	before := len(calls())
	// The head, then build and @b2.
	for _, k := range []string{"p", "j", "x", "z", "M", "j", "x", "F"} {
		if cmd := e.press(k); cmd != nil && k != "j" {
			t.Errorf("%s ran %v with a provider that only lists", k, cmd())
		}
	}
	if got := calls(); len(got) != before {
		t.Errorf("kitten ran %q", got[before:])
	}
}

// In the select mode a detached session's row, or its head for all of them, ticks as a worktree's does,
// and Move, M, moves the ticked ones in one form, with a provider that moves.
func TestSelectDetached(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	snap := detachedFixture()
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapMove: true}
	e.m.Update(snap)
	e.m.Update(tea.FocusMsg{})
	// gr@dev, fix, gr's head; then default's head, at the top.
	e.press("j", "j", "v", "space", "g", "j", "space")
	got := e.m.tickedDetached()
	want := []spaces.ProviderSession{snap.Detached["default"].Hosts[0].Sessions[0], snap.Detached["gr"].Hosts[0].Sessions[0], snap.Detached["gr"].Hosts[0].Sessions[1]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ticked %#v\nwant %#v", got, want)
	}
	var bar []string
	for _, l := range e.screen() {
		if strings.HasPrefix(l, " Move") {
			bar = append(bar, l)
		}
	}
	if want := []string{" Move                  3    M"}; !reflect.DeepEqual(bar, want) {
		t.Errorf("bar %q, want %q", bar, want)
	}
	if cmd := e.press("M"); cmd != nil {
		cmd()
	}
	if got, want := formsOpened(t, calls()), [][]string{{"sessions", "move", "--", "d4", "a1", "b2"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("forms %q, want %q", got, want)
	}
}
