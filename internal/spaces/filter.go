package spaces

import (
	"slices"

	"github.com/chancez/kspaces/internal/status"
)

// Narrow keeps the rows, as BuildRows lists them, that keep says to: a linked or missing worktree when
// keepLinked says so, and a space's own row when keepHead says so or when any of its rows is kept, so a
// kept worktree shows under its space. A group's header shows above the first of its spaces kept, and on
// its own when it is folded and keepHead says so, as it stands for its spaces. Blank rows go between
// spaces and above headers as BuildRows puts them.
func Narrow(rows []Row, keepHead, keepLinked func(Row) bool) []Row {
	var out []Row
	blank := func() {
		if len(out) > 0 {
			out = append(out, Row{})
		}
	}
	// The header of the group being gone over, until one of its spaces is kept.
	var header *Row
	for i := 0; i < len(rows); i++ {
		head := rows[i]
		switch {
		case head.Group != nil:
			header = &rows[i]
			if head.Folded && keepHead(head) {
				blank()
				out, header = append(out, head), nil
			}
			continue
		case head.Space == nil:
			continue
		}
		var linked []Row
		for i+1 < len(rows) && rows[i+1].Space == head.Space && !rows[i+1].Main {
			i++
			if keepLinked(rows[i]) {
				linked = append(linked, rows[i])
			}
		}
		if len(linked) == 0 && !keepHead(head) {
			continue
		}
		switch {
		case header != nil:
			blank()
			out, header = append(out, *header), nil
		case !head.Other:
			blank()
		}
		out = append(append(out, head), linked...)
	}
	return out
}

// AttentionRows keeps the rows whose session has a status entry that is not idle, with their spaces' rows
// for context. Working counts, unlike Attention's: the filter is for what is going on, not only for what
// waits for me. A folded space's row stands for all of its sessions, the hidden ones too, and a folded
// group's header for all of its spaces'.
func AttentionRows(rows []Row, statuses []status.Status) []Row {
	stirring := map[string]bool{}
	for _, s := range statuses {
		if s.State != "idle" {
			stirring[s.Session] = true
		}
	}
	return Narrow(rows, func(head Row) bool {
		var sessions []Worktree
		switch {
		case head.Group != nil:
			for _, s := range head.Group.Spaces {
				sessions = slices.Concat(sessions, s.Worktrees, s.Missing)
			}
		case head.Folded:
			sessions = slices.Concat(head.Space.Worktrees, head.Space.Missing)
		case head.Worktree != nil:
			sessions = []Worktree{*head.Worktree}
		}
		return slices.ContainsFunc(sessions, func(wt Worktree) bool { return stirring[wt.Session] })
	}, func(r Row) bool { return stirring[r.Worktree.Session] })
}
