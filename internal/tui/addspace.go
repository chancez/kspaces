package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// addSpaceForm asks for a repository, and the group to put it in when there are groups, shows the root it
// would add or why it cannot, and adds it to the config's spaces, in the file of the group's spaces. The
// sidebars pick it up when they next read the config. It is meant for a kitty popup, like the new worktree
// form.
type addSpaceForm struct {
	th *look.Theme
	// The configured spaces, to refuse one already there.
	roots []string
	path  lineField
	// The group, none for none, from the groups the spaces are in.
	group choiceFields[string]
	// focus is the index in fields of the field that has focus.
	focus int
	// root is what the path adds, or problem why it cannot be added, for the path as typed.
	root    string
	problem error
	// file is the config file the space goes in, for the group chosen.
	file    string
	busy    bool
	err     error
	width   int
	height  int
	zones   map[string][]look.Zone
	buttons controls[*addSpaceForm]
	kept    drafts
}

// groupZone is the zone the choice of group is drawn in, and its label.
const groupZone = "group"

// newAddSpaceForm is the form for adding a space, with path filled in, the home directory as ~ to keep it
// short, and group chosen of groups, the groups there are, when it is one.
func newAddSpaceForm(roots, groups []string, path, group string) *addSpaceForm {
	th := look.New(true)
	in := newLineField(th, "a directory in a git repository")
	in.fill(config.Tilde(path))
	f := &addSpaceForm{th: th, roots: roots, path: in,
		group: newChoiceFields(th, groupZone, groups, func(g string) string { return g }, func(string) []config.Input { return nil },
			nil, false, &noChoice{name: "none"}, group, nil, nil)}
	moveFocus(f.fields(), &f.focus, 0)
	f.check()
	f.locate()
	return f
}

type addedMsg struct{ err error }

func (f *addSpaceForm) Init() tea.Cmd { return textinput.Blink }

// fields are the form's fields in order: the path, and the choice of group, nil, when there is one.
func (f *addSpaceForm) fields() []field {
	if f.group.picks() {
		return []field{&f.path, nil}
	}
	return []field{&f.path}
}

