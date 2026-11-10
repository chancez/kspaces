package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// statusRows is how many rows status entries from through to take, with their group titles.
func (m *sidebar) statusRows(from, to int) int {
	n := 0
	for j := from; j <= to; j++ {
		n += len(m.statusGroupTitle(j)) + len(m.statusEntry(j))
	}
	return n
}

// statusGroupTitle is drawn above entry j when it starts a group of entries in one state, or is the first
// one shown after scrolling, so a scrolled list still says what it shows. Later groups get a blank row
// above their title.
func (m *sidebar) statusGroupTitle(j int) []string {
	all := m.kitty.Statuses
	if j != m.statusOffset && all[j-1].State == all[j].State {
		return nil
	}
	n := 0
	for _, s := range all {
		if s.State == all[j].State {
			n++
		}
	}
	width := m.contentWidth()
	title := m.th.Draw(look.Plain, width, look.StateGroup(all[j].State, n)...)
	if j == m.statusOffset {
		return []string{title}
	}
	return []string{m.th.Draw(look.Plain, width), title}
}

// statusEntry draws status entry j, marked as its entry. The first row has its state icon, the session it
// belongs to, or the window title for a window in no session, and how long ago the record changed to its
// state. Below it is the program's name and what it said: its progress, its title or else its message.
// With the cursor on it, the entry opens up to show all of it: what a blocked program waits for, title,
// progress, the whole message, the record's id and the window's other records.
func (m *sidebar) statusEntry(j int) []string {
	th, s, width := m.th, m.kitty.Statuses[j], m.contentWidth()
	inner := max(width-th.Width(look.NoMarker()), 1)
	zone := fmt.Sprintf(entryZone, len(m.rows)+j)
	expanded := len(m.rows)+j == m.cursor && m.navigating()
	lines := [][]look.Span{m.statusHeader(j)}
	if expanded {
		lines = append(lines, m.statusDetails(s, inner, 3, 6)...)
	} else if sum := th.Cut(inner, look.Summary(s.Record, inner)); th.Width(sum...) > 0 {
		lines = append(lines, append([]look.Span{look.NoMarker()}, sum...))
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		surface := look.Plain
		if i == 0 && expanded {
			surface = look.Cursor
		}
		out[i] = th.DrawZone(zone, surface, width, l...)
	}
	return out
}

// statusHeader is the first row of status entry j: its marker, label, and on the right its age.
func (m *sidebar) statusHeader(j int) []look.Span {
	th, s, width := m.th, m.kitty.Statuses[j], m.contentWidth()
	mark, right := look.StateMarker(s.State), look.StatusAge(s, time.Now())
	label := th.Cut(max(width-th.Width(mark...)-th.Width(right), 0), look.StatusLabel(s))
	pad := look.Default(strings.Repeat(" ", max(width-th.Width(mark...)-th.Width(label...)-th.Width(right), 0)))
	return append(append(append(mark, label...), pad), right)
}

// statusDetails is everything a status entry has to say, indented under its label and wrapped to inner
// columns: the program, what a blocked one waits for, the title, progress, the message, the record's id
// and the window's other records. Title and message are cut at titleRows and msgRows, 0 for no limit.
func (m *sidebar) statusDetails(s status.Status, inner, titleRows, msgRows int) [][]look.Span {
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

// hovered is the index of the status entry under the mouse, or -1. Kept by window rather than by index, as
// a refresh can reorder the list under a resting mouse.
func (m *sidebar) hovered() int {
	if m.hover == 0 {
		return -1
	}
	for j, s := range m.kitty.Statuses {
		if s.Window == m.hover {
			return j
		}
	}
	return -1
}

// hoverPopup is status entry j in full, for the mouse resting on it, and the screen row it starts at. It
// covers the entries below it so the list keeps its layout, with a rule closing it off from what it covers,
// and moves up over the entries above when it would run off the bottom. All of it is marked, so the mouse
// on it is on the popup rather than on what it covers.
func (m *sidebar) hoverPopup(j int) ([]string, int) {
	th, l, width := m.th, m.layout(), m.contentWidth()
	if j < m.statusOffset || j >= len(m.kitty.Statuses) {
		return nil, 0
	}
	top := l.statusTop
	for k := m.statusOffset; k < j; k++ {
		top += len(m.statusGroupTitle(k)) + len(m.statusEntry(k))
	}
	top += len(m.statusGroupTitle(j))
	popup := []string{th.DrawZone(popupZone, look.Cursor, width, m.statusHeader(j)...)}
	for _, l := range m.statusDetails(m.kitty.Statuses[j], max(width-th.Width(look.NoMarker()), 1), 0, 0) {
		popup = append(popup, th.DrawZone(popupZone, look.Plain, width, l...))
	}
	popup = append(popup, th.DrawZone(popupZone, look.Plain, width, look.Rule(width, true)))
	bottom := l.statusTop + l.status
	top = max(min(top, bottom-len(popup)), headerRows)
	return popup[:min(len(popup), bottom-top)], top
}
