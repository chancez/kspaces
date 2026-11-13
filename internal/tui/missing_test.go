package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// goneIcon heads a missing worktree's row, with the two spaces a marker takes.
const goneIcon = "\uf127  "

// missingFixture is gr with a linked worktree and two missing ones, gr@open with a window and gr@saved
// with only a saved file, beside default and nope@x, a session of a space that is not configured.
func missingFixture(sessionsDir string) spaces.Snapshot {
	cfg := config.Config{SessionsDir: sessionsDir}
	return spaces.Snapshot{
		Config: &cfg,
		Spaces: []spaces.Space{{
			Name: "gr", Root: "/src/gr",
			Worktrees: []spaces.Worktree{
				{Path: "/src/gr", Branch: "main", Session: "gr"},
				{Path: "/src/gr/.worktrees/dev", Branch: "dev", Session: "gr@dev"},
			},
			Missing: []spaces.Worktree{{Session: "gr@open"}, {Session: "gr@saved"}},
		}},
		Others: []spaces.Space{
			{Name: "default", Worktrees: []spaces.Worktree{{Session: "default"}}},
			{Name: "nope@x", Worktrees: []spaces.Worktree{{Session: "nope@x"}}},
		},
		Folded: map[string]bool{},
		Saved:  map[string]bool{"gr@saved": true, "default": true},
		Kitty: kitty.State{
			Sessions: map[string]int{"gr": 10, "gr@open": 11, "default": 12, "nope@x": 13},
			Windows:  map[string]int{"gr": 1, "gr@open": 2, "default": 1, "nope@x": 1},
			Current:  "gr",
			Home:     10,
		},
	}
}

func newMissingEnv(t *testing.T, sessionsDir string) *sidebarEnv {
	t.Helper()
	e := newSidebarEnv(t)
	e.m.Update(tea.WindowSizeMsg{Width: 30, Height: 16})
	e.m.Update(missingFixture(sessionsDir))
	e.m.render()
	return e
}

// A space's missing worktrees are under it after its linked ones, with the gone marker and `gone` for a
// branch, faint once their session has no windows; a session of a space that is not configured stays with
// default. Folding the space hides them with its linked worktrees.
func TestMissingWorktreesOnScreen(t *testing.T) {
	e := newMissingEnv(t, "")
	want := []string{
		spacesTitle,
		"",
		"  " + termIcon + "default",
		"  " + termIcon + "nope@x",
		"",
		"\u25bc " + repoIcon + "gr main",
		"  " + branchIcon + "dev dev",
		"  " + goneIcon + "open gone",
		"  " + goneIcon + "saved gone",
		strings.Repeat("\u2500", 29),
		"STATUS",
		"Nothing to report",
		"",
		"",
		"",
		"",
	}
	if got := e.screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("screen:\n%q\nwant\n%q", got, want)
	}
	// One with windows open is drawn as open, its marker yellow, so it does not read as finished; one with
	// only a saved file is faint.
	lines := strings.Split(e.m.render(), "\n")
	row := func(session string, open bool) string {
		r := e.m.rows[e.row(session)]
		spans := slices.Concat([]look.Span{look.NoCaret()}, look.DimIf(!open, look.GoneMarker(open)...),
			look.DimIf(!open, look.MissingWorktree(r)), []look.Span{look.Muted(" ")}, look.DimIf(!open, look.Gone()))
		return e.m.th.Draw(look.Plain, e.m.contentWidth(), spans...)
	}
	for i, c := range []struct {
		session string
		open    bool
	}{{"gr@open", true}, {"gr@saved", false}} {
		line := ansi.Truncate(lines[7+i], e.m.contentWidth(), "")
		if got, want := cells.Parse(line), cells.Parse(row(c.session, c.open)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", c.session, line, row(c.session, c.open))
		}
	}

	e.click(fmt.Sprintf(caretZone, e.row("gr")), tea.MouseLeft)
	want = append(append(append([]string{}, want[:5]...), "\u25b6 "+repoIcon+"gr main", "", "", ""), want[9:]...)
	if got := e.screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("folded:\n%q\nwant\n%q", got, want)
	}
}

