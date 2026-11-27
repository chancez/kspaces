package spaces

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// Each sidebar sees the shared windows from its own: the session of its tab, the window to go back to,
// and the errors as they were.
func TestSharedView(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dock := map[string]string{"kspaces": "1"}
	cfg := config.Config{SessionsDir: "/sessions", Space: map[string]config.SpaceSettings{"gr": {Group: "work"}}, Groups: []string{"work", "home"}}
	sh := Shared{
		Config: &cfg,
		Spaces: []sharedSpace{
			{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}, {Path: "/src/gr/dev", Branch: "dev", Session: "gr@dev"}}},
			{Name: "broken", Root: "/src/broken", Err: "not a git repository"},
		},
		Saved:   []string{"gr", "notes", "gr@gone"},
		Keeping: map[string]KeepMode{"gr@dev": KeepSuspending},
		Windows: []kitty.OSWindow{{Tabs: []kitty.Tab{
			{ID: 1, Windows: []kitty.Window{{ID: 10, SessionName: "gr", LastFocusedAt: 2, Cwd: "/src/gr"}, {ID: 11, SessionName: "gr", UserVars: dock}}},
			{ID: 2, Windows: []kitty.Window{{ID: 20, SessionName: "gr@dev", LastFocusedAt: 1, Cwd: "/src/gr/dev"}, {ID: 21, SessionName: "gr@dev", UserVars: dock}}},
		}}},
		KittyErr: "",
	}
	view := func(self int) Snapshot {
		snap := sh.View(self)
		// The spaces' worktrees are the shared ones, not copies.
		if &snap.Spaces[0].Worktrees[0] != &sh.Spaces[0].Worktrees[0] {
			t.Error("the view copied the worktrees")
		}
		return snap
	}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Group: "work", Worktrees: sh.Spaces[0].Worktrees, Missing: []Worktree{{Session: "gr@gone"}}},
		{Name: "broken", Root: "/src/broken", Err: errors.New("not a git repository")},
	}
	common := kitty.State{
		Sessions: map[string]int{"gr": 10, "gr@dev": 20}, Windows: map[string]int{"gr": 1, "gr@dev": 1},
		Focused: map[string]float64{"gr": 2, "gr@dev": 1},
		Screens: map[string]int{"gr": 10, "gr@dev": 20}, Own: map[int]bool{11: true, 21: true},
	}
	want := func(current string, home int, dir string) Snapshot {
		st := common
		st.Current, st.Home, st.HomeDir = current, home, dir
		return Snapshot{
			Config: &cfg, Spaces: spaces,
			Others: []Space{{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}}},
			Folded: map[string]bool{}, Hidden: map[string]bool{}, Saved: map[string]bool{"gr": true, "notes": true, "gr@gone": true}, Suspended: map[string]bool{}, Keeping: sh.Keeping, Kitty: st,
			// Nothing saved: manual, with kitty's times, and kspaces.toml's groups.
			Order: Order{Mode: SortManual, Worktrees: map[string][]string{}, Groups: []string{"work", "home"}, Focused: st.Focused},
		}
	}
	for self, w := range map[int]Snapshot{11: want("gr", 10, "/src/gr"), 21: want("gr@dev", 20, "/src/gr/dev")} {
		if got := view(self); !reflect.DeepEqual(got, w) {
			t.Errorf("from %d:\n%#v\nwant\n%#v", self, got, w)
		}
	}

	// A config that could not be read is all there is to show.
	if got, want := (Shared{Err: "kspaces.toml: unknown keys: space"}).View(11), (Snapshot{Err: errors.New("kspaces.toml: unknown keys: space")}); !reflect.DeepEqual(got, want) {
		t.Errorf("broken config: %#v, want %#v", got, want)
	}
}

// polling is a poller whose loads are load's.
func polling(load func() Shared) *Poller {
	return newPoller(func(*HostData) Shared { return load() })
}

