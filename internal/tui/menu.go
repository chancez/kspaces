package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// contextMenu is the menu a right click, or `m`, opens on a row of the spaces list: the actions of the
// row's entry that apply, from the registry, grouped by their sections, each with the key that runs it. It
// is drawn over the rows below the one it was opened on, or above when it would run off the bottom. Its
// actions are bound to the entry as it was, rather than to the row's index, as a refresh can rebuild the
// rows while it is open.
type contextMenu[M any] struct {
	items []interact.Bound[M]
	// keys are the keys that run items, by index, empty for an item with none.
	keys []string
	// own is the target of the menu's own keys, and of the targets of what its items act on, whose keys
	// run them from the menu when own does not take them.
	own interact.Target[M]
	of  []interact.Target[M]
	// rules are the rules drawn above items, by the item's index, each with the section it starts: the
	// first item's always, and one between sections. The section is empty when the menu has only one.
	rules map[int]string
	sel   int
	// The screen row of the rule above the items.
	top int
	// chosen are the items marked as chosen now, by index, as the sort mode in its menu; nil for a menu
	// of actions rather than choices, which has no column of marks.
	chosen map[int]bool
	// ofError is a menu of the last line's error, which acting in leaves the error there, see keepsError.
	ofError bool
}

// height is how many rows the menu takes: its items, the rules above them and the one below.
func (c *contextMenu[M]) height() int {
	return len(c.items) + len(c.rules) + 1
}

// newMenu is the menu of what applies to m of ts, each section's items together, the sections in the
// order their first items come, with own's keys; nil when nothing does.
func newMenu[M any](m M, own interact.Target[M], ts ...interact.Target[M]) *contextMenu[M] {
	var order []string
	by := map[string][]interact.Bound[M]{}
	var groups []interact.Group[M]
	for _, t := range ts {
		groups = append(groups, t.Groups(m)...)
	}
	for _, g := range groups {
		for _, it := range g.Actions {
			if _, ok := by[it.Section]; !ok {
				order = append(order, it.Section)
			}
			by[it.Section] = append(by[it.Section], it)
		}
	}
	if len(order) == 0 {
		return nil
	}
	menu := &contextMenu[M]{rules: map[int]string{}, own: own, of: ts}
	for _, s := range order {
		head := s
		if len(order) == 1 {
			head = ""
		}
		menu.rules[len(menu.items)] = head
		menu.items = append(menu.items, by[s]...)
	}
	keys := interact.Keys(m, append([]interact.Target[M]{own}, ts...)...)
	for _, it := range menu.items {
		menu.keys = append(menu.keys, keyOf(keys, it))
	}
	return menu
}

// keyOf is the first of keys that runs b, empty for none.
func keyOf[M any](keys []interact.Key[M], b interact.Bound[M]) string {
	for _, k := range keys {
		if k.Action.Same(b) {
			return k.Keys()[0]
		}
	}
	return ""
}

// lookup is what msg runs while the menu is open: one of its own keys, else an item's, item true for one
// of those, which closes the menu as choosing it does.
func (c *contextMenu[M]) lookup(m M, msg tea.KeyPressMsg) (a interact.Bound[M], item, ok bool) {
	if a, ok := interact.Lookup(m, msg, c.own); ok {
		return a, false, true
	}
	a, ok = interact.Lookup(m, msg, c.of...)
	return a, ok, ok
}

// lines draws the menu width cells wide, each item marked so the mouse finds it.
func (c *contextMenu[M]) lines(th *look.Theme, width int) []string {
	var drawn []string
	for i, it := range c.items {
		if head, ok := c.rules[i]; ok {
			drawn = append(drawn, drawMenuRule(th, head, width))
		}
		surface := look.Plain
		if i == c.sel {
			surface = look.Cursor
		}
		// A menu of choices marks the chosen ones before the labels, a space from them.
		lead := []look.Span{look.Text(" ")}
		if c.chosen != nil {
			lead = []look.Span{look.Chosen(c.chosen[i]), look.Text(" ")}
		}
		// The key goes at the right edge, a space in from it, while the label fits beside it: the label says
		// more of what the item does.
		var key []look.Span
		if k := c.keys[i]; k != "" && th.Width(lead...)+th.Width(look.Text(it.Title))+1+th.Width(look.Key(k))+1 <= width {
			key = []look.Span{look.Key(k), look.Text(" ")}
		}
		label := th.Cut(max(width-th.Width(lead...)-th.Width(key...), 0), look.Text(it.Title))
		pad := look.Text(strings.Repeat(" ", max(width-th.Width(lead...)-th.Width(label...)-th.Width(key...), 0)))
		spans := append(append(append(lead, label...), pad), key...)
		drawn = append(drawn, th.DrawZone(fmt.Sprintf(menuZone, i), surface, width, spans...))
	}
	return append(drawn, drawMenuRule(th, "", width))
}

