package tui

import (
	"fmt"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// Zones the sidebar marks what it draws with, so a click finds what is there.
const (
	// entryZone is entry i of the cursor's list: a row of the spaces list or a status entry, all of it.
	entryZone = "entry:%d"
	// caretZone is the fold caret of row i.
	caretZone = "caret:%d"
	// menuZone is item i of the context menu.
	menuZone = "menu:%d"
	// popupZone is the hover popup, all of it.
	popupZone = "popup"
)

// rowSessions are the worktrees a row stands for: its own, and for a folded space all of the space's.
func rowSessions(r spaces.Row) []spaces.Worktree {
	switch {
	case r.Folded:
		return r.Space.Worktrees
	case r.Worktree != nil:
		return []spaces.Worktree{*r.Worktree}
	}
	return nil
}

func rowHolds(r spaces.Row, session string) bool {
	for _, wt := range rowSessions(r) {
		if wt.Session == session {
			return true
		}
	}
	return false
}

// renderRow draws row i of the spaces list, marked as its entry. The marker says how its sessions are:
// the most urgent status of their windows, current, open, or none; a cross when the space's worktrees
// could not be listed. The caret column folds a space's linked worktrees, and indents those worktrees
// under its name.
func (m *sidebar) renderRow(i int) string {
	r, width, th := m.rows[i], m.contentWidth(), m.th
	if r.Space == nil {
		return th.Draw(look.Plain, width)
	}
	caret := []look.Span{look.NoCaret()}
	if r.Main && !r.Other && len(r.Space.Worktrees) > 1 {
		caret = look.Mark(fmt.Sprintf(caretZone, i), look.Caret(r.Folded)...)
	}
	var label []look.Span
	switch {
	case !r.Main:
		label = look.Worktree(*r.Worktree)
	case r.Other:
		label = []look.Span{look.OtherSession(*r.Space)}
	default:
		label = []look.Span{look.Space(*r.Space)}
	}
	// A folded space stands for all its worktrees: it is current, open or in a state if any of them is.
	current, open, state := false, false, ""
	for _, wt := range rowSessions(r) {
		current = current || wt.Session == m.kitty.Current
		open = open || m.isOpen(&wt)
		if s := status.OfSession(m.kitty.Statuses, wt.Session); status.Urgency(s) > status.Urgency(state) {
			state = s
		}
	}
	mark := []look.Span{look.NoMarker()}
	switch {
	case r.Worktree == nil:
		mark = look.BrokenMarker()
	case state != "":
		// The most urgent status of the session's windows, which the status list below details.
		mark = look.StateMarker(state)
	case current:
		mark = look.CurrentMarker()
	case open:
		mark = look.SessionMarker()
	}
	label = look.DimIf(!open, th.Cut(max(width-th.Width(mark...)-th.Width(caret...), 0), label...)...)
	spans := append(append(mark, caret...), label...)
	if rest := width - th.Width(spans...); r.Worktree != nil && r.Worktree.Branch != "" && rest > 2 {
		spans = append(append(spans, look.Muted(" ")), th.Cut(rest-1, look.Branch(*r.Worktree))...)
	}
	// A highlighted row is drawn on a surface across its whole width, so it reads as one bar.
	surface := look.Plain
	switch {
	case i == m.cursor && m.navigating():
		surface = look.Cursor
	case current:
		surface = look.Current
	}
	return th.DrawZone(fmt.Sprintf(entryZone, i), surface, width, spans...)
}
