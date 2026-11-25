package spaces

import (
	"cmp"
	"fmt"
	"maps"
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

// fakeStore is a host's stored layouts, by ref with the host's own name.
type fakeStore struct {
	layouts map[LayoutRef]StoredLayout
	saves   int
}

// fakeWorld is the hosts machines share layouts through: each one's store, the host of each session,
// "server" for one not named, and which are offline, reaching no other.
type fakeWorld struct {
	stores  map[string]*fakeStore
	hostOf  map[string]string
	offline map[string]bool
}

func newFakeWorld() *fakeWorld {
	return &fakeWorld{stores: map[string]*fakeStore{}, hostOf: map[string]string{}, offline: map[string]bool{}}
}

// store is host's store.
func (f *fakeWorld) store(host string) *fakeStore {
	if f.stores[host] == nil {
		f.stores[host] = &fakeStore{layouts: map[LayoutRef]StoredLayout{}}
	}
	return f.stores[host]
}

// host is session id's host.
func (f *fakeWorld) host(id string) string {
	return cmp.Or(f.hostOf[id], "server")
}

// stored is the layout of gr@dev stored on host.
func (f *fakeWorld) stored(host string) SharedLayout {
	l := f.store(host).layouts[LayoutRef{Space: "gr", Host: host, Key: "dev"}].Layout
	if l == nil {
		return SharedLayout{}
	}
	return *l
}

// machine is a kitty on host self sharing gr@dev's layout through world, which names a host as self does.
type machine struct {
	k     *fakeShareKitty
	ls    *layoutSync
	now   *time.Time
	self  string
	world *fakeWorld
}

func newMachine(t *testing.T, world *fakeWorld, self string, now *time.Time, tabs ...[]string) *machine {
	k := newFakeShareKitty("gr@dev", tabs...)
	m := &machine{k: k, now: now, self: self, world: world}
	m.ls = &layoutSync{
		path:   filepath.Join(t.TempDir(), "layouts.json"),
		kitty:  self,
		writer: self,
		now:    func() time.Time { return *now },
		tabs:   k.fullTabs,
		load:   m.load,
		save:   m.save,
		open: func(_ []string, s ProviderSession) ([]string, string, error) {
			return []string{"attach", s.ID}, "", nil
		},
		k: k,
	}
	return m
}

// real is the host m names host, its own for none.
func (m *machine) real(host string) string {
	return cmp.Or(host, m.self)
}

// reaches reports whether m reaches host.
func (m *machine) reaches(host string) bool {
	return host == m.self || !m.world.offline[m.self] && !m.world.offline[host]
}

// named is host as m names it, empty for its own.
func (m *machine) named(host string) string {
	if host == m.self {
		return ""
	}
	return host
}

// sessions are the provider's sessions in gr@dev as m's windows show them, and those of the world's hosts
// no window shows, those of a host it does not reach stale.
func (m *machine) sessions() []ProviderSession {
	var out []ProviderSession
	session := func(id string, windows []int) ProviderSession {
		h := m.world.host(id)
		return ProviderSession{ID: id, Name: id, Space: "gr", Host: m.named(h), Stale: !m.reaches(h), State: "running",
			Tags: map[string]string{"worktree": "dev"}, Windows: windows}
	}
	shown := map[string]bool{}
	for _, t := range m.k.tabs {
		for _, s := range t {
			shown[s] = true
			if !m.k.unlisted[s] {
				out = append(out, session(s, []int{m.k.ids[s]}))
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(m.world.hostOf)) {
		if !shown[id] {
			out = append(out, session(id, nil))
		}
	}
	return out
}

// load is the layouts of known, and the hosts m does not reach as unreachable.
func (m *machine) load(_ []string, known []StoredLayout) (LoadedLayouts, error) {
	var out LoadedLayouts
	for _, k := range known {
		h := m.real(k.Host)
		if !m.reaches(h) {
			if !slices.Contains(out.Unreachable, k.Host) {
				out.Unreachable = append(out.Unreachable, k.Host)
			}
			continue
		}
		s := m.world.store(h).layouts[LayoutRef{Space: k.Space, Host: h, Key: k.Key}]
		s.LayoutRef = k.LayoutRef
		if s.Revision != k.Revision {
			out.Layouts = append(out.Layouts, s)
		}
	}
	return out, nil
}

func (m *machine) save(_ []string, ref LayoutRef, revision int64, l SharedLayout) (int64, error) {
	h := m.real(ref.Host)
	if !m.reaches(h) {
		return 0, fmt.Errorf("%s does not answer", h)
	}
	store := m.world.store(h)
	key := LayoutRef{Space: ref.Space, Host: h, Key: ref.Key}
	s := store.layouts[key]
	if s.Revision != revision {
		return 0, errLayoutConflict
	}
	store.saves++
	s = StoredLayout{LayoutRef: key, Revision: s.Revision + 1, Layout: &l}
	store.layouts[key] = s
	return s.Revision, nil
}

// pass runs a pass of m a while after the last one.
func (m *machine) pass(t *testing.T) {
	t.Helper()
	*m.now = m.now.Add(5 * time.Second)
	if err := m.ls.pass(config.Config{}, m.sessions(), func(string) string { return "/src/gr/dev" }, map[string]int{}); err != nil {
		t.Fatal(err)
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

// Two machines showing one worktree converge on one state: a split and a tab opened on one open on the
// other, one closed closes, each saved once it settles and taken at the next load, and nothing saved back
// once both agree.
func TestLayoutSync(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", &now, []string{"s1", "s2"})
	b := newMachine(t, world, "desktop", &now, []string{"s1"})
	both := func(want ...[]string) {
		t.Helper()
		for _, m := range []*machine{a, b} {
			if got := m.shows(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s has %q, want %q", m.ls.writer, got, want)
			}
		}
	}

	// laptop saves once its panes settle; desktop takes them, opening s2 beside s1.
	a.pass(t)
	a.pass(t)
	b.pass(t)
	both([]string{"s1", "s2"})

	// Closing s2 on laptop closes it on desktop.
	a.k.remove("s2")
	a.pass(t)
	a.pass(t)
	b.pass(t)
	both([]string{"s1"})

	// A tab opened on desktop opens on laptop.
	b.k.tabs = append(b.k.tabs, nil)
	b.k.add(len(b.k.tabs)-1, "s3")
	// desktop took laptop's layout a pass ago, so it saves once settleAfterTake passes and its panes settle.
	b.pass(t)
	b.pass(t)
	b.pass(t)
	a.pass(t)
	both([]string{"s1"}, []string{"s3"})

	// Agreeing, neither saves again.
	saves := world.store("server").saves
	for range 3 {
		a.pass(t)
		b.pass(t)
	}
	if n := world.store("server").saves - saves; n != 0 {
		t.Errorf("%d saves once both agreed", n)
	}
	both([]string{"s1"}, []string{"s3"})
}

// A worktree this machine stops showing is forgotten, so showing it again takes the stored layout's
// sessions without closing what it opened with.
func TestLayoutSyncForgets(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", &now, []string{"s1", "s2"})
	a.pass(t)
	a.pass(t)
	b := newMachine(t, world, "desktop", &now, []string{"s1"})
	b.pass(t)
	b.k.remove("s1")
	b.k.remove("s2")
	b.pass(t)
	// Opened again with s4 alone: s1 and s2 come back in a tab of their own, and s4 stays.
	b.k.tabs = [][]string{nil}
	b.k.add(0, "s4")
	b.pass(t)
	if want := [][]string{{"s4"}, {"s1", "s2"}}; !reflect.DeepEqual(b.k.tabs, want) {
		t.Errorf("tabs %q, want %q", b.k.tabs, want)
	}
}

// Two machines changing before either sees the other keep both changes: the second to save takes the
// first's, then saves the two together.
func TestLayoutSyncBothChange(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", &now, []string{"s1"})
	b := newMachine(t, world, "desktop", &now, []string{"s1"})
	for range 2 {
		a.pass(t)
		b.pass(t)
	}
	a.k.tabs = append(a.k.tabs, nil)
	a.k.add(len(a.k.tabs)-1, "s5")
	b.k.add(0, "s6")
	// desktop's save conflicts, it takes laptop's, waits for its panes to settle, and saves; laptop takes.
	for range 6 {
		a.pass(t)
		b.pass(t)
	}
	for _, m := range []*machine{a, b} {
		var got []string
		for _, tab := range m.k.tabs {
			got = append(got, tab...)
		}
		slices.Sort(got)
		if want := []string{"s1", "s5", "s6"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s shows %q, want %q", m.ls.writer, got, want)
		}
	}
}

// A pane opened by taking another machine's layout is not saved as removed while the provider has yet to
// list its window, which would close it on the other machine.
func TestLayoutSyncWaitsForOpenedPanes(t *testing.T) {
	world := newFakeWorld()
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", &now, []string{"s1", "s2"})
	b := newMachine(t, world, "desktop", &now, []string{"s1"})
	a.pass(t)
	a.pass(t)
	b.pass(t)
	// desktop opened s2, but its provider does not list s2's window yet.
	b.k.unlisted = map[string]bool{"s2": true}
	for range 3 {
		now = now.Add(3 * time.Second)
		if err := b.ls.pass(config.Config{}, b.sessions(), func(string) string { return "" }, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := world.stored("server").Members(); got != "s1,s2" {
		t.Errorf("stored %q, want s1,s2", got)
	}
}

// Only a pane showing a session of its kitty session's space is shared: kitty's startup session, whatever
// its file is named, shows sessions in default, and a window left in another space is that space's. A
// worktree with windows showing none is there all the same, with no layouts.
func TestSharedWorktreesSpace(t *testing.T) {
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
	tabs := []kitty.FullTab{tab(1, "startup", 11), tab(2, "gr@dev", 21, 22), tab(3, "default", 31)}
	sessions := []ProviderSession{
		{ID: "@a", Windows: []int{11}},
		{ID: "@b", Space: "gr", Windows: []int{21}},
		{ID: "@c", Space: "docs", Windows: []int{22}},
		{ID: "@d", Windows: []int{31}},
	}
	got := sharedWorktrees(tabs, sessions)
	want := map[string]sharedWorktree{"gr@dev": {session: "gr@dev", space: "gr", key: "dev", hosts: map[string]sharedPanes{"": {
		layout: SharedLayout{Version: 1, Sessions: []string{"@b"}, Tabs: []SharedTab{{Layout: "stack", Panes: []SharedPane{{Session: 0}}}}},
		at:     sharedLocal{Window: map[string]int{"@b": 21}, Tab: map[string]int{"@b": 2}},
	}}}, "startup": {session: "startup", space: "startup", hosts: map[string]sharedPanes{}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shared %+v\nwant   %+v", got, want)
	}
}

// A tab showing sessions of two hosts is in each host's layout with that host's panes alone, each stored on
// its host: a split of one host's session opens beside that host's pane, and a tab of another's opens as a
// tab.
func TestLayoutSyncPerHost(t *testing.T) {
	world := newFakeWorld()
	world.hostOf = map[string]string{"w1": "white", "w2": "white", "l1": "laptop", "l2": "laptop"}
	now := time.Unix(1000, 0)
	a := newMachine(t, world, "laptop", &now, []string{"w1", "l1"})
	b := newMachine(t, world, "white", &now, []string{"w1", "l1"})
	for range 2 {
		a.pass(t)
		b.pass(t)
	}
	a.k.add(0, "w2")
	a.k.tabs = append(a.k.tabs, nil)
	a.k.add(len(a.k.tabs)-1, "l2")
	for range 3 {
		a.pass(t)
		b.pass(t)
	}
	if want := [][]string{{"w1", "l1", "w2"}, {"l2"}}; !reflect.DeepEqual(b.shows(), want) {
		t.Errorf("white shows %q, want %q", b.shows(), want)
	}
	got := map[string]string{"white": world.stored("white").Members(), "laptop": world.stored("laptop").Members()}
	if want := map[string]string{"white": "w1,w2", "laptop": "l1;l2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stored %q, want %q", got, want)
	}
}

// Opening a worktree loads the layouts of every host with sessions in it, though no pane here shows one,
// and opens what they hold beside this machine's own panes. A host whose sessions come later is left to
// the worktree's detached row.
func TestLayoutSyncLoadsOnOpen(t *testing.T) {
	world := newFakeWorld()
	world.hostOf = map[string]string{"w1": "white", "w2": "white", "w3": "white", "l1": "laptop"}
	now := time.Unix(1000, 0)
	b := newMachine(t, world, "white", &now, []string{"w1", "w2"}, []string{"w3"})
	b.pass(t)
	b.pass(t)
	a := newMachine(t, world, "laptop", &now, []string{"l1"})
	a.pass(t)
	if want := [][]string{{"l1"}, {"w1", "w2"}, {"w3"}}; !reflect.DeepEqual(a.shows(), want) {
		t.Errorf("laptop shows %q, want %q", a.shows(), want)
	}

	// black's layout, saved once laptop had the worktree open, does not open there.
	world.hostOf["k1"] = "black"
	world.store("black").layouts[LayoutRef{Space: "gr", Host: "black", Key: "dev"}] = StoredLayout{Revision: 1, Layout: new(layoutOf([]string{"k1"}))}
	for range 3 {
		a.pass(t)
	}
	if want := [][]string{{"l1"}, {"w1", "w2"}, {"w3"}}; !reflect.DeepEqual(a.shows(), want) {
		t.Errorf("laptop shows %q, want %q", a.shows(), want)
	}
}

// stackLayout is the layout a fakeShareKitty's tabs of sessions by id capture as.
func stackLayout(tabs ...[]string) SharedLayout {
	l := SharedLayout{Version: 1, Sessions: []string{}, Tabs: []SharedTab{}}
	for _, ids := range tabs {
		t := SharedTab{Layout: "stack"}
		for _, id := range ids {
			t.Panes = append(t.Panes, SharedPane{Session: len(l.Sessions)})
			l.Sessions = append(l.Sessions, id)
		}
		l.Tabs = append(l.Tabs, t)
	}
	return l
}

// savedBy is l as the kitty who saved it.
func savedBy(who string, l SharedLayout) SharedLayout {
	l.Writer, l.Kitty = who, who
	return l
}

// entry is what m's passes know of gr@dev's layout on host, as m names it.
func (m *machine) entry(t *testing.T, host string) *syncEntry {
	t.Helper()
	var e *syncEntry
	if err := m.ls.locked(func(st *syncState) {
		if sw := st.Layouts["gr@dev"]; sw != nil {
			e = sw.Hosts[host]
		}
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

// Apart, a machine's base is no longer valid: coming back, what the two last agreed on says which side
// changed. Only one side's change is that side's layout, taken or saved whole; both sides' is a divergence,
// neither saved nor taken, each machine keeping its own; and two sides that come to match agree again. A
// host is apart when its sessions are stale, when load-layouts says it did not answer, and when this kitty
// did not pass for a while, as on a laptop asleep.
func TestLayoutSyncApart(t *testing.T) {
	tests := []struct {
		name string
		// away parts laptop from white: offline lists white's sessions stale, unlisted does not, and asleep
		// runs no passes on laptop.
		away string
		// laptop and white are what each does while apart.
		laptop, white func(m *machine)
		// wantLaptop and wantWhite are what each shows once laptop is back, stored what white has stored,
		// and diverged laptop's divergence, its local and remote.
		wantLaptop, wantWhite [][]string
		stored                string
		diverged              *sharedDivergence
	}{
		{
			name:       "white closed a tab",
			away:       "offline",
			white:      func(m *machine) { m.k.remove("w3") },
			wantLaptop: [][]string{{"w1", "w2"}},
			wantWhite:  [][]string{{"w1", "w2"}},
			stored:     "w1,w2",
		},
		{
			name:       "laptop closed a tab",
			away:       "offline",
			laptop:     func(m *machine) { m.k.remove("w3") },
			wantLaptop: [][]string{{"w1", "w2"}},
			wantWhite:  [][]string{{"w1", "w2"}},
			stored:     "w1,w2",
		},
		{
			name:       "both changed",
			away:       "offline",
			laptop:     func(m *machine) { m.k.remove("w2") },
			white:      func(m *machine) { m.k.tabs = append(m.k.tabs, nil); m.k.add(len(m.k.tabs)-1, "w4") },
			wantLaptop: [][]string{{"w1"}, {"w3"}},
			wantWhite:  [][]string{{"w1", "w2"}, {"w3"}, {"w4"}},
			stored:     "w1,w2;w3;w4",
			diverged:   &sharedDivergence{Local: stackLayout([]string{"w1"}, []string{"w3"}), Remote: savedBy("white", stackLayout([]string{"w1", "w2"}, []string{"w3"}, []string{"w4"}))},
		},
		{
			name:       "both changed, white not answering load-layouts",
			away:       "unlisted",
			laptop:     func(m *machine) { m.k.remove("w2") },
			white:      func(m *machine) { m.k.tabs = append(m.k.tabs, nil); m.k.add(len(m.k.tabs)-1, "w4") },
			wantLaptop: [][]string{{"w1"}, {"w3"}},
			wantWhite:  [][]string{{"w1", "w2"}, {"w3"}, {"w4"}},
			stored:     "w1,w2;w3;w4",
			diverged:   &sharedDivergence{Local: stackLayout([]string{"w1"}, []string{"w3"}), Remote: savedBy("white", stackLayout([]string{"w1", "w2"}, []string{"w3"}, []string{"w4"}))},
		},
		{
			name: "both changed, laptop asleep",
			away: "asleep",
			// One pass sees the close, which the next would have saved.
			laptop:     func(m *machine) { m.k.remove("w2"); m.pass(t) },
			white:      func(m *machine) { m.k.tabs = append(m.k.tabs, nil); m.k.add(len(m.k.tabs)-1, "w4") },
			wantLaptop: [][]string{{"w1"}, {"w3"}},
			wantWhite:  [][]string{{"w1", "w2"}, {"w3"}, {"w4"}},
			stored:     "w1,w2;w3;w4",
			diverged:   &sharedDivergence{Local: stackLayout([]string{"w1"}, []string{"w3"}), Remote: savedBy("white", stackLayout([]string{"w1", "w2"}, []string{"w3"}, []string{"w4"}))},
		},
		{
			name:       "both made the same change",
			away:       "offline",
			laptop:     func(m *machine) { m.k.remove("w3") },
			white:      func(m *machine) { m.k.remove("w3") },
			wantLaptop: [][]string{{"w1", "w2"}},
			wantWhite:  [][]string{{"w1", "w2"}},
			stored:     "w1,w2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newFakeWorld()
			world.hostOf = map[string]string{"w1": "white", "w2": "white", "w3": "white", "w4": "white"}
			now := time.Unix(1000, 0)
			b := newMachine(t, world, "white", &now, []string{"w1", "w2"}, []string{"w3"})
			b.pass(t)
			b.pass(t)
			a := newMachine(t, world, "laptop", &now, []string{"w1", "w2"}, []string{"w3"})
			for range 2 {
				a.pass(t)
				b.pass(t)
			}
			unlisted := a.load
			switch tt.away {
			case "offline":
				world.offline["laptop"] = true
			case "unlisted":
				a.ls.load = func(p []string, known []StoredLayout) (LoadedLayouts, error) {
					return LoadedLayouts{Unreachable: []string{"white"}}, nil
				}
			}
			if tt.laptop != nil {
				tt.laptop(a)
			}
			if tt.white != nil {
				tt.white(b)
			}
			for range 3 {
				if tt.away != "asleep" {
					a.pass(t)
				}
				b.pass(t)
			}
			now = now.Add(time.Hour)
			world.offline["laptop"] = false
			a.ls.load = unlisted
			for range 4 {
				a.pass(t)
				b.pass(t)
			}
			if got := a.shows(); !reflect.DeepEqual(got, tt.wantLaptop) {
				t.Errorf("laptop shows %q, want %q", got, tt.wantLaptop)
			}
			if got := b.shows(); !reflect.DeepEqual(got, tt.wantWhite) {
				t.Errorf("white shows %q, want %q", got, tt.wantWhite)
			}
			if got := world.stored("white").Members(); got != tt.stored {
				t.Errorf("stored %q, want %q", got, tt.stored)
			}
			got := a.entry(t, "white").Diverged
			if got != nil {
				got.Since = time.Time{}
			}
			if !reflect.DeepEqual(got, tt.diverged) {
				t.Errorf("diverged %+v, want %+v", got, tt.diverged)
			}
		})
	}
}

// A divergence ends unasked once the two sides come to match: laptop opening what white has agrees with
// it, and nothing is saved.
func TestLayoutSyncDivergenceEnds(t *testing.T) {
	world := newFakeWorld()
	world.hostOf = map[string]string{"w1": "white", "w2": "white", "w3": "white"}
	now := time.Unix(1000, 0)
	b := newMachine(t, world, "white", &now, []string{"w1", "w2"})
	b.pass(t)
	b.pass(t)
	a := newMachine(t, world, "laptop", &now, []string{"w1", "w2"})
	for range 2 {
		a.pass(t)
		b.pass(t)
	}
	world.offline["laptop"] = true
	a.k.remove("w2")
	b.k.add(0, "w3")
	for range 3 {
		a.pass(t)
		b.pass(t)
	}
	world.offline["laptop"] = false
	for range 2 {
		a.pass(t)
	}
	if a.entry(t, "white").Diverged == nil {
		t.Fatal("laptop has not diverged")
	}
	saves := world.store("white").saves
	a.k.add(0, "w2")
	a.k.add(0, "w3")
	for range 3 {
		a.pass(t)
	}
	if d := a.entry(t, "white").Diverged; d != nil {
		t.Errorf("diverged %+v once both match", d)
	}
	if n := world.store("white").saves - saves; n != 0 {
		t.Errorf("%d saves once both match", n)
	}
}

// A host's layout first seen once the worktree has been open a while has no agreement to go by: the same
// as its panes here is agreed, and else it is a divergence, rather than opening every tab the host has or
// saving one pane over them.
func TestLayoutSyncHostSeenLater(t *testing.T) {
	world := newFakeWorld()
	world.hostOf = map[string]string{"l1": "laptop"}
	now := time.Unix(1000, 0)
	world.store("white").layouts[LayoutRef{Space: "gr", Host: "white", Key: "dev"}] = StoredLayout{Revision: 3, Layout: new(stackLayout([]string{"w1", "w2"}, []string{"w3"}))}
	a := newMachine(t, world, "laptop", &now, []string{"l1"})
	for range 3 {
		a.pass(t)
	}
	// Attached from the worktree's detached row.
	world.hostOf["w1"] = "white"
	a.k.tabs = append(a.k.tabs, nil)
	a.k.add(len(a.k.tabs)-1, "w1")
	for range 3 {
		a.pass(t)
	}
	if want := [][]string{{"l1"}, {"w1"}}; !reflect.DeepEqual(a.shows(), want) {
		t.Errorf("laptop shows %q, want %q", a.shows(), want)
	}
	got := a.entry(t, "white").Diverged
	if got != nil {
		got.Since = time.Time{}
	}
	want := &sharedDivergence{Local: stackLayout([]string{"w1"}), Remote: stackLayout([]string{"w1", "w2"}, []string{"w3"})}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diverged %+v, want %+v", got, want)
	}
	if got := world.stored("white").Members(); got != "w1,w2;w3" {
		t.Errorf("stored %q, want w1,w2;w3", got)
	}
}
