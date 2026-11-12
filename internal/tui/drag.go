package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// Dragging a row of the spaces list moves it, as K and J do: a space among the spaces, a linked worktree
// among its space's. A left press on a row is held until its release: let go on the row, it is a click
// and opens the row; moved onto another row, it drags. The rows show the order the drag will leave as
// it goes, the dragged row lit, and the release keeps it. esc or a right click puts them back.

// press is a left press on a row of the spaces list, until its release.
type press struct {
	row spaces.Row
	key string
	// drag is the order the rows will be left in, nil until the mouse moves off the row.
	drag *arrangement
}

// dragging reports whether r is the row being dragged.
func (m *sidebar) dragging(r spaces.Row) bool {
	return m.press != nil && m.press.drag != nil && rowKey(r) == m.press.key
}

// rowAtY is the index of the spaces row drawn at screen row y: -1 above the list, len(m.rows) below it.
func (m *sidebar) rowAtY(y int) int {
	switch {
	case y < headerRows:
		return -1
	case y >= headerRows+m.layout().spaces:
		return len(m.rows)
	}
	return min(m.offset+y-headerRows, len(m.rows))
}

// dragTo moves the row pressed on to where the mouse is, at screen row y, once it is off the row.
func (m *sidebar) dragTo(y int) {
	p := m.press
	if p == nil || m.filtering || !movable(p.row) {
		return
	}
	i := m.rowAtY(y)
	if p.drag == nil {
		if i >= 0 && i < len(m.rows) && rowKey(m.rows[i]) == p.key {
			return
		}
		a := m.shown()
		p.drag, m.hover = &a, 0
	}
	to, on, ok := m.dropAt(i)
	if from, _ := p.drag.place(p.row); !ok || from == to {
		return
	}
	next := p.drag.move(p.row, to)
	// Moving past a row taller than the dragged one would leave the mouse on it, which would move the
	// dragged one back: wait for the mouse to go further.
	if on != "" {
		o := m.rowOrder()
		o.Held = next.held()
		rows := spaces.BuildRows(m.others, m.spaces, m.folded, o)
		if i >= 0 && i < len(rows) && dragItem(rows[i], p.row) == on {
			return
		}
	}
	*p.drag = next
	m.held = next.held()
	m.setRows()
}

// dropAt is where the dragged row goes with the mouse on row i, among its own, and what the mouse is on
// that it moves past: a space's root or a worktree's session, empty for none.
func (m *sidebar) dropAt(i int) (to int, on string, ok bool) {
	p := m.press
	if p.row.Main {
		last := len(p.drag.roots) - 1
		switch {
		case i < 0:
			return 0, "", true
		case i >= len(m.rows):
			return last, "", true
		case m.rows[i].Space == nil:
			return 0, "", false
		case m.rows[i].Other:
			return 0, "", true
		}
		to, _ := p.drag.place(spaces.Row{Space: m.rows[i].Space, Main: true})
		return to, m.rows[i].Space.Root, true
	}
	root := p.row.Space.Root
	last := len(p.drag.linked[root]) - 1
	if i >= 0 && i < len(m.rows) && dragItem(m.rows[i], spaces.Row{Main: true}) == root {
		switch r := m.rows[i]; {
		case r.Main:
			return 0, "", true
		case r.Missing:
			return last, "", true
		default:
			to, _ := p.drag.place(r)
			return to, r.Worktree.Session, true
		}
	}
	for j := 0; j < len(m.rows) && j < i; j++ {
		if dragItem(m.rows[j], spaces.Row{Main: true}) == root {
			return last, "", true
		}
	}
	return 0, "", true
}

// dragItem is what r is among what dragged moves among: a space's root, or a worktree's session.
func dragItem(r, dragged spaces.Row) string {
	switch {
	case r.Space == nil || r.Other:
		return ""
	case dragged.Main:
		return r.Space.Root
	case r.Worktree != nil:
		return r.Worktree.Session
	}
	return ""
}

// release ends the press: a drag keeps the order it moved to, and a click opens its row when let go on it.
func (m *sidebar) release(x, y int) tea.Cmd {
	p := m.press
	m.press = nil
	switch {
	case p == nil:
		return nil
	case p.drag != nil:
		return m.keep(*p.drag, p.row)
	}
	i, ok := m.entryAt(x, y)
	if !ok || i >= len(m.rows) || rowKey(m.rows[i]) != p.key {
		return nil
	}
	m.cursor = i
	t, _ := m.entryTarget(i)
	return runFirst(m, t, actOpenWorktree)
}

// cancelPress lets go of the press, and puts the rows back where a drag found them.
func (m *sidebar) cancelPress() {
	dragged := m.press != nil && m.press.drag != nil
	m.press, m.held = nil, nil
	if dragged {
		m.setRows()
	}
}
