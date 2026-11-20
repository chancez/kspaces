package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// The picker's mouse: a left click on an entry switches to it, as enter does, and a right click opens its
// menu, see pickmenu.go. The wheel moves the cursor. While the menu is open the mouse lights the item under
// it, a left click on one runs it, and any other click closes the menu, a right click on an entry opening
// that entry's instead.

// inZone is whether x, y is in zone id as the last frame drew it.
func (m *picker) inZone(id string, x, y int) bool {
	for _, z := range m.zones[id] {
		if z.Contains(x, y) {
			return true
		}
	}
	return false
}

// matchAt is the index in the matches of the entry drawn at x, y.
func (m *picker) matchAt(x, y int) (int, bool) {
	for i := range m.matches {
		if m.inZone(fmt.Sprintf(entryZone, i), x, y) {
			return i, true
		}
	}
	return 0, false
}

// pickMenuItemAt is the menu item drawn at x, y.
func (m *picker) pickMenuItemAt(x, y int) (int, bool) {
	for i := range m.menu.items {
		if m.inZone(fmt.Sprintf(menuZone, i), x, y) {
			return i, true
		}
	}
	return 0, false
}

func (m *picker) mouse(msg tea.MouseMsg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMotionMsg:
		if m.menu != nil {
			if i, ok := m.pickMenuItemAt(msg.X, msg.Y); ok {
				m.menu.sel = i
			}
		}
	case tea.MouseWheelMsg:
		m.menu = nil
		switch msg.Button {
		case tea.MouseWheelUp:
			m.cursor = max(m.cursor-1, 0)
		case tea.MouseWheelDown:
			m.cursor = min(m.cursor+1, max(len(m.matches)-1, 0))
		}
	case tea.MouseClickMsg:
		if m.menu != nil {
			if i, ok := m.pickMenuItemAt(msg.X, msg.Y); ok && msg.Button == tea.MouseLeft {
				return m.runPickMenuItem(i)
			}
			m.menu = nil
			if msg.Button != tea.MouseRight {
				return nil
			}
		}
		i, ok := m.matchAt(msg.X, msg.Y)
		if !ok {
			return nil
		}
		m.cursor = i
		switch msg.Button {
		case tea.MouseLeft:
			return m.choose(spaces.OpenOptions{})
		case tea.MouseRight:
			m.openPickMenu()
		}
	}
	return nil
}
