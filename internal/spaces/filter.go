package spaces

import (
	"slices"

	"github.com/chancez/kspaces/internal/status"
)

// Narrow keeps the groups of rows, as BuildRows lists them, that keep says to: a linked or missing
// worktree when keepLinked says so, and a space's own row when keepHead says so or when any of its rows is
// kept, so a kept worktree shows under its space. Blank rows go between groups as BuildRows puts them.
func Narrow(rows []Row, keepHead, keepLinked func(Row) bool) []Row {
	var out []Row
	for i := 0; i < len(rows); i++ {
		head := rows[i]
		if head.Space == nil {
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
		if !head.Other && len(out) > 0 {
			out = append(out, Row{})
		}
		out = append(append(out, head), linked...)
	}
	return out
}

// AttentionRows keeps the rows whose session has a status entry that is not idle, with their spaces' rows
// for context. Working counts, unlike Attention's: the filter is for what is going on, not only for what
// waits for me. A folded space's row stands for all of its sessions, the hidden ones too.
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
		case head.Folded:
			sessions = slices.Concat(head.Space.Worktrees, head.Space.Missing)
		case head.Worktree != nil:
			sessions = []Worktree{*head.Worktree}
		}
		return slices.ContainsFunc(sessions, func(wt Worktree) bool { return stirring[wt.Session] })
	}, func(r Row) bool { return stirring[r.Worktree.Session] })
}