func (f *addSpaceForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *addSpaceForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onGroup says whether the choice of group has focus.
func (f *addSpaceForm) onGroup() bool { return f.group.picks() && f.focus == 1 }

// cycleGroup chooses the next group, or the one before, and focuses the choice.
func (f *addSpaceForm) cycleGroup(delta int) tea.Cmd {
	f.group.cycle(delta)
	f.focus = 1
	f.locate()
	return moveFocus(f.fields(), &f.focus, 0)
}

// chosenGroup is the group chosen, empty for none.
func (f *addSpaceForm) chosenGroup() string {
	g, _ := f.group.chosen()
	return g
}

// check works out what the path adds, as it is typed, so a path that cannot be added is refused before
// anything is written.
func (f *addSpaceForm) check() {
	f.root, f.problem = spaces.NewSpaceRoot(f.roots, f.path.Value())
}

// locate works out the file the space goes in, for the group chosen. A config that cannot be read says
// why when the space is added.
func (f *addSpaceForm) locate() {
	f.file, _ = config.GroupFile(f.chosenGroup())
}

func (f *addSpaceForm) add() tea.Cmd {
	f.busy, f.err = true, nil
	root, group := f.root, f.chosenGroup()
	return func() tea.Msg { return addedMsg{config.SaveSpace(root, group)} }
}

func (f *addSpaceForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While the list of groups is open, keys and the mouse are its.
	if isInput(msg) && f.group.listOpen() {
		cmd, picked := f.group.updateList(msg, f.zones)
		if picked {
			return f, tea.Batch(cmd, f.cycleGroup(0))
		}
		return f, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		// As in the new worktree form: the field starts after the padding and its label.
		f.path.SetWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case addedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			f.kept.done = true
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if f.focus == 0 && keptBy(&f.path, msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theAddForm.Target(addSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if clickedChoice(f.zones, groupZone, msg) {
			f.group.openList()
			return f, f.cycleGroup(0)
		}
		if i, ok := clickedField(f.fields(), f.zones, msg); ok {
			f.focus = i
			return f, moveFocus(f.fields(), &f.focus, 0)
		}
	}
	if f.focus != 0 {
		return f, nil
	}
	before := f.path.Value()
	cmd := f.path.update(msg)
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
	v.Cursor = fieldCursor(&f.path, zones)
	return v
}

func (f *addSpaceForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := append([]string{line(look.Heading("Add a space")), ""}, fieldLines(th, "repository", &f.path, width)...)
	groupAt := len(lines)
	lines = append(append(lines, f.group.lines(f.onGroup(), width)...), "")
	switch {
	case strings.TrimSpace(f.path.Value()) == "":
		lines = append(lines, line(look.Muted("Name a directory in the repository to add")))
	case f.problem != nil:
		lines = append(lines, line(th.Cut(width, look.Error(f.problem.Error()))...))
	default:
		says := "Adds " + config.Tilde(f.root) + " to the spaces"
		if g := f.chosenGroup(); g != "" {
			says += ", in " + g
		}
		if name := filepath.Base(f.file); f.file != "" && name != "config.toml" {
			says += ", in " + name
		}
		lines = append(lines, line(th.Cut(width, look.Muted(says))...))
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
	if f.group.listOpen() {
		lines = f.group.drawList(lines, groupAt, width, f.height)
		return boxed(append(lines, line(listHints()...)))
	}
	buttons := f.buttons.draw(f, theAddForm.Target(addSubject{}),
		control{action: actAdd, label: "Add", off: f.problem != nil},
		control{action: actAddCancel, label: "Cancel"},
		control{action: actAddClear, label: "Clear"})
	if f.group.picks() {
		buttons = append(buttons, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	}
	if f.onGroup() {
		buttons = append(buttons, choiceHints("group")...)
	}
	lines = append(lines, line(buttons...))
	return boxed(lines)
}

// openAddSpaceForm is newAddSpaceForm with the draft kept for path and group restored.
func openAddSpaceForm(roots, groups []string, path, group string) *addSpaceForm {
	f, _ := withDraft(draftKey("add space", []string{path, group}, nil), func(saved map[string]string) (*addSpaceForm, error) {
		f := newAddSpaceForm(roots, groups, path, group)
		f.restore(saved)
		return f, nil
	})
	return f
}

func (f *addSpaceForm) drafts() *drafts { return &f.kept }

// draft is the path and the group.
func (f *addSpaceForm) draft() map[string]string {
	return map[string]string{"path": f.path.Value(), "group": f.group.chosenName()}
}

// restore fills the form in from d, focus on the path.
func (f *addSpaceForm) restore(d map[string]string) tea.Cmd {
	if v, ok := d["path"]; ok {
		f.path.fill(v)
	}
	if v, ok := d["group"]; ok {
		f.group.choose(v)
	}
	f.check()
	f.locate()
	f.focus = 0
	return moveFocus(f.fields(), &f.focus, 0)
}

// RunAddSpace runs the add space form, meant for a kitty popup, with path filled in and group chosen,
// until the space is added or the form is left.
func RunAddSpace(ctx context.Context, path, group string) error {
	cfg, err := loadForAdd()
	if err != nil {
		return err
	}
	if err := runPopup(ctx, openAddSpaceForm(cfg.Spaces, groupNames(cfg), path, group)); err != nil {
		return fmt.Errorf("running the add space form: %w", err)
	}
	return nil
}

// AddSpace adds the repository at path to the spaces, in group, without asking, as the form does once
// confirmed.
func AddSpace(path, group string) error {
	cfg, err := loadForAdd()
	if err != nil {
		return err
	}
	root, err := spaces.NewSpaceRoot(cfg.Spaces, path)
	if err != nil {
		return err
	}
	return config.SaveSpace(root, group)
}

// loadForAdd is the config, empty when there is none yet, as adding a space creates it.
func loadForAdd() (config.Config, error) {
	cfg, err := config.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return config.Config{}, nil
	}
	return cfg, err
}

// groupNames are the groups cfg's spaces are in, in the sidebar's order.
func groupNames(cfg config.Config) []string {
	var listed []spaces.Space
	for _, root := range cfg.Spaces {
		listed = append(listed, spaces.Space{Root: root, Group: cfg.Space[filepath.Base(root)].Group})
	}
	return spaces.Order{Groups: cfg.Groups}.GroupNames(listed)
}
