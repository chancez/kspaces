package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// contextMenu is the menu a right click, or `m`, opens on a row of the spaces list: the actions of the
// row's entry that apply, from the registry, grouped by their sections. It is drawn over the rows below the
// one it was opened on, or above when it would run off the bottom. Its actions are bound to the entry as it
// was, rather than to the row's index, as a refresh can rebuild the rows while it is open.
type contextMenu struct {
	items []interact.Bound[*sidebar]
	// rules are the rules drawn above items, by the item's index, each with the section it starts: the
	// first item's always, and one between sections. The section is empty when the menu has only one.
	rules map[int]string
	sel   int
	// The screen row of the rule above the items.
	top int
	// chosen is the item marked as chosen now, as the sort mode in its menu, -1 for none.
	chosen int
}

// height is how many rows the menu takes: its items, the rules above them and the one below.
func (c *contextMenu) height() int {
	return len(c.items) + len(c.rules) + 1
}

// openMenu opens the menu of t, drawn from screen row y.
func (m *sidebar) openMenu(t interact.Target[*sidebar], y int) {
	// Each section's items together, the sections in the order their first items come.
	var order []string
	by := map[string][]interact.Bound[*sidebar]{}
	for _, g := range t.Groups(m) {
		for _, it := range g.Actions {
			if _, ok := by[it.Section]; !ok {
				order = append(order, it.Section)
			}
			by[it.Section] = append(by[it.Section], it)
		}
	}
	if len(order) == 0 {
		return
	}
	menu := &contextMenu{rules: map[int]string{}, chosen: -1}
	for _, s := range order {
		head := s
		if len(order) == 1 {
			head = ""
		}
		menu.rules[len(menu.items)] = head
		menu.items = append(menu.items, by[s]...)
	}
	menu.top = y + 1
	if menu.top+menu.height() > m.height-1 {
		menu.top = max(y-menu.height(), 0)
	}
	m.menu = menu
	m.unhover()
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
			// A click on a section's header or a rule is a near miss of an item, so the menu stays.
			if msg.Y < m.menu.top || msg.Y >= m.menu.top+m.menu.height() || msg.X >= m.contentWidth() {
				m.menu = nil
			}
			return nil, true
		}
		m.menu = nil
		return nil, false
	case tea.MouseWheelMsg:
		m.menu = nil
		return nil, false
	case tea.BlurMsg:
		// The sidebar lost focus, as to a click in another window.
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
	var drawn []string
	for i, it := range m.menu.items {
		if head, ok := m.menu.rules[i]; ok {
			drawn = append(drawn, m.menuRule(head, width))
		}
		surface := look.Plain
		if i == m.menu.sel {
			surface = look.Cursor
		}
		// A menu of choices marks the chosen one before the labels, a space from them.
		lead := []look.Span{look.Text(" ")}
		if m.menu.chosen >= 0 {
			lead = []look.Span{look.Chosen(i == m.menu.chosen), look.Text(" ")}
		}
		label := th.Cut(max(width-th.Width(lead...), 0), look.Text(it.Title))
		drawn = append(drawn, th.DrawZone(fmt.Sprintf(menuZone, i), surface, width, append(lead, label...)...))
	}
	drawn = append(drawn, m.menuRule("", width))
	for i, l := range drawn {
		if m.menu.top+i < len(lines) {
			lines[m.menu.top+i] = l
		}
	}
}

// menuRule is a rule width cells long with head, a section's name, muted on it, as ruleWith draws it.
func (m *sidebar) menuRule(head string, width int) string {
	if head == "" {
		return m.th.Render(look.Plain, ruleWith(m.th, width)...)
	}
	return m.th.Render(look.Plain, ruleWith(m.th, width, look.Muted(head))...)
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

// newWorktree opens the form that creates a worktree in the space at root, in a popup for the sidebar's
// tab: the dock is too narrow to type a name in and read back where it goes.
func (m *sidebar) newWorktree(root string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.NewWorktree(cfg, st, root); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// removeWorktree opens the form that confirms removing the worktree at path, in a popup as newWorktree's.
func (m *sidebar) removeWorktree(path string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskRemoveWorktree(cfg, st, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// runCommand opens the form that runs a command in r's worktree, the first of its space's chosen, in a
// popup as newWorktree's.
func (m *sidebar) runCommand(r spaces.Row) tea.Cmd {
	cfg, st, path, name := m.cfg, m.kitty, r.Worktree.Path, r.Space.ConfigOf(m.cfg).Commands[0].Name
	return func() tea.Msg {
		if err := spaces.AskRunCommand(cfg, st, name, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// openLayout opens the form that opens a layout in r's worktree, in a popup as newWorktree's, with the
// worktree's default layout chosen, else the first, as its space has them.
func (m *sidebar) openLayout(r spaces.Row) tea.Cmd {
	global, st, path, cfg := m.cfg, m.kitty, r.Worktree.Path, r.Space.ConfigOf(m.cfg)
	name := cfg.LayoutFor(r.Space.Name, !r.Main)
	if name == "" {
		name = cfg.Layouts[0].Name
	}
	return func() tea.Msg {
		if err := spaces.AskOpenLayout(global, st, name, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// saveLayout opens the form that saves the session of the worktree at path as a layout, in a popup as
// newWorktree's.
func (m *sidebar) saveLayout(path string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskSaveLayout(cfg, st, path); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// rowPR is the PR of r's worktree's branch the poller found, the one its row shows, nil for none.
func (m *sidebar) rowPR(r spaces.Row) *gitstate.PR {
	if r.Worktree == nil {
		return nil
	}
	return m.git[r.Worktree.Path].PR
}

// openPR opens PR number in the browser with gh, in the worktree at dir for its repository. It runs
// off the UI, as gh can take a moment to work out the repository.
func openPR(dir string, number int) tea.Cmd {
	return func() tea.Msg {
		if err := gh.Browse(dir, number); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// closeSession opens the form that confirms closing session, in a popup as newWorktree's.
func (m *sidebar) closeSession(session string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskCloseSession(cfg, st, session); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// forgetSession opens the form that confirms forgetting session, in a popup as newWorktree's.
func (m *sidebar) forgetSession(session string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskForgetSession(cfg, st, session); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// trustConfig opens the form that shows what the own config of the space at root adds, and trusts it,
// in a popup as newWorktree's.
func (m *sidebar) trustConfig(root string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskTrust(cfg, st, root); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

// addSpace opens the form that adds a space, in a popup as newWorktree's, with the repository of the
// window beside the sidebar filled in.
func (m *sidebar) addSpace() tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskAddSpace(cfg, st); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}
