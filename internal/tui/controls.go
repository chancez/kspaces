package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A control is an action drawn as a button, which a click runs. As in gr, it is found on the target its key
// runs it on, so a click does what the key does there, the button shows that key, and it is drawn only
// while the action applies.

// control is an action to draw as a button.
type control struct {
	action any
	label  string
	// danger draws the button to stand out, for what loses work.
	danger bool
	// off greys the button and leaves it unclickable, for an action that cannot succeed.
	off bool
}

const controlZone = "control:"

// controls are the buttons a form drew in its last frame, by zone id, and what each runs.
type controls[M any] map[string]interact.Bound[M]

// draw draws the controls of cs that apply on t, a space apart, marked for a click, and records what each
// runs. The zone id names the action, so a button keeps its id from frame to frame.
func (cm controls[M]) draw(m M, t interact.Target[M], cs ...control) []look.Span {
	keys := interact.Keys(m, t)
	var out []look.Span
	for _, c := range cs {
		if c.off {
			out = append(gap(out), look.DisabledButton(c.label)...)
			continue
		}
		b, key, ok := offered(m, t, keys, c.action)
		if !ok {
			continue
		}
		out = gap(out)
		spans := look.Button(c.label, key)
		if c.danger {
			spans = look.DangerButton(c.label, key)
		}
		id := controlZone + b.ID()
		cm[id] = b
		out = append(out, look.Mark(id, spans...)...)
	}
	return out
}

// gap is out with a space after it, to set the next button apart, unless it is empty.
func gap(out []look.Span) []look.Span {
	if len(out) == 0 {
		return out
	}
	return append(out, look.Text("  "))
}

// click runs the control drawn where msg is in the frame zones were scanned from, if any.
func (cm controls[M]) click(m M, zones map[string][]look.Zone, msg tea.MouseClickMsg) (tea.Cmd, bool) {
	if msg.Button != tea.MouseLeft {
		return nil, false
	}
	for id, b := range cm {
		for _, z := range zones[id] {
			if z.Contains(msg.X, msg.Y) {
				return b.Run(m), true
			}
		}
	}
	return nil, false
}

// offered is action a bound on t, and the first key that runs it there, if any, while it applies.
func offered[M any](m M, t interact.Target[M], keys []interact.Key[M], a any) (interact.Bound[M], string, bool) {
	for _, k := range keys {
		if k.Action.Is(a) {
			// A button shows one key: its help can cover two bindings, as "n/esc" does.
			return k.Action, k.Keys()[0], true
		}
	}
	for _, g := range t.Groups(m) {
		for _, b := range g.Actions {
			if b.Is(a) {
				return b, "", true
			}
		}
	}
	return interact.Bound[M]{}, "", false
}
