package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/state"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The spaces list's filter: `/` opens a query on the row under the SPACES title, which is blank
// otherwise, so the list loses no row to it and the query stays put as the list narrows below it. While
// it is open the list shows only what the query matches, with folds ignored so a match inside a folded
// space shows, and keys type into it but for the filter's own; see filterKind.

// newFilter is the filter's input. The cursor does not blink: the sidebar would redraw for it in every
// tab, and a dock has no use for it.
func newFilter(th *look.Theme) textinput.Model {
	in := newInput(th, th.SearchPrompt())
	in.Placeholder = "filter"
	st := th.InputStyles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	return in
}

// openFilter starts filtering the spaces list, with the cursor on the row it was on if that is a row.
func (m *sidebar) openFilter() tea.Cmd {
	m.filtering = true
	m.filter.Reset()
	m.filter.SetWidth(m.filterWidth())
	cmd := m.filter.Focus()
	m.setRows()
	m.scrollToCursor()
	return cmd
}

// closeFilter shows the whole list again, the cursor on the row it was on, or on the folded space that
// holds it.
func (m *sidebar) closeFilter() {
	m.filtering = false
	m.filter.Blur()
	m.filter.Reset()
	m.setRows()
	m.scrollToCursor()
}

// filterWidth leaves the input a cell past its text for the cursor, as the picker does.
func (m *sidebar) filterWidth() int {
	return max(m.contentWidth()-ansi.StringWidth(m.filter.Prompt)-1, 1)
}

// filterLine is what the row under the SPACES title shows: the query while filtering, else nothing.
func (m *sidebar) filterLine() string {
	if !m.filtering {
		return ""
	}
	return m.filter.View()
}

// typeFilter runs the filter's own keys, and types every other key into the query.
func (m *sidebar) typeFilter(msg tea.KeyPressMsg) tea.Cmd {
	if cmd, ok := runOn(m, msg, theFilter.Target(filterSubject{})); ok {
		return cmd
	}
	before := m.filter.Value()
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.setRows()
		m.scrollToCursor()
	}
	return cmd
}

// openFiltered opens the cursor's entry and ends filtering, as enter does.
func (m *sidebar) openFiltered() tea.Cmd {
	t, ok := m.entryTarget(m.cursor)
	m.closeFilter()
	m.remote = false
	if !ok {
		return nil
	}
	return runFirst(m, t, actOpenWorktree, actOpenWindow)
}

// clearFilter ends filtering, as esc does. Remote navigation ends with it, as kitty.conf's mapping leaves
// its keyboard mode on esc.
func (m *sidebar) clearFilter() {
	m.closeFilter()
	m.remote = false
}

// toggleAttention narrows the spaces list to the sessions whose programs are not idle, see
// spaces.AttentionRows, or shows all of it again, the cursor on the row it was on when that shows. It
// applies along with the text filter, and is saved, as the folds are, so a new sidebar starts with it.
func (m *sidebar) toggleAttention() tea.Cmd {
	m.attending = !m.attending
	m.filtersChanged()
	return nil
}

// toggleOpen narrows the spaces list to the sessions with windows, see spaces.OpenRows, or shows all of
// it again, as toggleAttention does.
func (m *sidebar) toggleOpen() tea.Cmd {
	m.openOnly = !m.openOnly
	m.filtersChanged()
	return nil
}

// toggleAwake hides the suspended sessions from the spaces list, see spaces.AwakeRows, or shows all of it
// again, as toggleAttention does.
func (m *sidebar) toggleAwake() tea.Cmd {
	m.awakeOnly = !m.awakeOnly
	m.filtersChanged()
	return nil
}

// filtersChanged saves the filters on and shows the rows they keep.
func (m *sidebar) filtersChanged() {
	if err := spaces.SaveFilters(state.Filters{Attention: m.attending, Open: m.openOnly, Awake: m.awakeOnly}); err != nil {
		m.failed = err
	}
	m.setRows()
	m.scrollToCursor()
}

// filterSummary names the filters on, for the SPACES title: active for the attention filter, open for the
// open filter and awake for the suspended filter, empty for none. The query shows on the line below, so it
// is left out.
func (m *sidebar) filterSummary() string {
	var on []string
	if m.attending {
		on = append(on, "active")
	}
	if m.openOnly {
		on = append(on, "open")
	}
	if m.awakeOnly {
		on = append(on, "awake")
	}
	return strings.Join(on, ", ")
}

// narrowed is whether a filter hides rows, so the rows shown are not the whole order to move a row in.
func (m *sidebar) narrowed() bool {
	return m.filtering || m.attending || m.openOnly || m.awakeOnly
}

// emptyText is what the spaces list says when a filter leaves it no rows: that nothing needs me when only
// the attention filter narrows it, that nothing is open when only the open filter does, that everything
// is suspended when only the suspended filter does, else that the filters match nothing.
func (m *sidebar) emptyText() string {
	switch {
	case m.filter.Value() != "":
	case m.attending && !m.openOnly && !m.awakeOnly:
		return "Nothing needs you"
	case m.openOnly && !m.attending && !m.awakeOnly:
		return "Nothing is open"
	case m.awakeOnly && !m.attending && !m.openOnly:
		return "Everything is suspended"
	}
	return "No matches"
}

