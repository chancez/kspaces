package spaces

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
)

// loader is a load that says which poller ran it and how many times, as its saved sessions.
type loader struct {
	name  string
	calls int
}

func (l *loader) load() Shared {
	l.calls++
	return Shared{Config: &config.Config{SessionsDir: "/sessions"}, Saved: []string{l.name, string(rune('0' + l.calls))}}
}

// The first sidebar to poll leads: it loads and writes, and the others read what it wrote, again only
// once it has written something new. When it goes, the next to poll takes the lead.
func TestPollerLeadAndFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "kitty.json")
	a, b, c := &loader{name: "a"}, &loader{name: "b"}, &loader{name: "c"}
	pa, pb, pc := newPoller(path, a.load), newPoller(path, b.load), newPoller(path, c.load)
	saved := func(p *Poller, fresh bool) []string { return p.Poll(fresh).Saved }

	got := [][]string{
		saved(pa, false), // leads, and writes a1
		saved(pb, false), // reads a1
		saved(pc, false),
		saved(pb, false), // nothing new: a1 as read before
		saved(pa, false), // writes a2
		saved(pb, false),
		saved(pb, true),  // loads itself, b1
		saved(pb, false), // keeps its own until the leader writes again
		saved(pa, false),
		saved(pb, false),
	}
	pa.Close()
	got = append(got,
		saved(pc, false), // leads now, and writes c1
		saved(pb, false),
	)
	want := [][]string{
		{"a", "1"}, {"a", "1"}, {"a", "1"}, {"a", "1"}, {"a", "2"}, {"a", "2"}, {"b", "1"}, {"b", "1"},
		{"a", "3"}, {"a", "3"}, {"c", "1"}, {"c", "1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("polls:\n%q\nwant\n%q", got, want)
	}
	if got, want := []int{a.calls, b.calls, c.calls}, []int{3, 1, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("loads by a, b, c = %v, want %v", got, want)
	}
}

// A follower whose leader has written nothing yet waits for it, then loads itself.
func TestPollerWithoutLeaderFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kitty.json")
	a, b := &loader{name: "a"}, &loader{name: "b"}
	pa, pb := newPoller(path, a.load), newPoller(path, b.load)
	if lead, ok := pa.tryLead(); !lead || !ok {
		t.Fatalf("a did not take the lock: lead %v, ok %v", lead, ok)
	}
	pb.wait = 0
	if got, want := pb.Poll(false).Saved, []string{"b", "1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("follower without a file got %q, want %q", got, want)
	}
}

// Where there is no lock to take, every poller loads for itself.
func TestPollerWithoutLock(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a := &loader{name: "a"}
	p := newPoller(filepath.Join(file, "kitty.json"), a.load)
	got := [][]string{p.Poll(false).Saved, p.Poll(false).Saved}
	if want := [][]string{{"a", "1"}, {"a", "2"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("polls %q, want %q", got, want)
	}
}

// Each sidebar sees the shared windows from its own: the session of its tab, the window to go back to,
// and the errors as they were.
func TestSharedView(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dock := map[string]string{"kspaces": "1"}
	cfg := config.Config{SessionsDir: "/sessions"}
	sh := Shared{
		Config: &cfg,
		Spaces: []sharedSpace{
			{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}, {Path: "/src/gr/dev", Branch: "dev", Session: "gr@dev"}}},
			{Name: "broken", Root: "/src/broken", Err: "not a git repository"},
		},
		Saved: []string{"gr", "notes", "gr@gone"},
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
		{Name: "gr", Root: "/src/gr", Worktrees: sh.Spaces[0].Worktrees, Missing: []Worktree{{Session: "gr@gone"}}},
		{Name: "broken", Root: "/src/broken", Err: errors.New("not a git repository")},
	}
	common := kitty.State{
		Sessions: map[string]int{"gr": 10, "gr@dev": 20}, Windows: map[string]int{"gr": 1, "gr@dev": 1},
		Screens: map[string]int{"gr": 10, "gr@dev": 20}, Own: map[int]bool{11: true, 21: true},
	}
	want := func(current string, home int, dir string) Snapshot {
		st := common
		st.Current, st.Home, st.HomeDir = current, home, dir
		return Snapshot{
			Config: &cfg, Spaces: spaces,
			Others: []Space{{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}}},
			Folded: map[string]bool{}, Saved: map[string]bool{"gr": true, "notes": true, "gr@gone": true}, Kitty: st,
		}
	}
	for self, w := range map[int]Snapshot{11: want("gr", 10, "/src/gr"), 21: want("gr@dev", 20, "/src/gr/dev")} {
		if got := view(self); !reflect.DeepEqual(got, w) {
			t.Errorf("from %d:\n%#v\nwant\n%#v", self, got, w)
		}
	}

	// A config that could not be read is all there is to show.
	if got, want := (Shared{Err: "config.toml: unknown keys: space"}).View(11), (Snapshot{Err: errors.New("config.toml: unknown keys: space")}); !reflect.DeepEqual(got, want) {
		t.Errorf("broken config: %#v, want %#v", got, want)
	}
}

