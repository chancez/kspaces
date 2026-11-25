package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// hostsFixture is detachedFixture with every session the provider lists: its detached ones on this
// machine, and on server one gr@dev's window 11 shows, one detached in gr's far, which only server has,
// and one detached in default, stale.
func hostsFixture() spaces.Snapshot {
	snap := detachedFixture()
	for _, space := range []string{"default", "docs", "gr"} {
		for _, h := range snap.Detached[space].Hosts {
			snap.Sessions = append(snap.Sessions, h.Sessions...)
		}
	}
	far := spaces.ProviderSession{ID: "server:@f6", Name: "deploy", Space: "gr", Host: "server", State: "running", Tags: map[string]string{"worktree": "far"}}
	logs := spaces.ProviderSession{ID: "server:@g7", Name: "logs", Host: "server", State: "running", Stale: true, Command: "tail -f"}
	snap.Sessions = append(snap.Sessions,
		spaces.ProviderSession{ID: "server:@e5", Name: "shell", Space: "gr", Host: "server", State: "running", Windows: []int{11}, Tags: map[string]string{"worktree": "dev"}},
		far, logs)
	gr := snap.Detached["gr"]
	gr.Hosts = append(gr.Hosts, spaces.HostSessions{Host: "server", Sessions: []spaces.ProviderSession{far}})
	snap.Detached["gr"] = gr
	def := snap.Detached["default"]
	def.Hosts = append(def.Hosts, spaces.HostSessions{Host: "server", Sessions: []spaces.ProviderSession{logs}})
	snap.Detached["default"] = def
	return snap
}

// tab lists the spaces tab's rows by host: this machine's whole list, then each other host with what it
// has, its detached sessions under its own head. A worktree opens its kitty session whichever host lists
// it, one only another host has opens a tab of its session there, and space folds a host. The tab is
// kept, so a sidebar started after shows it.
func TestHostsTab(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	opens := fakeProvider(t, map[string]string{"open": `{"argv": ["mux", "attach", "server:@f6"]}`}, "")
	e.m.Update(hostsFixture())
	e.m.Update(tea.FocusMsg{})
	e.press("tab")
	local := []string{
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
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
		"",
	}
	all := append(slices.Clone(local),
		"\u25bc server "+strings.Repeat("\u2500", 21),
		"  "+termIcon+"default",
		"  \u25b6 1 detached",
		"",
		"\u25bc "+repoIcon+"gr main",
		here+blocked+"dev pr/me/dev",
		"  "+branchIcon+"far",
		"  \u25b6 1 detached",
	)
	if got := e.spacesScreen(); !reflect.DeepEqual(got, all) {
		t.Errorf("hosts tab:\n%q\nwant\n%q", got, all)
	}

	// server's far, which only server has: its session opens in a tab of gr@far.
	e.m.cursor = e.hostRow("server", "gr@far")
	cmd := e.press("enter")
	if cmd == nil {
		t.Fatal("enter on far ran nothing")
	}
	cmd()
	if got, want := opens(), []string{`open {"session":"server:@f6"}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked the provider %q, want %q", got, want)
	}
	launched := slices.ContainsFunc(calls(), func(c []string) bool { return slices.Contains(c, "--add-to-session=gr@far") })
	if !launched {
		t.Errorf("enter on far opened no tab in gr@far")
	}

	// server's own row folds it.
	e.m.cursor = slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.HostHeader && r.Host == "server" })
	e.press("space")
	if got, want := e.spacesScreen(), append(slices.Clone(local), "\u25b6 server "+strings.Repeat("\u2500", 21)); !reflect.DeepEqual(got, want) {
		t.Errorf("server folded:\n%q\nwant\n%q", got, want)
	}

	if !newSidebar(0).hostsTab {
		t.Error("a new sidebar does not show the hosts tab")
	}
	e.press("tab")
	if got := e.spacesScreen()[2]; got != "  "+termIcon+"default" {
		t.Errorf("tab back shows %q first, want the spaces tab's default", got)
	}
}

// hostRow is the index of the row of the worktree whose kitty session is session, under host.
func (e *sidebarEnv) hostRow(host, session string) int {
	e.t.Helper()
	i := slices.IndexFunc(e.m.rows, func(r spaces.Row) bool {
		return r.Host == host && r.Worktree != nil && r.Worktree.Session == session && r.Detached == nil
	})
	if i < 0 {
		e.t.Fatalf("no row of %s under %q", session, host)
	}
	return i
}

// The cursor on a worktree under one host stays there across a poll, though another host lists it too.
func TestHostsTabKeepsTheCursorsHost(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(hostsFixture())
	e.m.Update(tea.FocusMsg{})
	e.press("tab")
	e.m.cursor = e.hostRow("server", "gr@dev")
	before := e.m.cursor
	e.m.Update(hostsFixture())
	if e.m.cursor != before || e.m.rows[e.m.cursor].Host != "server" {
		t.Errorf("cursor moved from %d to %d, on host %q", before, e.m.cursor, e.m.rows[e.m.cursor].Host)
	}
}

// On the hosts tab a group is inside its host, so its header is indented with no rule, while the host's
// row has one: a rule across the sidebar under a host read as the next host.
func TestHostsTabGroups(t *testing.T) {
	e := newSidebarEnv(t)
	snap := groupsFixture()
	snap.Config.Provider = []string{"prov"}
	snap.Caps = spaces.Capabilities{spaces.CapList: true}
	e.m.Update(snap)
	e.press("tab")
	var headers []string
	for i, r := range e.m.rows {
		if r.HostHeader || r.Group != nil {
			headers = append(headers, e.spacesScreen()[i+2])
		}
	}
	want := []string{"\u25bc local " + strings.Repeat("\u2500", 22), "   WORK \uf07c", "   PERSONAL \uf07c"}
	if !reflect.DeepEqual(headers, want) {
		t.Errorf("headers\n%q\nwant\n%q", headers, want)
	}
}

// A click on the title's tab that is not showing switches to it; the one that is does nothing.
func TestHostsTabClick(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(hostsFixture())
	e.m.render()
	e.click(spacesTabZone, tea.MouseLeft)
	if e.m.showingHosts() {
		t.Fatal("a click on SPACES showed the hosts tab")
	}
	e.click(hostsTabZone, tea.MouseLeft)
	if !e.m.showingHosts() {
		t.Fatal("a click on HOSTS did not show the hosts tab")
	}
	e.click(spacesTabZone, tea.MouseLeft)
	if e.m.showingHosts() {
		t.Fatal("a click on SPACES did not show the spaces tab")
	}
}

// Without a provider that lists sessions there is no hosts tab: the title is SPACES alone, and tab does
// nothing, even with the hosts tab kept from before.
func TestNoHostsTab(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.hostsTab = true
	e.m.setRows()
	e.press("tab")
	if got := e.spacesScreen()[0]; got != spacesTitle || e.m.showingHosts() {
		t.Errorf("title %q, hosts showing %v; want %q and false", got, e.m.showingHosts(), spacesTitle)
	}
}
