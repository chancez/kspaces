package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
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

// A space's missing worktrees are under it after its linked ones, faint with the gone marker and `gone`
// for a branch, open or not; a session of a space that is not configured stays with default. Folding the
// space hides them with its linked worktrees.
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
	// Faint even with windows open, where an open worktree is not.
	line := strings.Split(e.m.render(), "\n")[7]
	faint := e.m.th.Render(look.Plain, look.DimIf(true, look.MissingWorktree(e.m.rows[e.row("gr@open")]))...)
	if !strings.Contains(line, faint) {
		t.Errorf("gr@open is not faint: %q", line)
	}

	e.click(fmt.Sprintf(caretZone, e.row("gr")), tea.MouseLeft)
	want = append(append(append([]string{}, want[:5]...), "\u25b6 "+repoIcon+"gr main", "", "", ""), want[9:]...)
	if got := e.screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("folded:\n%q\nwant\n%q", got, want)
	}
}

// A missing worktree offers no more than its session does: opening and closing it while it has windows.
// One with none cannot be opened, as that would make it afresh in a directory that is gone.
func TestMissingWorktreeMenu(t *testing.T) {
	e := newMissingEnv(t, "")
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
		{"Open", "Fold worktrees", "Close session...", "Add space..."},
		{"Fold worktrees", "Add space..."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("menus %q, want %q", got, want)
	}
	if cmd := e.click(fmt.Sprintf(entryZone, e.row("gr@saved")), tea.MouseLeft); cmd != nil {
		t.Error("a click on gr@saved opened it")
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
