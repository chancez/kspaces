package spaces

import (
	"maps"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/state"
)

// GroupKey is how the folded spaces name the group called name. A root is a path, absolute once ~/ is
// expanded, as kitty opens sessions in it from anywhere, so it never starts with group:.
func GroupKey(name string) string {
	return "group:" + name
}

// FoldKey is how the folded spaces name the space at root as host has it, on the hosts tab: its root for
// this machine's, as the spaces tab has it, and HOST:ROOT for another host's, so folding a space under one
// host leaves the same root under another as it is.
func FoldKey(host, root string) string {
	if host == "" {
		return root
	}
	return host + ":" + root
}

// FoldedOn is folded as host's spaces are looked up in it, by root, see FoldKey: the group keys as they
// are, as a group folds under every host.
func FoldedOn(folded map[string]bool, host string) map[string]bool {
	if host == "" {
		return folded
	}
	out := map[string]bool{}
	for key, on := range folded {
		if root, ok := strings.CutPrefix(key, host+":"); ok {
			out[root] = on
		} else if strings.HasPrefix(key, "group:") {
			out[key] = on
		}
	}
	return out
}

// LoadFolded reads the spaces whose linked worktrees are hidden in the sidebar, by root, and the groups
// whose spaces are, by GroupKey. None are folded when none are saved.
func LoadFolded() map[string]bool {
	return toSet(state.Load().Sidebar.Folded)
}

// SaveFolded saves the folded spaces and groups.
func SaveFolded(folded map[string]bool) error {
	return state.Update(func(s *state.State) { s.Sidebar.Folded = fromSet(folded) })
}

// LoadHidden reads the groups left out of the sidebar, by name. Kept with the folds, so a hidden group
// stays hidden after a restart.
func LoadHidden() map[string]bool {
	return toSet(state.Load().Sidebar.Hidden)
}

// SaveHidden saves the hidden groups.
func SaveHidden(hidden map[string]bool) error {
	return state.Update(func(s *state.State) { s.Sidebar.Hidden = fromSet(hidden) })
}

// LoadHostsFolded reads the hosts folded on the hosts tab, "" for this machine.
func LoadHostsFolded() map[string]bool {
	return toSet(state.Load().Sidebar.HostsFolded)
}

// SaveHostsFolded saves the hosts folded on the hosts tab.
func SaveHostsFolded(folded map[string]bool) error {
	return state.Update(func(s *state.State) { s.Sidebar.HostsFolded = fromSet(folded) })
}

func toSet(names []string) map[string]bool {
	ans := map[string]bool{}
	for _, n := range names {
		ans[n] = true
	}
	return ans
}

// fromSet is the names set in set, sorted, so the file reads the same whatever order the map has.
func fromSet(set map[string]bool) []string {
	var ans []string
	for _, n := range slices.Sorted(maps.Keys(set)) {
		if set[n] {
			ans = append(ans, n)
		}
	}
	return ans
}
