package spaces

import (
	"maps"
	"slices"

	"github.com/chancez/kspaces/internal/state"
)

// GroupKey is how the folded spaces name the group called name. A root is a path, absolute once ~/ is
// expanded, as kitty opens sessions in it from anywhere, so it never starts with group:.
func GroupKey(name string) string {
	return "group:" + name
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
