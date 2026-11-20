package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// The picker's menu: tab opens it on the entry under the cursor, with what that entry offers, as the
// sidebar's menu does for the same row: opening it, and the forms its rows open, see rowAction. It is
// drawn over the list below the entry, or above when it would run off the bottom. Choosing a form opens it
// in a popup and closes the picker.

// onPick is a as the picker offers it.
func onPick(a *rowAction) *interact.Action[*picker, pickRowSubject] {
	act := &interact.Action[*picker, pickRowSubject]{
		Title:   a.title,
		Section: a.section,
		Applies: func(m *picker, s pickRowSubject) bool { return a.applies(m.snap, s.row) },
		Run: func(m *picker, s pickRowSubject) tea.Cmd {
			return m.ask(func(snap spaces.Snapshot) error { return a.ask(snap, s.row) })
		},
	}
	if a.name != nil {
		act.Name = func(m *picker, s pickRowSubject) string { return a.name(m.snap, s.row) }
	}
	return act
}

// ask runs f, which opens a form, off the UI loop, and closes the picker once it has, or shows why it could
// not. The form is for the window the picker is over rather than the picker's own, which is about to
// close: in an overlay the most recently focused window of its tab is the picker itself.
func (m *picker) ask(f func(snap spaces.Snapshot) error) tea.Cmd {
	snap := m.snap
	snap.Kitty.Home = formHome(snap.Kitty, m.self)
	m.choosing = true
	return func() tea.Msg {
		if err := f(snap); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return tea.QuitMsg{}
	}
}

// formHome is the window a form opened from the picker, the window self, is for: st's Home, unless that
// is the picker, then the window of its session the picker covers, the one focused before it.
func formHome(st kitty.State, self int) int {
	if st.Home == self && st.Screens[st.Current] != 0 {
		return st.Screens[st.Current]
	}
	return st.Home
}

// pickTarget is the entry under the cursor as the registry has it.
func (m *picker) pickTarget() (interact.Target[*picker], bool) {
	it, ok := m.chosen()
	switch {
	case !ok:
		return interact.Target[*picker]{}, false
	case it.status != nil:
		return thePickStatuses.Target(pickStatusSubject{status: *it.status}), true
	}
	return thePickRows.Target(pickRowSubject{row: it.row}), true
}

// openPickMenu opens the menu of the entry under the cursor.
func (m *picker) openPickMenu() {
	if t, ok := m.pickTarget(); ok {
		m.menu = newMenu(m, t)
	}
}

// runPickMenuItem closes the menu and runs its item i, if it still applies.
func (m *picker) runPickMenuItem(i int) tea.Cmd {
	menu := m.menu
	m.menu = nil
	if menu == nil || i < 0 || i >= len(menu.items) || !menu.items[i].Applies(m) {
		return nil
	}
	return menu.items[i].Run(m)
}

// drawPickMenu draws the open menu over lines, the frame's, width cells wide from their left edge, so over
// the list and the preview below it but not one beside it. It goes below the cursor's row, on line at,
// or above it when it would run past bottom, the first line it may not cover; with room for neither, as
// low as fits, but never over the query and its rule.
func (m *picker) drawPickMenu(lines []string, at, bottom, width int) {
	if m.menu == nil {
		return
	}
	drawn := m.menu.lines(m.th, width)
	top := at + 1
	switch {
	case top+len(drawn) <= bottom:
	case at-len(drawn) >= pickHeaderRows:
		top = at - len(drawn)
	default:
		top = max(min(top, bottom-len(drawn)), pickHeaderRows)
	}
	m.menu.top = top
	for i, l := range drawn {
		if y := top + i; y < min(bottom, len(lines)) {
			lines[y] = l + ansi.Cut(lines[y], width, ansi.StringWidth(lines[y]))
		}
	}
}
