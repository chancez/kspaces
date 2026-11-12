package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/tui/look"
)

// field is a form's text box, which typing goes to while it has focus.
type field interface {
	Focus() tea.Cmd
	Blur()
	View() string
	Value() string
	SetWidth(int)
	// fill sets the value, the cursor at its end.
	fill(string)
	// update hands msg, typing, to the box.
	update(tea.Msg) tea.Cmd
	// place puts the cursor where a click on cell x of row y of the box, as View draws it, lands.
	place(x, y int)
}

// lineField is one line of text, which scrolls sideways, for a branch, a path or a name.
type lineField struct{ textinput.Model }

// newLineField is a line field in th's styles showing placeholder while empty.
func newLineField(th *look.Theme, placeholder string) lineField {
	in := newInput(th, "")
	in.Placeholder = placeholder
	return lineField{in}
}

func (f *lineField) fill(s string) {
	f.SetValue(s)
	f.CursorEnd()
}

func (f *lineField) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.Model, cmd = f.Model.Update(msg)
	return cmd
}

// place puts the cursor on the character at cell x, or after the last, when the whole value shows. One too
// long for the box may be scrolled, which the box does not say, so the cursor goes to its end.
func (f *lineField) place(x, _ int) {
	v := f.Value()
	if ansi.StringWidth(v) > f.Width() {
		f.CursorEnd()
		return
	}
	at := 0
	for _, r := range v {
		w := ansi.StringWidth(string(r))
		if x < w {
			break
		}
		x -= w
		at++
	}
	f.SetCursor(at)
}

// fieldZone is the zone fl's row is drawn in: its address, as every field has its own.
func fieldZone(fl field) string { return fmt.Sprintf("field:%p", fl) }

// fieldLines draw fl after its label, labelWidth cells, a line for each of its box's, in width cells. Each
// is marked as fl's zone, so a click on the label or in the box finds it, see clickedField.
func fieldLines(th *look.Theme, label string, fl field, width int) []string {
	head := th.DrawCut(look.Plain, labelWidth-1, look.Label(label)) + " "
	var out []string
	for _, row := range strings.Split(fl.View(), "\n") {
		line := head + row
		out = append(out, th.DrawZone(fieldZone(fl), look.Plain, width, look.Styled(func(color.Color) string { return line })))
		head = strings.Repeat(" ", labelWidth)
	}
	return out
}

// clickedField is the index in fields, nil for a choice, of the one whose lines, as fieldLines draws them,
// msg is a left click on, in the frame zones were scanned from. A click in the box puts its cursor there.
func clickedField(fields []field, zones map[string][]look.Zone, msg tea.MouseClickMsg) (int, bool) {
	if msg.Button != tea.MouseLeft {
		return 0, false
	}
	for i, fl := range fields {
		if fl == nil {
			continue
		}
		for row, z := range zones[fieldZone(fl)] {
			if !z.Contains(msg.X, msg.Y) {
				continue
			}
			if x := msg.X - z.X - labelWidth; x >= 0 {
				fl.place(x, row)
			}
			return i, true
		}
	}
	return 0, false
}

// moveFocus moves focus by delta over fields, round from the last to the first, from *focus, which it
// updates. A nil field is a choice, which takes no typing.
func moveFocus(fields []field, focus *int, delta int) tea.Cmd {
	for _, in := range fields {
		if in != nil {
			in.Blur()
		}
	}
	if len(fields) == 0 {
		return nil
	}
	*focus = ((*focus+delta)%len(fields) + len(fields)) % len(fields)
	if in := fields[*focus]; in != nil {
		return in.Focus()
	}
	return nil
}
