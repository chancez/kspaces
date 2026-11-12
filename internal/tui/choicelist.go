package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/tui/look"
)

// choiceList is the list a choice field opens to choose from: every choice, the one chosen marked and
// selected, those whose names have what is typed in them. It is drawn like the sidebar's menu, below the
// field, or above it when it would run off the bottom. It holds the choices' names and details rather than
// the choices, so one list serves every kind of choice.
type choiceList struct {
	th *look.Theme
	// zone is the field's zone, which the list's zones are named after.
	zone    string
	names   []string
	details []string
	// current is the index in names of the one chosen when the list opened.
	current int
	filter  string
	// matches are the indexes in names of those the filter matches, sel the selected one's index in matches,
	// and offset the index in matches of the first one shown, when they do not all fit.
	matches     []int
	sel, offset int
	// done says the list is to close, picked that it chose the selected one.
	done, picked bool
}

func newChoiceList(th *look.Theme, zone string, names, details []string, current int) *choiceList {
	l := &choiceList{th: th, zone: zone, names: names, details: details, current: current}
	l.refilter()
	return l
}

// refilter matches the names against the filter, keeping the selected one selected if it still matches,
// else the first. The one chosen is selected as the list opens.
func (l *choiceList) refilter() {
	keep := l.current
	if l.matches != nil && l.sel < len(l.matches) {
		keep = l.matches[l.sel]
	}
	l.matches, l.sel, l.offset = []int{}, 0, 0
	f := strings.ToLower(l.filter)
	for i, n := range l.names {
		if strings.Contains(strings.ToLower(n), f) {
			if i == keep {
				l.sel = len(l.matches)
			}
			l.matches = append(l.matches, i)
		}
	}
}

func (l *choiceList) move(delta int) {
	l.sel = max(min(l.sel+delta, len(l.matches)-1), 0)
}

// typed adds s to the filter.
func (l *choiceList) typed(s string) {
	l.filter += s
	l.refilter()
}

// erase takes the last character off the filter.
func (l *choiceList) erase() {
	if r := []rune(l.filter); len(r) > 0 {
		l.filter = string(r[:len(r)-1])
		l.refilter()
	}
}

// pick chooses the selected one, when the filter leaves one.
func (l *choiceList) pick() {
	if len(l.matches) > 0 {
		l.done, l.picked = true, true
	}
}

func (l *choiceList) close() { l.done = true }

// chosen is the index in names of the one picked.
func (l *choiceList) chosen() int { return l.matches[l.sel] }

// key handles a key: a binding of the list's, else typing, which filters. Every other key does nothing
// while the list is open, so none reaches the form.
func (l *choiceList) key(msg tea.KeyPressMsg) tea.Cmd {
	if cmd, ok := runOn(l, msg, theChoiceList.Target(listSubject{})); ok {
		return cmd
	}
	if msg.Text != "" {
		l.typed(msg.Text)
	}
	return nil
}

// rowZone is the zone of the list's row of matches[i], listZone the zone of the rest of it.
func (l *choiceList) rowZone(i int) string { return fmt.Sprintf("%s/row:%d", l.zone, i) }
func (l *choiceList) listZone() string     { return l.zone + "/list" }

// click picks the row clicked, or closes the list for a click outside it; a click on a rule or a mark of
// what is scrolled out of view is a near miss of a row, so it stays.
func (l *choiceList) click(zones map[string][]look.Zone, msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		l.close()
		return
	}
	in := func(id string) bool {
		for _, z := range zones[id] {
			if z.Contains(msg.X, msg.Y) {
				return true
			}
		}
		return false
	}
	for i := range l.matches {
		if in(l.rowZone(i)) {
			l.sel = i
			l.pick()
			return
		}
	}
	if !in(l.listZone()) {
		l.close()
	}
}

// draw draws the list over lines, the form's, from column col to width, below line at, the field's, or
// above it when it would run off the bottom of a screen height rows high, 0 for unknown, where the form
// has a row of padding above and below its lines. The rows it takes are its, blank left of it, as the
// form's text cut there would read as part of it. Lines it runs past the end of are added.
func (l *choiceList) draw(lines []string, at, col, width, height int) []string {
	th, w := l.th, max(width-col, 1)
	rows := max(len(l.matches), 1)
	// A rule above the rows and one below; when they do not all fit, a row above and below them marks how
	// many are out of view.
	below, above := 1<<30, at
	if height > 0 {
		below = height - 2 - (at + 1)
	}
	shown, scroll, top := rows, false, at+1
	switch {
	case rows+2 <= below:
	case rows+2 <= above:
		top = at - (rows + 2)
	default:
		room := max(below, above)
		shown, scroll = max(min(room-4, rows), 1), true
		if above > below {
			top = max(at-(shown+4), 0)
		}
	}
	l.offset = max(min(l.offset, l.sel, len(l.matches)-shown), 0)
	if l.sel >= l.offset+shown {
		l.offset = l.sel - shown + 1
	}
	marked := func(spans ...look.Span) string {
		return th.Render(look.Plain, look.Mark(l.listZone(), spans...)...)
	}
	var head []look.Span
	if l.filter != "" {
		head = []look.Span{look.Text(l.filter)}
	}
	drawn := []string{marked(ruleWith(th, w, head...)...)}
	more := func(above bool, n int) {
		if n > 0 {
			drawn = append(drawn, marked(th.Cut(w, append([]look.Span{look.Text(" ")}, look.More(above, n)...)...)...))
		} else {
			drawn = append(drawn, marked(look.Text(strings.Repeat(" ", w))))
		}
	}
	if scroll {
		more(true, l.offset)
	}
	if len(l.matches) == 0 {
		drawn = append(drawn, marked(th.Cut(w, look.Muted(" no matches"))...))
	}
	for i := l.offset; i < min(l.offset+shown, len(l.matches)); i++ {
		n := l.matches[i]
		surface := look.Plain
		if i == l.sel {
			surface = look.Cursor
		}
		lead := []look.Span{look.Chosen(n == l.current), look.Text(" ")}
		item := []look.Span{look.Text(l.names[n])}
		if d := l.details[n]; d != "" {
			item = append(item, look.Muted("  "+d))
		}
		item = th.Cut(max(w-th.Width(lead...), 0), item...)
		drawn = append(drawn, th.DrawZone(l.rowZone(i), surface, w, append(lead, item...)...))
	}
	if scroll {
		more(false, len(l.matches)-l.offset-shown)
	}
	drawn = append(drawn, marked(ruleWith(th, w)...))
	for i, row := range drawn {
		r := top + i
		for r >= len(lines) {
			lines = append(lines, "")
		}
		lines[r] = strings.Repeat(" ", col) + row
	}
	return lines
}

// ruleWith is a rule width cells long with head on it after a dash. The head is cut to fit, and left out
// with no room for a letter of it, so the rule always spans its width.
func ruleWith(th *look.Theme, width int, head ...look.Span) []look.Span {
	// A dash and a space before the head, and a space and at least a dash after it.
	room := width - 4
	if len(head) == 0 || room < 1 {
		return []look.Span{look.Rule(width, true)}
	}
	name := th.Cut(room, head...)
	spans := append([]look.Span{look.Border("\u2500 ", true)}, name...)
	return append(spans, look.Border(" ", true), look.Rule(width-3-th.Width(name...), true))
}
