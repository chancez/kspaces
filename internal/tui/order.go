package tui

import (
	"maps"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// The spaces list's order: the sort mode every sidebar shares, and moving rows by hand.

// rowOrder is the order the rows are built in: the snapshot's, with what is held kept where it is.
func (m *sidebar) rowOrder() spaces.Order {
	o := m.order
	o.Held = m.held
	return o
}

// sortMode is the sort mode the rows are in, the zero mode being manual.
func (m *sidebar) sortMode() spaces.SortMode {
	if m.order.Mode == "" {
		return spaces.SortManual
	}
	return m.order.Mode
}

// setSort orders the rows by mode, in every sidebar, letting go of any order held. The cursor stays on
// its row, wherever that goes.
func (m *sidebar) setSort(mode spaces.SortMode) {
	m.order.Mode, m.held = mode, nil
	if err := spaces.SaveSort(mode); err != nil {
		m.failed = err
	}
	m.setRows()
	m.scrollToCursor()
}

// openSortMenu opens the spaces list's menu, of the sort button, the title and the list's empty rows,
// drawn from screen row y: the sort modes, the one chosen now marked and selected, the attention filter,
// marked while it is on, and each group, marked while it is shown.
func (m *sidebar) openSortMenu(y int) {
	targets := []interact.Target[*sidebar]{theSort.Target(sortSubject{})}
	for _, name := range m.order.GroupNames(m.spaces) {
		targets = append(targets, theGroups.Target(groupFilterSubject{name: name}))
	}
	m.openMenu(y, targets...)
	if m.menu == nil {
		return
	}
	m.menu.chosen = map[int]bool{}
	for i, it := range m.menu.items {
		switch {
		case it.Is(sortActions[m.sortMode()]):
			m.menu.chosen[i], m.menu.sel = true, i
		case it.Is(actAttentionItem):
			m.menu.chosen[i] = m.attending
		case it.Is(actShowGroup):
			m.menu.chosen[i] = !m.hidden[it.Title]
		}
	}
}

// arrangement is an order of the spaces, and of each one's linked worktrees, as the rows show it or as a
// move will leave it.
type arrangement struct {
	// The sessions in no space, which do not move but are held where they are with the rest.
	others []string
	// Each group's spaces together, as BuildRows lists them.
	roots []string
	// By root: the group of each space, and its linked worktrees.
	groups map[string]string
	linked map[string][]spaces.Worktree
}

// shown is the order the rows are in, folded spaces' worktrees too, and folded and hidden groups' spaces.
func (m *sidebar) shown() arrangement {
	a := arrangement{groups: map[string]string{}, linked: map[string][]spaces.Worktree{}}
	for _, r := range spaces.BuildRows(m.others, m.spaces, nil, nil, m.rowOrder()) {
		switch {
		case r.Space == nil || r.Missing:
		case r.Other:
			a.others = append(a.others, r.Worktree.Session)
		case r.Main:
			a.roots = append(a.roots, r.Space.Root)
			a.groups[r.Space.Root] = r.Space.Group
		default:
			a.linked[r.Space.Root] = append(a.linked[r.Space.Root], *r.Worktree)
		}
	}
	return a
}

// held ranks everything in a for spaces.Order.Held.
func (a arrangement) held() map[string]int {
	h := map[string]int{}
	for i, s := range a.others {
		h[s] = i
	}
	for i, root := range a.roots {
		h[root] = i
	}
	for _, wts := range a.linked {
		for i, wt := range wts {
			h[wt.Session] = i
		}
	}
	return h
}

// movable reports whether a row can be moved by hand: a space, or a linked worktree within its space.
// Sessions in no space keep their names' order, and a missing worktree stays last.
func movable(r spaces.Row) bool {
	return r.Space != nil && !r.Other && !r.Missing && (r.Main || r.Worktree != nil)
}

// place is where r is in a, and how many it is among: a space among the spaces of its group, a linked
// worktree among its space's. -1 when a does not have it.
func (a arrangement) place(r spaces.Row) (int, int) {
	if r.Main {
		start, n := a.group(r.Space.Root)
		return slices.Index(a.roots[start:start+n], r.Space.Root), n
	}
	wts := a.linked[r.Space.Root]
	return slices.IndexFunc(wts, func(wt spaces.Worktree) bool { return wt.Session == r.Worktree.Session }), len(wts)
}

// group is where the spaces of root's group start in a.roots, and how many there are; none when a does
// not have root.
func (a arrangement) group(root string) (start, n int) {
	g, ok := a.groups[root]
	if !ok {
		return 0, 0
	}
	start = slices.IndexFunc(a.roots, func(r string) bool { return a.groups[r] == g })
	for start+n < len(a.roots) && a.groups[a.roots[start+n]] == g {
		n++
	}
	return start, n
}

// move is a with r moved to place to among its own, a copy.
func (a arrangement) move(r spaces.Row, to int) arrangement {
	from, n := a.place(r)
	if from < 0 {
		return a
	}
	to = min(max(to, 0), n-1)
	out := arrangement{others: a.others, roots: a.roots, groups: a.groups, linked: maps.Clone(a.linked)}
	if r.Main {
		start, _ := a.group(r.Space.Root)
		out.roots = moved(a.roots, start+from, start+to)
	} else {
		out.linked[r.Space.Root] = moved(a.linked[r.Space.Root], from, to)
	}
	return out
}

// moved is a copy of s with its item from at to.
func moved[T any](s []T, from, to int) []T {
	return slices.Insert(slices.Delete(slices.Clone(s), from, from+1), to, s[from])
}

// dirs names the worktrees by directory, as the saved order does.
func dirs(wts []spaces.Worktree) []string {
	var ans []string
	for _, wt := range wts {
		ans = append(ans, filepath.Base(wt.Path))
	}
	return ans
}

// keep makes a, the order after moving r, the manual order, in every sidebar: the spaces' in config.toml
// and the linked worktrees' in the state directory. Moving in an automatic mode switches to manual, from
// the order shown, so every space's worktrees are saved as they are shown. The rows show a at once, and a
// fresh load follows for what the config now says.
func (m *sidebar) keep(a arrangement, r spaces.Row) tea.Cmd {
	automatic := m.sortMode() != spaces.SortManual
	// config.toml's order as the manual mode shows it, each group's spaces together: a move that leaves
	// it as it is, as of a worktree, need not rewrite the file only to group them there too.
	var listed []string
	for _, row := range spaces.BuildRows(nil, m.spaces, nil, nil, spaces.Order{Groups: m.order.Groups}) {
		if row.Main {
			listed = append(listed, row.Space.Root)
		}
	}
	if !slices.Equal(a.roots, listed) {
		if err := config.SaveSpacesOrder(a.roots); err != nil {
			m.failed = err
			return nil
		}
		byRoot := map[string]spaces.Space{}
		for _, s := range m.spaces {
			byRoot[s.Root] = s
		}
		m.spaces = make([]spaces.Space, 0, len(a.roots))
		for _, root := range a.roots {
			m.spaces = append(m.spaces, byRoot[root])
		}
	}
	saved := maps.Clone(m.order.Worktrees)
	if saved == nil {
		saved = map[string][]string{}
	}
	for root, wts := range a.linked {
		if automatic || !r.Main && root == r.Space.Root {
			saved[root] = dirs(wts)
		}
	}
	if !maps.EqualFunc(saved, m.order.Worktrees, slices.Equal) {
		if err := spaces.SaveOrder(saved); err != nil {
			m.failed = err
		}
		m.order.Worktrees = saved
	}
	m.held = nil
	if automatic {
		m.order.Mode = spaces.SortManual
		if err := spaces.SaveSort(spaces.SortManual); err != nil {
			m.failed = err
		}
	}
	m.setRows()
	m.scrollToCursor()
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Poll(true).View(self) }
}

// moveRow moves the cursor's row by delta among its own, as dragging it does: K and J.
func (m *sidebar) moveRow(delta int) tea.Cmd {
	if m.narrowed() || m.cursor < 0 || m.cursor >= len(m.rows) || !movable(m.rows[m.cursor]) {
		return nil
	}
	r, a := m.rows[m.cursor], m.shown()
	from, n := a.place(r)
	if from < 0 || from+delta < 0 || from+delta >= n {
		return nil
	}
	return m.keep(a.move(r, from+delta), r)
}

// keepOrder holds the order the rows show while the mouse is pressed on one or a menu is open, so a
// refresh, as in an automatic sort mode, does not move rows from under it, and lets go after. A drag holds
// the order it moves to itself.
func (m *sidebar) keepOrder() {
	switch {
	case m.press != nil || m.menu != nil:
		if m.held == nil {
			m.held = m.shown().held()
		}
	default:
		m.held = nil
	}
}
