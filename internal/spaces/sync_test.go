package spaces

import (
	"cmp"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// fakeShareKitty is a kitty of stack tabs of one worktree's session, whose windows each show a provider
// session, for layoutSync: a launch opens a window showing the session its last argument names, as the
// provider's open in these tests says.
type fakeShareKitty struct {
	session string
	tabs    [][]string
	next    int
	ids     map[string]int
	// unlisted are sessions whose windows the provider does not list yet.
	unlisted map[string]bool
}

func newFakeShareKitty(session string, tabs ...[]string) *fakeShareKitty {
	k := &fakeShareKitty{session: session, next: 100, ids: map[string]int{}}
	for _, t := range tabs {
		k.tabs = append(k.tabs, nil)
		for _, s := range t {
			k.add(len(k.tabs)-1, s)
		}
	}
	return k
}

func (k *fakeShareKitty) add(tab int, s string) int {
	k.next++
	k.tabs[tab] = append(k.tabs[tab], s)
	k.ids[s] = k.next
	return k.next
}

// fullTabs are k's tabs as `kitten @ ls` has them.
func (k *fakeShareKitty) fullTabs() ([]kitty.FullTab, error) {
	var out []kitty.FullTab
	for i, t := range k.tabs {
		if len(t) == 0 {
			continue
		}
		tab := kitty.FullTab{ID: i + 1, Layout: "stack"}
		for _, s := range t {
			id := k.ids[s]
			tab.Windows = append(tab.Windows, kitty.FullWindow{ID: id, SessionName: k.session})
			tab.LayoutState.AllWindows.WindowGroups = append(tab.LayoutState.AllWindows.WindowGroups, struct {
				ID        int   `json:"id"`
				WindowIDs []int `json:"window_ids"`
			}{ID: id, WindowIDs: []int{id}})
		}
		out = append(out, tab)
	}
	return out, nil
}

func (k *fakeShareKitty) Launch(_ string, args []string) (int, error) {
	s := args[len(args)-1]
	if slices.Contains(args, "--type=tab") {
		k.tabs = append(k.tabs, nil)
		return k.add(len(k.tabs)-1, s), nil
	}
	var of int
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "--next-to=id:"); ok {
			fmt.Sscan(v, &of)
		}
	}
	for i, t := range k.tabs {
		for _, o := range t {
			if k.ids[o] == of {
				return k.add(i, s), nil
			}
		}
	}
	return 0, fmt.Errorf("no window %d", of)
}

func (k *fakeShareKitty) GotoLayout(int, string) error { return nil }

func (k *fakeShareKitty) Close(id int) error {
	for i, t := range k.tabs {
		k.tabs[i] = slices.DeleteFunc(t, func(s string) bool { return k.ids[s] == id })
	}
	return nil
}

// remove closes s's window as a user would.
func (k *fakeShareKitty) remove(s string) { _ = k.Close(k.ids[s]) }

// fakeWorld is the hosts machines keep copies' layouts on, each its own store, and which are offline,
// reaching no other.
type fakeWorld struct {
	stores  map[string]*LocalHost
	saves   map[string]int
	offline map[string]bool
}

func newFakeWorld() *fakeWorld {
	return &fakeWorld{stores: map[string]*LocalHost{}, saves: map[string]int{}, offline: map[string]bool{}}
}

// store is host's server.
func (f *fakeWorld) store(host string) *LocalHost {
	if f.stores[host] == nil {
		f.stores[host] = NewLocalHost("", "")
	}
	return f.stores[host]
}

// stored is the layout host keeps of key, empty for none.
func (f *fakeWorld) stored(host, key string) SharedLayout {
	got, _ := f.store(host).LoadLayouts([]CopyLayout{{Key: key, Revision: -1}})
	if len(got) == 0 || got[0].Layout == nil {
		return SharedLayout{}
	}
	return *got[0].Layout
}

// countingHost is a host's server that counts its saves.
type countingHost struct {
	*LocalHost
	world *fakeWorld
	name  string
}

