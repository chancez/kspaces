package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/tui/look"
)

// popupModel is a form or the picker, as a popup runs it.
type popupModel interface {
	tea.Model
	// working is whether it is doing what it was asked. A blur then may be its own doing, as switching
	// sessions takes the keyboard, and quitting would cut it short.
	working() bool
	// failed is whether what it last did failed, which it stays open to show.
	failed() bool
}

func (f *addSpaceForm) working() bool       { return f.busy }
func (f *cleanupForm) working() bool        { return f.busy }
func (f *closeSessionForm) working() bool   { return f.busy }
func (f *forgetSessionForm) working() bool  { return f.busy }
func (f *newWorktreeForm) working() bool    { return f.busy }
func (f *layoutForm) working() bool         { return f.busy }
func (f *removeWorktreeForm) working() bool { return f.busy }
func (f *runForm) working() bool            { return f.busy }
func (f *saveLayoutForm) working() bool     { return f.busy }
func (f *trustForm) working() bool          { return f.busy }
func (m *picker) working() bool             { return m.choosing }

func (f *addSpaceForm) failed() bool       { return f.err != nil }
func (f *cleanupForm) failed() bool        { return f.err != nil }
func (f *closeSessionForm) failed() bool   { return f.err != nil }
func (f *forgetSessionForm) failed() bool  { return f.err != nil }
func (f *newWorktreeForm) failed() bool    { return f.err != nil }
func (f *layoutForm) failed() bool         { return f.err != nil }
func (f *removeWorktreeForm) failed() bool { return f.err != nil }
func (f *runForm) failed() bool            { return f.err != nil }
func (f *saveLayoutForm) failed() bool     { return f.err != nil }
func (f *trustForm) failed() bool          { return f.err != nil }
func (m *picker) failed() bool             { return m.err != nil }

// popup runs a form or the picker in kitty. In an overlay that is all: the overlay closes back to the
// window it is over. A panel is an OS window of its own, which would float on, unseen or in the way, once
// the keyboard is elsewhere, so it quits when it loses focus, and gives the keyboard back to the window it
// was opened from when it closes, as closing an overlay does.
type popup struct {
	inner popupModel
	// from is the window the panel was opened from, zero in an overlay.
	from int
	// focused is whether the panel has the keyboard, by kitty's focus reports. It has when it opens.
	focused bool
	// busy is inner's working after the last message, and done that it then finished and quit.
	busy, done bool
	// acted is kitty.Acted, but in tests.
	acted func() bool
	// focus is kitty.FocusWindow, but in tests.
	focus func(id int) error
	// self is the panel's own window.
	self int
	// th draws the panel's border.
	th *look.Theme
	// width and height are the panel's size, and rows how many rows the form drew last, boxed, which
	// clicks are mapped back to.
	width, height, rows int
}

// fillsPopup is a popup that lays itself out over the whole of the size it is given, as the picker
// does, rather than drawing a form with boxed.
type fillsPopup interface{ fillsPopup() }

func (m *picker) fillsPopup() {}

func newPopup(inner popupModel, from int) *popup {
	return &popup{
		inner: inner, from: from, focused: true, acted: kitty.Acted, focus: kitty.FocusWindow, self: kitty.SelfWindowID(),
		th: look.New(true),
	}
}

func (p *popup) Init() tea.Cmd { return p.inner.Init() }

func (p *popup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.from != 0 {
		switch msg.(type) {
		case tea.FocusMsg:
			p.focused = true
		case tea.BlurMsg:
			p.focused = false
			if !p.inner.working() {
				return p, tea.Quit
			}
		}
		msg = p.inPanel(msg)
	}
	m, cmd := p.inner.Update(msg)
	p.inner = m.(popupModel)
	was := p.busy
	p.busy = p.inner.working()
	if !was || p.busy || p.from == 0 {
		return p, cmd
	}
	// Done, it quits. Failed, it stays to show why, and takes the keyboard back if what it did moved it.
	if !p.inner.failed() {
		p.done = true
	} else if !p.focused {
		self, focus := p.self, p.focus
		cmd = tea.Batch(cmd, func() tea.Msg {
			if err := focus(self); err != nil {
				return tea.QuitMsg{}
			}
			return nil
		})
	}
	return p, cmd
}

