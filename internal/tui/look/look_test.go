package look

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/chancez/kspaces/internal/gh"
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

// Every glyphs' set draws every role, in one cell, so rows keep their columns whichever is used; ASCII's
// boxes in three, all of them, so a row keeps its columns ticked or not, and its cut mark, which ends a
// cut, so it moves no column after it.
func TestGlyphSets(t *testing.T) {
	th := New(true)
	for _, g := range []Glyphs{GlyphsASCII, GlyphsUnicode, GlyphsNerd} {
		th.SetGlyphs(g)
		for role := GlyphNone + 1; role < numGlyphs; role++ {
			want := 1
			if g == GlyphsASCII && (role == GlyphCut || role >= GlyphUnticked && role <= GlyphUntickable) {
				want = 3
			}
			if w := th.Width(glyph(elDefault, role, false)); w != want {
				t.Errorf("glyphs %d: role %d is %d cells wide, want %d", g, role, w, want)
			}
		}
	}
}

// Cut keeps width less the mark, and cuts bare where there is no room for the mark and a cell of text.
func TestCutMarksPerGlyphs(t *testing.T) {
	th := New(true)
	for _, tc := range []struct {
		glyphs Glyphs
		width  int
		want   string
	}{
		{GlyphsASCII, 0, ""},
		{GlyphsASCII, 1, "a"},
		{GlyphsASCII, 3, "abc"},
		{GlyphsASCII, 4, "a..."},
		{GlyphsASCII, 7, "abcd..."},
		{GlyphsASCII, 8, "abcdefgh"},
		{GlyphsUnicode, 0, ""},
		{GlyphsUnicode, 1, "a"},
		{GlyphsUnicode, 2, "a\u2026"},
		{GlyphsUnicode, 7, "abcdef\u2026"},
		{GlyphsUnicode, 8, "abcdefgh"},
		{GlyphsNerd, 1, "a"},
		{GlyphsNerd, 2, "a\u2026"},
		{GlyphsNerd, 4, "abc\u2026"},
		{GlyphsNerd, 9, "abcdefgh"},
	} {
		th.SetGlyphs(tc.glyphs)
		if got := th.TextOf(th.Cut(tc.width, Text("abcdefgh"))...); got != tc.want {
			t.Errorf("glyphs %d, width %d: got %q, want %q", tc.glyphs, tc.width, got, tc.want)
		}
	}
}

