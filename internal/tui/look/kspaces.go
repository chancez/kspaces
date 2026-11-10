package look

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// What kspaces draws with on top of draw.go and zones.go, kept apart from them so they stay as gr has
// them.

// Width is how many cells spans take, as the theme draws them.
func (t *Theme) Width(spans ...Span) int {
	return ansi.StringWidth(t.TextOf(spans...))
}

// Cut cuts spans to width cells, marking the cut with "..." in the style of the span it falls in, so a
// name and a branch each show where they were cut. A width of 3 or less has no room for the mark and is
// cut bare. Glyphs are drawn as their text, so a cut can fall in one.
func (t *Theme) Cut(width int, spans ...Span) []Span {
	if t.Width(spans...) <= width {
		return spans
	}
	keep, mark := width, ""
	if width > 3 {
		keep, mark = width-3, "..."
	}
	var out []Span
	for _, x := range spans {
		x.text, x.glyph, x.gap = t.text(x), GlyphNone, false
		r := []rune(x.text)
		if len(r) >= keep {
			x.text = string(r[:max(keep, 0)]) + mark
			return append(out, x)
		}
		keep -= len(r)
		out = append(out, x)
	}
	return out
}

// DrawZone is Draw, with the whole line, its padding too, marked as zone id, for a row a click anywhere
// on lands on.
func (t *Theme) DrawZone(id string, s Surface, width int, spans ...Span) string {
	return markZone(id, t.Draw(s, width, spans...))
}

// InputStyles are a text input's styles in the theme: the terminal's own colors, a muted placeholder, and
// a block cursor in reverse video.
func (t *Theme) InputStyles() textinput.Styles {
	state := textinput.StyleState{
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Suggestion:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: lipgloss.NoColor{}, Shape: tea.CursorBlock, Blink: true},
	}
}
