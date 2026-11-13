package tui

import (
	"fmt"
	"slices"
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
	// sortZone is the button on the SPACES title that shows and changes the sort mode.
	sortZone = "sort"
	// attentionZone is the button on the SPACES title that turns the attention filter on and off.
	attentionZone = "attention"
)

// rowSessions are the worktrees a row stands for: its own, for a folded space all of the space's, missing
// ones too, and for a folded group's header all of its spaces'.
func rowSessions(r spaces.Row) []spaces.Worktree {
	switch {
	case r.Group != nil && r.Folded:
		var ans []spaces.Worktree
		for _, s := range r.Group.Spaces {
			ans = slices.Concat(ans, s.Worktrees, s.Missing)
		}
		return ans
	case r.Group != nil:
		return nil
	case r.Folded:
		return slices.Concat(r.Space.Worktrees, r.Space.Missing)
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
// is, and is replaced by the most urgent status of its sessions' windows; see look.RowIcon. A missing
// worktree has the gone marker and `gone` for its branch, faint only once its session has no windows.
func (m *sidebar) renderRow(i int) string {
	r, width, th := m.rows[i], m.contentWidth(), m.th
	switch {
	case r.Group != nil:
		return m.renderHeader(i)
	case r.Space == nil:
		return th.Draw(look.Plain, width)
	}
	caret := []look.Span{look.NoCaret()}
	// Not while filtering, which ignores folds, so a caret would fold what the list does not show.
	if r.Main && !r.Other && r.Space.HasLinked() && !m.filtering {
		caret = look.Mark(fmt.Sprintf(caretZone, i), look.Caret(r.Folded)...)
	}
	kind, label := look.RowSpace, look.Space(*r.Space)
	switch {
	case r.Missing:
		kind, label = look.RowWorktree, look.MissingWorktree(r)
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
	switch {
	case r.Missing && state == "":
		icon = look.DimIf(!open, look.GoneMarker(open)...)
	case state == "" && r.Worktree != nil:
		icon = look.DimIf(!open, icon...)
	}
	faint := !open
	head := append(caret, icon...)
	room := max(width-th.Width(head...), 0)
	// The git state takes the right end, after a space, and the branch what is left between. A space whose
	// own config is not in use says so there instead, on its own row.
	gitRow := r.Worktree != nil && !r.Folded && !r.Other && !r.Missing
	var pieces gitRowPieces
	if gitRow {
		pieces = gitPieces(m.git[r.Worktree.Path])
	}
	// The name gives way to the changes and the PR, its checks and review included, down to labelFloor
	// cells, so a long name does not hide them. Ahead and behind get only what the name leaves.
	nameRoom := room
	if gitRow {
		kept := pieces
		kept[pieceAheadBehind] = nil
		if w := th.Width(m.fitGit(kept, room-min(th.Width(label), labelFloor)-1)...); w > 0 {
			nameRoom = room - w - 1
		}
	}
	spans := append(head, look.DimIf(faint, th.Cut(nameRoom, label)...)...)
	rest := width - th.Width(spans...)
	var git []look.Span
	switch {
	case r.Main && !r.Other && (r.Space.Untrusted || r.Space.ConfigErr != nil):
		git = look.DimIf(!open, m.configSpans(*r.Space, rest-1)...)
	case gitRow:
		git = look.DimIf(!open, m.fitGit(pieces, rest-1)...)
	}
	if w := th.Width(git...); w > 0 {
		rest -= w + 1
	}
	switch {
	case r.Missing && rest > 2:
		spans = append(append(spans, look.Muted(" ")), look.DimIf(faint, th.Cut(rest-1, look.Gone())...)...)
	case r.Worktree != nil && r.Worktree.Branch != "" && rest > 2:
		spans = append(append(spans, look.Muted(" ")), th.Cut(rest-1, look.Branch(*r.Worktree))...)
	}
	if len(git) > 0 {
		pad := look.Default(strings.Repeat(" ", max(width-th.Width(spans...)-th.Width(git...), 1)))
		spans = append(append(spans, pad), git...)
	}
	// A highlighted row is drawn on a surface across its whole width, so it reads as one bar. So is the row
	// whose popup shows, heading it.
	surface := look.Plain
	switch {
	case m.dragging(r), i == m.cursor && m.navigating(), m.hoverShown && rowKey(r) == m.hoverRow:
		surface = look.Cursor
	case current:
		surface = look.Current
	}
	return th.DrawZone(fmt.Sprintf(entryZone, i), surface, width, spans...)
}

// renderHeader draws row i, a group's header, marked as its entry, as a divider: a rule in the caret
// column, the group's name, its fold icon, but while filtering, which ignores folds, and the rule on to
// the right edge. A narrow sidebar loses the rule after the icon first, then the name's end, so the icon
// stays. A folded group stands for its spaces' sessions, so it is current when one of them is.
func (m *sidebar) renderHeader(i int) string {
	r, width, th := m.rows[i], m.contentWidth(), m.th
	lead := []look.Span{th.GroupRule(th.Width(look.NoCaret())), look.Muted(" ")}
	var fold []look.Span
	if !m.filtering {
		fold = append([]look.Span{look.Muted(" ")}, look.Mark(fmt.Sprintf(caretZone, i), look.GroupFold(r.Folded)...)...)
	}
	room := max(width-th.Width(lead...)-th.Width(fold...), 0)
	spans := append(append(lead, th.Cut(room, look.GroupHeader(r.Group.Name))...), fold...)
	if rest := width - th.Width(spans...); rest > 0 {
		if len(fold) == 0 {
			spans, rest = append(spans, look.Muted(" ")), rest-1
		}
		spans = append(spans, th.GroupRule(rest))
	}
	surface := look.Plain
	switch {
	case i == m.cursor && m.navigating():
		surface = look.Cursor
	case rowHolds(r, m.kitty.Current):
		surface = look.Current
	}
	return th.DrawZone(fmt.Sprintf(entryZone, i), surface, width, spans...)
}

// rowPopup is the hovered row's worktree in detail, as the picker's preview says it, and the screen row
// it starts at: under the row, over what is below it, with a rule closing it off, and moved up over the
// rows above when it would run off the bottom. Nil when no row's popup shows or the row is out of view.
// All of it is marked, so the mouse on it is on the popup rather than on what it covers.
func (m *sidebar) rowPopup() ([]string, int) {
	i := m.hoveredRow()
	y := i - m.offset
	h := m.layout().spaces
	above, below := m.spacesMore(m.offset)
	if i < 0 || y < 0 || y >= h || above && y == 0 || below && y == h-1 {
		return nil, 0
	}
	th, width := m.th, m.contentWidth()
	snap := spaces.Snapshot{Config: &m.cfg, Git: m.git, Kitty: m.kitty, Saved: m.saved}
	var popup []string
	for _, l := range worktreeFields(th, snap, m.rows[i], width) {
		popup = append(popup, th.DrawZone(popupZone, look.Plain, width, th.Cut(width, l...)...))
	}
	popup = append(popup, th.DrawZone(popupZone, look.Plain, width, look.Rule(width, true)))
	bottom := m.height - 1
	top := max(min(headerRows+y+1, bottom-len(popup)), headerRows)
	return popup[:max(min(len(popup), bottom-top), 0)], top
}

// configSpans mark a space whose own config is not in use, untrusted or broken, in at most width cells:
// what it is, in fewer words, or only its mark, when that does not fit.
func (m *sidebar) configSpans(s spaces.Space, width int) []look.Span {
	mark, words := look.BrokenConfig, look.BrokenWords
	if s.Untrusted {
		mark, words = look.UntrustedConfig, look.UntrustedWords
	}
	for _, w := range words {
		if out := mark(w); m.th.Width(out...) <= width {
			return out
		}
	}
	return nil
}

// labelFloor is the fewest cells a row's name gives way to its git state down to.
const labelFloor = 6

// The pieces of a worktree's git state on its row, in the order they are drawn.
const (
	pieceChanges = iota
	pieceAheadBehind
	piecePR
	pieceChecks
	pieceReview
	numPieces
)

// gitRowPieces are a worktree's git state, apart, by piece.
type gitRowPieces [numPieces][]look.Span

// pieceDrops are the pieces fitGit leaves out, in turn, until the rest fit: ahead and behind first and the
// changes last, as they are what removing the worktree loses, and the PR's review and checks before its
// number.
var pieceDrops = [numPieces]int{pieceAheadBehind, pieceReview, pieceChecks, piecePR, pieceChanges}

// gitPieces are a worktree's git state, apart: the files it has not committed, how far it is ahead of
// and behind its upstream, and its pull request, with what its checks come to and its review decision.
func gitPieces(st gitstate.State) gitRowPieces {
	var pieces gitRowPieces
	if s := st.Status; s != nil {
		if s.Changes > 0 {
			pieces[pieceChanges] = look.Changes(s.Changes)
		}
		pieces[pieceAheadBehind] = look.AheadBehind(s.Ahead, s.Behind)
	}
	if st.PR != nil {
		pieces[piecePR] = []look.Span{look.PRNumber(*st.PR)}
		pieces[pieceChecks] = look.PRChecks(*st.PR)
		pieces[pieceReview] = look.PRReview(*st.PR)
	}
	return pieces
}

// fitGit is gitPieces in at most width cells, a space apart. What does not fit is left out, in the order
// of pieceDrops.
func (m *sidebar) fitGit(pieces gitRowPieces, width int) []look.Span {
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
	for _, drop := range pieceDrops {
		if out := join(); m.th.Width(out...) <= width {
			return out
		}
		pieces[drop] = nil
	}
	return nil
}
