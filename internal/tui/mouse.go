package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The sidebar's mouse: what a click or a move lands on is found from the zones of the last frame, and
// runs what a key would there.

// inZone reports whether the cell at x, y was drawn as zone id in the last frame.
func (m *sidebar) inZone(id string, x, y int) bool {
	for _, z := range m.zones[id] {
		if z.Contains(x, y) {
			return true
		}
	}
	return false
}

// entryAt is the entry drawn at x, y in the last frame, by the zone it was marked with, if any.
func (m *sidebar) entryAt(x, y int) (int, bool) {
	for id, zones := range m.zones {
		n, ok := strings.CutPrefix(id, "entry:")
		if !ok {
			continue
		}
		for _, z := range zones {
			if i, err := strconv.Atoi(n); err == nil && z.Contains(x, y) {
				return i, i < m.numEntries()
			}
		}
	}
	return 0, false
}

// wheel scrolls the list the mouse is over, leaving the cursor where it is.
func (m *sidebar) wheel(msg tea.MouseWheelMsg) {
	l := m.layout()
	inStatus := l.status > 0 && msg.Y >= l.statusTop
	switch {
	case msg.Button == tea.MouseWheelUp && inStatus:
		m.statusOffset = max(m.statusOffset-1, 0)
	case msg.Button == tea.MouseWheelDown && inStatus:
		m.statusOffset = min(m.statusOffset+1, max(len(m.kitty.Statuses)-1, 0))
	case msg.Button == tea.MouseWheelUp:
		m.offset = max(m.offset-1, 0)
	case msg.Button == tea.MouseWheelDown:
		m.offset = min(m.offset+1, max(len(m.rows)-l.spaces, 0))
	}
}

// click acts on what was drawn where the mouse was pressed. A left click moves the cursor to the entry and
// opens it, or folds its space on the caret; a click on the hover popup is one on its entry rather than on
// the one it covers. A right click opens a spaces row's menu.
func (m *sidebar) click(msg tea.MouseClickMsg) tea.Cmd {
	switch msg.Button {
	case tea.MouseLeft:
		if list, dir, ok := m.moreAt(msg.X, msg.Y); ok && !m.inZone(popupZone, msg.X, msg.Y) {
			m.scrollPage(list, dir)
			return nil
		}
		if j := m.hovered(); j >= 0 && m.inZone(popupZone, msg.X, msg.Y) {
			m.hover, m.cursor = 0, len(m.rows)+j
			t, _ := m.entryTarget(m.cursor)
			return runFirst(m, t, actOpenWindow)
		}
		m.hover = 0
		i, ok := m.entryAt(msg.X, msg.Y)
		if !ok {
			return nil
		}
		m.cursor = i
		t, _ := m.entryTarget(i)
		if m.inZone(fmt.Sprintf(caretZone, i), msg.X, msg.Y) {
			return runFirst(m, t, actFold, actUnfold)
		}
		return runFirst(m, t, actOpenWorktree, actOpenWindow)
	case tea.MouseRight:
		if i, ok := m.entryAt(msg.X, msg.Y); ok && i < len(m.rows) {
			m.cursor = i
			t, _ := m.entryTarget(i)
			m.openMenu(t, msg.Y)
		}
	}
	return nil
}

// motion shows the status entry the mouse rests on in full. Moving within the popup keeps it, rather than
// switching to the entry it covers; moving anywhere but a status entry clears it.
func (m *sidebar) motion(msg tea.MouseMotionMsg) {
	if m.hovered() >= 0 && m.inZone(popupZone, msg.X, msg.Y) {
		m.hoverAt = time.Now()
		return
	}
	m.hover = 0
	if i, ok := m.entryAt(msg.X, msg.Y); ok && i >= len(m.rows) {
		m.hover, m.hoverAt = m.kitty.Statuses[i-len(m.rows)].Window, time.Now()
	}
}
