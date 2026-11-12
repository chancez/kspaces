package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui/look"
)

// choiceFields choose one of the configured commands or layouts, T, and fill in its inputs, for the new
// worktree form, the run form and the layout form. Each choice keeps its own text boxes, so switching
// between them loses nothing typed.
type choiceFields[T any] struct {
	th *look.Theme
	// label is what the form calls the choice, and the zone it is drawn in, which a click changes.
	label   string
	choices []T
	nameOf  func(T) string
	inputOf func(T) []config.Input
	// optional offers none, the first choice, as the new worktree form does.
	optional bool
	// sel is the chosen one's index in choices, -1 for none.
	sel    int
	inputs [][]textinput.Model
}

// The zones the choices are drawn in.
const (
	commandZone = "command"
	layoutZone  = "layout"
)

// newCommandFields are the fields for cmds, see newChoiceFields.
func newCommandFields(th *look.Theme, cmds []config.Command, optional bool, name string, values map[string]string) choiceFields[config.Command] {
	return newChoiceFields(th, commandZone, cmds, func(c config.Command) string { return c.Name },
		func(c config.Command) []config.Input { return c.Inputs }, optional, name, values)
}

// newLayoutFields are the fields for layouts, see newChoiceFields.
func newLayoutFields(th *look.Theme, layouts []config.Layout, optional bool, name string, values map[string]string) choiceFields[config.Layout] {
	return newChoiceFields(th, layoutZone, layouts, func(l config.Layout) string { return l.Name },
		func(l config.Layout) []config.Input { return l.Inputs }, optional, name, values)
}

// newChoiceFields are the fields for choices, with the one called name chosen, or none or the first when
// there is no such choice, and its inputs filled in from values, the rest with their defaults.
func newChoiceFields[T any](th *look.Theme, label string, choices []T, nameOf func(T) string, inputOf func(T) []config.Input, optional bool, name string, values map[string]string) choiceFields[T] {
	c := choiceFields[T]{th: th, label: label, choices: choices, nameOf: nameOf, inputOf: inputOf, optional: optional, sel: -1}
	if !optional && len(choices) > 0 {
		c.sel = 0
	}
	for i, ch := range choices {
		var ins []textinput.Model
		for _, in := range inputOf(ch) {
			box := newInput(th, "")
			v := in.Default
			if given, ok := values[in.Name]; ok && nameOf(ch) == name {
				v = given
			}
			box.SetValue(v)
			box.CursorEnd()
			if in.Required {
				box.Placeholder = "required"
			}
			ins = append(ins, box)
		}
		if nameOf(ch) == name {
			c.sel = i
		}
		c.inputs = append(c.inputs, ins)
	}
	return c
}

// picks says whether there is a choice to show.
func (c *choiceFields[T]) picks() bool {
	return len(c.choices) > 1 || (c.optional && len(c.choices) > 0)
}

// chosen is the one chosen, if any.
func (c *choiceFields[T]) chosen() (T, bool) {
	if c.sel < 0 {
		var none T
		return none, false
	}
	return c.choices[c.sel], true
}

// cycle chooses the next, or the one before, round through none when that is a choice.
func (c *choiceFields[T]) cycle(delta int) {
	n, first := len(c.choices), 0
	if c.optional {
		n, first = n+1, -1
	}
	if n == 0 {
		return
	}
	c.sel = ((c.sel-first+delta)%n+n)%n + first
}

// fields are the chosen one's text boxes, in order.
func (c *choiceFields[T]) fields() []*textinput.Model {
	if c.sel < 0 {
		return nil
	}
	out := make([]*textinput.Model, len(c.inputs[c.sel]))
	for i := range c.inputs[c.sel] {
		out[i] = &c.inputs[c.sel][i]
	}
	return out
}

func (c *choiceFields[T]) setWidth(w int) {
	for i := range c.inputs {
		for j := range c.inputs[i] {
			c.inputs[i][j].SetWidth(w)
		}
	}
}

// values are what the chosen one's fields have, by input name.
func (c *choiceFields[T]) values() map[string]string {
	ch, ok := c.chosen()
	if !ok {
		return nil
	}
	out := map[string]string{}
	for i, in := range c.inputOf(ch) {
		out[in.Name] = c.inputs[c.sel][i].Value()
	}
	return out
}

// lines draw the choice, when there is one, focused or not, and the chosen one's inputs.
func (c *choiceFields[T]) lines(selFocused bool) []string {
	th := c.th
	label := func(s string) string { return th.DrawCut(look.Plain, labelWidth-1, look.Label(s)) + " " }
	var out []string
	if c.picks() {
		name := "none"
		if ch, ok := c.chosen(); ok {
			name = c.nameOf(ch)
		}
		arrow := look.Muted
		if selFocused {
			arrow = look.Key
		}
		spans := look.Mark(c.label, arrow("<"), look.Text(" "+name+" "), arrow(">"))
		out = append(out, label(c.label)+th.Render(look.Plain, spans...))
	}
	if ch, ok := c.chosen(); ok {
		for i, in := range c.inputOf(ch) {
			out = append(out, label(in.Label)+c.inputs[c.sel][i].View())
		}
	}
	return out
}

// clickedChoice says whether msg is a click on the choice drawn in zone, in the frame zones were scanned
// from.
func clickedChoice(zones map[string][]look.Zone, zone string, msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft {
		return false
	}
	for _, z := range zones[zone] {
		if z.Contains(msg.X, msg.Y) {
			return true
		}
	}
	return false
}

// moveFocus moves focus by delta over fields, round from the last to the first, from *focus, which it
// updates. A nil field is a choice, which takes no typing.
func moveFocus(fields []*textinput.Model, focus *int, delta int) tea.Cmd {
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
