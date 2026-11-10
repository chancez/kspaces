package tui

import (
	"fmt"

	"github.com/chancez/kspaces/internal/tui/look"
)

// The sidebar's two lists scroll on their own. As in gr, a list scrolled away from its top or bottom gives
// its first or last row to an indicator of how many entries are out of view that way, and a click on one
// scrolls a page that way.

// moreZone marks the indicator of a list, "spaces" or "status", in a direction, -1 up or 1 down.
const moreZone = "more:%s:%d"

// minIndicatorRows is the fewest rows a list shows indicators in: fewer would leave no room for entries.
const minIndicatorRows = 3

// moreLine is the indicator of n entries of list out of view in direction dir.
func (m *sidebar) moreLine(list string, dir, n int) string {
	return m.th.DrawZone(fmt.Sprintf(moreZone, list, dir), look.Plain, m.contentWidth(), look.More(dir < 0, n)...)
}

// spacesMore reports whether the spaces list, scrolled to top, has an indicator in its first and last row.
func (m *sidebar) spacesMore(top int) (above, below bool) {
	h := m.layout().spaces
	if h < minIndicatorRows {
		return false, false
	}
	return top > 0, top+h < len(m.rows)
}

// scrollSpacesToCursor scrolls the spaces list so the cursor's row shows, clear of the indicators. Taking
// a row for an indicator can bring the other one, hence the passes.
func (m *sidebar) scrollSpacesToCursor() {
	h, n, pos := m.layout().spaces, len(m.rows), m.cursor
	for range 3 {
		above, below := m.spacesMore(m.offset)
		switch {
		case above && pos <= m.offset:
			// Leave the indicator a row above the cursor, or none at the very top.
			m.offset = pos - 1
		case !above && pos < m.offset:
			m.offset = pos
		case below && pos >= m.offset+h-1:
			m.offset = pos - h + 2
		case !below && pos >= m.offset+h:
			m.offset = pos - h + 1
		default:
			m.offset = min(max(m.offset, 0), max(n-h, 0))
			return
		}
		m.offset = min(max(m.offset, 0), max(n-h, 0))
	}
}

// entriesIn counts the rows from up to to that are entries, not the blank rows between groups of spaces.
func (m *sidebar) entriesIn(from, to int) int {
	n := 0
	for _, r := range m.rows[from:to] {
		if r.Space != nil {
			n++
		}
	}
	return n
}

// spacesLines draws the rows of the spaces list that are in view.
func (m *sidebar) spacesLines() []string {
	h := m.layout().spaces
	above, below := m.spacesMore(m.offset)
	var out []string
	for y := 0; y < h && m.offset+y < len(m.rows); y++ {
		i := m.offset + y
		switch {
		case above && y == 0:
			// The indicator covers this row, so it counts as out of view too.
			out = append(out, m.moreLine("spaces", -1, m.entriesIn(0, i+1)))
		case below && y == h-1:
			out = append(out, m.moreLine("spaces", 1, m.entriesIn(i, len(m.rows))))
		default:
			out = append(out, m.renderRow(i))
		}
	}
	return out
}

// statusAbove reports whether the status list has an indicator in its first row. It takes a row of its
// own, as the list scrolls by entries of more than one row.
func (m *sidebar) statusAbove() bool {
	return m.statusOffset > 0 && m.layout().status >= minIndicatorRows
}

// statusLines draws the status list from m.statusOffset in the rows it has, and says how many entries it
// shows in full. An entry cut off at the bottom counts as out of view, and shows only from its first row:
// a group title or blank row alone above the indicator would head nothing.
func (m *sidebar) statusLines() (lines []string, shown int) {
	h, all := m.layout().status, m.kitty.Statuses
	room := h
	if m.statusAbove() {
		room--
		lines = append(lines, m.moreLine("status", -1, m.statusOffset))
	}
	var body []string
	// Where each entry's own rows start and end in body, after its group title.
	var starts, ends []int
	for j := m.statusOffset; j < len(all); j++ {
		body = append(body, m.statusGroupTitle(j)...)
		starts = append(starts, len(body))
		body = append(body, m.statusEntry(j)...)
		ends = append(ends, len(body))
	}
	below := len(body) > room && h >= minIndicatorRows
	if below {
		room--
	}
	for _, end := range ends {
		if end <= room {
			shown++
		}
	}
	cut := min(len(body), room)
	if shown < len(starts) && starts[shown] >= cut {
		cut = 0
		if shown > 0 {
			cut = ends[shown-1]
		}
	}
	lines = append(lines, body[:cut]...)
	for below && len(lines) < h-1 {
		lines = append(lines, "")
	}
	if below {
		lines = append(lines, m.moreLine("status", 1, len(all)-m.statusOffset-shown))
	}
	return lines, shown
}

// scrollStatusToEntry scrolls the status list so entry j shows in full, as far as it can: the cursor opens
// an entry up, and one taller than the list shows from its top.
func (m *sidebar) scrollStatusToEntry(j int) {
	m.statusOffset = min(m.statusOffset, j)
	for m.statusOffset < j {
		if _, shown := m.statusLines(); j < m.statusOffset+shown {
			return
		}
		m.statusOffset++
	}
}

// moreAt is the list and direction of the indicator drawn at x, y in the last frame, if one is there.
func (m *sidebar) moreAt(x, y int) (list string, dir int, ok bool) {
	for _, list := range []string{"spaces", "status"} {
		for _, dir := range []int{-1, 1} {
			if m.inZone(fmt.Sprintf(moreZone, list, dir), x, y) {
				return list, dir, true
			}
		}
	}
	return "", 0, false
}

// scrollPage scrolls list a page in direction dir, leaving the cursor where it is, as the wheel does.
func (m *sidebar) scrollPage(list string, dir int) {
	if list == "spaces" {
		h := m.layout().spaces
		m.offset = min(max(m.offset+dir*max(h-2, 1), 0), max(len(m.rows)-h, 0))
		return
	}
	_, shown := m.statusLines()
	m.statusOffset = min(max(m.statusOffset+dir*max(shown, 1), 0), max(len(m.kitty.Statuses)-1, 0))
}
