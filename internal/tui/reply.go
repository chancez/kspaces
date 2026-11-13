package tui

import (
	"context"
	"fmt"
	"image/color"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// replyForm types into another window without switching to it, as to answer a program that waits for me:
// it shows the bottom of the window's screen, kept current, over a field whose text it sends. It is meant
// for a kitty popup, like the new worktree form.
type replyForm struct {
	th *look.Theme
	// window is the window replied to, session its session, empty for none, and title its title.
	window  int
	session string
	title   string
	in      textField
	// lines are the window's screen, safe to draw, nil until fetched and when the fetch failed; seq tells
	// the latest fetch from those before it.
	lines []string
	seq   int
	busy  bool
	err   error
	width int
	// height is the form's, which the screen fills what the rest leaves of.
	height int
	// getText fetches a window's screen and send sends to it, kitty's but in tests.
	getText func(window int) (string, error)
	send    func(window int, text string, enter bool) error
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*replyForm]
}

type sentMsg struct{ err error }

// newReplyForm is the form replying to window, in session, titled title.
func newReplyForm(window int, session, title string) *replyForm {
	th := look.New(true)
	f := &replyForm{th: th, window: window, session: session, title: title, in: newTextField(th, "Enter alone sends Enter"),
		getText: kitty.ScreenText, send: kitty.SendText}
	f.in.Focus()
	return f
}

func (f *replyForm) Init() tea.Cmd { return f.fetchScreen() }

func (f *replyForm) working() bool { return f.busy }
func (f *replyForm) failed() bool  { return f.err != nil }

// fetchScreen fetches the window's screen, in the background.
func (f *replyForm) fetchScreen() tea.Cmd {
	f.seq++
	get, window, seq := f.getText, f.window, f.seq
	return func() tea.Msg {
		text, err := get(window)
		return screenMsg{window: window, seq: seq, text: text, err: err}
	}
}

// gotScreen shows a fetched screen and fetches it again after screenRefresh, as the picker's preview does,
// so what the program says next shows while I type.
func (f *replyForm) gotScreen(msg screenMsg) tea.Cmd {
	if msg.seq != f.seq {
		return nil
	}
	f.lines = nil
	// A failed fetch, such as of a window just closed, shows no screen rather than an error.
	if msg.err == nil {
		f.lines = screenLines(msg.text)
	}
	window, seq := f.window, f.seq
	return tea.Tick(screenRefresh, func(time.Time) tea.Msg { return screenTickMsg{window: window, seq: seq} })
}

// reply sends the field's text and then Enter, or as it is without enter, and leaves.
func (f *replyForm) reply(enter bool) tea.Cmd {
	f.busy, f.err = true, nil
	send, window, text := f.send, f.window, f.in.Value()
	return func() tea.Msg { return sentMsg{send(window, text, enter)} }
}

func (f *replyForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.in.SetWidth(max(f.width-4-labelWidth, 1))
		return f, nil
	case screenMsg:
		return f, f.gotScreen(msg)
	case screenTickMsg:
		if msg.seq != f.seq {
			return f, nil
		}
		return f, f.fetchScreen()
	case sentMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if keptBy(&f.in, msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theReplyForm.Target(replySubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if _, ok := clickedField([]field{&f.in}, f.zones, msg); ok {
			return f, nil
		}
	}
	if f.busy {
		return f, nil
	}
	return f, f.in.update(msg)
}

func (f *replyForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *replyForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	to := f.session
	if to == "" {
		to = "window " + strconv.Itoa(f.window)
	}
	head := []look.Span{look.Heading("Reply to " + to)}
	if f.title != "" {
		head = append(head, look.Muted("  "+f.title))
	}
	top := []string{line(th.Cut(width, head...)...), ""}

	var bottom []string
	bottom = append(bottom, fieldLines(th, "reply", &f.in, width)...)
	bottom = append(bottom, "")
	switch {
	case f.busy:
		bottom = append(bottom, line(look.Busy("Sending...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			bottom = append(bottom, line(row))
		}
		bottom = append(bottom, "")
	}
	f.buttons = controls[*replyForm]{}
	spans := f.buttons.draw(f, theReplyForm.Target(replySubject{}),
		control{action: actReplySend, label: "Send"},
		control{action: actReplyType, label: "Send without enter"},
		control{action: actReplyCancel, label: "Cancel"})
	bottom = append(bottom, line(append(spans, newlineHints(&f.in)...)...))

	// The screen's last rows fill what the rest leaves, under a rule, boxed's two rows of padding counted.
	var screen []string
	if rows := f.height - 2 - len(top) - len(bottom) - 1; rows > 0 && len(f.lines) > 0 {
		for _, l := range screenBlock(f.lines, width, rows) {
			screen = append(screen, line(look.Styled(func(color.Color) string { return l })))
		}
		screen = append(screen, line(look.Rule(width, false)))
	}
	return boxed(append(append(top, screen...), bottom...))
}

// RunReply replies to window: sends text, then Enter unless noEnter, without asking when text is given,
// else runs the reply form, meant for a kitty popup.
func RunReply(ctx context.Context, window int, text *string, noEnter bool) error {
	if text != nil {
		return kitty.SendText(window, *text, !noEnter)
	}
	data, err := kitty.ListWindows()
	if err != nil {
		return err
	}
	w, ok := kitty.FindWindow(data, window)
	if !ok {
		return fmt.Errorf("no window %d", window)
	}
	if err := runPopup(ctx, newReplyForm(window, w.SessionName, w.Title)); err != nil {
		return fmt.Errorf("running the reply form: %w", err)
	}
	return nil
}

// reply opens the form that replies to window, in a popup as newWorktree's.
func (m *sidebar) reply(window int) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskReply(cfg, st, window); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}
