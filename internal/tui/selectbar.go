package tui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The select mode's bar, at the bottom of the spaces list above the STATUS rule: a rule with how many rows
// are ticked set into it, then a row for each action on them, as a menu's items, with how many of the
// ticked rows it applies to and its key each in a column of their own, greyed while it applies to none,
// Cancel last, and a hint line saying why one does nothing, or what the one under the mouse does. A row
// to an action keeps every row's columns in line, where buttons flowed over rows at their own widths.
// Each action opens its form, which confirms in a popup of its own, with what it applies to.

// barHeight is how many rows the bar takes from the spaces list: its title, an action's row each and
// Cancel's, and the hint; none outside the select mode.
func (m *sidebar) barHeight() int {
	if !m.selecting {
		return 0
	}
	return len(m.barActions()) + 3
}

// controlOffZone marks a greyed row of the bar, which a click does nothing on but the mouse can point at,
// for the hint to say why.
const controlOffZone = "control-off:"

// Why each action is greyed, what none of the ticked rows is, said on the hint line and by its key: short,
// for a narrow sidebar.
const (
	noneToRemove = "None can be removed"
	noneToClose  = "None has windows open"
	noneToForget = "None has only a saved file"
	noneToRun    = "None has commands to run"
)

// selected are the ticked rows, folds and filters aside, in the list's order.
func (m *sidebar) selected() []spaces.Row {
	var out []spaces.Row
	for _, r := range m.allRows() {
		if r.Worktree != nil && m.ticked[r.Worktree.Session] {
			out = append(out, r)
		}
	}
	return out
}

// selectedWhere are the ticked rows ok is true of.
func (m *sidebar) selectedWhere(ok func(r spaces.Row) bool) []spaces.Row {
	var out []spaces.Row
	for _, r := range m.selected() {
		if ok(r) {
			out = append(out, r)
		}
	}
	return out
}

// What each action applies to, of the ticked rows: Remove the linked and missing worktrees, Close the
// sessions with windows open, Forget those with only a saved file, and Run the worktrees whose space has
// commands, not a missing one, whose directory is gone.
func (m *sidebar) toRemove() []spaces.Row {
	return m.selectedWhere(func(r spaces.Row) bool { return !r.Main })
}

func (m *sidebar) toClose() []spaces.Row {
	return m.selectedWhere(func(r spaces.Row) bool { return m.kitty.Windows[r.Worktree.Session] > 0 })
}

func (m *sidebar) toForget() []spaces.Row {
	return m.selectedWhere(func(r spaces.Row) bool {
		return m.kitty.Windows[r.Worktree.Session] == 0 && m.saved[r.Worktree.Session]
	})
}

func (m *sidebar) toRun() []spaces.Row {
	return m.selectedWhere(func(r spaces.Row) bool {
		return !r.Other && !r.Missing && r.Worktree.Path != "" && len(r.Space.ConfigOf(m.cfg).Commands) > 0
	})
}

// runs reports whether any space has commands to run, without which the bar has no Run.
func (m *sidebar) runs() bool {
	for _, s := range m.spaces {
		if len(s.ConfigOf(m.cfg).Commands) > 0 {
			return true
		}
	}
	return false
}

func sessionsOf(rows []spaces.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Worktree.Session
	}
	return out
}

// launch opens a form with ask, and ends the select mode once done says the rows show the form confirmed:
// the form runs in a popup of its own, and cancelled it changes nothing, so the ticks stay for another go.
func (m *sidebar) launch(ask func(cfg config.Config, st kitty.State) error, done func(m *sidebar) bool) tea.Cmd {
	m.remote, m.pending = false, done
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := ask(cfg, st); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}

// settle ends the select mode once the action handed to a form shows done, see launch.
func (m *sidebar) settle() {
	if m.selecting && m.pending != nil && m.pending(m) {
		m.endSelect()
	}
}

// refuse says why an action applies to none of the ticked rows, on the last line.
func (m *sidebar) refuse(why string) tea.Cmd {
	if len(m.selected()) == 0 {
		why = "Nothing is ticked"
	}
	m.failed = errors.New(why)
	return nil
}

