package tui

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
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

// remoteFixture is fixture with two other hosts the config names: white, whose server has its copy of
// homelab, and nas, whose server could not be reached.
func remoteFixture() spaces.Snapshot {
	snap := fixture(time.Now())
	snap.Config = &config.Config{Launch: []string{"launch"}}
	dns := spaces.Worktree{Path: "/home/me/homelab/.worktrees/dns", Branch: "dns", Session: "white:homelab@dns", Host: "white"}
	snap.Remotes = map[string]spaces.RemoteView{
		"white": {Loaded: true, Spaces: []spaces.Space{{Name: "homelab", Root: "/home/me/homelab", Worktrees: []spaces.Worktree{
			{Path: "/home/me/homelab", Branch: "main", Session: "white:homelab", Host: "white"}, dns,
		}}}},
		"nas": {Err: "ssh: connect to host nas: Connection refused"},
	}
	snap.Git = map[string]gitstate.State{spaces.GitKey(dns): {Status: &gitstate.Status{Changes: 2}}}
	return snap
}

// Other hosts the config names make a hosts tab without a provider: each lists its own copies, with their
// git state, and one that cannot be reached says so. Nothing done in a directory here is done to a copy;
// enter opens it in a kitty session of its own, a tab of kitten ssh to the host, in its directory there.
func TestHostsTabRemote(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	e.m.Update(remoteFixture())
	e.m.Update(tea.FocusMsg{})
	e.press("tab")
	got := e.spacesScreen()
	nas := "▼ nas unreachable " + strings.Repeat("─", 12)
	i := slices.Index(got, nas)
	if i < 0 {
		t.Fatalf("no unreachable nas in\n%q", got)
	}
	want := []string{
		nas,
		"",
		"▼ white " + strings.Repeat("─", 22),
		"▼ " + repoIcon + "homelab main",
		"  " + branchIcon + "dns dns" + strings.Repeat(" ", 16) + "●2",
	}
	if !reflect.DeepEqual(got[i:], want) {
		t.Errorf("other hosts:\n%q\nwant\n%q", got[i:], want)
	}
	e.m.cursor = e.hostRow("white", "white:homelab@dns")
	for _, key := range []string{"N", "R", "c"} {
		if cmd := e.press(key); cmd != nil {
			t.Errorf("%s on white's copy ran something", key)
		}
	}
	// New worktree and Remove worktree open the copy's forms, which white's server does them for.
	for key, form := range map[string][]string{
		"n": {"new", "--host", "white", "/home/me/homelab"},
		"x": {"remove", "--host", "white", "/home/me/homelab/.worktrees/dns"},
	} {
		cmd := e.press(key)
		if cmd == nil {
			t.Fatalf("%s on white's copy ran nothing", key)
		}
		cmd()
		got := calls()
		if last := got[len(got)-1]; !slices.Equal(last[len(last)-len(form):], form) {
			t.Errorf("%s launched %q, want it to end in %q", key, last, form)
		}
	}
	start := len(calls())
	cmd := e.press("enter")
	if cmd == nil {
		t.Fatal("enter on white's copy ran nothing")
	}
	cmd()
	dir := "/home/me/homelab/.worktrees/dns"
	wantCalls := [][]string{
		{"@", "focus-window", "--match", "id:11"},
		{"@", "action", "launch", "--type=tab", "--cwd=" + home, "--add-to-session=white:homelab@dns",
			"--var=kspaces_space=homelab", "--var=kspaces_worktree=dns", "--var=kspaces_host=white", "--var=kspaces_dir=" + dir,
			"kitten", "ssh", "--kitten", "cwd=" + dir, "white"},
		{"@", "action", "goto_session", "white:homelab@dns.kitty-session"},
	}
	if got := calls()[start:]; !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("kitten ran\n%q\nwant\n%q", got, wantCalls)
	}

	// o on white's own row opens a shell there, its default copy.
	e.m.cursor = slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.HostHeader && r.Host == "white" })
	before := len(calls())
	if cmd := e.press("o"); cmd == nil {
		t.Fatal("o on white ran nothing")
	} else {
		cmd()
	}
	shell := []string{"@", "action", "launch", "--type=tab", "--cwd=" + home, "--add-to-session=white:default",
		"--var=kspaces_space=default", "--var=kspaces_host=white", "kitten", "ssh", "white"}
	if got := calls()[before:]; len(got) < 2 || !reflect.DeepEqual(got[1], shell) {
		t.Errorf("o ran\n%q\nwant second\n%q", got, shell)
	}
}