// A group's header draws its rule and its fold icon in each glyphs' own symbols, all muted.
func TestGroupHeaderGlyphs(t *testing.T) {
	th := New(true)
	var got []string
	for _, g := range []Glyphs{GlyphsASCII, GlyphsUnicode, GlyphsNerd} {
		th.SetGlyphs(g)
		for _, folded := range []bool{false, true} {
			spans := append([]Span{th.GroupRule(2), Muted(" "), GroupHeader("work"), Muted(" ")}, GroupFold(folded)...)
			got = append(got, th.TextOf(append(spans, th.GroupRule(3))...))
		}
	}
	want := []string{
		"-- WORK -  ---", "-- WORK +  ---",
		"\u2500\u2500 WORK \u2212  \u2500\u2500\u2500", "\u2500\u2500 WORK +  \u2500\u2500\u2500",
		"\u2500\u2500 WORK \uf07c  \u2500\u2500\u2500", "\u2500\u2500 WORK \uf07b  \u2500\u2500\u2500",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	muted := lipgloss.NewStyle().Foreground(th.roles.muted)
	if got, want := th.Render(Plain, GroupFold(false)...), muted.Render("\uf07c")+muted.Render("  "); got != want {
		t.Errorf("fold icon %q, want %q", got, want)
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
		"the cursor is a bar of the chips' shade, the text in its own color, its bold kept": {
			got: th.Draw(Cursor, 6, Space(spaces.Space{Name: "gr"}), Branch(spaces.Worktree{Branch: " x"})),
			want: lipgloss.NewStyle().Background(charcoal).Bold(true).Render("gr") +
				lipgloss.NewStyle().Background(charcoal).Render(" x  "),
		},
		"the current session is marked at its edge in the accent, its name bold": {
			got: th.Render(Plain, append(CurrentEdge(), Worktree(spaces.Worktree{Path: "/src/gr/dev"}).With(Strong))...),
			want: lipgloss.NewStyle().Foreground(p.accent).Render("\u258e") + lipgloss.NewStyle().Foreground(p.muted).Render(" ") +
				lipgloss.NewStyle().Bold(true).Render("dev"),
		},
		"a current space's caret is in the accent": {
			got:  th.Render(Plain, CurrentCaret(false)...),
			want: lipgloss.NewStyle().Foreground(p.accent).Render("\u25bc") + " ",
		},
		"dim is faint and keeps the element's look": {
			got:  th.Render(Plain, Space(spaces.Space{Name: "gr"}).With(Dim)),
			want: lipgloss.NewStyle().Bold(true).Faint(true).Foreground(p.accent).Render("gr"),
		},
		"text is made safe": {
			got:  th.Render(Plain, Text("a\x1b[5m")),
			want: "a^[[5m",
		},
		"a button is a chip: its label bold in the text's color, its key quiet": {
			got: th.Render(Plain, Button("Remove", "y")...),
			want: lipgloss.NewStyle().Background(charcoal).Render(" ") + lipgloss.NewStyle().Background(charcoal).Bold(true).Render("Remove") +
				lipgloss.NewStyle().Background(charcoal).Render(" ") + lipgloss.NewStyle().Background(charcoal).Foreground(ash).Render("y") +
				lipgloss.NewStyle().Background(charcoal).Render(" "),
		},
		"a greyed button keeps its chip": {
			got: th.Render(Plain, DisabledButton("Close")...),
			want: lipgloss.NewStyle().Background(charcoal).Render(" ") + lipgloss.NewStyle().Background(charcoal).Foreground(ash).Render("Close") +
				lipgloss.NewStyle().Background(charcoal).Render(" "),
		},
		"on a light background the chip is light": {
			got: New(false).Render(Plain, Button("Cancel", "n")...),
			want: lipgloss.NewStyle().Background(paleGray).Render(" ") + lipgloss.NewStyle().Background(paleGray).Bold(true).Render("Cancel") +
				lipgloss.NewStyle().Background(paleGray).Render(" ") + lipgloss.NewStyle().Background(paleGray).Foreground(slate).Render("n") +
				lipgloss.NewStyle().Background(paleGray).Render(" "),
		},
		"a button for what loses work is solid in danger's color": {
			got:  th.Render(Plain, DangerButton("Remove", "y")...),
			want: lipgloss.NewStyle().Background(p.danger).Foreground(p.onSolid).Bold(true).Render(" Remove y "),
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

// A PR's checks are drawn green passing, red failing and yellow pending, its review green approved and
// red with changes requested, and faint on a row with nothing open, as its number is. A review required
// and no checks draw nothing.
func TestPRChecksAndReview(t *testing.T) {
	th := New(true)
	th.SetGlyphs(GlyphsASCII)
	p := th.roles
	fg := func(c color.Color, s string) string { return lipgloss.NewStyle().Foreground(c).Render(s) }
	faint := func(c color.Color, s string) string { return lipgloss.NewStyle().Faint(true).Foreground(c).Render(s) }
	pr := func(c gh.Checks, r gh.Review) gh.PR { return gh.PR{Number: 7, State: "open", Checks: c, Review: r} }
	for name, tc := range map[string]struct {
		got  []Span
		want string
	}{
		"passing":           {PRChecks(pr(gh.ChecksPassing, gh.ReviewNone)), fg(p.success, "v")},
		"failing":           {PRChecks(pr(gh.ChecksFailing, gh.ReviewNone)), fg(p.danger, "x")},
		"pending":           {PRChecks(pr(gh.ChecksPending, gh.ReviewNone)), fg(p.attention, "~")},
		"no checks":         {PRChecks(pr(gh.ChecksNone, gh.ReviewApproved)), ""},
		"approved":          {PRReview(pr(gh.ChecksNone, gh.ReviewApproved)), fg(p.success, "A")},
		"changes requested": {PRReview(pr(gh.ChecksNone, gh.ReviewChangesRequested)), fg(p.danger, "R")},
		"review required":   {PRReview(pr(gh.ChecksPassing, gh.ReviewRequired)), ""},
		"no review":         {PRReview(pr(gh.ChecksPassing, gh.ReviewNone)), ""},
		"dim": {
			DimIf(true, PRNumber(pr(gh.ChecksFailing, gh.ReviewApproved)), PRChecks(pr(gh.ChecksFailing, gh.ReviewApproved))[0], PRReview(pr(gh.ChecksFailing, gh.ReviewApproved))[0]),
			faint(p.success, "#7") + faint(p.danger, "x") + faint(p.success, "A"),
		},
	} {
		if got := th.Render(Plain, tc.got...); !reflect.DeepEqual(cells.Parse(got), cells.Parse(tc.want)) {
			t.Errorf("%s:\ngot  %q\nwant %q", name, got, tc.want)
		}
	}
	words := func(c gh.Checks, r gh.Review) []string {
		return []string{th.TextOf(PRChecksWords(pr(c, r))), th.TextOf(PRReviewWords(pr(c, r)))}
	}
	got := [][]string{words(gh.ChecksPassing, gh.ReviewApproved), words(gh.ChecksFailing, gh.ReviewChangesRequested), words(gh.ChecksPending, gh.ReviewRequired)}
	want := [][]string{{"passing", "approved"}, {"failing", "changes requested"}, {"pending", "review required"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("words %q, want %q", got, want)
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
		{8, "\ue725  rele\u2026"},
		{3, "\ue725 \u2026"},
		{2, "\ue725\u2026"},
		{1, "\ue725"},
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