// moveFiltered moves the cursor by delta over the rows the filter shows, not into the status list, which
// the query does not narrow.
func (m *sidebar) moveFiltered(delta int) {
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if !m.rows[i].Blank() {
			m.cursor = i
			break
		}
	}
	m.scrollToCursor()
}

// cursorPos is where the cursor is as an entry rather than an index, so it can be found again in rows
// built afresh: the key of its row, or the index of its status entry, -1 for none.
type cursorPos struct {
	key   string
	entry int
}

func (m *sidebar) cursorAt() cursorPos {
	switch {
	case m.cursor >= 0 && m.cursor < len(m.rows):
		return cursorPos{key: rowKey(m.rows[m.cursor]), entry: -1}
	case m.cursor >= len(m.rows) && m.cursor < m.numEntries():
		return cursorPos{entry: m.cursor - len(m.rows)}
	}
	return cursorPos{entry: -1}
}

// setRows builds the rows afresh, keeping the cursor on its entry, see rebuildRows.
func (m *sidebar) setRows() {
	m.rebuildRows(m.cursorAt())
}

// rebuildRows builds the rows, all of them or the filters', and puts the cursor back on the entry it was
// on: the same row, or the folded space that holds it, or the same status entry. A row a filter hides
// moves the cursor to the first row shown; with none shown, to none, -1, under the text filter, which
// keeps the cursor out of the status list, else to the status list's top. Without a filter the cursor
// keeps its place, as it did before the rows changed.
func (m *sidebar) rebuildRows(at cursorPos) {
	key, entry := at.key, at.entry
	if m.showingHosts() {
		m.rows = m.hostRows()
	} else {
		m.rows = m.listRows(m.others, m.listed(), m.detached, m.detachedOpen)
	}
	m.pruneTicks()
	if entry >= 0 {
		m.cursor = min(len(m.rows)+entry, max(m.numEntries()-1, 0))
		return
	}
	if key != "" {
		for i, r := range m.rows {
			if rowKey(r) == key {
				m.cursor = i
				return
			}
		}
		for i, r := range m.rows {
			if r.Folded && rowHolds(r, key) {
				m.cursor = i
				return
			}
		}
	}
	if !m.narrowed() {
		m.cursor = min(max(m.cursor, 0), max(m.numEntries()-1, 0))
		return
	}
	for i, r := range m.rows {
		if !r.Blank() {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
	if m.filtering {
		m.cursor = -1
	}
}

// listRows are the rows of others and spaces, all of them or the filters', with the detached sessions of
// detached after their spaces' rows, those of the spaces in open under their head, while no filter
// narrows the rows.
func (m *sidebar) listRows(others, list []spaces.Space, detached map[string]spaces.Detached, open map[string]bool) []spaces.Row {
	var rows []spaces.Row
	if m.filtering {
		rows = filterRows(spaces.BuildRows(others, list, nil, m.hidden, m.rowOrder()), m.filter.Value())
	} else {
		rows = spaces.BuildRows(others, list, m.folded, m.hidden, m.rowOrder())
	}
	if m.attending {
		rows = spaces.AttentionRows(rows, m.kitty.Statuses)
	}
	if m.openOnly {
		rows = spaces.OpenRows(rows, m.kitty.Sessions)
	}
	if m.awakeOnly {
		rows = spaces.AwakeRows(rows, m.suspended)
	}
	// The provider's sessions no window shows are not what the query, the attention filter or the open filter
	// look for, and a drag moves only the rows it knows.
	if !m.filtering && !m.attending && !m.openOnly && (m.press == nil || m.press.drag == nil) {
		rows = spaces.WithDetached(rows, detached, open)
	}
	return rows
}

// rowKey names the entry a row is, the same across refreshes and filters: its session, for a space whose
// worktrees could not be listed its root, for a group's header the group's key in the folded file, and for
// a detached session's row, see detachedKey, one no session's name can be, each on the hosts tab under its
// host's. The blank row is none.
func rowKey(r spaces.Row) string {
	// The hosts tab can list a worktree under several hosts, each its own entry.
	if r.ByHost {
		if r.HostHeader {
			return "host:" + r.Host
		}
		r.ByHost = false
		return "host:" + r.Host + ":" + rowKey(r)
	}
	switch {
	case r.Detached != nil:
		return detachedKey(*r.Detached)
	case r.Worktree != nil:
		return r.Worktree.Session
	case r.Space != nil:
		return r.Space.Root
	case r.Group != nil:
		return spaces.GroupKey(r.Group.Name)
	}
	return ""
}

// filterRows keeps the rows that match query, as a case-insensitive substring: a session in no space by
// its name, a linked worktree by its directory or branch, and a space by its name or its main worktree's
// branch, or when any of its linked worktrees matches, so a match shows under its space. A substring
// rather than the picker's fuzzy match: the sidebar keeps its order rather than ranking, so a row that
// matches a scatter of letters would stay among the real matches.
func filterRows(rows []spaces.Row, query string) []spaces.Row {
	q := strings.ToLower(query)
	matches := func(texts ...string) bool {
		for _, t := range texts {
			if strings.Contains(strings.ToLower(t), q) {
				return true
			}
		}
		return false
	}
	return spaces.Narrow(rows, func(head spaces.Row) bool {
		own := []string{head.Space.Name}
		if head.Worktree != nil {
			own = append(own, head.Worktree.Branch)
		}
		return matches(own...)
	}, func(r spaces.Row) bool { return matches(r.Dir(), r.Worktree.Branch) })
}