func (h countingHost) SaveLayout(key string, revision int64, l SharedLayout) (int64, error) {
	rev, err := h.LocalHost.SaveLayout(key, revision, l)
	if err == nil {
		h.world.saves[h.name]++
	}
	return rev, err
}

// machine is a kitty on host self showing one copy, its kitty session, through world: its panes are the
// sessions they show, as self's provider names them, HOST:ID for another host's.
type machine struct {
	k     *fakeShareKitty
	ls    *layoutSync
	now   *time.Time
	self  string
	world *fakeWorld
}

func newMachine(t *testing.T, world *fakeWorld, self, session string, now *time.Time, tabs ...[]string) *machine {
	k := newFakeShareKitty(session, tabs...)
	m := &machine{k: k, now: now, self: self, world: world}
	m.ls = &layoutSync{
		path:   filepath.Join(t.TempDir(), "layouts.json"),
		kitty:  self,
		writer: self,
		now:    func() time.Time { return *now },
		tabs:   k.fullTabs,
		host:   m.host,
		open: func(_ []string, s ProviderSession) ([]string, string, error) {
			return []string{"attach", s.ID}, "", nil
		},
		k: k,
	}
	return m
}

// host is the server of the host m names name, its own for none, nil when one of them is offline.
func (m *machine) host(name string) Host {
	real := cmp.Or(name, m.self)
	if real != m.self && (m.world.offline[m.self] || m.world.offline[real]) {
		return nil
	}
	return countingHost{LocalHost: m.world.store(real), world: m.world, name: real}
}

// sessions are the provider's sessions m's windows show, in gr.
func (m *machine) sessions() []ProviderSession {
	var out []ProviderSession
	for _, t := range m.k.tabs {
		for _, s := range t {
			if m.k.unlisted[s] {
				continue
			}
			host, _, ok := strings.Cut(s, ":")
			if !ok {
				host = ""
			}
			out = append(out, ProviderSession{ID: s, Name: s, Space: "gr", Host: host, State: "running", Windows: []int{m.k.ids[s]}})
		}
	}
	return out
}

// cfg names the hosts the tests' copies are of.
var syncCfg = config.Config{Hosts: map[string]config.HostSettings{"white": {}, "desktop": {}}}

// pass runs a pass of m a while after the last one.
func (m *machine) pass(t *testing.T) {
	t.Helper()
	*m.now = m.now.Add(5 * time.Second)
	if err := m.ls.pass(syncCfg, m.sessions(), func(string) string { return "/src/gr/dev" }, map[string]int{}); err != nil {
		t.Fatal(err)
	}
}

// passes runs n passes of each of ms in turn.
func passes(t *testing.T, n int, ms ...*machine) {
	t.Helper()
	for range n {
		for _, m := range ms {
			m.pass(t)
		}
	}
}

// shows is m's tabs with a pane, each the sessions it shows.
func (m *machine) shows() [][]string {
	var got [][]string
	for _, tab := range m.k.tabs {
		if len(tab) > 0 {
			got = append(got, tab)
		}
	}
	return got
}

// Two machines showing white's copy of gr@dev converge on one layout, kept on white: a split and a tab
// opened on one open on the other, one closed closes, each saved once it settles and taken at the next
// load, and nothing saved again once both agree.
func TestLayoutSync(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", "white:gr@dev", &now, []string{"white:@1", "white:@2"})
	b := newMachine(t, world, "desktop", "white:gr@dev", &now, []string{"white:@1"})
	both := func(want ...[]string) {
		t.Helper()
		for _, m := range []*machine{a, b} {
			if got := m.shows(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s has %q, want %q", m.self, got, want)
			}
		}
	}

	// laptop saves once its panes settle; desktop takes them, opening @2 beside @1.
	passes(t, 4, a)
	b.pass(t)
	b.pass(t)
	b.pass(t)
	both([]string{"white:@1", "white:@2"})
	if got := world.stored("white", "gr@dev").Members(); got != "@1,@2" {
		t.Errorf("white keeps %q, want its own sessions, @1,@2", got)
	}

	// Closing @2 on laptop closes it on desktop.
	a.k.remove("white:@2")
	passes(t, 2, a)
	b.pass(t)
	both([]string{"white:@1"})

	// A tab opened on desktop opens on laptop, once desktop's take settles.
	b.k.tabs = append(b.k.tabs, nil)
	b.k.add(len(b.k.tabs)-1, "white:@3")
	passes(t, 3, b)
	a.pass(t)
	both([]string{"white:@1"}, []string{"white:@3"})

	// Agreeing, neither saves again.
	saves := world.saves["white"]
	passes(t, 3, a, b)
	if n := world.saves["white"] - saves; n != 0 {
		t.Errorf("%d saves once both agreed", n)
	}
	both([]string{"white:@1"}, []string{"white:@3"})
}

