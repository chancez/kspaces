package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// newWorktreeForm asks for a branch, or a PR to check out, and, optionally, where its worktree goes, shows
// what it will do, creates the worktree and switches to its session. It is meant for a kitty overlay, as
// the sidebar is too narrow to type a name and read back the path in.
type newWorktreeForm struct {
	th     *look.Theme
	cfg    config.Config
	space  spaces.Space
	branch textinput.Model
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir textinput.Model
	// typed is the branch field as followBranch last saw it, so it works out what that names only when it
	// changes, not at every cursor blink.
	typed string
	// The PR the branch field names, when isPR, and the lookups of each PR it has named, by gh's argument.
	// prSeq counts the changes of the field, so only a lookup it rested on starts.
	pr      spaces.PRRef
	isPR    bool
	prSeq   int
	lookups map[string]prLookup
	busy    bool
	err     error
	width   int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*newWorktreeForm]
}

// prLookup is what looking a PR up found, or, until done, that it is under way.
type prLookup struct {
	pr   spaces.PR
	err  error
	done bool
}

// prDebounce is how long the branch field rests on a PR before it is looked up, so typing a number does
// not run gh for every digit.
const prDebounce = 300 * time.Millisecond

// prDueMsg is the branch field having rested on a PR, change seq of it, for prDebounce.
type prDueMsg struct {
	seq int
	ref spaces.PRRef
}

// prFoundMsg is the lookup of the PR gh knows as arg.
type prFoundMsg struct {
	arg string
	prLookup
}

// newForm is the form for a worktree in space. The branch starts with the configured prefix, so typing
// just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	branch := newInput(th, "")
	branch.SetValue(spaces.BranchStart(cfg))
	branch.CursorEnd()
	branch.Focus()
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, dir: newInput(th, ""), lookups: map[string]prLookup{}}
	f.followBranch()
	return f
}

type createdMsg struct{ err error }

func (f *newWorktreeForm) Init() tea.Cmd {
	return textinput.Blink
}

// lookup is what looking up the PR the branch field names has found so far, ok false when it names none.
func (f *newWorktreeForm) lookup() (l prLookup, ok bool) {
	if !f.isPR {
		return prLookup{}, false
	}
	return f.lookups[f.pr.Arg], true
}

// plan is the branch and directory the form would create now. For a PR that is its local branch, empty
// until it has been looked up.
func (f *newWorktreeForm) plan() (branch, dir string) {
	branch = strings.TrimSpace(f.branch.Value())
	if l, ok := f.lookup(); ok {
		branch = l.pr.Branch
	}
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, space := f.cfg, f.space
	if l, ok := f.lookup(); ok {
		ref, dir := f.pr, f.dir.Value()
		return func() tea.Msg {
			// Enter may come before the lookup, or after one that failed for a reason since fixed.
			pr, err := l.pr, l.err
			if !l.done || err != nil {
				pr, err = spaces.ResolvePR(space, ref)
			}
			if err != nil {
				return createdMsg{err}
			}
			wt, err := spaces.CreatePRWorktree(space, pr, spaces.WorktreeDir(cfg, space.Root, pr.Branch, dir))
			if err != nil {
				return createdMsg{err}
			}
			return createdMsg{spaces.OpenFromOverlay(cfg, wt)}
		}
	}
	branch, dir := f.plan()
	return func() tea.Msg {
		wt, err := spaces.CreateWorktree(space, branch, dir)
		if err != nil {
			return createdMsg{err}
		}
		return createdMsg{spaces.OpenFromOverlay(cfg, wt)}
	}
}

// switchField moves typing between the branch and the directory.
func (f *newWorktreeForm) switchField() tea.Cmd {
	if f.branch.Focused() {
		f.branch.Blur()
		return f.dir.Focus()
	}
	f.dir.Blur()
	return f.branch.Focus()
}