// A poll loads this host's data, unless a subscription pushed it, and the session files beside it.
func TestPollerHostData(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{SessionsDir: dir}
	if err := os.WriteFile(filepath.Join(dir, "gr.kitty-session"), []byte("launch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := HostData{Config: &cfg, Spaces: []sharedSpace{{Name: "gr", Root: "/src/gr"}}}
	pushed := HostData{Config: &cfg, Spaces: []sharedSpace{{Name: "kitty", Root: "/src/kitty"}}}
	h := &fakeHost{data: loaded}
	p := newPoller(func(pu *HostData) Shared { return loadShared(h, pu) })
	got := []Shared{p.Poll(), p.Pushed(pushed)}
	want := []Shared{
		{Config: &cfg, Spaces: loaded.Spaces, Saved: []string{"gr"}, Keeping: map[string]KeepMode{}},
		{Config: &cfg, Spaces: pushed.Spaces, Saved: []string{"gr"}, Keeping: map[string]KeepMode{}},
	}
	if !reflect.DeepEqual(got, want) || h.loads != 1 {
		t.Errorf("got\n%#v\nwant\n%#v\nloads %d", got, want, h.loads)
	}
	// A host that cannot be reached is the error shown.
	h.err = errors.New("kspaces server: gone")
	if got, want := p.Poll(), (Shared{Err: "kspaces server: gone"}); !reflect.DeepEqual(got, want) {
		t.Errorf("unreachable: %+v, want %+v", got, want)
	}
}

// fakeHost is a Host whose loads are data, or err.
type fakeHost struct {
	data  HostData
	err   error
	loads int
}

func (h *fakeHost) Load(bool) (HostData, error) {
	h.loads++
	return h.data, h.err
}

func (h *fakeHost) NewWorktree(string, string, string) (Worktree, error)  { return Worktree{}, nil }
func (h *fakeHost) RemoveWorktree(string, RemoveOptions) error            { return nil }
func (h *fakeHost) LoadLayouts([]CopyLayout) ([]CopyLayout, error)        { return nil, nil }
func (h *fakeHost) SaveLayout(string, int64, SharedLayout) (int64, error) { return 0, nil }
func (h *fakeHost) Refresh() error                                        { return nil }
func (h *fakeHost) Subscribe(func(HostData)) (stop func())                { return func() {} }

// Windows lists kitty's windows again into the latest poll without loading, and polls when there is none.
func TestPollerWindows(t *testing.T) {
	loads, lists := 0, 0
	load := func() Shared {
		loads++
		return Shared{Config: &config.Config{}, Saved: []string{string(rune('0' + loads))}}
	}
	p := polling(load)
	p.list = func() ([]kitty.OSWindow, error) {
		lists++
		if lists == 3 {
			return nil, errors.New("kitty is gone")
		}
		return []kitty.OSWindow{{Tabs: []kitty.Tab{{ID: lists}}}}, nil
	}
	got := []Shared{p.Windows(), p.Windows(), p.Windows(), p.Poll(), p.Windows()}
	windows := func(tab int) []kitty.OSWindow { return []kitty.OSWindow{{Tabs: []kitty.Tab{{ID: tab}}}} }
	want := []Shared{
		{Config: &config.Config{}, Saved: []string{"1"}, Windows: windows(1)},
		{Config: &config.Config{}, Saved: []string{"1"}, Windows: windows(2)},
		{Config: &config.Config{}, Saved: []string{"1"}, KittyErr: "kitty is gone"},
		{Config: &config.Config{}, Saved: []string{"2"}, Windows: windows(4)},
		{Config: &config.Config{}, Saved: []string{"2"}, Windows: windows(5)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

// A poll that finds a session listed as keeping has it settled, given the provider's list, or nil before
// the provider has listed, and kitty's windows; a poll that finds none does not.
func TestPollerSettles(t *testing.T) {
	keeping := map[string]KeepMode{"gr@dev": KeepSuspending}
	polls := 0
	p := polling(func() Shared {
		polls++
		if polls == 3 {
			return Shared{Config: &config.Config{Provider: []string{"prov"}}}
		}
		return Shared{Config: &config.Config{Provider: []string{"prov"}}, Keeping: keeping}
	})
	windows := []kitty.OSWindow{{Tabs: []kitty.Tab{{ID: 1}}}}
	p.list = func() ([]kitty.OSWindow, error) { return windows, nil }
	p.capabilities = func([]string) Capabilities { return Capabilities{CapList: true} }
	listed := &ProviderState{Sessions: []ProviderSession{{ID: "a", State: "shelved"}}}
	lists := 0
	p.listSessions = func([]string, []ProviderWindow) (*ProviderState, error) {
		if lists++; lists == 1 {
			return nil, errors.New("not yet")
		}
		return listed, nil
	}
	var got []any
	p.settle = func(cfg config.Config, st *ProviderState, data []kitty.OSWindow) { got = append(got, st, data) }
	p.Poll()
	p.Poll()
	p.Poll()
	want := []any{(*ProviderState)(nil), windows, &ProviderState{Caps: Capabilities{CapList: true}, Sessions: listed.Sessions}, windows}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("settled with\n%v\nwant\n%v", got, want)
	}
}

// kitty's windows listed again while a poll waits on the provider's list keep the provider's last list,
// so the sidebar does not drop the detached sessions until the poll ends.
func TestPollerWindowsDuringPoll(t *testing.T) {
	p := polling(func() Shared {
		return Shared{Config: &config.Config{Provider: []string{"prov"}}}
	})
	p.list = func() ([]kitty.OSWindow, error) { return nil, nil }
	p.capabilities = func([]string) Capabilities { return Capabilities{CapList: true} }
	lists := 0
	var during []*ProviderState
	p.listSessions = func([]string, []ProviderWindow) (*ProviderState, error) {
		lists++
		during = append(during, p.Windows().Provider)
		return &ProviderState{Sessions: []ProviderSession{{ID: string(rune('0' + lists))}}}, nil
	}
	p.Poll()
	p.Poll()
	caps := Capabilities{CapList: true}
	want := []*ProviderState{nil, {Caps: caps, Sessions: []ProviderSession{{ID: "1"}}}}
	if !reflect.DeepEqual(during, want) {
		t.Errorf("provider during the polls\n%+v\nwant\n%+v", during, want)
	}
}

// A poll carries the diverged layouts, and windows listed after it keep them until the next.
func TestPollerDiverged(t *testing.T) {
	p := polling(func() Shared { return Shared{Config: &config.Config{}} })
	p.list = func() ([]kitty.OSWindow, error) { return nil, nil }
	diverged := []Divergence{{Session: "gr@dev", Host: "desktop", LocalTabs: 5, RemoteTabs: 2}}
	p.diverged = func() []Divergence { return diverged }
	got := []Shared{p.Poll(), p.Windows()}
	want := []Shared{{Config: &config.Config{}, Diverged: diverged}, {Config: &config.Config{}, Diverged: diverged}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}