// A machine's own copy is kept on its own server, and another machine showing it names its sessions with
// the host before them: opened there with one of its panes, it opens the rest.
func TestLayoutSyncOwnCopy(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	desk := newMachine(t, world, "desktop", "gr@dev", &now, []string{"@1", "@2"}, []string{"@3"})
	passes(t, 4, desk)
	if got := world.stored("desktop", "gr@dev").Members(); got != "@1,@2;@3" {
		t.Errorf("desktop keeps %q", got)
	}
	laptop := newMachine(t, world, "laptop", "desktop:gr@dev", &now, []string{"desktop:@1"})
	passes(t, 3, laptop)
	if want := [][]string{{"desktop:@1", "desktop:@2"}, {"desktop:@3"}}; !reflect.DeepEqual(laptop.shows(), want) {
		t.Errorf("laptop shows %q, want %q", laptop.shows(), want)
	}
}

// A copy this machine stops showing is forgotten, so showing it again takes the stored layout's sessions
// without closing what it opened with.
func TestLayoutSyncForgets(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", "white:gr@dev", &now, []string{"white:@1", "white:@2"})
	passes(t, 4, a)
	b := newMachine(t, world, "desktop", "white:gr@dev", &now, []string{"white:@1"})
	passes(t, 3, b)
	b.k.remove("white:@1")
	b.k.remove("white:@2")
	b.pass(t)
	// Opened again with @4 alone: @1 and @2 come back in a tab of their own, and @4 stays.
	b.k.tabs = [][]string{nil}
	b.k.add(0, "white:@4")
	passes(t, 3, b)
	if want := [][]string{{"white:@4"}, {"white:@1", "white:@2"}}; !reflect.DeepEqual(b.k.tabs, want) {
		t.Errorf("tabs %q, want %q", b.k.tabs, want)
	}
}

// Two machines changing before either sees the other keep both changes: the second to save is behind, takes
// the first's, then saves the two together.
func TestLayoutSyncBothChange(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", "white:gr@dev", &now, []string{"white:@1"})
	b := newMachine(t, world, "desktop", "white:gr@dev", &now, []string{"white:@1"})
	passes(t, 4, a, b)
	a.k.tabs = append(a.k.tabs, nil)
	a.k.add(len(a.k.tabs)-1, "white:@5")
	b.k.add(0, "white:@6")
	passes(t, 6, a, b)
	for _, m := range []*machine{a, b} {
		var got []string
		for _, tab := range m.k.tabs {
			got = append(got, tab...)
		}
		slices.Sort(got)
		if want := []string{"white:@1", "white:@5", "white:@6"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s shows %q, want %q", m.self, got, want)
		}
	}
}

// A machine apart from the copy's host keeps its panes as they are, and once back takes what was saved
// meanwhile, keeping what it changed itself, which it then saves: there is nothing to pick between, as only
// the host keeps the layout.
func TestLayoutSyncApart(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", "white:gr@dev", &now, []string{"white:@1", "white:@2"})
	b := newMachine(t, world, "desktop", "white:gr@dev", &now, []string{"white:@1", "white:@2"})
	passes(t, 4, a, b)
	world.offline["laptop"] = true
	a.k.add(0, "white:@3")
	b.k.remove("white:@2")
	passes(t, 4, a, b)
	if want := [][]string{{"white:@1", "white:@2", "white:@3"}}; !reflect.DeepEqual(a.shows(), want) {
		t.Errorf("laptop apart shows %q, want %q", a.shows(), want)
	}
	delete(world.offline, "laptop")
	passes(t, 6, a, b)
	for _, m := range []*machine{a, b} {
		if want := [][]string{{"white:@1", "white:@3"}}; !reflect.DeepEqual(m.shows(), want) {
			t.Errorf("%s shows %q, want %q", m.self, m.shows(), want)
		}
	}
}

