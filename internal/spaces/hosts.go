package spaces

import (
	"maps"
	"slices"
	"strings"
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

// HostViews are the hosts tab's parts: this machine's first, which is the spaces tab's whole list but the
// sessions detached on other hosts, then each other host, by name: those the config names, in remotes, and
// those the provider lists a session on. A host in remotes that has loaded has its own copies of its
// spaces. Another host has the configured spaces and the sessions in no space it has a session in, from
// sessions, every one the provider lists. A configured space keeps its main worktree's row, which heads it,
// and of its linked worktrees those a session there is tagged with; one tagged with a directory no worktree
// here has is listed after them as that host's, see Worktree.Host. A space the host has that is neither is
// a space of its own, as a topic space is. The spaces are copies.
func HostViews(others, spaces []Space, sessions []ProviderSession, detached map[string]Detached, remotes map[string]RemoteView) []HostView {
	views := []HostView{{Others: others, Spaces: spaces, Detached: detachedOn(detached, "")}}
	// The worktrees each other host's sessions are tagged with, by space, "" for none.
	has := map[string]map[string]map[string]bool{}
	for _, s := range sessions {
		if s.Host == "" {
			continue
		}
		if has[s.Host] == nil {
			has[s.Host] = map[string]map[string]bool{}
		}
		if has[s.Host][s.SpaceName()] == nil {
			has[s.Host][s.SpaceName()] = map[string]bool{}
		}
		has[s.Host][s.SpaceName()][s.Tags["worktree"]] = true
	}
	hosts := map[string]bool{}
	for host := range has {
		hosts[host] = true
	}
	for host := range remotes {
		hosts[host] = true
	}
	for _, host := range slices.Sorted(maps.Keys(hosts)) {
		v := HostView{Host: host, Detached: detachedOn(detached, host), Err: remotes[host].Err}
		listed := map[string]bool{}
		if rv := remotes[host]; rv.Loaded {
			v.Spaces = rv.Spaces
			for _, s := range rv.Spaces {
				listed[s.Name] = true
			}
		} else {
			for _, s := range spaces {
				dirs, ok := has[host][s.Name]
				if !ok {
					continue
				}
				listed[s.Name] = true
				v.Spaces = append(v.Spaces, spaceOn(s, host, dirs))
			}
		}
		for _, o := range others {
			if _, ok := has[host][o.Name]; ok && !listed[o.Name] {
				listed[o.Name] = true
				v.Others = append(v.Others, o)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(has[host])) {
			if !listed[name] {
				v.Others = append(v.Others, Space{Name: name, Worktrees: []Worktree{{Session: name}}})
			}
		}
		views = append(views, v)
	}
	return views
}

// spaceOn is a copy of s as host has it: its main worktree, the linked ones in dirs, and the directories
// in dirs no worktree of s has, by name, as host's own.
func spaceOn(s Space, host string, dirs map[string]bool) Space {
	out := s
	out.Worktrees, out.Missing = nil, nil
	here := map[string]bool{}
	for i, wt := range s.Worktrees {
		dir, linked := strings.CutPrefix(wt.Session, s.Name+"@")
		here[dir] = linked
		if i == 0 || linked && dirs[dir] {
			out.Worktrees = append(out.Worktrees, wt)
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		if dir != "" && !here[dir] {
			out.Worktrees = append(out.Worktrees, Worktree{Session: s.Name + "@" + dir, Host: host})
		}
	}
	return out
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
