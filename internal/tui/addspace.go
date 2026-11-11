package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// addSpaceForm asks for a repository, shows the root it would add or why it cannot, and adds it to the
// config's spaces. The sidebars pick it up when they next read the config. It is meant for a kitty overlay,
// like the new worktree form.
type addSpaceForm struct {
	th *look.Theme
	// The configured spaces, to refuse one already there.
	roots []string
	path  textinput.Model
	// root is what the path adds, or problem why it cannot be added, for the path as typed.
	root    string
	problem error
	busy    bool
	err     error
	width   int
	zones   map[string][]look.Zone
	buttons controls[*addSpaceForm]
}

// newAddSpaceForm is the form for adding a space, with path filled in, the home directory as ~ to keep it
// short.
func newAddSpaceForm(roots []string, path string) *addSpaceForm {
	th := look.New(true)
	in := newInput(th, "")
	in.Placeholder = "a directory in a git repository"
	in.SetValue(config.Tilde(path))
	in.CursorEnd()
	in.Focus()
	f := &addSpaceForm{th: th, roots: roots, path: in}
	f.check()
	return f
}

type addedMsg struct{ err error }

func (f *addSpaceForm) Init() tea.Cmd { return textinput.Blink }

// check works out what the path adds, as it is typed, so a path that cannot be added is refused before
// anything is written.
func (f *addSpaceForm) check() {
	f.root, f.problem = spaces.NewSpaceRoot(f.roots, f.path.Value())
}

func (f *addSpaceForm) add() tea.Cmd {
	f.busy, f.err = true, nil
	root := f.root
	return func() tea.Msg { return addedMsg{config.SaveSpace(root)} }
}

func (f *addSpaceForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		// As in the new worktree form: the field starts after the padding and its label.
		f.path.SetWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case addedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theAddForm.Target(addSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	var cmd tea.Cmd
	before := f.path.Value()
	f.path, cmd = f.path.Update(msg)
	if f.path.Value() != before {
		f.check()
	}
	return f, cmd
}

func (f *addSpaceForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *addSpaceForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{
		line(look.Heading("Add a space")), "",
		th.Draw(look.Plain, labelWidth, look.Label("repository")) + f.path.View(),
		"",
	}
	switch {
	case strings.TrimSpace(f.path.Value()) == "":
		lines = append(lines, line(look.Muted("Name a directory in the repository to add")))
	case f.problem != nil:
		lines = append(lines, line(th.Cut(width, look.Error(f.problem.Error()))...))
	default:
		lines = append(lines, line(th.Cut(width, look.Muted("Adds "+config.Tilde(f.root)+" to the spaces"))...))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Adding...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*addSpaceForm]{}
	buttons := f.buttons.draw(f, theAddForm.Target(addSubject{}),
		control{action: actAdd, label: "Add", off: f.problem != nil},
		control{action: actAddCancel, label: "Cancel"})
	lines = append(lines, line(buttons...))
	return boxed(lines)
}

// RunAddSpace runs the add space form, meant for a kitty overlay, with path filled in, until the space is
// added or the form is left.
func RunAddSpace(ctx context.Context, path string) error {
	roots, err := configuredSpaces()
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newAddSpaceForm(roots, path), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the add space form: %w", err)
	}
	return nil
}

// AddSpace adds the repository at path to the spaces without asking, as the form does once confirmed.
func AddSpace(path string) error {
	roots, err := configuredSpaces()
	if err != nil {
		return err
	}
	root, err := spaces.NewSpaceRoot(roots, path)
	if err != nil {
		return err
	}
	return config.SaveSpace(root)
}

// configuredSpaces are the config's spaces, none when there is no config yet, as adding one creates it.
func configuredSpaces() ([]string, error) {
	cfg, err := config.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return cfg.Spaces, err
}
