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
