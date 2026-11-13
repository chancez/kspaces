package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// panelScreen is what p draws, its styles left out.
func panelScreen(p *popup) []string {
	return strings.Split(ansi.Strip(p.View().Content), "\n")
}

// newWorktreePanel is the new worktree form for gr in a panel of its size, opened from window 11.
func newWorktreePanel(t *testing.T) *popup {
	t.Helper()
	gittest.Isolate(t)
	root := t.TempDir()
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	p, _ := stubPopup(newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}, space), 11, false)
	p.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return p
}

// In a panel a form is drawn in a rounded border, its heading set into the top, inset a cell, its buttons
// on the row above the bottom, and blank rows between: nothing outside the border. ASCII draws it in
// +, - and |.
func TestFormInPanel(t *testing.T) {
	for _, c := range []struct {
		glyphs                                           look.Glyphs
		topLeft, topRight, bottomLeft, bottomRight, h, v string
	}{
		{look.GlyphsNerd, "\u256d", "\u256e", "\u2570", "\u256f", "\u2500", "\u2502"},
		{look.GlyphsUnicode, "\u256d", "\u256e", "\u2570", "\u256f", "\u2500", "\u2502"},
		{look.GlyphsASCII, "+", "+", "+", "+", "-", "|"},
	} {
		p := newWorktreePanel(t)
		p.th.SetGlyphs(c.glyphs)
		row := func(s string) string { return c.v + " " + s + strings.Repeat(" ", 86-len(s)) + " " + c.v }
		want := []string{
			c.topLeft + c.h + " New worktree in gr " + strings.Repeat(c.h, 67) + c.topRight,
			row(""),
			row(" Branch   Pull request"),
			row(""),
			row("branch     pr/me/"),
			row("directory  .worktrees"),
			row(""),
			row("Name the branch to create or check out, or # for a PR"),
		}
		for len(want) < 22 {
			want = append(want, row(""))
		}
		want = append(want,
			row(" Create enter    Cancel esc   tab next field"),
			c.bottomLeft+strings.Repeat(c.h, 88)+c.bottomRight,
		)
		if got := panelScreen(p); !reflect.DeepEqual(got, want) {
			t.Errorf("glyphs %d:\n%s\nwant\n%s", c.glyphs, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// A click on a button the frame moved to the bottom row reaches it.
func TestFormInPanelClick(t *testing.T) {
	p := newWorktreePanel(t)
	p.View()
	// Cancel, on the row above the bottom border.
	if _, cmd := p.Update(tea.MouseClickMsg{X: 20, Y: 22, Button: tea.MouseLeft}); !quits(cmd) {
		t.Error("a click on Cancel did not quit")
	}
}

// In an overlay a form is drawn as it always was.
func TestFormInOverlay(t *testing.T) {
	gittest.Isolate(t)
	root := t.TempDir()
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}, space)
	p, _ := stubPopup(f, 0, false)
	p.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	if got, want := p.View().Content, f.View().Content; got != want {
		t.Errorf("overlay:\n%s\nwant\n%s", got, want)
	}
}

// The picker lays itself out inside the border, a cell in from its sides, filling it.
func TestPickerInPanelFrame(t *testing.T) {
	m := newPicker(0)
	p, _ := stubPopup(m, 11, false)
	p.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	p.Update(fixture(time.Now()))
	if m.width != 116 || m.height != 30 {
		t.Fatalf("the picker is %dx%d, want 116x30", m.width, m.height)
	}
	want := []string{"\u256d" + strings.Repeat("\u2500", 118) + "\u256e"}
	for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
		want = append(want, "\u2502 "+l+strings.Repeat(" ", 116-ansi.StringWidth(l))+" \u2502")
	}
	want = append(want, "\u2570"+strings.Repeat("\u2500", 118)+"\u256f")
	got := panelScreen(p)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("picker:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(got) != 32 {
		t.Errorf("%d rows, want 32", len(got))
	}
}