// removeSelected opens the clean up form with the ticked rows: it removes the linked worktrees, closes and
// forgets the missing ones, and says why it leaves a main worktree or a session in no space. Done once one
// of those it can remove is gone from the list.
func (m *sidebar) removeSelected() tea.Cmd {
	gone := sessionsOf(m.toRemove())
	if len(gone) == 0 {
		return m.refuse(noneToRemove)
	}
	all := sessionsOf(m.selected())
	return m.launch(func(cfg config.Config, st kitty.State) error { return spaces.AskRemoveSessions(cfg, st, all) },
		func(m *sidebar) bool {
			listed := map[string]bool{}
			for _, r := range m.allRows() {
				if r.Worktree != nil {
					listed[r.Worktree.Session] = true
				}
			}
			for _, s := range gone {
				if !listed[s] {
					return true
				}
			}
			return false
		})
}

// closeSelected opens the close session form with the ticked sessions that have windows. Done once one of
// them has none.
func (m *sidebar) closeSelected() tea.Cmd {
	sessions := sessionsOf(m.toClose())
	if len(sessions) == 0 {
		return m.refuse(noneToClose)
	}
	return m.launch(func(cfg config.Config, st kitty.State) error { return spaces.AskCloseSession(cfg, st, sessions...) },
		func(m *sidebar) bool {
			for _, s := range sessions {
				if m.kitty.Windows[s] == 0 {
					return true
				}
			}
			return false
		})
}

// forgetSelected opens the forget session form with the ticked sessions that have only a saved file. Done
// once one of the files is gone.
func (m *sidebar) forgetSelected() tea.Cmd {
	sessions := sessionsOf(m.toForget())
	if len(sessions) == 0 {
		return m.refuse(noneToForget)
	}
	return m.launch(func(cfg config.Config, st kitty.State) error { return spaces.AskForgetSession(cfg, st, sessions...) },
		func(m *sidebar) bool {
			for _, s := range sessions {
				if !m.saved[s] {
					return true
				}
			}
			return false
		})
}

// runSelected opens the run form for the ticked worktrees, the first one's space's first command chosen.
// Done once one of their sessions has more windows, as the command opens one.
func (m *sidebar) runSelected() tea.Cmd {
	rows := m.toRun()
	if len(rows) == 0 {
		return m.refuse(noneToRun)
	}
	name := rows[0].Space.ConfigOf(m.cfg).Commands[0].Name
	paths := make([]string, len(rows))
	for i, r := range rows {
		paths[i] = r.Worktree.Path
	}
	before := maps.Clone(m.kitty.Windows)
	return m.launch(func(cfg config.Config, st kitty.State) error { return spaces.AskRunCommand(cfg, st, name, paths...) },
		func(m *sidebar) bool {
			for _, r := range rows {
				if m.kitty.Windows[r.Worktree.Session] > before[r.Worktree.Session] {
					return true
				}
			}
			return false
		})
}

// barAction is one of the bar's buttons: its action, its name, how many ticked rows it applies to, what it
// does with them and why it does nothing.
type barAction struct {
	action any
	name   string
	n      int
	does   string
	none   string
}

// barActions are the bar's buttons for what is ticked now, Run only with commands configured.
func (m *sidebar) barActions() []barAction {
	remove, closing, forget := len(m.toRemove()), len(m.toClose()), len(m.toForget())
	out := []barAction{
		{actSelRemove, "Remove", remove, "Removes " + plural(remove, "worktree") + ", asking first", noneToRemove},
		{actSelClose, "Close", closing, "Closes " + plural(closing, "session") + ", asking first", noneToClose},
		{actSelForget, "Forget", forget, "Deletes " + plural(forget, "saved file") + ", asking first", noneToForget},
	}
	if m.runs() {
		run := len(m.toRun())
		out = append(out, barAction{actSelRun, "Run", run, "Runs a command in " + plural(run, "worktree"), noneToRun})
	}
	return out
}

// barRow is one of the bar's rows: its action, its name, how many ticked rows it applies to, empty for
// Cancel, what the hint says of it, and whether it is greyed.
type barRow struct {
	action      any
	name, count string
	hint        string
	off         bool
}

