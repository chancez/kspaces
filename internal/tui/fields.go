package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
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
	// keeps says whether the box takes msg rather than the form, as up and down between its lines.
	keeps(msg tea.KeyPressMsg) bool
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

func (*lineField) keeps(tea.KeyPressMsg) bool { return false }

// textField is text that can take several lines, for the inputs of commands and layouts, which go to a
// program as they are. shift+enter starts a line, or alt+enter or ctrl+j, for a terminal that sends
// shift+enter as enter, which the form keeps for submitting. The box grows to show the lines, wrapped, up
// to textLines, then scrolls with the cursor.
type textField struct{ textarea.Model }

// textLines is the most lines a text field shows.
const textLines = 6

// newTextField is a text field in th's styles showing placeholder while empty.
func newTextField(th *look.Theme, placeholder string) textField {
	in := textarea.New()
	in.Prompt = ""
	in.ShowLineNumbers = false
	in.Placeholder = placeholder
	in.DynamicHeight = true
	in.MaxHeight = textLines
	// Unset, MaxHeight would also stop the lines at textLines; this is textarea's own limit.
	in.MaxContentHeight = 10000
	in.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	in.SetStyles(th.AreaStyles())
	return textField{in}
}

func (f *textField) fill(s string) {
	f.SetValue(s)
	f.MoveToEnd()
}

func (f *textField) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.Model, cmd = f.Model.Update(msg)
	return cmd
}

// place puts the cursor where the click lands, which textarea works out through wrapping and scrolling.
func (f *textField) place(x, y int) {
	f.BeginSelection(x, y)
	f.EndSelection()
}

// keeps up and down while they move between the box's lines, so only up on its first and down on its
// last go on to the form, to the field above or below.
func (f *textField) keeps(msg tea.KeyPressMsg) bool {
	li := f.LineInfo()
	switch msg.String() {
	case "up":
		return f.Line() > 0 || li.RowOffset > 0
	case "down":
		return f.Line() < f.LineCount()-1 || li.RowOffset < li.Height-1
	}
	return false
}

// keptBy says whether fl, the focused field, nil for a choice, keeps msg from the form's keys.
func keptBy(fl field, msg tea.KeyPressMsg) bool { return fl != nil && fl.keeps(msg) }

// newlineHints are the keys that start a line in fl, for a form's key hints, when it takes several.
func newlineHints(fl field) []look.Span {
	if _, ok := fl.(*textField); !ok {
		return nil
	}
	return []look.Span{look.Text("  "), look.Key("shift+enter"), look.KeyDesc(" newline")}
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
