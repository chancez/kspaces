package spaces

import (
	"maps"
	"slices"
)

// HostView is one host's part of the hosts tab, listed as the spaces tab lists everything: the sessions
// in no space and the spaces it has, and its detached sessions by space.
type HostView struct {
	// Host is empty for this machine.
	Host     string
	Others   []Space
	Spaces   []Space
	Detached map[string]Detached
	// Err is why another host's server could not load, see RemoteView.
	Err string
}

// HostViews are the list's hosts: this machine's first, with its spaces, its sessions in no space and its
// own detached sessions, then each host the config names, in remotes, by name, with its own copies of its
// spaces, once it has loaded, and its sessions in no space and detached ones, from sessions, every one the
// provider lists. A host with no kspaces server, which the config does not name, is left out: nothing says
// what its sessions are of. The spaces are copies.
func HostViews(others, spaces []Space, sessions []ProviderSession, detached map[string]Detached, remotes map[string]RemoteView) []HostView {
	views := []HostView{{Others: others, Spaces: spaces, Detached: detachedOn(detached, "")}}
	for _, host := range slices.Sorted(maps.Keys(remotes)) {
		rv := remotes[host]
		v := HostView{Host: host, Spaces: rv.Spaces, Detached: detachedOn(detached, host), Err: rv.Err}
		listed := map[string]bool{}
		for _, s := range rv.Spaces {
			listed[s.Name] = true
		}
		// The host's sessions in no space it has, as its own spaces of them.
		var names []string
		for _, s := range sessions {
			if s.Host == host && !listed[s.SpaceName()] && !slices.Contains(names, s.SpaceName()) {
				names = append(names, s.SpaceName())
			}
		}
		slices.Sort(names)
		for _, name := range names {
			v.Others = append(v.Others, Space{Name: name, Worktrees: []Worktree{{Session: CopyName(host, name), Host: host}}})
		}
		views = append(views, v)
	}
	return views
}

// DetachedOn are the sessions of detached that run on host, "" for this machine, by space.
func DetachedOn(detached map[string]Detached, host string) map[string]Detached {
	return detachedOn(detached, host)
}

// detachedOn are the sessions of detached that run on host, by space.
func detachedOn(detached map[string]Detached, host string) map[string]Detached {
	out := map[string]Detached{}
	for space, d := range detached {
		for _, h := range d.Hosts {
			if h.Host == host {
				out[space] = Detached{Hosts: []HostSessions{h}}
			}
		}
	}
	return out
}
