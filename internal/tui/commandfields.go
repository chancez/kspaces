package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// commandFields choose a configured command and fill in its inputs, for the new worktree form and the run
// form. Each command keeps its own text boxes, so switching between commands loses nothing typed.
type commandFields struct {
	th   *look.Theme
	cmds []config.Command
	// optional offers no command, the first choice, as the new worktree form does.
	optional bool
	// sel is the chosen command's index in cmds, -1 for none.
	sel    int
	inputs [][]textinput.Model
}

// commandZone is where the chosen command is drawn, which a click changes.
const commandZone = "command"

// newCommandFields are the fields for cmds, with the one called name chosen, or none or the first when
// there is no such command, and its inputs filled in from values, the rest with their defaults.
func newCommandFields(th *look.Theme, cmds []config.Command, optional bool, name string, values map[string]string) commandFields {
	c := commandFields{th: th, cmds: cmds, optional: optional, sel: -1}
	if !optional && len(cmds) > 0 {
		c.sel = 0
	}
	for i, cmd := range cmds {
		var ins []textinput.Model
		for _, in := range cmd.Inputs {
			box := newInput(th, "")
			v := in.Default
			if given, ok := values[in.Name]; ok && cmd.Name == name {
				v = given
			}
			box.SetValue(v)
			box.CursorEnd()
			if in.Required {
				box.Placeholder = "required"
			}
			ins = append(ins, box)
		}
		if cmd.Name == name {
			c.sel = i
		}
		c.inputs = append(c.inputs, ins)
	}
	return c
}

// picks says whether there is a choice of command to show.
func (c *commandFields) picks() bool {
	return len(c.cmds) > 1 || (c.optional && len(c.cmds) > 0)
}

// chosen is the chosen command, if any.
func (c *commandFields) chosen() (config.Command, bool) {
	if c.sel < 0 {
		return config.Command{}, false
	}
	return c.cmds[c.sel], true
}

// cycle chooses the next command, or the one before, round through none when it is a choice.
func (c *commandFields) cycle(delta int) {
	n, first := len(c.cmds), 0
	if c.optional {
		n, first = n+1, -1
	}
	if n == 0 {
		return
	}
	c.sel = ((c.sel-first+delta)%n+n)%n + first
}

// fields are the chosen command's text boxes, in order.
func (c *commandFields) fields() []*textinput.Model {
	if c.sel < 0 {
		return nil
	}
	out := make([]*textinput.Model, len(c.inputs[c.sel]))
	for i := range c.inputs[c.sel] {
		out[i] = &c.inputs[c.sel][i]
	}
	return out
}

func (c *commandFields) setWidth(w int) {
	for i := range c.inputs {
		for j := range c.inputs[i] {
			c.inputs[i][j].SetWidth(w)
		}
	}
}

// run is the chosen command to run with what its fields have, or why it cannot.
func (c *commandFields) run() (spaces.Run, error) {
	cmd, _ := c.chosen()
	values := map[string]string{}
	for i, in := range cmd.Inputs {
		values[in.Name] = c.inputs[c.sel][i].Value()
	}
	return spaces.NewRun(cmd, values)
}

// lines draw the choice of command, when there is one, focused or not, and the chosen command's inputs.
func (c *commandFields) lines(selFocused bool) []string {
	th := c.th
	label := func(s string) string { return th.DrawCut(look.Plain, labelWidth-1, look.Label(s)) + " " }
	var out []string
	if c.picks() {
		name := "none"
		if cmd, ok := c.chosen(); ok {
			name = cmd.Name
		}
		arrow := look.Muted
		if selFocused {
			arrow = look.Key
		}
		spans := look.Mark(commandZone, arrow("<"), look.Text(" "+name+" "), arrow(">"))
		out = append(out, label("command")+th.Render(look.Plain, spans...))
	}
	if cmd, ok := c.chosen(); ok {
		for i, in := range cmd.Inputs {
			out = append(out, label(in.Label)+c.inputs[c.sel][i].View())
		}
	}
	return out
}

// clickedCommand says whether msg is a click on the chosen command, in the frame zones were scanned from.
func clickedCommand(zones map[string][]look.Zone, msg tea.MouseClickMsg) bool {
	if msg.Button != tea.MouseLeft {
		return false
	}
	for _, z := range zones[commandZone] {
		if z.Contains(msg.X, msg.Y) {
			return true
		}
	}
	return false
}

// moveFocus moves focus by delta over fields, round from the last to the first, from *focus, which it
// updates. A nil field is the choice of command, which takes no typing.
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
