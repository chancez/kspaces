package tui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The picker's preview of the entry under the cursor: beside the list in a wide overlay, below it in a
// narrow one, and left out when it would leave the list too few rows. What it says of the entry comes
// first, then the screen of the entry's window where there is room.

const (
	// previewBesideWidth is the narrowest overlay that has the preview beside the list, each half of it.
	previewBesideWidth = 90
	// previewBelowRows is the most rows the preview takes below the list, so the list keeps most of a tall
	// overlay; previewMinRows the fewest worth showing.
	previewBelowRows = 10
	previewMinRows   = 3
	// previewLabelWidth is the column a worktree's fields start at, after their labels.
	previewLabelWidth = 9
)

// pickLayout is how the picker splits the rows between its input and its last line.
type pickLayout struct {
	listRows, listWidth int
	// No rows for no preview.
	previewRows, previewWidth int
	beside                    bool
}

// pickLayoutFor splits rows rows of width cells. Below the list the preview has a fixed height, whatever
// the entry, so the list does not jump as the cursor moves.
func pickLayoutFor(width, rows int) pickLayout {
	if width >= previewBesideWidth {
		// A space, the rule and a space between the two.
		list := (width - 3) / 2
		return pickLayout{listRows: rows, listWidth: list, previewRows: rows, previewWidth: width - 3 - list, beside: true}
	}
	// The rule above the preview is one of its rows.
	below := min(rows/2, previewBelowRows+1)
	if below-1 < previewMinRows {
		return pickLayout{listRows: rows, listWidth: width}
	}
	return pickLayout{listRows: rows - below, listWidth: width, previewRows: below - 1, previewWidth: width}
}

func (m *picker) layout() pickLayout {
	return pickLayoutFor(max(m.width, 1), max(m.height-3, 1))
}

// current is the entry under the cursor, if any.
func (m *picker) current() (pickItem, bool) {
	if m.cursor >= len(m.matches) || m.snap.Config == nil {
		return pickItem{}, false
	}
	return m.items[m.matches[m.cursor].Index], true
}

// preview is the preview of the cursor's entry in width cells and rows rows: what it says of the entry,
// then under a rule the bottom of its window's screen, when it has been fetched and there is room.
func (m *picker) preview(width, rows int) [][]look.Span {
	it, ok := m.current()
	if !ok {
		return nil
	}
	lines := m.details(it, width)
	n := screenRoom(len(lines), rows)
	if n == 0 || m.screen.lines == nil {
		return lines
	}
	lines = append(lines, []look.Span{look.Rule(width, false)})
	for _, l := range screenBlock(m.screen.lines, width, n) {
		lines = append(lines, []look.Span{look.Styled(func(color.Color) string { return l })})
	}
	return lines
}

// details is what the picker says of an entry, width cells wide at most: a status entry as the sidebar's
// opened entry says it, or a worktree's space, path, branch, session and statuses.
func (m *picker) details(it pickItem, width int) [][]look.Span {
	if s := it.status; s != nil {
		lines := [][]look.Span{statusHeader(m.th, *s, width, time.Now())}
		return append(lines, statusDetails(*s, max(width-m.th.Width(look.NoMarker()), 1), 0, 0)...)
	}
	lines := [][]look.Span{{look.Heading(it.text[:it.nameLen])}, {}}
	return append(lines, worktreeFields(m.th, m.snap, it.row, width)...)
}

