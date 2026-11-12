package tui

import "github.com/chancez/kspaces/internal/spaces"

// The spaces list's order: the sort mode every sidebar shares, and moving rows by hand.

// rowOrder is the order the rows are built in.
func (m *sidebar) rowOrder() spaces.Order {
	return m.order
}
