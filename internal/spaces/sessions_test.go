package spaces

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// The view groups the provider's sessions no window shows, those it lists with no windows, by their
// space. Its spaces that are no configured space's are sessions in no space, each a topic space, default
// among them.
func TestViewDetached(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := config.Config{}
	sess := func(id, name, space string, windows ...int) ProviderSession {
		return ProviderSession{ID: id, Name: name, Space: space, State: "running", Windows: windows}
	}
	caps := Capabilities{CapList: true, CapOpen: true}
	sh := Shared{
		Config:  &cfg,
		Spaces:  []sharedSpace{{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}},
		Windows: []kitty.OSWindow{{Tabs: []kitty.Tab{{ID: 1, Windows: []kitty.Window{{ID: 10, SessionName: "gr"}}}}}},
		Provider: &ProviderState{
			Sessions: []ProviderSession{
				sess("a1", "s1", "gr", 10), sess("b2", "s2", "gr"),
				sess("e5", "z-note", "notes"), sess("f6", "a-note", "notes"), sess("g7", "old", ""),
			},
			Spaces: []string{"default", "gr", "notes", "empty"},
			Caps:   caps,
		},
	}
	snap := sh.View(10)
	type view struct {
		Detached map[string]Detached
		Others   []Space
		Caps     Capabilities
	}
	got := view{snap.Detached, snap.Others, snap.Caps}
	want := view{
		map[string]Detached{
			"gr":      {Hosts: []HostSessions{{Sessions: []ProviderSession{sess("b2", "s2", "gr")}}}},
			"notes":   {Hosts: []HostSessions{{Sessions: []ProviderSession{sess("f6", "a-note", "notes"), sess("e5", "z-note", "notes")}}}},
			"default": {Hosts: []HostSessions{{Sessions: []ProviderSession{sess("g7", "old", "")}}}},
		},
		[]Space{
			{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
			{Name: "empty", Worktrees: []Worktree{{Session: "empty"}}},
			{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}},
		},
		caps,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
	// Without a provider there are none.
	sh.Provider = nil
	if snap := sh.View(10); snap.Detached != nil || snap.Others != nil || snap.Caps != nil {
		t.Errorf("without a provider: %#v, %#v, %#v", snap.Detached, snap.Others, snap.Caps)
	}
}

// Another host's sessions are grouped after this machine's, by host.
func TestDetachedHosts(t *testing.T) {
	st := &ProviderState{Sessions: []ProviderSession{
		{ID: "1", Name: "b", Space: "gr", Host: "server"},
		{ID: "2", Name: "a", Space: "gr", Host: "server"},
		{ID: "3", Name: "c", Space: "gr"},
		{ID: "4", Name: "d", Space: "gr", Host: "laptop"},
		{ID: "5", Name: "e", Space: "gr", Windows: []int{7}},
	}}
	got := detachedBySpace(st, nil)
	want := map[string]Detached{"gr": {Hosts: []HostSessions{
		{Sessions: []ProviderSession{{ID: "3", Name: "c", Space: "gr"}}},
		{Host: "laptop", Sessions: []ProviderSession{{ID: "4", Name: "d", Space: "gr", Host: "laptop"}}},
		{Host: "server", Sessions: []ProviderSession{{ID: "2", Name: "a", Space: "gr", Host: "server"}, {ID: "1", Name: "b", Space: "gr", Host: "server"}}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
	if n := got["gr"].Count(); n != 4 {
		t.Errorf("count %d, want 4", n)
	}
}

// The poller asks the provider's capabilities once, again at a refresh, and lists with kitty's windows,
// kspaces' own left out, only from a provider that lists. A list that fails leaves the last that did not,
// so a provider restarting blanks nothing; after three in a row it is not asked for a minute, but at a
// refresh.
func TestPollerSessions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := config.Config{Provider: []string{"prov"}}
		windows := []kitty.OSWindow{{Tabs: []kitty.Tab{{Windows: []kitty.Window{
			{ID: 1, PID: 100, Cmdline: []string{"mux", "attach", "s1"}, UserVars: map[string]string{"mux": "s1"}, Cwd: "/src"},
			{ID: 2, UserVars: map[string]string{"kspaces": "1"}},
		}}}}}
		p := newPoller(filepath.Join(t.TempDir(), "kitty.git.json"), func() Shared { return Shared{Config: &cfg} })
		p.list = func() ([]kitty.OSWindow, error) { return windows, nil }
		asked, lists, fail := 0, 0, true
		caps := Capabilities{CapList: true}
		p.capabilities = func([]string) Capabilities { asked++; return caps }
		var sent [][]ProviderWindow
		listed := ProviderState{Spaces: []string{"default"}}
		p.listSessions = func(_ []string, w []ProviderWindow) (*ProviderState, error) {
			lists++
			sent = append(sent, w)
			if fail {
				return nil, errors.New("server restarting")
			}
			return &ProviderState{Spaces: listed.Spaces}, nil
		}
		var got []*ProviderState
		poll := func() { got = append(got, p.Poll().Provider) }
		poll() // fails, with nothing listed yet
		fail = false
		poll() // lists
		fail = true
		poll() // fails: the last list stays
		poll()
		poll() // the third in a row: a minute's wait
		poll() // waits
		time.Sleep(listRetry)
		fail = false
		poll() // asked again
		fail = true
		got = append(got, p.Refresh().Provider)
		none := &ProviderState{Caps: caps}
		good := &ProviderState{Spaces: []string{"default"}, Caps: caps}
		want := []*ProviderState{none, good, good, good, good, good, good, good}
		if !reflect.DeepEqual(got, want) || asked != 2 || lists != 7 {
			t.Errorf("got %+v after %d capabilities and %d lists, want %+v after 2 and 7", got, asked, lists, want)
		}
		w := ProviderWindow{ID: 1, PID: 100, Cmdline: []string{"mux", "attach", "s1"}, Foreground: [][]string{}, UserVars: map[string]string{"mux": "s1"}, Cwd: "/src"}
		if len(sent) != 7 || !reflect.DeepEqual(sent[0], []ProviderWindow{w}) {
			t.Errorf("sent %+v, want 7 lists of %+v", sent, w)
		}
		// A provider that does not list is not asked to, and one that keeps alone shows nothing.
		lists = 0
		caps = Capabilities{CapKeep: true}
		if got := p.Refresh().Provider; !reflect.DeepEqual(got, &ProviderState{Caps: caps}) || lists != 0 {
			t.Errorf("a provider that does not list: %+v after %d lists", got, lists)
		}
	})
}

// A space's detached sessions follow its rows: a head, then, expanded, each session, under its host's row
// when they run on several. A folded space has none.
func TestWithDetached(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Session: "gr"}, {Path: "/src/gr/dev", Session: "gr@dev"}}}
	docs := Space{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Path: "/src/docs", Session: "docs"}, {Path: "/src/docs/x", Session: "docs@x"}}}
	notes := Space{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}}
	a, b, c := ProviderSession{ID: "a"}, ProviderSession{ID: "b", Host: "server"}, ProviderSession{ID: "c"}
	grD := Detached{Hosts: []HostSessions{{Sessions: []ProviderSession{a}}, {Host: "server", Sessions: []ProviderSession{b}}}}
	notesD := Detached{Hosts: []HostSessions{{Sessions: []ProviderSession{c}}}}
	detached := map[string]Detached{"gr": grD, "docs": notesD, "notes": notesD}
	rows := BuildRows([]Space{notes}, []Space{gr, docs}, map[string]bool{"/src/docs": true}, nil, Order{})
	got := WithDetached(rows, detached, map[string]bool{"gr": true})
	s := func(i int) *Space { return rows[i].Space }
	want := []Row{
		rows[0],
		{Detached: &DetachedRow{Of: s(0), All: notesD}},
		{},
		rows[2],
		rows[3],
		{Detached: &DetachedRow{Of: s(2), All: grD, Expanded: true}},
		{Detached: &DetachedRow{Of: s(2), HostRow: true}},
		{Detached: &DetachedRow{Of: s(2), Session: &a}},
		{Detached: &DetachedRow{Of: s(2), Host: "server", HostRow: true}},
		{Detached: &DetachedRow{Of: s(2), Host: "server", Session: &b}},
		{},
		rows[5],
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// A session the provider kept for a kept kitty session, its id or its name one of the handles in
// .kept-sessions, is that session's, whose row stands for it, not a detached one; once the kitty session
// is opened again and its handles go, it is listed again.
func TestDetachedLeavesOutKept(t *testing.T) {
	dir, _ := fakeProvider(t, map[string]string{
		"capabilities": `{"capabilities": ["list"]}`,
		"list": `{"sessions": [{"id": "@a1", "name": "kitty.1631", "space": "gr"}, {"id": "@b2", "name": "build", "space": "gr"},
			{"id": "@c3", "name": "other", "space": "gr"}]}`,
	}, nil)
	fakeKittenLs(t, dir, `[]`)
	sessions := t.TempDir()
	cfg := config.Config{Provider: []string{"prov"}, SessionsDir: sessions}
	if err := os.WriteFile(filepath.Join(sessions, keptFile), []byte("close\tgr@dev\tkitty.1631\nsuspend\tgr@fix\t@b2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DetachedOf(cfg, "gr")
	if want := []ProviderSession{{ID: "@c3", Name: "other", Space: "gr"}}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, %v\nwant %v", got, err, want)
	}
	if err := os.WriteFile(filepath.Join(sessions, keptFile), []byte("suspend\tgr@fix\t@b2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = DetachedOf(cfg, "gr")
	if want := []ProviderSession{{ID: "@a1", Name: "kitty.1631", Space: "gr"}, {ID: "@c3", Name: "other", Space: "gr"}}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("after opening gr@dev: got %v, %v\nwant %v", got, err, want)
	}
	// The sidebar's view leaves them out the same way, from the handles its load read.
	sh := Shared{Config: &cfg, Provider: &ProviderState{Sessions: []ProviderSession{{ID: "@a1", Name: "kitty.1631", Space: "gr"}}}, Kept: []string{"kitty.1631"}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if d := sh.View(0).Detached; len(d) != 0 {
		t.Errorf("the view lists a kept session: %v", d)
	}
}
