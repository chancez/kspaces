package tui

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// errorForm shows an error in full, wrapped and scrolled, for when the sidebar's last line cuts it, and
// copies it to the clipboard. It is meant for a kitty popup, like the new worktree form.
type errorForm struct {
	th   *look.Theme
	text string
	// offset is the first row of the wrapped text shown.
	offset int
	// copied is whether y has copied the text, which the form says.
	copied        bool
	width, height int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*errorForm]
}

func newErrorForm(text string) *errorForm {
	return &errorForm{th: look.New(true), text: text}
}

func (f *errorForm) Init() tea.Cmd { return nil }

func (f *errorForm) working() bool { return false }
func (f *errorForm) failed() bool  { return false }

// textWidth is the width the text wraps to, inside boxed's columns of padding.
func (f *errorForm) textWidth() int { return max(f.width-4, 20) }

// rows is the wrapped text.
func (f *errorForm) rows() []look.Span { return look.Error(f.text).Wrapped(f.textWidth()) }

// shown is how many rows of the text fit: what the heading, the buttons and boxed's padding leave.
func (f *errorForm) shown() int { return max(f.height-6, 1) }

// scroll moves the text by delta rows, keeping the last row at the bottom at most.
func (f *errorForm) scroll(delta int) {
	f.offset = max(min(f.offset+delta, len(f.rows())-f.shown()), 0)
}

// copyText puts the text on the clipboard. It is an OSC 52 write to kitty, which the form's window takes
// as kitty.conf's clipboard_control allows, as it does by default, and needs no remote control.
func (f *errorForm) copyText() tea.Cmd {
	f.copied = true
	return tea.SetClipboard(f.text)
}

func (f *errorForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.scroll(0)
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theShowForm.Target(showSubject{})); ok {
			return f, cmd
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			f.scroll(-1)
		case tea.MouseWheelDown:
			f.scroll(1)
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *errorForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *errorForm) render() string {
	th := f.th
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{line(look.Heading("Error")), ""}
	rows := f.rows()
	end := min(f.offset+f.shown(), len(rows))
	for _, row := range rows[f.offset:end] {
		lines = append(lines, line(row))
	}
	lines = append(lines, "")
	f.buttons = controls[*errorForm]{}
	footer := f.buttons.draw(f, theShowForm.Target(showSubject{}),
		control{action: actShowCopy, label: "Copy"},
		control{action: actShowClose, label: "Close"})
	if len(rows) > f.shown() {
		footer = append(footer, look.Muted(fmt.Sprintf("  rows %d-%d of %d", f.offset+1, end, len(rows))))
	}
	if f.copied {
		footer = append(footer, look.Muted("  copied"))
	}
	return boxed(append(lines, line(footer...)))
}

// RunShowError shows the error in the file at path in the error form, meant for a kitty popup, and
// deletes the file, which spaces.AskShowError wrote for it.
func RunShowError(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := runPopup(ctx, newErrorForm(string(data))); err != nil {
		return fmt.Errorf("running the error form: %w", err)
	}
	return nil
}

// showError opens the form that shows text, the error on the last line, in full, in a popup as
// newWorktree's.
func (m *sidebar) showError(text string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskShowError(cfg, st, text); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}
