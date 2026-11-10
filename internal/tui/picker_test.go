package tui

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

func pickerScreen(m *picker) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

func typeInto(m tea.Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// The picker lists status entries, then every session and worktree in sidebar order, and narrows them as
// the query is typed, with the matched characters highlighted.
func TestPicker(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 14})
	m.Update(fixture(time.Now()))
	got := pickerScreen(m)[1:12]
	want := []string{
		strings.Repeat("\u2500", 40),
		blocked + "gr@dev  approve",
		"\ue654  scratch  exit 1",
		"\uf110  docs@notes  make",
		"\uf10c  default  claude",
		open + "default",
		open + "gr  main",
		blocked + "gr / dev  pr/me/dev",
		"   gr / fix  fix",
		"   kitty  master",
		"   docs  main",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("picker:\n%q\nwant\n%q", got, want)
	}

	if got := pickerScreen(m)[0]; got != "\uf002  space, worktree or status" {
		t.Errorf("prompt %q", got)
	}

	typeInto(m, "grfx")
	if got := pickerScreen(m)[2:4]; !reflect.DeepEqual(got, []string{"   gr / fix  fix", ""}) {
		t.Errorf("after grfx:\n%q", got)
	}
	// The cursor's entry is a bar; the others show what matched.
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	lines := strings.Split(m.render(), "\n")
	if !strings.Contains(lines[2], m.th.Render(look.Plain, look.Match("g"))) || !strings.Contains(lines[3], "\x1b[7m") {
		t.Errorf("after gr and ctrl+n:\n%q", lines[2:4])
	}
	if got := pickerScreen(m)[13]; got != "4/11" {
		t.Errorf("count line %q", got)
	}

	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Error("enter chose nothing")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil || cmd() != tea.Quit() {
		t.Error("esc does not quit")
	}
}

// The form starts the branch with the configured prefix, shows where the worktree goes and what creating
// it does, and keeps git's refusal on screen.
func TestNewWorktreeForm(t *testing.T) {
	root := t.TempDir()
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}, space)
	f.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	screen := func() []string {
		var out []string
		for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
		return out
	}
	want := []string{
		"",
		"  New worktree in gr",
		"",
		"  branch     pr/me/",
		"  directory  .worktrees",
		"",
		"  Name the branch to create or check out",
		"",
		"   Create enter    Cancel esc   tab next field",
		"",
	}
	if got := screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}

	typeInto(f, "fix/x")
	want[3], want[4], want[6] = "  branch     pr/me/fix/x", "  directory  .worktrees/fix-x", "  Creates pr/me/fix/x from main"
	if got := screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("after typing a branch:\n%q\nwant\n%q", got, want)
	}
	// tab moves typing to the directory.
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	typeInto(f, "../x")
	if branch, dir := f.plan(); branch != "pr/me/fix/x" || dir != root[:strings.LastIndex(root, "/")]+"/x" {
		t.Errorf("plan %q in %q", branch, dir)
	}

	_, cmd := f.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !f.busy || screen()[8] != "  Creating..." {
		t.Fatalf("enter: busy %v:\n%s", f.busy, strings.Join(screen(), "\n"))
	}
	if _, again := f.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); again != nil {
		t.Error("a second enter while creating created again")
	}
	// The space is no repository, so git refuses, and the form says why and stays.
	if _, quit := f.Update(cmd()); quit != nil || f.busy || !strings.Contains(strings.Join(screen(), "\n"), "not a git repository") {
		t.Errorf("after git refused: busy %v, quit %v:\n%s", f.busy, quit != nil, strings.Join(screen(), "\n"))
	}
}

// The remove worktree form says what goes with the worktree, and y starts removing it while n leaves.
func TestRemoveWorktreeForm(t *testing.T) {
	// Resolved, as git reports a worktree's path.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{
		{Path: root, Branch: "main", Session: "gr"},
		{Path: filepath.Join(root, ".worktrees", "dev"), Branch: "pr/me/dev", Session: "gr@dev"},
	}}
	form := func(changes int) *removeWorktreeForm {
		f := &removeWorktreeForm{th: look.New(true), space: space, wt: space.Worktrees[1], changes: changes, windows: 2}
		f.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		return f
	}
	screen := func(f *removeWorktreeForm) []string {
		var out []string
		for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
		return out
	}
	f := form(3)
	want := []string{
		"",
		"  Remove worktree dev from gr",
		"",
		"  directory  .worktrees/dev",
		"  branch     pr/me/dev kept",
		"  session    2 windows closed",
		"  changes    3 files not committed, lost",
		"",
		"   Remove y    Cancel n",
		"",
	}
	if got := screen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd == nil || f.busy {
		t.Errorf("n: quit %v, busy %v", cmd != nil, f.busy)
	}

	f = form(0)
	if got := screen(f)[6]; got != "  changes    none" {
		t.Errorf("changes row of a clean worktree = %q", got)
	}
	// The command removes the worktree for real, and closes kitty windows, so it is not run here.
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd == nil || !f.busy || screen(f)[8] != "  Removing..." {
		t.Errorf("y: busy %v:\n%s", f.busy, strings.Join(screen(f), "\n"))
	}
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd != nil {
		t.Error("n while removing left the form")
	}
}

