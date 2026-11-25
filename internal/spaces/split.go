package spaces

import "github.com/chancez/kspaces/internal/state"

// LoadSplit reads the height of the sidebar's STATUS list in rows, as dragging the rule above it left it,
// 0 for the default.
func LoadSplit() int {
	return max(state.Load().Sidebar.StatusRows, 0)
}

// SaveSplit saves the STATUS list's height.
func SaveSplit(rows int) error {
	return state.Update(func(s *state.State) { s.Sidebar.StatusRows = rows })
}

// LoadFilters reads the spaces list's filters on when the sidebar last changed them.
func LoadFilters() state.Filters {
	return state.Load().Sidebar.Filters
}

// SaveFilters saves the spaces list's filters on.
func SaveFilters(f state.Filters) error {
	return state.Update(func(s *state.State) { s.Sidebar.Filters = f })
}

// LoadHostsTab reads whether the sidebar last showed the hosts tab.
func LoadHostsTab() bool {
	return state.Load().Sidebar.Hosts
}

// SaveHostsTab saves whether the sidebar shows the hosts tab.
func SaveHostsTab(on bool) error {
	return state.Update(func(s *state.State) { s.Sidebar.Hosts = on })
}
