package look

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/cells"
)

func TestEveryElementAndSurfaceIsNamed(t *testing.T) {
	for i, n := range elementNames {
		if n == "" {
			t.Errorf("element %d has no name", i)
		}
	}
	for i, n := range surfaceNames {
		if n == "" {
			t.Errorf("surface %d has no name", i)
		}
	}
}

// Every glyphs' set draws every role, in one cell, so rows keep their columns whichever is used.
func TestGlyphSets(t *testing.T) {
	th := New(true)
	for _, g := range []Glyphs{GlyphsASCII, GlyphsUnicode, GlyphsNerd} {
		th.SetGlyphs(g)
		for role := GlyphNone + 1; role < numGlyphs; role++ {
			if w := th.Width(glyph(elDefault, role, false)); w != 1 {
				t.Errorf("glyphs %d: role %d is %d cells wide", g, role, w)
			}
		}
	}
}

func TestThemeDraw(t *testing.T) {
	th := New(true)
	p := th.roles
	for name, tc := range map[string]struct {
		got  string
		want string
	}{
		"plain keeps element colors": {
			got:  th.Draw(Plain, 6, Space(spaces.Space{Name: "gr"}), Branch(spaces.Worktree{Branch: " x"})),
			want: lipgloss.NewStyle().Bold(true).Foreground(p.accent).Render("gr") + lipgloss.NewStyle().Foreground(p.muted).Render(" x") + "  ",
		},
		"the cursor reverses the whole line": {
			got:  th.Draw(Cursor, 6, Space(spaces.Space{Name: "gr"}), Branch(spaces.Worktree{Branch: " x"})),
			want: lipgloss.NewStyle().Reverse(true).Render("gr x  "),
		},
		"the current session is a solid bar": {
			got:  th.Render(Current, Space(spaces.Space{Name: "gr"})),
			want: lipgloss.NewStyle().Background(p.muted).Foreground(p.onSolid).Bold(true).Render("gr"),
		},
		"dim is faint and keeps the element's look": {
			got:  th.Render(Plain, Space(spaces.Space{Name: "gr"}).With(Dim)),
			want: lipgloss.NewStyle().Bold(true).Faint(true).Foreground(p.accent).Render("gr"),
		},
		"text is made safe": {
			got:  th.Render(Plain, Text("a\x1b[5m")),
			want: "a^[[5m",
		},
	} {
		if !reflect.DeepEqual(cells.Parse(tc.got), cells.Parse(tc.want)) {
			t.Errorf("%s:\ngot  %q\nwant %q", name, tc.got, tc.want)
		}
	}
}

// A missing worktree's marker is yellow while its session has windows, so it reads as running but not
// right, and muted once it has none.
func TestGoneMarker(t *testing.T) {
	th := New(true)
	th.SetGlyphs(GlyphsASCII)
	p := th.roles
	for _, c := range []struct {
		open bool
		want string
	}{
		{true, lipgloss.NewStyle().Foreground(p.attention).Render("x  ")},
		{false, lipgloss.NewStyle().Foreground(p.muted).Render("x  ")},
	} {
		if got := th.Render(Plain, GoneMarker(c.open)...); !reflect.DeepEqual(cells.Parse(got), cells.Parse(c.want)) {
			t.Errorf("open %v:\ngot  %q\nwant %q", c.open, got, c.want)
		}
	}
}

func TestCut(t *testing.T) {
	th := New(true)
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/release", Branch: "main"}
	for _, tc := range []struct {
		width int
		want  string
	}{
		{12, "\ue725  release"},
		{8, "\ue725  re..."},
		{3, "\ue725  "},
		{2, "\ue725 "},
	} {
		if got := th.TextOf(th.Cut(tc.width, append(RowIcon(RowWorktree, "", true, false), Worktree(wt))...)...); got != tc.want {
			t.Errorf("cut to %d = %q, want %q", tc.width, got, tc.want)
		}
	}
}

func TestZones(t *testing.T) {
	th := New(true)
	line := th.DrawZone("row", Plain, 10, append([]Span{Text("ab ")}, Mark("caret", Caret(false)...)...)...)
	frame := "first line\n" + line
	clean, zones := Scan(frame)
	if strings.Contains(clean, "grz") {
		t.Errorf("Scan left marks in %q", clean)
	}
	// Each marked span is a zone of its own: the caret is its glyph and the space after it.
	want := map[string][]Zone{"row": {{X: 0, Y: 1, W: 10}}, "caret": {{X: 3, Y: 1, W: 1}, {X: 4, Y: 1, W: 1}}}
	if !reflect.DeepEqual(zones, want) {
		t.Errorf("zones = %+v, want %+v", zones, want)
	}
}

func TestWrapWords(t *testing.T) {
	got := WrapWords("run the full test suite\nthen deploy supercalifragilistic", 10)
	want := []string{"run the", "full test", "suite", "then", "deploy", "supercalif", "ragilistic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
