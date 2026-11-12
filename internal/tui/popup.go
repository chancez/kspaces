package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/kitty"
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
}

func newPopup(inner popupModel, from int) *popup {
	return &popup{inner: inner, from: from, focused: true, acted: kitty.Acted, focus: kitty.FocusWindow, self: kitty.SelfWindowID()}
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

func (p *popup) View() tea.View {
	v := p.inner.View()
	if p.from != 0 {
		v.ReportFocus = true
	}
	return v
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
