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

// providerHostsFixture is remoteFixture with a provider listing sessions on this machine, on white, in
// homelab's dns and in a space white has no copy of, and on dune, which has no kspaces server.
func providerHostsFixture() spaces.Snapshot {
	snap := remoteFixture()
	snap.Config.Provider = []string{"prov"}
	snap.Caps = spaces.Capabilities{spaces.CapList: true, spaces.CapOpen: true}
	build := spaces.ProviderSession{ID: "a1", Name: "build", Space: "gr", State: "running"}
	logs := spaces.ProviderSession{ID: "white:@b2", Name: "logs", Space: "homelab", Host: "white", State: "running", Tags: map[string]string{"worktree": "dns"}}
	media := spaces.ProviderSession{ID: "white:@c3", Name: "jellyfin", Space: "homelab-media", Host: "white", State: "running"}
	far := spaces.ProviderSession{ID: "dune:@d4", Name: "far", Space: "gr", Host: "dune", State: "running"}
	snap.Sessions = []spaces.ProviderSession{build, logs, media, far}
	snap.Detached = map[string]spaces.Detached{
		"gr":            {Hosts: []spaces.HostSessions{{Sessions: []spaces.ProviderSession{build}}, {Host: "dune", Sessions: []spaces.ProviderSession{far}}}},
		"homelab":       {Hosts: []spaces.HostSessions{{Host: "white", Sessions: []spaces.ProviderSession{logs}}}},
		"homelab-media": {Hosts: []spaces.HostSessions{{Host: "white", Sessions: []spaces.ProviderSession{media}}}},
	}
	return snap
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

// hostHeaderRow is the index of host's own row.
func (e *sidebarEnv) hostHeaderRow(host string) int {
	e.t.Helper()
	i := slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.HostHeader && r.Host == host })
	if i < 0 {
		e.t.Fatalf("no row of host %q", host)
	}
	return i
}

// With other hosts the config names, the list is by host: this machine first, by its name, then each other
// host by name, with its own copies and their git state, and one that cannot be reached says so. A host
// with no kspaces server is not listed, nor its sessions anywhere; each host's detached sessions are under
// its own copies, and its sessions in a space it has no copy of are under a row of that space.
func TestByHost(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(providerHostsFixture())
	got := e.spacesScreen()
	if got[0] != spacesTitle {
		t.Errorf("title %q, want %q", got[0], spacesTitle)
	}
	var hosts []string
	for _, r := range e.m.rows {
		if r.HostHeader {
			hosts = append(hosts, hostName(r.Host))
		}
		if r.Host == "dune" {
			t.Errorf("a row of dune, which has no kspaces server: %+v", r)
		}
		if r.Detached != nil {
			for _, h := range r.Detached.All.Hosts {
				if h.Host != r.Host {
					t.Errorf("%q's detached sessions under %q's %s", h.Host, r.Host, r.Detached.Of.Name)
				}
			}
		}
	}
	if want := []string{hostName(""), "nas", "white"}; !reflect.DeepEqual(hosts, want) {
		t.Errorf("hosts %q, want %q", hosts, want)
	}
	nas := "▼ nas unreachable " + strings.Repeat("─", 12)
	i := slices.Index(got, nas)
	if i < 0 {
		t.Fatalf("no unreachable nas in\n%q", got)
	}
	want := []string{
		nas,
		"",
		"▼ white " + strings.Repeat("─", 22),
		"  " + termIcon + "homelab-media",
		"  ▶ 1 detached",
		"",
		"▼ " + repoIcon + "homelab main",
		"  " + branchIcon + "dns dns" + strings.Repeat(" ", 16) + "●2",
		"  ▶ 1 detached",
	}
	if !reflect.DeepEqual(got[i:], want) {
		t.Errorf("other hosts:\n%q\nwant\n%q", got[i:], want)
	}
}

// Without other hosts the config names, the list is this machine's, with no host's row, and its detached
// sessions alone: another host has no server to say what its sessions are of.
func TestNoHosts(t *testing.T) {
	e := newSidebarEnv(t)
	snap := providerHostsFixture()
	snap.Remotes = nil
	e.m.Update(snap)
	for _, r := range e.m.rows {
		if r.HostHeader {
			t.Errorf("a host's row: %+v", r)
		}
		if r.Detached != nil {
			for _, h := range r.Detached.All.Hosts {
				if h.Host != "" {
					t.Errorf("%q's detached sessions under %s", h.Host, r.Detached.Of.Name)
				}
			}
		}
	}
}