// barLines draws the bar width cells wide: the rule with `N selected` set into it, the number alone when
// the words do not fit, the rows, see barRowLines, and the hint. Nothing outside the select mode.
func (m *sidebar) barLines(width int) []string {
	m.bar, m.barHints = nil, nil
	if !m.selecting {
		return nil
	}
	th := m.th
	m.bar, m.barHints = controls[*sidebar]{}, map[string]string{}
	n := len(m.selected())
	count := look.Title(fmt.Sprintf("%d selected", n))
	if th.Width(count)+5 > width {
		count = look.Title(fmt.Sprint(n))
	}
	lines := []string{th.Draw(look.Plain, width, ruleWith(th, width, count)...)}
	var rows []barRow
	var greyed []string
	for _, a := range m.barActions() {
		r := barRow{action: a.action, name: a.name, count: "-", hint: a.none, off: a.n == 0}
		if a.n > 0 {
			r.count, r.hint = fmt.Sprint(a.n), a.does
		} else {
			greyed = append(greyed, strings.ToLower(a.name))
		}
		rows = append(rows, r)
	}
	rows = append(rows, barRow{action: actEndSelect, name: "Cancel", hint: "Leaves the select mode"})
	lines = append(lines, m.barRowLines(rows, width)...)
	return append(lines, th.Draw(look.Plain, width, th.Cut(width, look.Muted(m.barHint(greyed)))...))
}

// barRowLines draws rows width cells wide, each marked for the mouse across all of it: its name on the
// left, then how many it applies to and its key, each right-aligned in a column as wide as the widest, so
// they line up. A narrow sidebar loses the counts' column first, then the keys', rather than wrap a row;
// the keys work whether shown or not. The row under the mouse is lit as the cursor's bar, but a greyed one,
// which a click does nothing on; the hint says why.
func (m *sidebar) barRowLines(rows []barRow, width int) []string {
	th, t := m.th, theSelection.Target(selectSubject{})
	keys := interact.Keys(m, t)
	nameW, countW, keyW := 0, 0, 0
	key := map[any]string{}
	for _, r := range rows {
		_, key[r.action], _ = offered(m, t, keys, r.action)
		nameW, countW, keyW = max(nameW, len(r.name)), max(countW, len(r.count)), max(keyW, len(key[r.action]))
	}
	// A cell before the name and after the key, as a menu's items have, and two between the columns.
	if 1+nameW+2+countW+2+keyW+1 > width {
		countW = 0
	}
	if 1+nameW+2+keyW+1 > width {
		keyW = 0
	}
	var out []string
	for _, r := range rows {
		b, _, ok := offered(m, t, keys, r.action)
		if !ok {
			continue
		}
		text := look.Text
		if r.off {
			text = look.Muted
		}
		var right []look.Span
		if countW > 0 {
			right = append(right, text(fmt.Sprintf("%*s", countW, r.count)))
		}
		if keyW > 0 {
			right = append(gap(right), look.Key(fmt.Sprintf("%*s", keyW, key[r.action])))
		}
		right = append(right, look.Default(" "))
		name := th.Cut(max(width-1-th.Width(right...)-2, 1), text(r.name))
		fill := look.Default(strings.Repeat(" ", max(width-1-th.Width(name...)-th.Width(right...), 0)))
		spans := slices.Concat([]look.Span{look.Default(" ")}, name, []look.Span{fill}, right)
		id, surface := controlZone+b.ID(), look.Plain
		m.barHints[b.ID()] = r.hint
		switch {
		case r.off:
			id = controlOffZone + b.ID()
		case m.barHot == b.ID():
			surface = look.Cursor
		}
		if !r.off {
			m.bar[id] = b
		}
		out = append(out, th.DrawZone(id, surface, width, spans...))
	}
	return out
}

// barHint is what the hint line says: what the row under the mouse does, or why it is greyed; else what
// the greyed rows have nothing to do to, as `Nothing to close or forget`, or with nothing ticked how to
// tick rows. Short, as a narrow sidebar cuts it; pointing at one says why.
func (m *sidebar) barHint(greyed []string) string {
	if hint, ok := m.barHints[m.barHot]; ok {
		return hint
	}
	switch {
	case len(m.ticked) == 0:
		return "Click or drag over rows to tick them"
	case len(greyed) == 1:
		return "Nothing to " + greyed[0]
	case len(greyed) > 1:
		return "Nothing to " + strings.Join(greyed[:len(greyed)-1], ", ") + " or " + greyed[len(greyed)-1]
	}
	return ""
}

// hoverBar notes the bar's row under the mouse at x, y, by its action's ID, or none.
func (m *sidebar) hoverBar(x, y int) {
	m.barHot = ""
	for id, zones := range m.zones {
		action, ok := strings.CutPrefix(id, controlZone)
		if !ok {
			action, ok = strings.CutPrefix(id, controlOffZone)
		}
		for _, z := range zones {
			if ok && z.Contains(x, y) {
				m.barHot = action
			}
		}
	}
}
