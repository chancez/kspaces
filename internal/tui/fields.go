package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

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