// inPanel is msg as the popup in a panel's border sees it: the size inside the border for one that fills
// it, and a click where boxed drew what the frame shows there.
func (p *popup) inPanel(msg tea.Msg) tea.Msg {
	_, fills := p.inner.(fillsPopup)
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		p.width, p.height = size.Width, size.Height
		if fills {
			return tea.WindowSizeMsg{Width: max(size.Width-4, 1), Height: max(size.Height-2, 1)}
		}
		// boxed's padding is the inset, and its heading's row goes in the border.
		return size
	}
	if fills {
		return msg
	}
	// The frame's row y is boxed's y+1, but for its foot, on the row above the bottom border.
	y := func(y int) int {
		if y == p.height-2 {
			return p.rows - 2
		}
		return y + 1
	}
	switch m := msg.(type) {
	case tea.MouseClickMsg:
		m.Y = y(m.Y)
		return m
	case tea.MouseReleaseMsg:
		m.Y = y(m.Y)
		return m
	case tea.MouseMotionMsg:
		m.Y = y(m.Y)
		return m
	case tea.MouseWheelMsg:
		m.Y = y(m.Y)
		return m
	}
	return msg
}

func (p *popup) View() tea.View {
	v := p.inner.View()
	if p.from == 0 {
		return v
	}
	v.ReportFocus = true
	if _, fills := p.inner.(fillsPopup); fills {
		v.SetContent(frame(p.th, p.width, p.height, "", strings.Split(v.Content, "\n"), ""))
		return v
	}
	title, body, foot := unboxed(v.Content)
	p.rows = len(body) + 4
	v.SetContent(frame(p.th, p.width, p.height, title, body, foot))
	return v
}

// unboxed takes apart what boxed drew: the heading, the lines under it and the last line, its buttons or
// keys, without boxed's padding.
func unboxed(s string) (title string, body []string, foot string) {
	rows := strings.Split(s, "\n")
	if len(rows) < 4 {
		return "", rows, ""
	}
	unpad := func(row string) string { return strings.TrimRight(strings.TrimPrefix(row, "  "), " ") }
	for _, row := range rows[2 : len(rows)-2] {
		body = append(body, unpad(row))
	}
	return unpad(rows[1]), body, unpad(rows[len(rows)-2])
}

// frame draws a popup in a panel, width by height cells: a rounded border in the accent, title set into
// its top, body inset a cell from its sides, and foot, when not empty, on the row above its bottom. A body
// too long loses its last lines; a short one leaves the rows below it blank.
func frame(th *look.Theme, width, height int, title string, body []string, foot string) string {
	if width < 4 || height < 2 {
		return strings.Join(body, "\n")
	}
	g := th.Glyph
	border := func(s string) string { return th.Render(look.Plain, look.Border(s, true)) }
	h := func(n int) string { return strings.Repeat(g(look.GlyphFrameHorizontal), max(n, 0)) }
	top := border(g(look.GlyphFrameTopLeft) + h(width-2) + g(look.GlyphFrameTopRight))
	if title != "" {
		title = ansi.Truncate(title, width-6, "")
		top = border(g(look.GlyphFrameTopLeft)+h(1)+" ") + title + border(" "+h(width-5-ansi.StringWidth(title))+g(look.GlyphFrameTopRight))
	}
	rows := height - 2
	if foot != "" {
		rows--
	}
	body = body[:min(len(body), max(rows, 0))]
	for len(body) < rows {
		body = append(body, "")
	}
	if foot != "" {
		body = append(body, foot)
	}
	side := border(g(look.GlyphFrameVertical))
	out := []string{top}
	for _, row := range body {
		out = append(out, side+" "+look.Fit(row, width-4)+" "+side)
	}
	out = append(out, border(g(look.GlyphFrameBottomLeft)+h(width-2)+g(look.GlyphFrameBottomRight)))
	return strings.Join(out, "\n")
}

// leave gives the keyboard back to the window a panel was opened from, unless it is already elsewhere:
// the panel lost focus, or what the popup did, such as switching sessions, moved it. The picker quits
// still working, with a message rather than an update.
func (p *popup) leave() error {
	if p.from == 0 || !p.focused || (p.done || p.busy) && p.acted() {
		return nil
	}
	return p.focus(p.from)
}

// runPopup runs m in the popup kspaces runs in, see popup.
func runPopup(ctx context.Context, m popupModel) error {
	p := newPopup(m, kitty.PanelFrom())
	if _, err := tea.NewProgram(p, tea.WithContext(ctx)).Run(); err != nil {
		return err
	}
	return p.leave()
}