func (f *newWorktreeForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		f.branch.SetWidth(max(f.width-4-labelWidth-1, 1))
		f.dir.SetWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case prDueMsg:
		if msg.seq != f.prSeq || !f.isPR {
			return f, nil
		}
		if _, asked := f.lookups[msg.ref.Arg]; asked {
			return f, nil
		}
		f.lookups[msg.ref.Arg] = prLookup{}
		space, ref := f.space, msg.ref
		// gh asks GitHub, which takes a while, so off the UI loop.
		return f, func() tea.Msg {
			pr, err := spaces.ResolvePR(space, ref)
			return prFoundMsg{ref.Arg, prLookup{pr: pr, err: err, done: true}}
		}
	case prFoundMsg:
		f.lookups[msg.arg] = msg.prLookup
		f.followBranch()
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theForm.Target(formSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	// Everything else is typing in the field that has focus.
	var cmd tea.Cmd
	if f.branch.Focused() {
		f.branch, cmd = f.branch.Update(msg)
	} else {
		f.dir, cmd = f.dir.Update(msg)
	}
	return f, tea.Batch(cmd, f.followBranch())
}

// followBranch works out whether the branch field names a PR, and makes the directory's placeholder the
// default for the branch it would create, so the form always shows where the worktree would go. A PR the
// field has come to name is looked up once it rests on it, which the command it returns waits for.
func (f *newWorktreeForm) followBranch() tea.Cmd {
	var cmd tea.Cmd
	if typed := f.branch.Value(); typed != f.typed {
		f.typed = typed
		ref, isPR := spaces.ParsePRRef(f.cfg, f.space, typed)
		if isPR != f.isPR || ref != f.pr {
			f.pr, f.isPR = ref, isPR
			f.prSeq++
			if _, asked := f.lookups[ref.Arg]; isPR && !asked {
				seq := f.prSeq
				cmd = tea.Tick(prDebounce, func(time.Time) tea.Msg { return prDueMsg{seq, ref} })
			}
		}
	}
	branch, _ := f.plan()
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, branch, "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	f.dir.Placeholder = dir
	return cmd
}

func (f *newWorktreeForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// labelWidth is the column the fields start at, after their labels.
const labelWidth = 11

func (f *newWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	branch, _ := f.plan()
	lines := []string{
		line(look.Heading("New worktree in " + f.space.Name)), "",
		th.Draw(look.Plain, labelWidth, look.Label("branch")) + f.branch.View(),
		th.Draw(look.Plain, labelWidth, look.Label("directory")) + f.dir.View(),
		"",
	}
	failed := f.err
	if l, ok := f.lookup(); ok {
		pr := fmt.Sprintf("PR #%d", f.pr.Number)
		switch {
		case !l.done:
			lines = append(lines, muted("Looking up "+pr+"..."))
		case l.err != nil:
			lines = append(lines, muted("Could not look up "+pr))
			if failed == nil {
				failed = l.err
			}
		default:
			lines = append(lines, muted(pr+" "+l.pr.Title), muted(prDoes(l.pr)))
		}
	} else {
		switch {
		case branch == "" || strings.HasSuffix(branch, "/"):
			lines = append(lines, muted("Name the branch to create or check out, or a PR"))
		case f.space.HasBranch(branch):
			lines = append(lines, muted("Checks out the existing branch "+branch))
		case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
			lines = append(lines, muted("Creates "+branch+" from "+f.space.Worktrees[0].Branch))
		default:
			lines = append(lines, muted("Creates "+branch))
		}
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Creating...")), "")
	case failed != nil:
		for _, row := range look.Error(failed.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*newWorktreeForm]{}
	buttons := f.buttons.draw(f, theForm.Target(formSubject{}),
		control{action: actCreate, label: "Create"},
		control{action: actCancel, label: "Cancel"})
	lines = append(lines, line(append(buttons, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))...))
	return boxed(lines)
}

// prDoes is what creating the worktree does with pr.
func prDoes(pr spaces.PR) string {
	switch {
	case pr.Exists:
		return "Checks out the existing branch " + pr.Branch
	case pr.Cross && pr.Branch != pr.HeadRef:
		return "Checks out " + pr.Owner + ":" + pr.HeadRef + " from " + pr.Remote + " as " + pr.Branch
	case pr.Cross:
		return "Checks out " + pr.Owner + ":" + pr.HeadRef + " from " + pr.Remote
	default:
		return "Checks out " + pr.Branch + " from " + pr.Remote + ", tracking it"
	}
}

// RunNewWorktree runs the new worktree form for space, meant for a kitty overlay, until the worktree is
// created and opened or the form is left.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space) error {
	if _, err := tea.NewProgram(newForm(cfg, space), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