// A pane opened by taking another machine's layout is not saved as removed while the provider has yet to
// list its window, which would close it on the other machine.
func TestLayoutSyncWaitsForOpenedPanes(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", "white:gr@dev", &now, []string{"white:@1", "white:@2"})
	b := newMachine(t, world, "desktop", "white:gr@dev", &now, []string{"white:@1"})
	passes(t, 4, a)
	passes(t, 3, b)
	// desktop opened @2, but its provider does not list @2's window yet.
	b.k.unlisted = map[string]bool{"white:@2": true}
	for range 3 {
		now = now.Add(3 * time.Second)
		if err := b.ls.pass(syncCfg, b.sessions(), func(string) string { return "" }, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := world.stored("white", "gr@dev").Members(); got != "@1,@2" {
		t.Errorf("stored %q, want @1,@2", got)
	}
}

// A copy's layout is the panes showing sessions of its host and space: kitty's startup session, whatever
// its file is named, shows sessions in default, this machine's default is not shared, a pane of this
// machine's in another host's copy is not that host's, and a copy of a host the config does not name has
// nowhere to be kept.
func TestSharedCopies(t *testing.T) {
	tab := func(id int, session string, windows ...int) kitty.FullTab {
		ft := kitty.FullTab{ID: id, Layout: "stack"}
		for _, w := range windows {
			ft.Windows = append(ft.Windows, kitty.FullWindow{ID: w, SessionName: session})
			ft.LayoutState.AllWindows.WindowGroups = append(ft.LayoutState.AllWindows.WindowGroups, struct {
				ID        int   `json:"id"`
				WindowIDs []int `json:"window_ids"`
			}{ID: w, WindowIDs: []int{w}})
		}
		return ft
	}
	tabs := []kitty.FullTab{tab(1, "startup", 11), tab(2, "gr@dev", 21, 22), tab(3, "default", 31), tab(4, "white:homelab", 41, 42), tab(5, "nas:gr", 51)}
	sessions := []ProviderSession{
		{ID: "@a", Windows: []int{11}},
		{ID: "@b", Space: "gr", Windows: []int{21}},
		{ID: "@c", Space: "docs", Windows: []int{22}},
		{ID: "@d", Windows: []int{31}},
		{ID: "white:@e", Space: "homelab", Host: "white", Windows: []int{41}},
		{ID: "@f", Space: "homelab", Windows: []int{42}},
		{ID: "nas:@g", Space: "gr", Host: "nas", Windows: []int{51}},
	}
	got := sharedCopies(config.Config{Hosts: map[string]config.HostSettings{"white": {}}}, tabs, sessions)
	stack := func(id string) SharedLayout {
		return SharedLayout{Version: 1, Sessions: []string{id}, Tabs: []SharedTab{{Layout: "stack", Panes: []SharedPane{{Session: 0}}}}}
	}
	empty := SharedLayout{Version: 1, Sessions: []string{}, Tabs: []SharedTab{}}
	none := sharedLocal{Window: map[string]int{}, Tab: map[string]int{}}
	want := map[string]sharedCopy{
		"gr@dev":        {key: "gr@dev", layout: stack("@b"), at: sharedLocal{Window: map[string]int{"@b": 21}, Tab: map[string]int{"@b": 2}}},
		"startup":       {key: "startup", layout: empty, at: none},
		"white:homelab": {host: "white", key: "homelab", layout: stack("@e"), at: sharedLocal{Window: map[string]int{"@e": 41}, Tab: map[string]int{"@e": 4}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shared %+v\nwant   %+v", got, want)
	}
}
