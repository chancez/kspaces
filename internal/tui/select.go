package tui

import (
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The select mode, for acting on several rows at once: v, Select in a row's menu or the list's, or alt and
// a press on a row turn it on. While it is on every row that can be acted on has a box after its caret, a
// click on a row ticks it rather than opening it, and pressing and dragging over rows gives each the mouse
// passes what the first one changed to, rather than moving it; a plain click and drag cannot say this, as
// they open and move. Ticks are kept by session, so they stay on their rows as the rows are built again.

// startSelect turns the select mode on, with nothing ticked.
func (m *sidebar) startSelect() {
	if m.selecting {
		return
	}
	m.selecting, m.ticked = true, map[string]bool{}
	m.scrollToCursor()
}

// endSelect turns the select mode off, letting go of the ticks.
func (m *sidebar) endSelect() {
	m.selecting, m.ticked = false, nil
	m.scrollToCursor()
}

// tickSet are the sessions row i stands for in the select mode: a worktree's or a session in no space's
// own; for a space's row its main worktree's and those of the rows under it, all of its worktrees while it
// is folded; for a group's header those of its spaces' rows, all of their worktrees while it is folded. The
// rows a filter hides are left out, so nothing ticked is out of sight.
func (m *sidebar) tickSet(i int) []string {
	r := m.rows[i]
	var under func(o spaces.Row) bool
	switch {
	case r.Group != nil:
		under = func(o spaces.Row) bool { return o.Group == nil && slices.Contains(r.Group.Spaces, o.Space) }
	case r.Main && !r.Other && r.Space != nil:
		under = func(o spaces.Row) bool { return o.Space == r.Space }
	}
	var out []string
	seen := map[string]bool{}
	add := func(o spaces.Row) {
		for _, wt := range rowSessions(o) {
			if !seen[wt.Session] {
				seen[wt.Session] = true
				out = append(out, wt.Session)
			}
		}
	}
	add(r)
	if under != nil {
		for j, o := range m.rows {
			if j != i && !o.Blank() && under(o) {
				add(o)
			}
		}
	}
	return out
}

// tick is how far row i is ticked, and false for a row with nothing to tick, as a space whose worktrees
// could not be listed.
func (m *sidebar) tick(i int) (look.Tick, bool) {
	set := m.tickSet(i)
	n := 0
	for _, s := range set {
		if m.ticked[s] {
			n++
		}
	}
	switch {
	case len(set) == 0:
		return look.Unticked, false
	case n == len(set):
		return look.Ticked, true
	case n > 0:
		return look.PartlyTicked, true
	}
	return look.Unticked, true
}

// setTicks ticks the sessions row i stands for, or unticks them.
func (m *sidebar) setTicks(i int, on bool) {
	for _, s := range m.tickSet(i) {
		if on {
			m.ticked[s] = true
		} else {
			delete(m.ticked, s)
		}
	}
}

// toggleTick ticks row i, or unticks it when it is ticked all through, and says which.
func (m *sidebar) toggleTick(i int) bool {
	t, _ := m.tick(i)
	on := t != look.Ticked
	m.setTicks(i, on)
	return on
}

// tickAll ticks every row the list shows, and what the folded ones stand for.
func (m *sidebar) tickAll() {
	for i := range m.rows {
		m.setTicks(i, true)
	}
}

// tickCursor ticks or unticks the cursor's row, as a click on it does.
func (m *sidebar) tickCursor() {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		m.toggleTick(m.cursor)
	}
}

// tickRow turns the select mode on with the row of key ticked, for a row's menu, which knows its row by
// what it was rather than where.
func (m *sidebar) tickRow(key string) {
	m.startSelect()
	for i, r := range m.rows {
		if !r.Blank() && rowKey(r) == key {
			m.setTicks(i, true)
		}
	}
}

// pressTick starts painting with the press on row i: the row is ticked or unticked, and each row the mouse
// then passes gets the same, see paintTo. A row with nothing to tick starts nothing.
func (m *sidebar) pressTick(i int) {
	if _, ok := m.tick(i); ok {
		m.press.painting, m.press.paint, m.press.last = true, m.toggleTick(i), i
	}
}

// paintTo gives the rows from the one painted last to the one at screen row y what the press gave its
// own, every row between too, as the mouse can pass several between two reports. It goes no further than
// the rows in view.
func (m *sidebar) paintTo(y int) {
	p := m.press
	if len(m.rows) == 0 {
		return
	}
	// A refresh can have taken rows away since.
	p.last = min(p.last, len(m.rows)-1)
	h := m.layout().spaces
	i := min(max(m.rowAtY(y), m.offset), min(m.offset+h, len(m.rows))-1)
	for i != p.last {
		if i > p.last {
			p.last++
		} else {
			p.last--
		}
		m.setTicks(p.last, p.paint)
	}
}

// box is row i's box in the select mode, after its caret, or a blank as wide on a row with nothing to
// tick; nothing outside the mode.
func (m *sidebar) box(i int) []look.Span {
	if !m.selecting {
		return nil
	}
	if t, ok := m.tick(i); ok {
		return look.RowBox(t)
	}
	return []look.Span{look.Default(strings.Repeat(" ", m.th.Width(look.RowBox(look.Unticked)...)))}
}

// allRows are every row of the list, as though nothing were folded or filtered: the hidden groups' are
// left out, as what is ticked is what I can see.
func (m *sidebar) allRows() []spaces.Row {
	return spaces.BuildRows(m.others, m.spaces, nil, m.hidden, m.rowOrder())
}

// pruneTicks lets go of the ticks of sessions no longer listed, as one removed, or one in a group just
// hidden.
func (m *sidebar) pruneTicks() {
	if !m.selecting || len(m.ticked) == 0 {
		return
	}
	listed := map[string]bool{}
	for _, r := range m.allRows() {
		if r.Worktree != nil {
			listed[r.Worktree.Session] = true
		}
	}
	for s := range m.ticked {
		if !listed[s] {
			delete(m.ticked, s)
		}
	}
}
