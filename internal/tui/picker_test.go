package tui

import (
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
		"  tab next field  enter create  esc cancel",
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
