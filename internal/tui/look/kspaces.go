package look

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
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

// Wrapped is s broken into rows of at most width cells, each drawn as s is, by WrapWords.
func (s Span) Wrapped(width int) []Span {
	var rows []Span
	for _, text := range WrapWords(s.text, width) {
		r := s
		r.text = text
		rows = append(rows, r)
	}
	return rows
}

// WrapWords breaks text into rows of at most width cells at spaces, and inside words longer than a row.
func WrapWords(text string, width int) []string {
	var rows []string
	for _, para := range strings.Split(text, "\n") {
		row := ""
		for _, word := range strings.Fields(para) {
			for ansi.StringWidth(word) > width {
				if row != "" {
					rows, row = append(rows, row), ""
				}
				r := []rune(word)
				rows, word = append(rows, string(r[:width])), string(r[width:])
			}
			switch {
			case row == "":
				row = word
			case ansi.StringWidth(row)+1+ansi.StringWidth(word) <= width:
				row += " " + word
			default:
				rows, row = append(rows, row), word
			}
		}
		if row != "" {
			rows = append(rows, row)
		}
	}
	return rows
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

// AreaStyles are a text area's styles in the theme, as InputStyles are a text input's, so the two look
// alike side by side.
func (t *Theme) AreaStyles() textarea.Styles {
	state := textarea.StyleState{Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240"))}
	return textarea.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textarea.CursorStyle{Color: lipgloss.NoColor{}, Shape: tea.CursorBlock, Blink: true},
	}
}