// drawMenuRule is a rule width cells long with head, a section's name, muted on it, as ruleWith draws it.
func drawMenuRule(th *look.Theme, head string, width int) string {
	if head == "" {
		return th.Render(look.Plain, ruleWith(th, width)...)
	}
	return th.Render(look.Plain, ruleWith(th, width, look.Muted(head))...)
}

// openMenu opens the menu of ts, drawn from screen row y.
func (m *sidebar) openMenu(y int, ts ...interact.Target[*sidebar]) {
	menu := newMenu(m, theMenu.Target(menuSubject{}), ts...)
	if menu == nil {
		return
	}
	menu.top = y + 1
	if menu.top+menu.height() > m.height-1 {
		menu.top = max(y-menu.height(), 0)
	}
	m.menu = menu
	m.unhover()
}

// menuAtCursor opens the menu of the entry under the cursor, when it is on screen: a spaces row, or a
// status entry under its first line as the last frame drew it.
func (m *sidebar) menuAtCursor() {
	ts := m.menuTargets(m.cursor)
	switch {
	case len(ts) == 0:
	case m.cursor < len(m.rows):
		if m.cursor >= m.offset {
			m.openMenu(headerRows+m.cursor-m.offset, ts...)
		}
	default:
		if zones := m.zones[fmt.Sprintf(entryZone, m.cursor)]; len(zones) > 0 {
			m.openMenu(zones[0].Y, ts...)
		}
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
		// Keys neither the menu nor its items take do nothing while it is open.
		a, item, ok := m.menu.lookup(m, msg)
		if item {
			m.menu = nil
		}
		if !ok {
			return nil, true
		}
		return a.Run(m), true
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
	for i, l := range m.menu.lines(m.th, m.contentWidth()) {
		if m.menu.top+i < len(lines) {
			lines[m.menu.top+i] = l
		}
	}
}

// toggleFold hides or shows the linked worktrees of the space at root, in every sidebar.
func (m *sidebar) toggleFold(root string) {
	m.folded[root] = !m.folded[root]
	if err := spaces.SaveFolded(m.folded); err != nil {
		m.failed = err
	}
	m.setRows()
	for i, r := range m.rows {
		if r.Main && r.Space.Root == root {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// toggleGroupFold hides or shows the spaces of the group called name, in every sidebar, as toggleFold
// does a space's worktrees.
func (m *sidebar) toggleGroupFold(name string) {
	key := spaces.GroupKey(name)
	m.folded[key] = !m.folded[key]
	if err := spaces.SaveFolded(m.folded); err != nil {
		m.failed = err
	}
	m.setRows()
	for i, r := range m.rows {
		if r.Group != nil && r.Group.Name == name {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// toggleHidden leaves the group called name out of the list, or puts it back, in every sidebar. Its
// status entries go and come with it, see spaces.Shown.
func (m *sidebar) toggleHidden(name string) {
	if m.hidden == nil {
		m.hidden = map[string]bool{}
	}
	m.hidden[name] = !m.hidden[name]
	if err := spaces.SaveHidden(m.hidden); err != nil {
		m.failed = err
	}
	at := m.cursorAt()
	m.kitty.Statuses = spaces.Shown(m.cfg, m.hidden, m.statuses)
	m.statusOffset = min(m.statusOffset, max(len(m.kitty.Statuses)-1, 0))
	m.rebuildRows(at)
	m.scrollToCursor()
}

// openPicker opens the picker in a popup for the window beside the sidebar, as `kspaces pick --from` does.
func (m *sidebar) openPicker() tea.Cmd {
	m.remote = false
	cfg, home := m.cfg, m.kitty.Home
	return func() tea.Msg {
		if err := spaces.LaunchPicker(cfg, home); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}

// addSpace opens the form that adds a space, in a popup as newWorktree's, with the repository of the
// window beside the sidebar filled in, and group chosen, empty for none.
func (m *sidebar) addSpace(group string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskAddSpace(cfg, st, group); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}