// On the list by host a group is inside its host, so its header is indented with no rule, while the host's
// row has one: a rule across the sidebar under a host read as the next host.
func TestByHostGroups(t *testing.T) {
	e := newSidebarEnv(t)
	snap := groupsFixture()
	snap.Remotes = remoteFixture().Remotes
	e.m.Update(snap)
	var headers []string
	for i, r := range e.m.rows {
		if (r.HostHeader || r.Group != nil) && r.Host == "" {
			headers = append(headers, e.spacesScreen()[i+2])
		}
	}
	local := "▼ " + hostName("") + " "
	want := []string{local + strings.Repeat("─", 30-len([]rune(local))), "   WORK ", "   PERSONAL "}
	if !reflect.DeepEqual(headers, want) {
		t.Errorf("headers\n%q\nwant\n%q", headers, want)
	}
}

// The cursor on a row of one host stays there across a poll.
func TestByHostKeepsTheCursorsHost(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(remoteFixture())
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.hostRow("white", "white:homelab@dns")
	before := e.m.cursor
	e.m.Update(remoteFixture())
	if e.m.cursor != before || e.m.rows[e.m.cursor].Host != "white" {
		t.Errorf("cursor moved from %d to %d, on host %q", before, e.m.cursor, e.m.rows[e.m.cursor].Host)
	}
}

// Nothing done in a directory here is done to another host's copy; enter opens it in a kitty session of
// its own, a tab of kitten ssh to the host, in its directory there, n and x open the copy's forms, which
// its host's server does them for, and o on the host's row opens a shell there.
func TestByHostCopies(t *testing.T) {
	e := newSidebarEnv(t)
	calls := fakeKittenCalls(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	e.m.Update(remoteFixture())
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.hostRow("white", "white:homelab@dns")
	for _, key := range []string{"N", "R", "c"} {
		if cmd := e.press(key); cmd != nil {
			t.Errorf("%s on white's copy ran something", key)
		}
	}
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

	e.m.cursor = e.hostHeaderRow("white")
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

// sameRootFixture is remoteFixture with white's copy of homelab at gr's root here, as two hosts can have a
// space at one path.
func sameRootFixture() spaces.Snapshot {
	snap := remoteFixture()
	rv := snap.Remotes["white"]
	s := rv.Spaces[0]
	s.Root = "/src/gr"
	rv.Spaces = []spaces.Space{s}
	snap.Remotes["white"] = rv
	return snap
}

// Folding a space under one host leaves a space at the same root under another as it is, and the cursor on
// the row it folded.
func TestByHostFoldsPerHost(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(sameRootFixture())
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.Host == "white" && r.Main && r.Space != nil })
	e.press("space")
	folded := map[string]bool{}
	for _, r := range e.m.rows {
		if r.Main && r.Space != nil && r.Space.Root == "/src/gr" {
			folded[hostName(r.Host)] = r.Folded
		}
	}
	if want := map[string]bool{hostName(""): false, "white": true}; !reflect.DeepEqual(folded, want) {
		t.Errorf("folded %v, want %v", folded, want)
	}
	if r := e.m.rows[e.m.cursor]; r.Host != "white" || !r.Main {
		t.Errorf("cursor on %+v, want white's homelab", r)
	}
}

// A host's row folded is no space's: the sidebar taking focus, which puts the cursor on the current
// session's row, passes over it.
func TestByHostFoldedHostTakesFocus(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(remoteFixture())
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.hostHeaderRow("")
	e.press("space")
	e.m.Update(tea.BlurMsg{})
	e.m.Update(tea.FocusMsg{})
	if r := e.m.rows[0]; !r.HostHeader || !r.Folded {
		t.Errorf("first row %+v, want this machine's, folded", r)
	}
}

// A host folded and a worktree's detached sessions shown are so in a sidebar started after, as the folded
// spaces are.
func TestByHostKeepsFolds(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(providerHostsFixture())
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.hostHeaderRow("nas")
	e.press("space")
	e.m.cursor = slices.IndexFunc(e.m.rows, func(r spaces.Row) bool { return r.Detached != nil && r.Detached.Head() && r.Host == "white" })
	e.press("space")
	m := newSidebar(0)
	got := []map[string]bool{m.hostsFolded, m.detachedOpen}
	want := []map[string]bool{{"nas": true}, e.m.detachedOpen}
	if !reflect.DeepEqual(got, want) || len(e.m.detachedOpen) != 1 {
		t.Errorf("a new sidebar has %v, want %v", got, want)
	}
}