// Only the leader asks git and gh, and the others read what it found. A refresh in a follower has the
// leader ask again, and a new leader carries on from the last one's pull requests.
func TestPollerGitStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kitty.json")
	now := time.Unix(1_000_000, 0)
	var asked []string
	watcher := func() *gitstate.Watcher {
		return gitstate.WatcherOf(
			func(dir string) (gitstate.Status, error) {
				asked = append(asked, "status "+dir)
				return gitstate.Status{Changes: 2}, nil
			},
			func(root, branch string) (*gitstate.PR, error) {
				asked = append(asked, "pr "+branch)
				return &gitstate.PR{Number: 7, State: "merged"}, nil
			},
			func() time.Time { return now },
			func(f func()) { f() },
		)
	}
	// gr@gone is a missing worktree of gr, which git is not asked about.
	load := func() Shared {
		return Shared{Config: &config.Config{}, Spaces: []sharedSpace{
			{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}, {Path: "/src/gr/dev", Branch: "dev", Session: "gr@dev"}}},
			{Name: "broken", Root: "/src/broken", Err: "not a git repository"},
		}, Saved: []string{"gr@gone"}}
	}
	poller := func() *Poller {
		p := newPoller(path, load)
		p.newWatcher = watcher
		return p
	}
	pa, pb, pc := poller(), poller(), poller()
	poll := func(p *Poller, fresh bool) []string {
		asked = nil
		p.Poll(fresh)
		return asked
	}

	states := map[string]gitstate.State{
		"/src/gr":     {Status: &gitstate.Status{Changes: 2}, PR: &gitstate.PR{Number: 7, State: "merged"}, PRAt: now, PRBranch: "main"},
		"/src/gr/dev": {Status: &gitstate.Status{Changes: 2}, PR: &gitstate.PR{Number: 7, State: "merged"}, PRAt: now, PRBranch: "dev"},
	}
	got := [][]string{poll(pa, false), poll(pb, false)}
	if sh := pb.Poll(false); !reflect.DeepEqual(sh.Git, states) {
		t.Errorf("follower's states:\n%+v\nwant\n%+v", sh.Git, states)
	}
	// A follower's own load keeps the states it read.
	if sh := pb.Poll(true); !reflect.DeepEqual(sh.Git, states) {
		t.Errorf("follower's own load lost the states: %+v", sh.Git)
	}
	now = now.Add(time.Second)
	asked = nil
	pb.Refresh()
	got = append(got, asked, poll(pa, false), poll(pa, false))
	pa.Close()
	now = now.Add(time.Minute)
	got = append(got, poll(pc, false))
	want := [][]string{
		{"status /src/gr", "status /src/gr/dev", "pr main", "pr dev"},
		nil,
		nil,
		{"status /src/gr", "status /src/gr/dev", "pr main", "pr dev"},
		nil,
		// Statuses are read again, but the pull requests are not due.
		{"status /src/gr", "status /src/gr/dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("asked:\n%q\nwant\n%q", got, want)
	}
	// What the sidebars see of the leader's load has gr@gone missing, and the states none for it.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sh := pc.Poll(false)
	if got, want := sh.View(0).Spaces[0].Missing, []Worktree{{Session: "gr@gone"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("missing %#v, want %#v", got, want)
	}
	if got := slices.Sorted(maps.Keys(sh.Git)); !reflect.DeepEqual(got, []string{"/src/gr", "/src/gr/dev"}) {
		t.Errorf("states for %q", got)
	}
}
