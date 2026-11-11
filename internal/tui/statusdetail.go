package tui

import (
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// What a status entry says, shared by the sidebar's status list and the picker's preview.

// statusHeader is the first row of a status entry, width cells wide: its marker, label, and on the right
// its age at now.
func statusHeader(th *look.Theme, s status.Status, width int, now time.Time) []look.Span {
	mark, right := look.StateMarker(s.State), look.StatusAge(s, now)
	label := th.Cut(max(width-th.Width(mark...)-th.Width(right), 0), look.StatusLabel(s))
	pad := look.Default(strings.Repeat(" ", max(width-th.Width(mark...)-th.Width(label...)-th.Width(right), 0)))
	return append(append(append(mark, label...), pad), right)
}

// statusDetails is everything a status entry has to say, indented under its label and wrapped to inner
// columns: the program, what a blocked one waits for, the title, progress, the message, the record's id
// and the window's other records. Title and message are cut at titleRows and msgRows, 0 for no limit.
func statusDetails(s status.Status, inner, titleRows, msgRows int) [][]look.Span {
	var lines [][]look.Span
	add := func(x look.Span, maxRows int) {
		for i, row := range x.Wrapped(inner) {
			if maxRows > 0 && i == maxRows {
				break
			}
			lines = append(lines, []look.Span{look.NoMarker(), row})
		}
	}
	add(look.Muted(s.App), 1)
	add(look.WaitsFor(s.Record), 1)
	add(look.Text(s.Title), titleRows)
	if s.Progress != nil {
		add(look.Text(look.ProgressBar(*s.Progress, inner)), 1)
	}
	add(look.Muted(s.Msg), msgRows)
	if s.ID != "" {
		add(look.Muted("id "+s.ID), 1)
	}
	now := time.Now()
	for _, o := range s.Others {
		name := o.ID
		if name == "" {
			name = o.App
		}
		add(look.Muted(strings.Join(strings.Fields(strings.Join([]string{o.State, name, status.Age(o.Since, now), o.Title}, " ")), " ")), 1)
	}
	return lines
}
