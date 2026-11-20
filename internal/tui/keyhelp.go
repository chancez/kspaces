package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The keys ? lists: what each subject where it was pressed takes, read off the chain its keys are looked
// up on, see interact.Help, so the list is of what a key would run there now. The sidebar draws it over
// itself.

// keyHelp is the open list of keys, scrolled offset lines down.
type keyHelp struct {
	// sections are the keys, read again for every frame, so what applies stays current.
	sections func() []interact.Section
	offset   int
	// last is the furthest offset scrolls to, as the last frame drew the list.
	last int
	// closed is whether a key or a click closed it, for what drew it to drop it.
	closed bool
}

// helpOf is the list of the keys of chain to m.
func helpOf[M any](m M, chain ...interact.Target[M]) *keyHelp {
	return &keyHelp{sections: func() []interact.Section { return interact.Help(m, chain...) }}
}

// helpBody draws sections width cells wide: each subject's rule with its label, then a line for each of
// its bindings, the keys in a column as wide as the widest, up to a third of width, and what they do,
// wrapped under itself.
func helpBody(th *look.Theme, sections []interact.Section, width int) []string {
	kw := 0
	for _, s := range sections {
		for _, l := range s.Lines {
			kw = max(kw, th.Width(look.Key(l.Key)))
		}
	}
	kw = min(kw, max(width/3, 1))
	indent := look.Text(strings.Repeat(" ", 1+kw+1))
	var out []string
	for _, s := range sections {
		out = append(out, drawMenuRule(th, s.Label, width))
		for _, l := range s.Lines {
			key := th.Cut(kw, look.Key(l.Key))
			lead := append(append([]look.Span{look.Text(" ")}, key...), look.Text(strings.Repeat(" ", kw-th.Width(key...)+1)))
			for i, d := range look.WrapWords(l.Desc, max(width-th.Width(indent), 1)) {
				if i > 0 {
					lead = []look.Span{indent}
				}
				out = append(out, th.Render(look.Plain, th.Cut(width, append(lead, look.KeyDesc(d))...)...))
			}
		}
	}
	return out
}

// lines draws the help in width by height cells: a title saying how to close it, and scroll it when it
// needs to, then the keys
// from offset, which it keeps in range, a line at either end saying how many more there are past it, and
// blank lines to the bottom.
func (h *keyHelp) lines(th *look.Theme, width, height int) []string {
	body := helpBody(th, h.sections(), width)
	rows := max(height-1, 0)
	h.last = max(len(body)-rows, 0)
	h.offset = min(h.offset, h.last)
	title := look.Title("KEYS")
	hints := [][]look.Span{{look.Key("?"), look.KeyDesc(" close")}}
	if h.last > 0 {
		hints = append([][]look.Span{{look.Key("j/k"), look.KeyDesc(" scroll  "), look.Key("?"), look.KeyDesc(" close")}}, hints...)
	}
	var hint []look.Span
	for _, spans := range hints {
		if th.Width(title)+1+th.Width(spans...) <= width {
			hint = spans
			break
		}
	}
	gap := look.Text(strings.Repeat(" ", max(width-th.Width(title)-th.Width(hint...), 0)))
	out := []string{th.Draw(look.Plain, width, append([]look.Span{title, gap}, hint...)...)}
	shown := append([]string{}, body[h.offset:min(len(body), h.offset+rows)]...)
	if n := h.offset; n > 0 && len(shown) > 0 {
		shown[0] = th.Render(look.Plain, th.Cut(width, look.More(true, n+1)...)...)
	}
	if n := len(body) - h.offset - rows; n > 0 && len(shown) > 1 {
		shown[len(shown)-1] = th.Render(look.Plain, th.Cut(width, look.More(false, n+1)...)...)
	}
	// Blank to the bottom, so nothing under the list shows through.
	for len(shown) < rows {
		shown = append(shown, "")
	}
	return append(out, shown...)
}

// scroll moves the list by delta lines, up to its ends.
func (h *keyHelp) scroll(delta int) { h.offset = max(min(h.offset+delta, h.last), 0) }

// update handles input while the list is open, which takes the keys and the mouse: it scrolls, and closes
// on a click or its keys, which leaves closed true. It reports false for what is not input.
func (h *keyHelp) update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		cmd, _ := runOn(h, msg, theHelp.Target(helpSubject{}))
		return cmd, true
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			h.scroll(-1)
		case tea.MouseWheelDown:
			h.scroll(1)
		}
		return nil, true
	case tea.MouseClickMsg:
		h.closed = true
		return nil, true
	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return nil, true
	}
	return nil, false
}

// openHelp opens the list of the keys of chain over the sidebar.
func (m *sidebar) openHelp(chain ...interact.Target[*sidebar]) {
	m.help = helpOf(m, chain...)
	m.unhover()
}

// updateHelp handles input while the list of keys is open, see keyHelp.update, and closes it when the
// sidebar loses focus. It reports false for what should reach the sidebar as usual.
func (m *sidebar) updateHelp(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(tea.BlurMsg); ok {
		m.help = nil
		return nil, false
	}
	cmd, handled := m.help.update(msg)
	if m.help.closed {
		m.help = nil
	}
	return cmd, handled
}

// drawHelp draws the open list of keys over lines.
func (m *sidebar) drawHelp(lines []string) {
	if m.help != nil {
		copy(lines, m.help.lines(m.th, m.contentWidth(), len(lines)))
	}
}
