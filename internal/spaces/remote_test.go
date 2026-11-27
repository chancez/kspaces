package spaces

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gitstate"
)

// Another host's data is seen as its copies: its spaces, their worktrees' sessions named after the host,
// in the groups its config puts them in and with its config, and git's states of them kept by host and
// path.
func TestRemoteViews(t *testing.T) {
	cfg := config.Config{Space: map[string]config.SpaceSettings{"homelab": {Group: "home"}}}
	state := gitstate.State{Status: &gitstate.Status{Changes: 3}, PR: &gh.PR{Number: 9}}
	hosts := map[string]RemoteHost{
		"white": {Data: &HostData{Config: &cfg, Spaces: []sharedSpace{
			{Name: "homelab", Root: "/home/me/homelab", Worktrees: []Worktree{
				{Path: "/home/me/homelab", Branch: "main", Session: "homelab"},
				{Path: "/home/me/homelab/.worktrees/dns", Branch: "dns", Session: "homelab@dns"},
			}},
			{Name: "broken", Root: "/home/me/broken", Err: "not a git repository"},
		}, Git: map[string]gitstate.State{"/home/me/homelab": state}}, Err: "lost it a moment ago"},
		"nas": {Err: "unreachable"},
	}
	views, git := RemoteViewsOf(hosts)
	wantViews := map[string]RemoteView{
		"white": {Loaded: true, Err: "lost it a moment ago", Spaces: []Space{
			{Name: "homelab", Root: "/home/me/homelab", Group: "home", Config: &cfg, Worktrees: []Worktree{
				{Path: "/home/me/homelab", Branch: "main", Session: "white:homelab", Host: "white"},
				{Path: "/home/me/homelab/.worktrees/dns", Branch: "dns", Session: "white:homelab@dns", Host: "white"},
			}},
			{Name: "broken", Root: "/home/me/broken", Config: &cfg, Err: errors.New("not a git repository")},
		}},
		"nas": {Err: "unreachable"},
	}
	if !reflect.DeepEqual(views, wantViews) {
		t.Errorf("views:\n%#v\nwant\n%#v", views, wantViews)
	}
	if want := map[string]gitstate.State{"white:/home/me/homelab": state}; !reflect.DeepEqual(git, want) {
		t.Errorf("git %#v, want %#v", git, want)
	}
}

// scriptedHost is a Host whose loads answer from answers in turn, the last again once they run out.
type scriptedHost struct {
	mu      sync.Mutex
	answers []error
	loads   int
	closed  bool
	// When each load was, since start.
	start time.Time
	at    []time.Duration
}

func (h *scriptedHost) Load(bool) (HostData, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.answers[min(h.loads, len(h.answers)-1)]
	h.loads++
	h.at = append(h.at, time.Since(h.start))
	return HostData{Spaces: []sharedSpace{{Name: "homelab"}}}, err
}

func (h *scriptedHost) NewWorktree(string, string, string) (Worktree, error) { return Worktree{}, nil }
func (h *scriptedHost) RemoveWorktree(string, RemoveOptions) error           { return nil }
func (h *scriptedHost) Refresh() error                                       { return nil }
func (h *scriptedHost) Subscribe(func(HostData)) (stop func())               { return func() {} }
func (h *scriptedHost) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return nil
}

// The other hosts load in the background every two seconds, and after failures less and less often; a
// failure keeps the host data loaded before. A host the config no longer names stops loading.
func TestRemotesLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		down := errors.New("connection refused")
		white := &scriptedHost{answers: []error{nil, down, down, down, nil}}
		white.start = time.Now()
		r := newRemotes(func(name string, h config.HostSettings) Host { return white })
		hosts := map[string]config.HostSettings{"white": {}}
		if got := r.update(hosts); !reflect.DeepEqual(got, map[string]RemoteHost{"white": {}}) {
			t.Errorf("before loading: %+v", got)
		}
		data := &HostData{Spaces: []sharedSpace{{Name: "homelab"}}}
		var seen []RemoteHost
		r.changed = func() { seen = append(seen, r.known()["white"]) }
		time.Sleep(31 * time.Second)
		synctest.Wait()
		// Loading, failing, and loading again.
		want := []RemoteHost{{Data: data}, {Data: data, Err: "connection refused"}, {Data: data}}
		if !reflect.DeepEqual(seen, want) {
			t.Errorf("seen\n%+v\nwant\n%+v", seen, want)
		}
		// Loaded at 0, then 2s on, then 4s, 8s and 16s after each failure.
		if want := []time.Duration{0, 2 * time.Second, 6 * time.Second, 14 * time.Second, 30 * time.Second}; !reflect.DeepEqual(white.at, want) {
			t.Errorf("loaded at %v, want %v", white.at, want)
		}
		if got := r.update(nil); got != nil || !white.closed {
			t.Errorf("after dropping it: %+v, closed %v", got, white.closed)
		}
		loads := white.loads
		time.Sleep(time.Minute)
		if white.loads != loads {
			t.Errorf("loaded %d more times after dropping it", white.loads-loads)
		}
	})
}
