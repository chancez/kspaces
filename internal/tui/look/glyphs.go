package look

import (
	"os"
	"strings"
)

// Glyphs are the symbols a theme draws with: plain ASCII, Unicode symbols nearly every font has, or
// Nerd Font icons. A font's glyphs cannot be asked about, so Nerd Font icons are used only where the
// terminal is known to bring them.
type Glyphs int

// A theme draws in ASCII until told otherwise.
const (
	GlyphsASCII Glyphs = iota
	GlyphsUnicode
	GlyphsNerd
)

// ParseGlyphs is the glyphs named by s: "ascii", "unicode", or "nerd".
func ParseGlyphs(s string) (Glyphs, bool) {
	switch s {
	case "ascii":
		return GlyphsASCII, true
	case "unicode":
		return GlyphsUnicode, true
	case "nerd":
		return GlyphsNerd, true
	}
	return 0, false
}

// GlyphsFor is the glyphs a terminal can draw, from what it calls itself in answer to XTVERSION:
// Nerd Font icons in the terminals that bundle them, as kitty, WezTerm, and Ghostty do, and
// Unicode elsewhere.
func GlyphsFor(terminal string) Glyphs {
	t := strings.ToLower(terminal)
	for _, name := range []string{"kitty", "wezterm", "ghostty"} {
		if strings.Contains(t, name) {
			return GlyphsNerd
		}
	}
	return GlyphsUnicode
}

// GlyphsFromEnv is the glyphs the environment says the terminal can draw, before it answers
// XTVERSION or where it does not: ASCII on the Linux console, Nerd Font icons where a terminal that
// bundles them says so, and Unicode elsewhere.
func GlyphsFromEnv() Glyphs {
	switch {
	case os.Getenv("TERM") == "linux":
		return GlyphsASCII
	case os.Getenv("KITTY_WINDOW_ID") != "", os.Getenv("TERM") == "xterm-kitty", os.Getenv("GHOSTTY_RESOURCES_DIR") != "":
		return GlyphsNerd
	}
	return GlyphsFor(os.Getenv("TERM_PROGRAM"))
}

// Glyph is a symbol's role, drawn as the theme's glyphs have it. An empty glyph is not drawn, as a
// button's icon in ASCII.
//
// The roles kspaces draws, and each glyphs' symbols for them, are in symbols.go.
type Glyph int

// SetGlyphs makes the theme draw with g.
func (t *Theme) SetGlyphs(g Glyphs) { t.glyphs = g }

// Glyph is how the theme draws g.
func (t *Theme) Glyph(g Glyph) string { return glyphSets[t.glyphs][g] }

// glyph is a span drawn as the theme's g, in el. A gap puts a space after it, when it is drawn at
// all, as between a button's icon and its label.
func glyph(el element, g Glyph, gap bool) Span {
	return Span{el: el, glyph: g, gap: gap}
}
