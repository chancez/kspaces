package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui/look"
)

// choiceFields choose one of the configured commands or layouts, T, and fill in its inputs, for the new
// worktree form, the run form and the layout form. Each choice keeps its own text boxes, so switching
// between them loses nothing typed. left and right go through the choices, and enter, space or a click
// opens a list of them to choose from.
type choiceFields[T any] struct {
	th *look.Theme
	// label is what the form calls the choice.
	label string
	// zone is the zone it is drawn in, which a click changes: the label, unless a form has several choices
	// with the same label.
	zone    string
	choices []T
	nameOf  func(T) string
	inputOf func(T) []config.Input
	// detailOf says a little about a choice, after its name in the list; nil for nothing.
	detailOf func(T) string
	// optional offers none, the first choice, as the new worktree form does.
	optional bool
	// sel is the chosen one's index in choices, -1 for none.
	sel    int
	inputs [][]field
	// list is the list of choices, while it is open.
	list *choiceList
}

// The zones the choices are drawn in.
const (
	commandZone = "command"
	layoutZone  = "layout"
)

// newCommandFields are the fields for cmds, see newChoiceFields. The list says what each runs.
func newCommandFields(th *look.Theme, cmds []config.Command, optional bool, name string, values map[string]string) choiceFields[config.Command] {
	return newChoiceFields(th, commandZone, cmds, func(c config.Command) string { return c.Name },
		func(c config.Command) []config.Input { return c.Inputs }, commandDetail, optional, name, values)
}

// commandDetail is the command line c runs.
func commandDetail(c config.Command) string { return strings.Join(c.Run, " ") }

// newLayoutFields are the fields for layouts, see newChoiceFields. The list says how many tabs and panes
// each has.
func newLayoutFields(th *look.Theme, layouts []config.Layout, optional bool, name string, values map[string]string) choiceFields[config.Layout] {
	return newChoiceFields(th, layoutZone, layouts, func(l config.Layout) string { return l.Name },
		func(l config.Layout) []config.Input { return l.Inputs }, layoutDetail, optional, name, values)
}

// layoutDetail is how many tabs and panes l has.
func layoutDetail(l config.Layout) string {
	panes := 0
	for _, t := range l.Tabs {
		panes += len(t.Panes)
	}
	return fmt.Sprintf("%s, %s", plural(len(l.Tabs), "tab"), plural(panes, "pane"))
}

// newChoiceFields are the fields for choices, with the one called name chosen, or none or the first when
// there is no such choice, and its inputs filled in from values, the rest with their defaults.
func newChoiceFields[T any](th *look.Theme, label string, choices []T, nameOf func(T) string, inputOf func(T) []config.Input, detailOf func(T) string, optional bool, name string, values map[string]string) choiceFields[T] {
	c := choiceFields[T]{th: th, label: label, zone: label, choices: choices, nameOf: nameOf, inputOf: inputOf, detailOf: detailOf, optional: optional, sel: -1}
	if !optional && len(choices) > 0 {
		c.sel = 0
	}
	for i, ch := range choices {
		var ins []field
		for _, in := range inputOf(ch) {
			placeholder := ""
			if in.Required {
				placeholder = "required"
			}
			box := newLineField(th, placeholder)
			v := in.Default
			if given, ok := values[in.Name]; ok && nameOf(ch) == name {
				v = given
			}
			box.fill(v)
			ins = append(ins, &box)
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
func (c *choiceFields[T]) fields() []field {
	if c.sel < 0 {
		return nil
	}
	return c.inputs[c.sel]
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

// lines draw the choice, when there is one, focused or not, and the chosen one's inputs, in width cells.
// The focused choice says how to change it, as width leaves room.
func (c *choiceFields[T]) lines(selFocused bool, width int) []string {
	th := c.th
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
		spans := look.Mark(c.zone, arrow("<"), look.Text(" "+name+" "), arrow(">"))
		if room := width - labelWidth - th.Width(spans...); selFocused && room > 0 {
			spans = append(spans, th.Cut(room, look.Muted("  left/right change  enter list"))...)
		}
		out = append(out, th.DrawCut(look.Plain, labelWidth-1, look.Label(c.label))+" "+th.Render(look.Plain, spans...))
	}
	if ch, ok := c.chosen(); ok {
		for i, in := range c.inputOf(ch) {
			out = append(out, fieldLines(th, in.Label, c.inputs[c.sel][i], width)...)
		}
	}
	return out
}

// openList opens the list of choices, the one chosen selected.
func (c *choiceFields[T]) openList() {
	var names, details []string
	if c.optional {
		names, details = append(names, "none"), append(details, "")
	}
	for _, ch := range c.choices {
		d := ""
		if c.detailOf != nil {
			d = c.detailOf(ch)
		}
		names, details = append(names, c.nameOf(ch)), append(details, d)
	}
	c.list = newChoiceList(c.th, c.zone, names, details, c.sel+c.first())
}

// first is the index in the list of the first choice: none comes before it when it is a choice.
func (c *choiceFields[T]) first() int {
	if c.optional {
		return 1
	}
	return 0
}

// listOpen says whether the list of choices is open.
func (c *choiceFields[T]) listOpen() bool { return c.list != nil }

// updateList hands msg, a key or the mouse, to the open list, and reports whether it picked a choice,
// for the form to move focus as a change of choice does.
func (c *choiceFields[T]) updateList(msg tea.Msg, zones map[string][]look.Zone) (tea.Cmd, bool) {
	l := c.list
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		cmd = l.key(msg)
	case tea.MouseClickMsg:
		l.click(zones, msg)
	}
	if !l.done {
		return cmd, false
	}
	c.list = nil
	if l.picked {
		c.sel = l.chosen() - c.first()
	}
	return cmd, l.picked
}

// drawList draws the open list over lines, below line at, the choice's, in a form width cells wide on a
// screen height rows high, the form's key hints on a row of their own after lines.
func (c *choiceFields[T]) drawList(lines []string, at, width, height int) []string {
	if c.list == nil {
		return lines
	}
	if height > 0 {
		height--
	}
	return c.list.draw(lines, at, labelWidth, width, height)
}

// listHints are the keys of an open list, for a form's key hints.
func listHints() []look.Span {
	return []look.Span{look.Key("up/down"), look.KeyDesc(" move  "), look.Key("type"), look.KeyDesc(" filter  "),
		look.Key("enter"), look.KeyDesc(" choose  "), look.Key("esc"), look.KeyDesc(" close")}
}

// choiceHints are the keys of a focused choice, what changes called what, for a form's key hints.
func choiceHints(what string) []look.Span {
	return []look.Span{look.Text("  "), look.Key("left/right"), look.KeyDesc(" " + what), look.Text("  "), look.Key("enter/space"), look.KeyDesc(" list")}
}

// isInput says whether msg is a key or the mouse, which an open list takes from its form.
func isInput(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.MouseMsg, tea.PasteMsg:
		return true
	}
	return false
}

// zoneName is the zone the choice is drawn in.
func (c *choiceFields[T]) zoneName() string { return c.zone }

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
