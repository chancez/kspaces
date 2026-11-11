package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// contextMenu is the menu a right click, or `m`, opens on a row of the spaces list: the actions of the
// row's entry that apply, from the registry. It is drawn over the rows below the one it was opened on, or
// above when it would run off the bottom. Its actions are bound to the entry as it was, rather than to the
// row's index, as a refresh can rebuild the rows while it is open.
type contextMenu struct {
	items []interact.Bound[*sidebar]
	sel   int
	// The screen row of the rule above the items.
	top int
}

// openMenu opens the menu of t, drawn from screen row y.
func (m *sidebar) openMenu(t interact.Target[*sidebar], y int) {
	var items []interact.Bound[*sidebar]
	for _, g := range t.Groups(m) {
		items = append(items, g.Actions...)
	}
	if len(items) == 0 {
		return
	}
	menu := &contextMenu{items: items, top: y + 1}
	if menu.top+len(items)+2 > m.height-1 {
		menu.top = max(y-len(items)-2, 0)
	}
	m.menu, m.hover = menu, 0
}

// menuAtCursor opens the menu of the spaces row under the cursor, when it is on screen.
func (m *sidebar) menuAtCursor() {
	if m.cursor >= len(m.rows) || m.cursor < m.offset {
		return
	}
	if t, ok := m.entryTarget(m.cursor); ok {
		m.openMenu(t, headerRows+m.cursor-m.offset)
	}
}

func (m *sidebar) runMenuItem(i int) tea.Cmd {
	menu := m.menu
	m.menu = nil
	if menu == nil || i < 0 || i >= len(menu.items) || !menu.items[i].Applies(m) {
		return nil
	}
	return menu.items[i].Run(m)
}

// updateMenu handles input while the menu is open. It reports false for what should reach the sidebar as
// usual, such as a right click elsewhere, which opens that row's menu instead.
func (m *sidebar) updateMenu(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.MouseMotionMsg:
		if i, ok := m.menuItemAt(msg.X, msg.Y); ok {
			m.menu.sel = i
		}
		return nil, true
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if i, ok := m.menuItemAt(msg.X, msg.Y); ok {
				return m.runMenuItem(i), true
			}
			m.menu = nil
			return nil, true
		}
		m.menu = nil
		return nil, false
	case tea.MouseWheelMsg:
		m.menu = nil
		return nil, false
	case tea.KeyPressMsg:
		// Keys the menu does not take do nothing while it is open.
		cmd, _ := runOn(m, msg, theMenu.Target(menuSubject{}))
		return cmd, true
	}
	return nil, false
}

// menuItemAt is the menu item drawn at x, y in the last frame.
func (m *sidebar) menuItemAt(x, y int) (int, bool) {
	for i := range m.menu.items {
		if m.inZone(fmt.Sprintf(menuZone, i), x, y) {
			return i, true
		}
	}
	return 0, false
}

// drawMenu draws the open menu over lines, each item marked so the mouse finds it.
func (m *sidebar) drawMenu(lines []string) {
	if m.menu == nil {
		return
	}
	th, width := m.th, m.contentWidth()
	rule := th.Render(look.Plain, look.Rule(width, true))
	drawn := []string{rule}
	for i, it := range m.menu.items {
		surface := look.Plain
		if i == m.menu.sel {
			surface = look.Cursor
		}
		label := th.Cut(width-1, look.Text(it.Title))
		drawn = append(drawn, th.DrawZone(fmt.Sprintf(menuZone, i), surface, width, append([]look.Span{look.Text(" ")}, label...)...))
	}
	drawn = append(drawn, rule)
	for i, l := range drawn {
		if m.menu.top+i < len(lines) {
			lines[m.menu.top+i] = l
		}
	}
}

// toggleFold hides or shows the linked worktrees of the space at root, in every sidebar.
func (m *sidebar) toggleFold(root string) {
	m.folded[root] = !m.folded[root]
	if err := spaces.SaveFolded(m.folded); err != nil {
		m.err = err
	}
	m.setRows()
	for i, r := range m.rows {
		if r.Main && r.Space.Root == root {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// newWorktree opens the form that creates a worktree in the space at root, in an overlay over the
// sidebar's tab: the dock is too narrow to type a name in and read back where it goes.
func (m *sidebar) newWorktree(root string) tea.Cmd {
	st := m.kitty
	return func() tea.Msg {
		if err := spaces.NewWorktree(st, root); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// removeWorktree opens the form that confirms removing the worktree at path, in an overlay as newWorktree's.
func (m *sidebar) removeWorktree(path string) tea.Cmd {
	st := m.kitty
	return func() tea.Msg {
		if err := spaces.AskRemoveWorktree(st, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// runCommand opens the form that runs a command in the worktree at path, the first command chosen, in an
// overlay as newWorktree's.
func (m *sidebar) runCommand(path string) tea.Cmd {
	st, name := m.kitty, m.cfg.Commands[0].Name
	return func() tea.Msg {
		if err := spaces.AskRunCommand(st, name, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// closeSession opens the form that confirms closing session, in an overlay as newWorktree's.
func (m *sidebar) closeSession(session string) tea.Cmd {
	st := m.kitty
	return func() tea.Msg {
		if err := spaces.AskCloseSession(st, session); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// addSpace opens the form that adds a space, in an overlay as newWorktree's, with the repository of the
// window beside the sidebar filled in.
func (m *sidebar) addSpace() tea.Cmd {
	st := m.kitty
	return func() tea.Msg {
		if err := spaces.AskAddSpace(st); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}
