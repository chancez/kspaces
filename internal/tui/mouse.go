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

// click acts on what was drawn where the mouse was pressed. A left press on a spaces row moves the cursor
// to it and holds it until the release, see drag.go; on its caret it folds its space, or on a group's
// header its group. A left click on a status entry opens it. On a hover popup a click is one on its entry
// or row rather than on what it covers. A left click on the add button does what a does, on the sort
// button what s does, and on the attention filter's button what f does. A right click opens a spaces
// row's menu; on the sort button, the title or the list's empty rows the list's menu, its sort modes, its
// filter and its groups; while a row is pressed it
// lets go of it instead. The last line's error, and its popup, take their own clicks, see clickError.
func (m *sidebar) click(msg tea.MouseClickMsg) tea.Cmd {
	if m.press != nil {
		m.cancelPress()
		if msg.Button != tea.MouseLeft {
			return nil
		}
	}
	if t, ok := m.errorTarget(); ok && m.onError(msg.X, msg.Y) {
		return m.clickError(t, msg)
	}
	switch msg.Button {
	case tea.MouseLeft:
		if m.inZone(addZone, msg.X, msg.Y) {
			m.unhover()
			return runFirst(m, thePane.Target(paneSubject{}), actAddSpace)
		}
		if m.inZone(sortZone, msg.X, msg.Y) {
			m.unhover()
			return runFirst(m, thePane.Target(paneSubject{}), actCycleSort)
		}
		if m.inZone(attentionZone, msg.X, msg.Y) {
			m.unhover()
			return runFirst(m, thePane.Target(paneSubject{}), actAttention)
		}
		if list, dir, ok := m.moreAt(msg.X, msg.Y); ok && !m.inZone(popupZone, msg.X, msg.Y) {
			m.scrollPage(list, dir)
			return nil
		}
		if j := m.hovered(); j >= 0 && m.inZone(popupZone, msg.X, msg.Y) {
			m.hover, m.cursor = 0, len(m.rows)+j
			t, _ := m.entryTarget(m.cursor)
			return runFirst(m, t, actOpenWindow)
		}
		i, ok := m.clickedEntry(msg.X, msg.Y)
		if !ok {
			m.unhover()
			return nil
		}
		m.cursor = i
		t, _ := m.entryTarget(i)
		if m.inZone(fmt.Sprintf(caretZone, i), msg.X, msg.Y) {
			m.unhover()
			return runFirst(m, t, actFold, actUnfold, actFoldGroup, actUnfoldGroup)
		}
		if i < len(m.rows) {
			// A row's popup stays until the release, which finds the row by it as the press did.
			m.hover = 0
			m.press = &press{row: m.rows[i], key: rowKey(m.rows[i])}
			return nil
		}
		m.unhover()
		return runFirst(m, t, actOpenWindow)
	case tea.MouseRight:
		if i, ok := m.clickedEntry(msg.X, msg.Y); ok {
			m.cursor = i
			t, _ := m.entryTarget(i)
			m.openMenu(msg.Y, t)
			return nil
		}
		if msg.Y < headerRows+m.layout().spaces && msg.X < m.contentWidth() {
			m.openSortMenu(msg.Y)
		}
	}
	return nil
}

// clickedEntry is the entry a click at x, y is on: on a hover popup the entry it shows, a spaces row or a
// status entry, rather than the one it covers, and none on the error's; else the one drawn there.
func (m *sidebar) clickedEntry(x, y int) (int, bool) {
	if m.inZone(popupZone, x, y) {
		if m.hoverError {
			return 0, false
		}
		if i := m.hoveredRow(); i >= 0 {
			return i, true
		}
		if j := m.hovered(); j >= 0 {
			return len(m.rows) + j, true
		}
	}
	return m.entryAt(x, y)
}

// unhover closes the hover popup, or stops the delay before a row's.
func (m *sidebar) unhover() {
	m.hover, m.hoverRow, m.hoverShown, m.hoverError = 0, "", false, false
}

// hoveredRow is the index of the spaces row whose popup shows, or -1. Kept by rowKey, as a refresh can
// rebuild the rows under a resting mouse.
func (m *sidebar) hoveredRow() int {
	if !m.hoverShown {
		return -1
	}
	for i, r := range m.rows {
		if r.Space != nil && rowKey(r) == m.hoverRow {
			return i
		}
	}
	return -1
}

// motion lights the button under the mouse, and shows what the mouse rests on in a popup: a status entry
// in full at once, when that says more than the entry does, the last line's error in full at once, when
// the line cuts it, and a spaces row's worktree once the mouse has rested on it rowHoverDelay. Moving
// within a popup keeps it, rather than switching to what it covers; moving anywhere else clears it.
func (m *sidebar) motion(msg tea.MouseMotionMsg) tea.Cmd {
	m.addHot, m.sortHot = m.inZone(addZone, msg.X, msg.Y), m.inZone(sortZone, msg.X, msg.Y)
	m.attentionHot = m.inZone(attentionZone, msg.X, msg.Y)
	if (m.hovered() >= 0 || m.hoveredRow() >= 0 || m.hoverError) && m.inZone(popupZone, msg.X, msg.Y) {
		m.hoverAt = time.Now()
		return nil
	}
	m.hover = 0
	if m.hoverError = m.inZone(errorZone, msg.X, msg.Y) && m.errorCut(); m.hoverError {
		m.hoverRow, m.hoverShown, m.hoverAt = "", false, time.Now()
		return nil
	}
	i, ok := m.entryAt(msg.X, msg.Y)
	if ok && i < len(m.rows) && m.rows[i].Worktree != nil {
		m.hoverAt = time.Now()
		key := rowKey(m.rows[i])
		if key == m.hoverRow {
			return nil
		}
		m.hoverRow, m.hoverShown = key, false
		m.hoverGen++
		gen := m.hoverGen
		return tea.Tick(rowHoverDelay, func(time.Time) tea.Msg { return rowHoverMsg{gen: gen} })
	}
	m.hoverRow, m.hoverShown = "", false
	if ok && i >= len(m.rows) && m.popupAdds(i-len(m.rows)) {
		m.hover, m.hoverAt = m.kitty.Statuses[i-len(m.rows)].Window, time.Now()
	}
	return nil
}