// A missing worktree offers no more than its session does: opening and closing it while it has windows,
// and forgetting its saved file when that is all that is left. One with no windows cannot be opened, as
// that would make it afresh in a directory that is gone.
func TestMissingWorktreeMenu(t *testing.T) {
	e := newMissingEnv(t, "")
	launched := fakeKitten(t)
	titles := func(session string) []string {
		e.click(fmt.Sprintf(entryZone, e.row(session)), tea.MouseRight)
		var out []string
		for _, it := range e.m.menu.items {
			out = append(out, it.Title)
		}
		e.m.menu = nil
		e.m.render()
		return out
	}
	got := [][]string{titles("gr@open"), titles("gr@saved")}
	want := [][]string{
		{"Open", "Close session", "Fold worktrees", "Select", "Add space"},
		{"Forget", "Fold worktrees", "Select", "Add space"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("menus %q, want %q", got, want)
	}
	if cmd := e.click(fmt.Sprintf(entryZone, e.row("gr@saved")), tea.MouseLeft); cmd != nil {
		t.Error("a click on gr@saved opened it")
	}

	// x, which removes a worktree, forgets a missing one, in a form.
	e.m.Update(tea.FocusMsg{})
	e.m.cursor = e.row("gr@saved")
	cmd := e.press("x")
	if cmd == nil {
		t.Fatal("x on gr@saved ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("forgetting said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:10", exe, "forget", "gr@saved"}; !reflect.DeepEqual(got, want) {
		t.Errorf("x launched %q, want %q", got, want)
	}
}

// The forget form says what it deletes, and Delete deletes that file only.
func TestForgetSessionForm(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"gr", "gr@dev", "gr@saved"} {
		if err := os.WriteFile(filepath.Join(dir, name+".kitty-session"), []byte("launch\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &forgetSessionForm{th: look.New(true), cfg: config.Config{SessionsDir: dir}, session: "gr@saved"}
	f.Update(tea.WindowSizeMsg{Width: 300, Height: 20})
	var screen []string
	for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
		screen = append(screen, strings.TrimRight(l, " "))
	}
	want := []string{
		"",
		"  Forget session gr@saved",
		"",
		"  Deletes its saved file, so kitty does not restore it. Its worktree directory is gone.",
		"  " + config.Tilde(filepath.Join(dir, "gr@saved.kitty-session")),
		"",
		"   Delete y    Cancel n",
		"",
	}
	if !reflect.DeepEqual(screen, want) {
		t.Errorf("form:\n%q\nwant\n%q", screen, want)
	}
	if _, cmd := f.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd == nil || f.busy {
		t.Errorf("n: quit %v, busy %v", cmd != nil, f.busy)
	}
	f.View()
	cmd := clickOn(t, f, "Delete y")
	if cmd == nil || !f.busy {
		t.Fatalf("Delete: cmd %v, busy %v", cmd != nil, f.busy)
	}
	if _, cmd := f.Update(cmd()); cmd == nil || f.err != nil {
		t.Fatalf("after deleting: quit %v, err %v", cmd != nil, f.err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if want := []string{"gr.kitty-session", "gr@dev.kitty-session"}; !reflect.DeepEqual(left, want) {
		t.Errorf("left %q, want %q", left, want)
	}

	// Again, with the file gone, the error stays on screen.
	f.Update(f.forget()())
	if f.err == nil || f.busy {
		t.Errorf("forgetting a forgotten session: err %v, busy %v", f.err, f.busy)
	}
}

// The picker lists missing worktrees with the gone marker, its preview says their directory is gone, and
// one with no windows is not opened.
func TestPickerMissingWorktrees(t *testing.T) {
	m := newPicker(0)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 26})
	m.Update(missingFixture("/sessions"))
	want := []string{
		"\uf002  space, worktree or status",
		strings.Repeat("\u2500", 40),
		open + "default",
		open + "nope@x",
		"\uf054  gr  main",
		"   gr / dev  dev",
		goneIcon + "gr / open  gone",
		goneIcon + "gr / saved  gone",
	}
	if got := pickerScreen(m)[:len(want)]; !reflect.DeepEqual(got, want) {
		t.Errorf("picker:\n%q\nwant\n%q", got, want)
	}
	preview := func() []string {
		var out []string
		for _, l := range m.preview(40, 10) {
			out = append(out, m.th.TextOf(l...))
		}
		return out
	}
	down(m, 4)
	got := [][]string{preview()}
	down(m, 1)
	got = append(got, preview())
	wantPreviews := [][]string{
		{
			"gr / open", "",
			"space    gr",
			"path     gone, the worktree directory was removed",
			"session  gr@open",
			"windows  2 open",
			"saved    no",
		},
		{
			"gr / saved", "",
			"space    gr",
			"path     gone, the worktree directory was removed",
			"session  gr@saved",
			"windows  none open",
			"saved    yes",
		},
	}
	if !reflect.DeepEqual(got, wantPreviews) {
		t.Errorf("previews:\n%q\nwant\n%q", got, wantPreviews)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := cmd()
	if snap, ok := msg.(spaces.Snapshot); !ok || snap.Err == nil || snap.Err.Error() != "the worktree directory of gr@saved is gone" {
		t.Errorf("choosing gr@saved: %#v", msg)
	}
}
