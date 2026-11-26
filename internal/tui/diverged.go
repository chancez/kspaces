package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// resolveLayout has the layout pass resolve a divergence, a fake in tests.
var resolveLayout = spaces.ResolveLayout

// divergedSide is one side of a divergence in its menu: how many tabs it has, and how old it is.
func divergedSide(tabs int, age string) string {
	noun := "tabs"
	if tabs == 1 {
		noun = "tab"
	}
	if age == "" {
		return fmt.Sprintf("%d %s", tabs, noun)
	}
	return fmt.Sprintf("%d %s %s", tabs, noun, age)
}

// resolveRun asks the layout pass to resolve a divergence as r, off the UI loop.
func resolveRun(r spaces.Resolution) func(m *sidebar, s divergedSubject) tea.Cmd {
	return func(m *sidebar, s divergedSubject) tea.Cmd {
		d := s.d
		return func() tea.Msg {
			if err := resolveLayout(d.Session, d.Host, r); err != nil {
				return failedMsg{fmt.Errorf("resolving %s: %w", d.Session, err)}
			}
			return nil
		}
	}
}

// menuTargets are what row i's menu offers: its own, then a diverged row's resolutions, when it has one
// divergence, last, so the item the menu opens on is not one.
func (m *sidebar) menuTargets(i int) []interact.Target[*sidebar] {
	t, ok := m.entryTarget(i)
	if !ok {
		return nil
	}
	if i < len(m.rows) && m.rows[i].Space != nil {
		if ds := m.rowDiverged(m.rows[i]); len(ds) == 1 {
			return []interact.Target[*sidebar]{t, divergedRows.Target(divergedSubject{d: ds[0]})}
		}
	}
	return []interact.Target[*sidebar]{t}
}

// divergedHint is what enter on a diverged row says: the menu resolves a row of one divergence, and a
// row of several, a folded space's or a worktree's on several hosts, has each resolved from its own.
func (m *sidebar) divergedHint(r spaces.Row) string {
	ds := m.rowDiverged(r)
	d := ds[0]
	if len(ds) == 1 {
		return fmt.Sprintf("%s diverged from %s: pick a layout from the menu (m)", d.Session, hostName(d.Host))
	}
	return fmt.Sprintf("%s diverged from %s, and %d more: pick a layout from each one's row on the hosts tab", d.Session, hostName(d.Host), len(ds)-1)
}
