package spaces

import (
	"cmp"
	"maps"
	"slices"
)

// The provider's sessions with no window, see provider.go, as the sidebar lists them: by space, then by
// host, opened in a tab of their space, and the provider's spaces that are no configured space's.

// Detached are a space's sessions no kitty window shows, by the host they run on, this machine's first,
// then by name; each host's by name, then id.
type Detached struct {
	Hosts []HostSessions
}

// HostSessions are one host's sessions of a Detached.
type HostSessions struct {
	// Host is empty for this machine.
	Host     string
	Sessions []ProviderSession
}

// Count is how many sessions d has, on every host.
func (d Detached) Count() int {
	n := 0
	for _, h := range d.Hosts {
		n += len(h.Sessions)
	}
	return n
}

// detachedBySpace are the provider's sessions no window shows, by space: those it lists with no windows.
func detachedBySpace(st *ProviderState) map[string]Detached {
	if st == nil {
		return nil
	}
	by := map[string]map[string][]ProviderSession{}
	for _, s := range st.Sessions {
		if len(s.Windows) > 0 {
			continue
		}
		space := s.SpaceName()
		if by[space] == nil {
			by[space] = map[string][]ProviderSession{}
		}
		by[space][s.Host] = append(by[space][s.Host], s)
	}
	out := map[string]Detached{}
	for space, hosts := range by {
		var d Detached
		for _, host := range slices.Sorted(maps.Keys(hosts)) {
			sessions := hosts[host]
			slices.SortFunc(sessions, func(a, b ProviderSession) int {
				return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
			})
			d.Hosts = append(d.Hosts, HostSessions{Host: host, Sessions: sessions})
		}
		out[space] = d
	}
	return out
}

// topicSpaces are the provider's spaces that are no configured space's, each a topic space: a kitty
// session of its name, in no space, see placeSessions. default is one too, kitty's default session.
func topicSpaces(st *ProviderState, spaces []Space) []string {
	if st == nil {
		return nil
	}
	var out []string
	for _, name := range st.Spaces {
		if !slices.ContainsFunc(spaces, func(s Space) bool { return s.Name == name }) {
			out = append(out, name)
		}
	}
	return out
}
