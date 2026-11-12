package tui

import "github.com/chancez/kspaces/internal/spaces"

// The spaces list's order: the sort mode every sidebar shares, and moving rows by hand.

// rowOrder is the order the rows are built in.
func (m *sidebar) rowOrder() spaces.Order {
	return m.order
}

// sortMode is the sort mode the rows are in, the zero mode being manual.
func (m *sidebar) sortMode() spaces.SortMode {
	if m.order.Mode == "" {
		return spaces.SortManual
	}
	return m.order.Mode
}

// setSort orders the rows by mode, in every sidebar. The cursor stays on its row, wherever that goes.
func (m *sidebar) setSort(mode spaces.SortMode) {
	m.order.Mode = mode
	if err := spaces.SaveSort(mode); err != nil {
		m.err = err
	}
	m.setRows()
	m.scrollToCursor()
}

// openSortMenu opens the sort button's menu, drawn from screen row y, with the mode chosen now marked and
// selected.
func (m *sidebar) openSortMenu(y int) {
	m.openMenu(theSort.Target(sortSubject{}), y)
	if m.menu == nil {
		return
	}
	for i, it := range m.menu.items {
		if it.Is(sortActions[m.sortMode()]) {
			m.menu.chosen, m.menu.sel = i, i
		}
	}
}
