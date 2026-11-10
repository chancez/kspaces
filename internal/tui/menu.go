package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chancez/kspaces/internal/spaces"
)

type menuAction int

const (
	menuOpen menuAction = iota
	menuFold
	menuNewWorktree
)

type menuItem struct {
	label  string
	action menuAction
}

// contextMenu is the menu a right click, or `m`, opens on a row of the spaces list. It is drawn over the
// rows below the one it was opened on, or above when it would run off the bottom. It keeps what it acts on
// rather than the row's index, as a refresh can rebuild the rows while it is open.
type contextMenu struct {
	items []menuItem
	sel   int
	// The screen row of the rule above the items.
	top int
	// What the items act on: the row's worktree, if it has one, and its space's root.
	wt   *spaces.Worktree
	root string
}

// menuFor lists what can be done with row i, nil when nothing can.
func (m *sidebar) menuFor(i int) *contextMenu {
	if i < 0 || i >= len(m.rows) || m.rows[i].Space == nil {
		return nil
	}
	r := m.rows[i]
	menu := &contextMenu{root: r.Space.Root}
	if r.Worktree != nil {
		wt := *r.Worktree
		menu.wt = &wt
		menu.items = append(menu.items, menuItem{"Open", menuOpen})
	}
	if !r.Other && len(r.Space.Worktrees) > 1 {
		label := "Fold worktrees"
		if m.folded[r.Space.Root] {
			label = "Unfold worktrees"
		}
		menu.items = append(menu.items, menuItem{label, menuFold})
	}
	if !r.Other && r.Space.Err == nil {
		menu.items = append(menu.items, menuItem{"New worktree...", menuNewWorktree})
	}
	if len(menu.items) == 0 {
		return nil
	}
	return menu
}

// openMenu opens the menu of row i, drawn from screen row y.
func (m *sidebar) openMenu(i, y int) {
	menu := m.menuFor(i)
	if menu == nil {
		return
	}
	menu.top = y + 1
	if menu.top+len(menu.items)+2 > m.height-1 {
		menu.top = max(y-len(menu.items)-2, 0)
	}
	m.menu, m.hover = menu, 0
}

// menuItemAt is the item drawn at screen row y, or -1.
func (m *sidebar) menuItemAt(y int) int {
	if m.menu == nil {
		return -1
	}
	if i := y - m.menu.top - 1; i >= 0 && i < len(m.menu.items) {
		return i
	}
	return -1
}

func (m *sidebar) runMenuItem(i int) tea.Cmd {
	menu := m.menu
	m.menu = nil
	if menu == nil || i < 0 || i >= len(menu.items) {
		return nil
	}
	switch menu.items[i].action {
	case menuOpen:
		m.remote = false
		return m.openWorktreeCmd(*menu.wt)
	case menuFold:
		m.toggleFold(menu.root)
	case menuNewWorktree:
		m.remote = false
		return m.newWorktree(menu.root)
	}
	return nil
}

// updateMenu handles input while the menu is open. It reports false for what should reach the sidebar as
// usual, such as a right click elsewhere, which opens that row's menu instead.
func (m *sidebar) updateMenu(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		i := m.menuItemAt(msg.Y)
		switch {
		case msg.Action == tea.MouseActionMotion:
			if i >= 0 {
				m.menu.sel = i
			}
			return nil, true
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			if i >= 0 {
				return m.runMenuItem(i), true
			}
			m.menu = nil
			return nil, true
		case msg.Action == tea.MouseActionPress:
			m.menu = nil
			return nil, false
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.menu.sel = min(m.menu.sel+1, len(m.menu.items)-1)
		case "k", "up":
			m.menu.sel = max(m.menu.sel-1, 0)
		case "enter", "l":
			return m.runMenuItem(m.menu.sel), true
		case "esc", "q", "m":
			m.menu = nil
		case "ctrl+c":
			return tea.Quit, true
		}
		return nil, true
	}
	return nil, false
}

// drawMenu draws the open menu over lines.
func (m *sidebar) drawMenu(lines []string) {
	if m.menu == nil {
		return
	}
	width := m.contentWidth()
	rule := workingStyle.Render(strings.Repeat("\u2500", width))
	drawn := []string{rule}
	for i, it := range m.menu.items {
		text := " " + truncate(it.label, width-1)
		text += strings.Repeat(" ", max(width-cells(text), 0))
		if i == m.menu.sel {
			text = cursorStyle.Render(text)
		}
		drawn = append(drawn, text)
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
	m.rows = spaces.BuildRows(m.others, m.spaces, m.folded)
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
