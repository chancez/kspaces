package spaces

import (
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

// sessions are the provider's sessions as k's windows show them.
func (k *fakeShareKitty) sessions() []ProviderSession {
	var out []ProviderSession
	for _, t := range k.tabs {
		for _, s := range t {
			if !k.unlisted[s] {
				out = append(out, ProviderSession{ID: s, Name: s, Space: "gr", Host: "server", State: "running", Windows: []int{k.ids[s]}})
			}
		}
	}
	return out
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

// fakeStore is the provider's stored layouts, by ref.
type fakeStore struct {
	layouts map[LayoutRef]StoredLayout
	saves   int
}

func (f *fakeStore) load(_ []string, known []StoredLayout) ([]StoredLayout, error) {
	var out []StoredLayout
	for _, k := range known {
		s := f.layouts[k.LayoutRef]
		s.LayoutRef = k.LayoutRef
		if s.Revision != k.Revision {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) save(_ []string, ref LayoutRef, revision int64, l SharedLayout) (int64, error) {
	s := f.layouts[ref]
	if s.Revision != revision {
		return 0, errLayoutConflict
	}
	f.saves++
	s = StoredLayout{LayoutRef: ref, Revision: s.Revision + 1, Layout: &l}
	f.layouts[ref] = s
	return s.Revision, nil
}

// machine is a kitty sharing gr@dev's layout through store.
type machine struct {
	k   *fakeShareKitty
	ls  *layoutSync
	now *time.Time
}

func newMachine(t *testing.T, name string, store *fakeStore, now *time.Time, tabs ...[]string) *machine {
	k := newFakeShareKitty("gr@dev", tabs...)
	ls := &layoutSync{
		path:   filepath.Join(t.TempDir(), "layouts.json"),
		kitty:  name,
		writer: name,
		now:    func() time.Time { return *now },
		tabs:   k.fullTabs,
		load:   store.load,
		save:   store.save,
		open: func(_ []string, s ProviderSession) ([]string, string, error) {
			return []string{"attach", s.ID}, "", nil
		},
		k: k,
	}
	return &machine{k: k, ls: ls, now: now}
}

// pass runs a pass of m a while after the last one.
func (m *machine) pass(t *testing.T) {
	t.Helper()
	*m.now = m.now.Add(5 * time.Second)
	if err := m.ls.pass(config.Config{}, m.k.sessions(), func(string) string { return "/src/gr/dev" }, map[string]int{}); err != nil {
		t.Fatal(err)
	}
}

// Two machines showing one worktree converge on one state: a split and a tab opened on one open on the
// other, one closed closes, each saved once it settles and taken at the next load, and nothing saved back
// once both agree.
func TestLayoutSync(t *testing.T) {
	store := &fakeStore{layouts: map[LayoutRef]StoredLayout{}}
	now := time.Unix(1000, 0)
	a := newMachine(t, "laptop", store, &now, []string{"s1", "s2"})
	b := newMachine(t, "desktop", store, &now, []string{"s1"})
	both := func(want ...[]string) {
		t.Helper()
		for _, m := range []*machine{a, b} {
			var got [][]string
			for _, tab := range m.k.tabs {
				if len(tab) > 0 {
					got = append(got, tab)
				}
			}
			if !reflect.DeepEqual(got, want) {
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
	saves := store.saves
	for range 3 {
		a.pass(t)
		b.pass(t)
	}
	if store.saves != saves {
		t.Errorf("%d saves once both agreed", store.saves-saves)
	}
	both([]string{"s1"}, []string{"s3"})
}

// A worktree this machine stops showing is forgotten, so showing it again takes the stored layout's
// sessions without closing what it opened with.
func TestLayoutSyncForgets(t *testing.T) {
	store := &fakeStore{layouts: map[LayoutRef]StoredLayout{}}
	now := time.Unix(1000, 0)
	a := newMachine(t, "laptop", store, &now, []string{"s1", "s2"})
	a.pass(t)
	a.pass(t)
	b := newMachine(t, "desktop", store, &now, []string{"s1"})
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
	store := &fakeStore{layouts: map[LayoutRef]StoredLayout{}}
	now := time.Unix(1000, 0)
	a := newMachine(t, "laptop", store, &now, []string{"s1"})
	b := newMachine(t, "desktop", store, &now, []string{"s1"})
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
	store := &fakeStore{layouts: map[LayoutRef]StoredLayout{}}
	now := time.Unix(1000, 0)
	a := newMachine(t, "laptop", store, &now, []string{"s1", "s2"})
	b := newMachine(t, "desktop", store, &now, []string{"s1"})
	a.pass(t)
	a.pass(t)
	b.pass(t)
	// desktop opened s2, but its provider does not list s2's window yet.
	b.k.unlisted = map[string]bool{"s2": true}
	for range 3 {
		now = now.Add(3 * time.Second)
		if err := b.ls.pass(config.Config{}, b.k.sessions(), func(string) string { return "" }, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := store.layouts[LayoutRef{Space: "gr", Host: "server", Key: "dev"}].Layout.Members(); got != "s1,s2" {
		t.Errorf("stored %q, want s1,s2", got)
	}
}

// Only a pane showing a session of its kitty session's space is shared: kitty's startup session, whatever
// its file is named, shows sessions in default, and a window left in another space is that space's.
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
	if len(got) != 1 || got["gr@dev"].layout.Members() != "@b" {
		t.Errorf("shared %+v, want gr@dev showing @b alone", got)
	}
}
