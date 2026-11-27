package tui

import (
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

// The forms for another host's copy of a space, see docs/stories.md: creating a worktree there and removing
// one, which the host's server does, with its git. Each is meant for a kitty popup, like the new worktree
// form, and smaller: the branch is typed, as the host's branches and PRs are not listed here.

// copyNewForm creates a worktree in host's copy of the space at root, on a new branch from base, or the
// main worktree's, and opens it.
type copyNewForm struct {
	th   *look.Theme
	cfg  config.Config
	host string
	// space is the host's copy, its worktrees' sessions named after the host.
	space  spaces.Space
	branch lineField
	base   lineField
	// focus is the index in fields of the field that has focus.
	focus   int
	busy    bool
	err     error
	width   int
	zones   map[string][]look.Zone
	buttons controls[*copyNewForm]
	// newWorktree creates it on the host, and open opens the copy made, the host's and kitty's but in
	// tests.
	newWorktree func(root, branch, base string) (spaces.Worktree, error)
	open        func(wt spaces.Worktree) error
}

type copyCreatedMsg struct{ err error }

// newCopyNewForm is the form for a worktree in space, host's, the branch filled in with the host's
// branch prefix, start.
func newCopyNewForm(cfg config.Config, host string, space spaces.Space, start string) *copyNewForm {
	th := look.New(true)
	f := &copyNewForm{th: th, cfg: cfg, host: host, space: space,
		branch: newLineField(th, "the new branch"), base: newLineField(th, "the main worktree's branch")}
	f.branch.fill(start)
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

func (f *copyNewForm) Init() tea.Cmd { return textinput.Blink }

func (f *copyNewForm) fields() []field { return []field{&f.branch, &f.base} }

func (f *copyNewForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *copyNewForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// ready says whether there is a branch to create.
func (f *copyNewForm) ready() bool {
	return strings.TrimSpace(f.branch.Value()) != "" && strings.TrimSpace(f.branch.Value()) != spaces.BranchStart(f.space.ConfigOf(f.cfg))
}

// create has the host create the worktree, then opens it, as the host names its session, after the host.
func (f *copyNewForm) create() tea.Cmd {
	f.busy, f.err = true, nil
	create, open, host, root := f.newWorktree, f.open, f.host, f.space.Root
	branch, base := strings.TrimSpace(f.branch.Value()), strings.TrimSpace(f.base.Value())
	return func() tea.Msg {
		wt, err := create(root, branch, base)
		if err != nil {
			return copyCreatedMsg{err}
		}
		wt.Host, wt.Session = host, spaces.CopyName(host, wt.Session)
		return copyCreatedMsg{open(wt)}
	}
}

func (f *copyNewForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		for _, in := range []*lineField{&f.branch, &f.base} {
			in.SetWidth(max(f.width-4-labelWidth-1, 1))
		}
		return f, nil
	case copyCreatedMsg:
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
		if cmd, ok := runOn(f, msg, theCopyNewForm.Target(copyNewSubject{})); ok {
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

func (f *copyNewForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.Cursor = fieldCursor(f.fields()[f.focus], zones)
	return v
}

func (f *copyNewForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{line(th.Cut(width, look.Heading("New worktree in "+f.space.Name+" on "+f.host))...), ""}
	lines = append(lines, fieldLines(th, "branch", &f.branch, width)...)
	lines = append(lines, fieldLines(th, "from", &f.base, width)...)
	lines = append(lines, "")
	if f.ready() {
		dir := spaces.WorktreeDir(f.space.ConfigOf(f.cfg), f.space.Root, strings.TrimSpace(f.branch.Value()), "")
		lines = append(lines, line(th.Cut(width, look.Muted("Creates "+dir+" on "+f.host+", and opens it"))...), "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Creating on "+f.host+"...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*copyNewForm]{}
	buttons := f.buttons.draw(f, theCopyNewForm.Target(copyNewSubject{}),
		control{action: actCopyCreate, label: "Create", off: !f.ready()},
		control{action: actCopyNewCancel, label: "Cancel"})
	buttons = append(buttons, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	return boxed(append(lines, line(buttons...)))
}

// RunNewCopyWorktree runs the form that creates a worktree in host's copy of the space at root, meant for a
// kitty popup, asking host's server for the space as it has it.
func RunNewCopyWorktree(ctx context.Context, cfg config.Config, host, root string) error {
	h, hangUp, err := spaces.HostOf(cfg, host)
	if err != nil {
		return err
	}
	defer hangUp()
	space, err := copySpace(h, host, root)
	if err != nil {
		return err
	}
	f := newCopyNewForm(cfg, host, space, spaces.BranchStart(space.ConfigOf(cfg)))
	f.newWorktree = h.NewWorktree
	f.open = func(wt spaces.Worktree) error { return spaces.OpenFromOverlay(cfg, wt, spaces.OpenOptions{}) }
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}

// copySpace is host's copy of the space at root, as h, its server, has it.
func copySpace(h spaces.Host, host, root string) (spaces.Space, error) {
	hd, err := h.Load(false)
	if err != nil {
		return spaces.Space{}, fmt.Errorf("asking %s: %w", host, err)
	}
	views, _ := spaces.RemoteViewsOf(map[string]spaces.RemoteHost{host: {Data: &hd}})
	for _, s := range views[host].Spaces {
		if s.Root == root {
			return s, nil
		}
	}
	return spaces.Space{}, fmt.Errorf("%s has no space at %s", host, root)
}

// copyRemoveForm removes host's worktree, wt, a copy, once confirmed: its kitty session here first, with
// the provider's sessions its windows show, then the worktree, on the host, with the provider's sessions
// there tagged with it, and its branch if asked.
type copyRemoveForm struct {
	th   *look.Theme
	cfg  config.Config
	host string
	wt   spaces.Worktree
	// changes are the files the worktree has not committed, as the host last said, -1 for not known.
	changes int
	busy    bool
	err     error
	width   int
	zones   map[string][]look.Zone
	buttons controls[*copyRemoveForm]
	// end ends the copy's kitty session here, and remove removes the worktree on the host, kitty's and the
	// host's but in tests.
	end    func(session string) error
	remove func(path string, opts spaces.RemoveOptions) error
}

type copyRemovedMsg struct{ err error }

func (f *copyRemoveForm) Init() tea.Cmd { return nil }

// removeWith removes the worktree as opts says.
func (f *copyRemoveForm) removeWith(opts spaces.RemoveOptions) tea.Cmd {
	f.busy, f.err = true, nil
	end, remove, wt := f.end, f.remove, f.wt
	return func() tea.Msg {
		if err := end(wt.Session); err != nil {
			return copyRemovedMsg{err}
		}
		return copyRemovedMsg{remove(wt.Path, opts)}
	}
}

func (f *copyRemoveForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case copyRemovedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theCopyRemoveForm.Target(copyRemoveSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *copyRemoveForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *copyRemoveForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	_, session := spaces.SplitCopy(f.wt.Session)
	lines := []string{line(th.Cut(width, look.Heading("Remove "+session+" on "+f.host))...), ""}
	says := fmt.Sprintf("Closes %s here, then removes %s on %s, ending the sessions there tagged with it.", f.wt.Session, f.wt.Path, f.host)
	for _, row := range look.Text(says).Wrapped(width) {
		lines = append(lines, line(row))
	}
	if f.wt.Branch != "" {
		lines = append(lines, line(th.Cut(width, look.Muted("branch "+f.wt.Branch))...))
	}
	if f.changes > 0 {
		lines = append(lines, line(th.Cut(width, look.Error(fmt.Sprintf("%d files not committed, which only Force remove loses", f.changes)))...))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Removing on "+f.host+"...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*copyRemoveForm]{}
	lines = append(lines, line(f.buttons.draw(f, theCopyRemoveForm.Target(copyRemoveSubject{}),
		control{action: actCopyRemove, label: "Remove", off: f.changes > 0},
		control{action: actCopyRemoveBranch, label: "Remove and delete branch", off: f.changes > 0},
		control{action: actCopyForceRemove, label: "Force remove", danger: true},
		control{action: actCopyRemoveCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunRemoveCopyWorktree runs the form that removes host's worktree at path, meant for a kitty popup.
func RunRemoveCopyWorktree(ctx context.Context, cfg config.Config, host, path string) error {
	h, hangUp, err := spaces.HostOf(cfg, host)
	if err != nil {
		return err
	}
	defer hangUp()
	hd, err := h.Load(false)
	if err != nil {
		return fmt.Errorf("asking %s: %w", host, err)
	}
	views, git := spaces.RemoteViewsOf(map[string]spaces.RemoteHost{host: {Data: &hd}})
	for _, s := range views[host].Spaces {
		for i, wt := range s.Worktrees {
			if wt.Path != path {
				continue
			}
			if i == 0 {
				return fmt.Errorf("%s is the main worktree of %s on %s, which cannot be removed", path, s.Name, host)
			}
			f := &copyRemoveForm{th: look.New(true), cfg: cfg, host: host, wt: wt, changes: -1,
				end:    func(session string) error { return spaces.EndSession(cfg, session) },
				remove: h.RemoveWorktree}
			if st := git[spaces.GitKey(wt)].Status; st != nil {
				f.changes = st.Changes
			}
			if err := runPopup(ctx, f); err != nil {
				return fmt.Errorf("running the remove worktree form: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("%s has no worktree at %s", host, path)
}

// copyNewSubject is the new worktree form of another host's copy, and copyRemoveSubject its remove form.
type copyNewSubject struct{}

type copyRemoveSubject struct{}

// The copies' forms' actions.
var (
	actCopyCreate = &interact.Action[*copyNewForm, copyNewSubject]{
		Title:   "create",
		Applies: func(f *copyNewForm, _ copyNewSubject) bool { return !f.busy && f.ready() },
		Run:     cmd[*copyNewForm, copyNewSubject]((*copyNewForm).create),
	}
	actCopyNewCancel    = quit[*copyNewForm, copyNewSubject]()
	actCopyNewNextField = &interact.Action[*copyNewForm, copyNewSubject]{
		Title: "next field",
		Run:   cmd[*copyNewForm, copyNewSubject]((*copyNewForm).nextField),
	}
	actCopyNewPrevField = &interact.Action[*copyNewForm, copyNewSubject]{
		Title: "previous field",
		Run:   cmd[*copyNewForm, copyNewSubject]((*copyNewForm).prevField),
	}
	actCopyRemove = &interact.Action[*copyRemoveForm, copyRemoveSubject]{
		Title:   "remove",
		Applies: func(f *copyRemoveForm, _ copyRemoveSubject) bool { return !f.busy && f.changes <= 0 },
		Run: func(f *copyRemoveForm, _ copyRemoveSubject) tea.Cmd {
			return f.removeWith(spaces.RemoveOptions{})
		},
	}
	actCopyRemoveBranch = &interact.Action[*copyRemoveForm, copyRemoveSubject]{
		Title: "remove and delete branch",
		Applies: func(f *copyRemoveForm, _ copyRemoveSubject) bool {
			return !f.busy && f.changes <= 0 && f.wt.Branch != ""
		},
		Run: func(f *copyRemoveForm, _ copyRemoveSubject) tea.Cmd {
			return f.removeWith(spaces.RemoveOptions{DeleteBranch: true})
		},
	}
	actCopyForceRemove = &interact.Action[*copyRemoveForm, copyRemoveSubject]{
		Title:   "force remove",
		Applies: func(f *copyRemoveForm, _ copyRemoveSubject) bool { return !f.busy },
		Run: func(f *copyRemoveForm, _ copyRemoveSubject) tea.Cmd {
			return f.removeWith(spaces.RemoveOptions{Force: true})
		},
	}
	actCopyRemoveCancel = &interact.Action[*copyRemoveForm, copyRemoveSubject]{
		Title:   "cancel",
		Applies: func(f *copyRemoveForm, _ copyRemoveSubject) bool { return !f.busy },
		Run:     func(*copyRemoveForm, copyRemoveSubject) tea.Cmd { return tea.Quit },
	}
	actCopyRemoveQuit = quit[*copyRemoveForm, copyRemoveSubject]()
)

var (
	theCopyNewForm    *interact.Place[*copyNewForm, copyNewSubject]
	theCopyRemoveForm *interact.Place[*copyRemoveForm, copyRemoveSubject]
)

func init() {
	newKind := interact.NewKind(func(copyNewSubject) string { return "New worktree" },
		actCopyNewNextField, actCopyNewPrevField, actCopyCreate, actCopyNewCancel)
	newKind.Bind(actCopyNewNextField, "tab", "next field", "tab", "down")
	newKind.Bind(actCopyNewPrevField, "shift+tab", "previous field", "shift+tab", "up")
	newKind.Bind(actCopyCreate, "enter", "create", "enter")
	newKind.Bind(actCopyNewCancel, "esc", "cancel", "esc", "ctrl+c")
	theCopyNewForm = interact.List(newKind)

	removeKind := interact.NewKind(func(copyRemoveSubject) string { return "Remove worktree" },
		actCopyRemove, actCopyRemoveBranch, actCopyForceRemove, actCopyRemoveCancel, actCopyRemoveQuit)
	removeKind.Bind(actCopyRemove, "y/enter", "remove", "y", "enter")
	removeKind.Bind(actCopyRemoveBranch, "d", "remove and delete the branch", "d")
	removeKind.Bind(actCopyForceRemove, "f", "remove, losing changes", "f")
	removeKind.Bind(actCopyRemoveCancel, "n/esc", "cancel", "n", "esc")
	removeKind.Bind(actCopyRemoveQuit, "ctrl+c", "quit", "ctrl+c")
	theCopyRemoveForm = interact.List(removeKind)
}

func (f *copyNewForm) theme() *look.Theme    { return f.th }
func (f *copyRemoveForm) theme() *look.Theme { return f.th }
func (f *copyNewForm) working() bool         { return f.busy }
func (f *copyRemoveForm) working() bool      { return f.busy }
func (f *copyNewForm) failed() bool          { return f.err != nil }
func (f *copyRemoveForm) failed() bool       { return f.err != nil }

func (f *copyNewForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theCopyNewForm.Target(copyNewSubject{})), true
}

func (f *copyRemoveForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theCopyRemoveForm.Target(copyRemoveSubject{})), false
}
