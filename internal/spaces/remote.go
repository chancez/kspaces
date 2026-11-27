package spaces

import (
	"cmp"
	"errors"
	"io"
	"maps"
	"reflect"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/logs"
)

// Other hosts are the machines the config's [hosts] names, whose copies of their spaces the hosts tab
// lists from their own servers' host data, see docs/server.md.

var (
	dialMu sync.Mutex
	// dialHost reaches another host's server, nil for no other host.
	dialHost func(name string, h config.HostSettings) Host
)

// UseRemotes has the sidebar reach the other hosts with dial, as the command line does with ssh.
func UseRemotes(dial func(name string, h config.HostSettings) Host) {
	dialMu.Lock()
	defer dialMu.Unlock()
	dialHost = dial
}

func remoteDialer() func(name string, h config.HostSettings) Host {
	dialMu.Lock()
	defer dialMu.Unlock()
	return dialHost
}

// RemoteHost is what is known of another host: its host data as it last loaded, nil before it has, and
// why it could not load since, empty when it could.
type RemoteHost struct {
	Data *HostData `json:"data,omitempty"`
	Err  string    `json:"err,omitempty"`
}

// How often another host is asked to load, and the longest wait between tries at one that fails, each
// failure in a row doubling it from remoteEvery.
const (
	remoteEvery = 2 * time.Second
	remoteRetry = time.Minute
)

// remotes loads the other hosts in the background, each on its own, so one slow or gone never holds up a
// poll.
type remotes struct {
	dial func(name string, h config.HostSettings) Host
	// changed is called when what is known of a host changes, nil for nothing to call.
	changed func()

	mu    sync.Mutex
	hosts map[string]*remote
}

// remote is one other host, loading until stop is closed.
type remote struct {
	settings config.HostSettings
	host     Host
	stop     chan struct{}
	known    RemoteHost
}

func newRemotes(dial func(name string, h config.HostSettings) Host) *remotes {
	return &remotes{dial: dial, hosts: map[string]*remote{}}
}

// update loads the hosts the config names from now on, and no other, and is what is known of each.
func (r *remotes) update(hosts map[string]config.HostSettings) map[string]RemoteHost {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, h := range r.hosts {
		if s, ok := hosts[name]; !ok || s != h.settings {
			r.drop(name, h)
		}
	}
	for name, s := range hosts {
		if _, ok := r.hosts[name]; ok {
			continue
		}
		h := &remote{settings: s, host: r.dial(name, s), stop: make(chan struct{})}
		r.hosts[name] = h
		go r.load(name, h)
	}
	return r.knownLocked()
}

// known is what is known of each host now.
func (r *remotes) known() map[string]RemoteHost {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.knownLocked()
}

func (r *remotes) knownLocked() map[string]RemoteHost {
	if len(r.hosts) == 0 {
		return nil
	}
	out := make(map[string]RemoteHost, len(r.hosts))
	for name, h := range r.hosts {
		out[name] = h.known
	}
	return out
}

// drop stops loading host. r.mu is held.
func (r *remotes) drop(name string, h *remote) {
	close(h.stop)
	if c, ok := h.host.(io.Closer); ok {
		c.Close()
	}
	delete(r.hosts, name)
}

// stopAll stops loading every host.
func (r *remotes) stopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, h := range r.hosts {
		r.drop(name, h)
	}
}

// load asks h's server to load every remoteEvery, and after a failure waits longer each time, up to
// remoteRetry. A failure keeps the host data it had, with the error.
func (r *remotes) load(name string, h *remote) {
	wait := remoteEvery
	for {
		hd, err := h.host.Load(true)
		r.mu.Lock()
		if r.hosts[name] != h {
			r.mu.Unlock()
			return
		}
		was := h.known
		if err != nil {
			h.known.Err = err.Error()
			wait = min(wait*2, remoteRetry)
		} else {
			h.known = RemoteHost{Data: &hd}
			wait = remoteEvery
		}
		changed := !reflect.DeepEqual(was, h.known)
		r.mu.Unlock()
		if err != nil && was.Err != err.Error() {
			logs.Error("loading host "+name, err)
		}
		if changed && r.changed != nil {
			r.changed()
		}
		select {
		case <-h.stop:
			return
		case <-time.After(wait):
		}
	}
}

// RemoteView is another host as the hosts tab shows it: its spaces, which are its copies, and why it could
// not load, if it could not.
type RemoteView struct {
	Spaces []Space
	// Loaded is whether it has loaded at all, so its spaces are what it has.
	Loaded bool
	Err    string
}

// RemoteViewsOf are the other hosts' views, and git's states of their worktrees, by GitKey.
func RemoteViewsOf(hosts map[string]RemoteHost) (map[string]RemoteView, map[string]gitstate.State) {
	if len(hosts) == 0 {
		return nil, nil
	}
	views := make(map[string]RemoteView, len(hosts))
	git := map[string]gitstate.State{}
	for name, rh := range hosts {
		v := RemoteView{Err: rh.Err}
		if hd := rh.Data; hd != nil {
			v.Loaded = true
			for _, s := range hd.Spaces {
				v.Spaces = append(v.Spaces, remoteSpace(name, hd, s))
			}
			for path, st := range hd.Git {
				git[GitKey(Worktree{Host: name, Path: path})] = st
			}
		}
		views[name] = v
	}
	return views, git
}

// remoteSpace is s, a space host has, as a copy of it there: its worktrees' sessions named after the host.
func remoteSpace(host string, hd *HostData, s sharedSpace) Space {
	// Its config is the host's, its own file merged in when it has one in use, see Space.Config.
	out := Space{Name: s.Name, Root: s.Root, Config: cmp.Or(s.Config, hd.Config), Untrusted: s.Untrusted}
	if hd.Config != nil {
		out.Group = hd.Config.Space[s.Name].Group
	}
	if s.Err != "" {
		out.Err = errors.New(s.Err)
	}
	for _, wt := range s.Worktrees {
		wt.Host, wt.Session = host, CopyName(host, wt.Session)
		out.Worktrees = append(out.Worktrees, wt)
	}
	return out
}

// GitKey is what git's states of wt are kept by: its path, and another host's copy's by its host too, as
// two hosts can have a worktree at the same path.
func GitKey(wt Worktree) string {
	if wt.Host == "" {
		return wt.Path
	}
	return wt.Host + ":" + wt.Path
}

// withRemoteGit is git's states with the other hosts', by GitKey, a copy, as the poll's is shared.
func withRemoteGit(local, remote map[string]gitstate.State) map[string]gitstate.State {
	if len(remote) == 0 {
		return local
	}
	out := maps.Clone(local)
	if out == nil {
		out = map[string]gitstate.State{}
	}
	maps.Copy(out, remote)
	return out
}