// y on a locked worktree says git would refuse, rather than starting a removal that cannot succeed.
func TestRemoveLockedWorktree(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{
		{Path: root, Branch: "main", Session: "gr"},
		{Path: filepath.Join(root, "dev"), Branch: "dev", Session: "gr@dev", Locked: true},
	}}
	f := &removeWorktreeForm{th: look.New(true), space: space, wt: space.Worktrees[1]}
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd != nil || f.busy {
		t.Errorf("y on a locked worktree: cmd %v, busy %v", cmd != nil, f.busy)
	}
	if got := ansi.Strip(f.render()); !strings.Contains(got, "is locked; git worktree unlock it first") {
		t.Errorf("no word of the lock:\n%s", got)
	}
}

// The close session form says how many windows close, y closes and n leaves.
func TestCloseSessionForm(t *testing.T) {
	f := &closeSessionForm{th: look.New(true), session: "gr@dev", windows: 3}
	f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	var got []string
	for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
		got = append(got, strings.TrimRight(l, " "))
	}
	want := []string{
		"",
		"  Close session gr@dev",
		"",
		"  Closes its 3 windows, and what runs in them.",
		"",
		"   Close y    Cancel n",
		"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd == nil || f.busy {
		t.Errorf("n: quit %v, busy %v", cmd != nil, f.busy)
	}
	// The command closes kitty windows, so it is not run here.
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd == nil || !f.busy {
		t.Errorf("y: cmd %v, busy %v", cmd != nil, f.busy)
	}
}

// clickOn clicks the first cell of label on the form's screen, as the last View drew it.
func clickOn(t *testing.T, f tea.Model, label string) tea.Cmd {
	t.Helper()
	frame := f.View().Content
	for y, l := range strings.Split(ansi.Strip(frame), "\n") {
		if x := strings.Index(l, label); x >= 0 {
			_, cmd := f.Update(tea.MouseClickMsg{X: ansi.StringWidth(l[:x]), Y: y, Button: tea.MouseLeft})
			return cmd
		}
	}
	t.Fatalf("no %q on screen:\n%s", label, ansi.Strip(frame))
	return nil
}

// A click on a form's button does what its key does, and a greyed one does nothing.
func TestFormButtons(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{
		{Path: root, Branch: "main", Session: "gr"},
		{Path: filepath.Join(root, "dev"), Branch: "dev", Session: "gr@dev"},
	}}
	remove := func(locked bool) *removeWorktreeForm {
		wt := space.Worktrees[1]
		wt.Locked = locked
		f := &removeWorktreeForm{th: look.New(true), space: space, wt: wt}
		f.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		return f
	}
	type result struct {
		Cmd  bool
		Busy bool
	}
	f := remove(false)
	cmd := clickOn(t, f, "Cancel")
	if got, want := (result{cmd != nil, f.busy}), (result{true, false}); got != want {
		t.Errorf("Cancel: %+v, want %+v", got, want)
	}
	// The command launches kitty's background process, so it is not run here.
	cmd = clickOn(t, f, "Remove y")
	if got, want := (result{cmd != nil, f.busy}), (result{true, true}); got != want {
		t.Errorf("Remove: %+v, want %+v", got, want)
	}
	f = remove(true)
	cmd = clickOn(t, f, "Remove")
	if got, want := (result{cmd != nil, f.busy}), (result{false, false}); got != want {
		t.Errorf("Remove on a locked worktree: %+v, want %+v", got, want)
	}

	n := newForm(config.Config{}, space)
	n.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	cmd = clickOn(t, n, "Cancel")
	if got, want := (result{cmd != nil, n.busy}), (result{true, false}); got != want {
		t.Errorf("Cancel a new worktree: %+v, want %+v", got, want)
	}
	// The command creates a worktree, so it is not run here.
	cmd = clickOn(t, n, "Create")
	if got, want := (result{cmd != nil, n.busy}), (result{true, true}); got != want {
		t.Errorf("Create: %+v, want %+v", got, want)
	}

	c := &closeSessionForm{th: look.New(true), session: "gr@dev", windows: 1}
	c.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	cmd = clickOn(t, c, "Close y")
	if got, want := (result{cmd != nil, c.busy}), (result{true, true}); got != want {
		t.Errorf("Close: %+v, want %+v", got, want)
	}
}
