package tui

import (
	"fmt"
	"strings"

	"github.com/chancez/kspaces/internal/gitstate"
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
	// addZone is the button on the SPACES title that adds a space.
	addZone = "add"
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

// renderRow draws row i of the spaces list, marked as its entry: the fold caret, or as wide a blank, which
// indents linked worktrees under their space, then the row's icon and its name. The icon says what the row
// is, and is replaced by the most urgent status of its sessions' windows; see look.RowIcon.
func (m *sidebar) renderRow(i int) string {
	r, width, th := m.rows[i], m.contentWidth(), m.th
	if r.Space == nil {
		return th.Draw(look.Plain, width)
	}
	caret := []look.Span{look.NoCaret()}
	// Not while filtering, which ignores folds, so a caret would fold what the list does not show.
	if r.Main && !r.Other && len(r.Space.Worktrees) > 1 && !m.filtering {
		caret = look.Mark(fmt.Sprintf(caretZone, i), look.Caret(r.Folded)...)
	}
	kind, label := look.RowSpace, look.Space(*r.Space)
	switch {
	case !r.Main:
		kind, label = look.RowWorktree, look.Worktree(*r.Worktree)
	case r.Other:
		kind, label = look.RowOther, look.OtherSession(*r.Space)
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
	icon := look.RowIcon(kind, state, open, r.Worktree == nil)
	if state == "" && r.Worktree != nil {
		icon = look.DimIf(!open, icon...)
	}
	head := append(caret, icon...)
	spans := append(head, look.DimIf(!open, th.Cut(max(width-th.Width(head...), 0), label)...)...)
	rest := width - th.Width(spans...)
	// The git state takes the right end, after a space, and the branch what is left between.
	var git []look.Span
	if r.Worktree != nil && !r.Folded && !r.Other {
		git = look.DimIf(!open, m.gitSpans(m.git[r.Worktree.Path], rest-1)...)
	}
	if w := th.Width(git...); w > 0 {
		rest -= w + 1
	}
	if r.Worktree != nil && r.Worktree.Branch != "" && rest > 2 {
		spans = append(append(spans, look.Muted(" ")), th.Cut(rest-1, look.Branch(*r.Worktree))...)
	}
	if len(git) > 0 {
		pad := look.Default(strings.Repeat(" ", max(width-th.Width(spans...)-th.Width(git...), 1)))
		spans = append(append(spans, pad), git...)
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

// gitSpans is a worktree's git state in at most width cells: the files it has not committed, how far it
// is ahead of and behind its upstream, and its pull request, a space apart. What does not fit is left
// out, ahead and behind first and the changes last, as they are what removing the worktree loses.
func (m *sidebar) gitSpans(st gitstate.State, width int) []look.Span {
	var pieces [3][]look.Span
	if s := st.Status; s != nil {
		if s.Changes > 0 {
			pieces[0] = look.Changes(s.Changes)
		}
		pieces[1] = look.AheadBehind(s.Ahead, s.Behind)
	}
	if st.PR != nil {
		pieces[2] = []look.Span{look.PRNumber(*st.PR)}
	}
	join := func() []look.Span {
		var out []look.Span
		for _, p := range pieces {
			if len(p) == 0 {
				continue
			}
			if len(out) > 0 {
				out = append(out, look.Default(" "))
			}
			out = append(out, p...)
		}
		return out
	}
	for _, drop := range []int{1, 2, 0} {
		if out := join(); m.th.Width(out...) <= width {
			return out
		}
		pieces[drop] = nil
	}
	return nil
}