// worktreeFields are what the picker's preview and the sidebar's row popup say of a worktree's row, width
// cells wide at most: its space, path, branch, git state, session, windows, saved file and statuses.
func worktreeFields(th *look.Theme, snap spaces.Snapshot, r spaces.Row, width int) [][]look.Span {
	wt := r.Worktree
	field := func(label string, value look.Span) []look.Span {
		return []look.Span{look.Label(fmt.Sprintf("%-*s", previewLabelWidth, label)), value}
	}
	var lines [][]look.Span
	switch {
	case r.Other:
		lines = append(lines, field("space", look.Muted("none")))
	case r.Missing:
		// git no longer lists it, so there is no path, branch or state to show.
		lines = append(lines, field("space", look.Text(r.Space.Name)), field("path", look.Muted("gone, the worktree directory was removed")))
	default:
		lines = append(lines, field("space", look.Text(r.Space.Name)), field("path", look.Text(worktreePath(r.Space.Root, wt.Path))))
		if wt.Branch == "" {
			lines = append(lines, field("branch", look.Muted("none, HEAD is detached")))
		} else {
			lines = append(lines, field("branch", look.Text(wt.Branch)))
		}
		lines = append(lines, gitFields(snap.Git[wt.Path])...)
	}
	lines = append(lines, field("session", look.Text(wt.Session)))
	if n := snap.Kitty.Windows[wt.Session]; n > 0 {
		lines = append(lines, field("windows", look.Text(fmt.Sprintf("%d open", n))))
	} else {
		lines = append(lines, field("windows", look.Muted("none open")))
	}
	switch {
	case snap.Config.SessionsDir == "":
		lines = append(lines, field("saved", look.Muted("no sessions directory")))
	case snap.Saved[wt.Session]:
		lines = append(lines, field("saved", look.Text("yes")))
	default:
		lines = append(lines, field("saved", look.Muted("no")))
	}
	first := true
	for _, s := range snap.Kitty.Statuses {
		if s.Session != wt.Session {
			continue
		}
		if first {
			lines, first = append(lines, []look.Span{}), false
		}
		lines = append(lines, statusSummary(th, s, width))
	}
	return lines
}

// statusSummary is one row for a window's status: its marker, what the program is up to and its age.
func statusSummary(th *look.Theme, s status.Status, width int) []look.Span {
	mark, right := look.StateMarker(s.State), look.StatusAge(s, time.Now())
	inner := max(width-th.Width(mark...)-th.Width(right), 0)
	sum := look.Summary(s.Record, inner)
	if th.Width(sum) == 0 {
		sum = look.Muted(s.State)
	}
	text := th.Cut(inner, sum)
	pad := look.Default(strings.Repeat(" ", max(inner-th.Width(text...), 0)))
	return append(append(append(mark, text...), pad), right)
}

// worktreePath is where a worktree is: relative to its space's root when inside it, else in full, with
// the home directory as ~.
func worktreePath(root, path string) string {
	// git reports worktree paths with symlinks resolved, and the config may not have the root so.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return config.Tilde(path)
}

// gitFields are what git and gh say of a worktree, as preview fields: its changes, its upstream and how
// far it is from it, and its PR. What is not known yet is left out.
func gitFields(st gitstate.State) [][]look.Span {
	field := func(label string, values ...look.Span) []look.Span {
		return append([]look.Span{look.Label(fmt.Sprintf("%-*s", previewLabelWidth, label))}, values...)
	}
	var lines [][]look.Span
	if s := st.Status; s != nil {
		changes := []look.Span{look.Muted("none")}
		if s.Changes > 0 {
			changes = append(look.Changes(s.Changes), look.Text(" not committed"))
		}
		upstream := []look.Span{look.Muted("none")}
		switch ab := look.AheadBehind(s.Ahead, s.Behind); {
		case s.Upstream == "":
		case s.Gone:
			upstream = []look.Span{look.Text(s.Upstream), look.Muted(", gone")}
		case len(ab) > 0:
			upstream = append([]look.Span{look.Text(s.Upstream), look.Muted(" ")}, ab...)
		default:
			upstream = []look.Span{look.Text(s.Upstream), look.Muted(", up to date")}
		}
		lines = append(lines, field("changes", changes...), field("upstream", upstream...))
	}
	switch {
	case st.PR != nil:
		lines = append(lines, field("pr", look.PRNumber(*st.PR), look.Muted(" "), look.PRState(*st.PR)))
	case !st.PRAt.IsZero():
		lines = append(lines, field("pr", look.Muted("none")))
	}
	return lines
}
