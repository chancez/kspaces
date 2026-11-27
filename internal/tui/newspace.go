package tui

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// newSpaceForm makes an ad hoc space on host, empty for this machine, see spaces.AdHocSpace: a name, and
// the directory its windows start in, the host's home directory when none is given. It then opens the
// space. It is meant for a kitty popup, like the new worktree form.
type newSpaceForm struct {
	th   *look.Theme
	cfg  config.Config
	host string
	name lineField
	dir  lineField
	// focus is the index in fields of the field that has focus.
	focus   int
	busy    bool
	err     error
	width   int
	zones   map[string][]look.Zone
	buttons controls[*newSpaceForm]
	// newSpace makes it on the host, and open opens it, the host's and kitty's but in tests.
	newSpace func(name, dir string) error
	open     func(wt spaces.Worktree) error
}

type spaceMadeMsg struct{ err error }

// newNewSpaceForm is the form for an ad hoc space on host, its directory filled in with dir.
func newNewSpaceForm(cfg config.Config, host, dir string) *newSpaceForm {
	th := look.New(true)
	f := &newSpaceForm{th: th, cfg: cfg, host: host,
		name: newLineField(th, "the space's name"), dir: newLineField(th, "the home directory")}
	f.dir.fill(dir)
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

func (f *newSpaceForm) Init() tea.Cmd { return textinput.Blink }

func (f *newSpaceForm) fields() []field { return []field{&f.name, &f.dir} }

func (f *newSpaceForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *newSpaceForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// problem is why the space cannot be made as typed, nil when it can: a name that is none, or, on this
// machine, a directory that is none. Another host's directory is the host's to check.
func (f *newSpaceForm) problem() error {
	if err := spaces.CheckAdHocName(f.cfg, strings.TrimSpace(f.name.Value())); err != nil {
		return err
	}
	if f.host == "" {
		_, err := spaces.AdHocDir(f.dir.Value())
		return err
	}
	return nil
}

// ready says whether there is a space to make.
func (f *newSpaceForm) ready() bool {
	return strings.TrimSpace(f.name.Value()) != "" && f.problem() == nil
}

// create has the host make the space, then opens it.
func (f *newSpaceForm) create() tea.Cmd {
	f.busy, f.err = true, nil
	newSpace, open, host := f.newSpace, f.open, f.host
	name, dir := strings.TrimSpace(f.name.Value()), strings.TrimSpace(f.dir.Value())
	if host == "" {
		dir, _ = spaces.AdHocDir(dir)
	}
	return func() tea.Msg {
		if err := newSpace(name, dir); err != nil {
			return spaceMadeMsg{err}
		}
		return spaceMadeMsg{open(spaces.AdHocWorktree(host, name, dir))}
	}
}

func (f *newSpaceForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		for _, in := range []*lineField{&f.name, &f.dir} {
			in.SetWidth(max(f.width-4-labelWidth-1, 1))
		}
		return f, nil
	case spaceMadeMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		on := f.fields()[f.focus]
		msg = formKey(f.th, on, msg)
		if keptBy(on, msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theNewSpaceForm.Target(newSpaceSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if i, ok := clickedField(f.fields(), f.zones, msg); ok {
			f.focus = i
			return f, moveFocus(f.fields(), &f.focus, 0)
		}
	}
	if f.busy {
		return f, nil
	}
	return f, f.fields()[f.focus].update(msg)
}

func (f *newSpaceForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.Cursor = fieldCursor(f.fields()[f.focus], zones)
	return v
}

func (f *newSpaceForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	host := hostName(f.host)
	lines := []string{line(th.Cut(width, look.Heading("New space on "+host))...), ""}
	lines = append(lines, fieldLines(th, "name", &f.name, width)...)
	lines = append(lines, fieldLines(th, "directory", &f.dir, width)...)
	lines = append(lines, "")
	switch {
	case strings.TrimSpace(f.name.Value()) == "":
		lines = append(lines, line(th.Cut(width, look.Muted("Name the space, and where its windows start"))...), "")
	case f.problem() != nil:
		lines = append(lines, line(th.Cut(width, look.Error(f.problem().Error()))...), "")
	default:
		dir := cmp.Or(strings.TrimSpace(f.dir.Value()), "the home directory")
		lines = append(lines, line(th.Cut(width, look.Muted("Makes "+strings.TrimSpace(f.name.Value())+" on "+host+", starting in "+dir+", and opens it"))...), "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Making on "+host+"...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*newSpaceForm]{}
	buttons := f.buttons.draw(f, theNewSpaceForm.Target(newSpaceSubject{}),
		control{action: actNewSpaceMake, label: "Make", off: !f.ready()},
		control{action: actNewSpaceCancel, label: "Cancel"})
	buttons = append(buttons, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	return boxed(append(lines, line(buttons...)))
}

// RunNewSpace runs the form that makes an ad hoc space on host, empty for this machine, with dir filled
// in, meant for a kitty popup.
func RunNewSpace(ctx context.Context, cfg config.Config, host, dir string) error {
	h, hangUp := spaces.ThisHost(), func() {}
	if host != "" {
		var err error
		if h, hangUp, err = spaces.HostOf(cfg, host); err != nil {
			return err
		}
	}
	defer hangUp()
	f := newNewSpaceForm(cfg, host, dir)
	f.newSpace = h.NewSpace
	f.open = func(wt spaces.Worktree) error { return spaces.OpenFromOverlay(cfg, wt, spaces.OpenOptions{}) }
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the new space form: %w", err)
	}
	return nil
}

// newSpaceSubject is the new space form.
type newSpaceSubject struct{}

// The new space form's actions.
var (
	actNewSpaceMake = &interact.Action[*newSpaceForm, newSpaceSubject]{
		Title:   "make",
		Applies: func(f *newSpaceForm, _ newSpaceSubject) bool { return !f.busy && f.ready() },
		Run:     cmd[*newSpaceForm, newSpaceSubject]((*newSpaceForm).create),
	}
	actNewSpaceCancel    = quit[*newSpaceForm, newSpaceSubject]()
	actNewSpaceNextField = &interact.Action[*newSpaceForm, newSpaceSubject]{
		Title: "next field",
		Run:   cmd[*newSpaceForm, newSpaceSubject]((*newSpaceForm).nextField),
	}
	actNewSpacePrevField = &interact.Action[*newSpaceForm, newSpaceSubject]{
		Title: "previous field",
		Run:   cmd[*newSpaceForm, newSpaceSubject]((*newSpaceForm).prevField),
	}
)

var theNewSpaceForm *interact.Place[*newSpaceForm, newSpaceSubject]

func init() {
	kind := interact.NewKind(func(newSpaceSubject) string { return "New space" },
		actNewSpaceNextField, actNewSpacePrevField, actNewSpaceMake, actNewSpaceCancel)
	kind.Bind(actNewSpaceNextField, "tab", "next field", "tab", "down")
	kind.Bind(actNewSpacePrevField, "shift+tab", "previous field", "shift+tab", "up")
	kind.Bind(actNewSpaceMake, "enter", "make", "enter")
	kind.Bind(actNewSpaceCancel, "esc", "cancel", "esc", "ctrl+c")
	theNewSpaceForm = interact.List(kind)
}

func (f *newSpaceForm) theme() *look.Theme { return f.th }
func (f *newSpaceForm) working() bool      { return f.busy }
func (f *newSpaceForm) failed() bool       { return f.err != nil }

func (f *newSpaceForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theNewSpaceForm.Target(newSpaceSubject{})), true
}
