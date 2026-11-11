package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/tui/look"
)

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
	lines := [][]look.Span{statusHeader(th, s, width, time.Now())}
	if expanded {
		lines = append(lines, statusDetails(s, inner, 3, 6)...)
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

// popupAdds reports whether status entry j in full says more than its summary row does, so the popup is
// worth covering the entries below for: a row of the details the summary does not show, or shows cut. The
// summary always shows progress, in a bar of its own width.
func (m *sidebar) popupAdds(j int) bool {
	th, s := m.th, m.kitty.Statuses[j]
	inner := max(m.contentWidth()-th.Width(look.NoMarker()), 1)
	shown := th.TextOf(th.Cut(inner, look.Summary(s.Record, inner))...)
	for _, row := range statusDetails(s, inner, 0, 0) {
		text := strings.TrimSpace(th.TextOf(row...))
		if s.Progress != nil && text == look.ProgressBar(*s.Progress, inner) {
			continue
		}
		if !strings.Contains(shown, text) {
			return true
		}
	}
	return false
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
	if m.statusAbove() {
		top++
	}
	for k := m.statusOffset; k < j; k++ {
		top += len(m.statusGroupTitle(k)) + len(m.statusEntry(k))
	}
	top += len(m.statusGroupTitle(j))
	s := m.kitty.Statuses[j]
	popup := []string{th.DrawZone(popupZone, look.Cursor, width, statusHeader(th, s, width, time.Now())...)}
	for _, l := range statusDetails(s, max(width-th.Width(look.NoMarker()), 1), 0, 0) {
		popup = append(popup, th.DrawZone(popupZone, look.Plain, width, l...))
	}
	popup = append(popup, th.DrawZone(popupZone, look.Plain, width, look.Rule(width, true)))
	bottom := l.statusTop + l.status
	top = max(min(top, bottom-len(popup)), headerRows)
	return popup[:min(len(popup), bottom-top)], top
}
