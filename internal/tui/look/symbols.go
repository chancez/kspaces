package look

// The roles kspaces draws symbols for, and each glyphs' symbols for them. kspaces runs inside kitty,
// which ships the Nerd Fonts symbols, so its themes draw in Nerd Font icons unless told otherwise.
const (
	// GlyphNone draws nothing, as for a control without an icon.
	GlyphNone Glyph = iota
	// GlyphOpen and GlyphFolded head a space whose linked worktrees are shown or hidden.
	GlyphOpen
	GlyphFolded
	// The program status states, as kitty draws them for tab.program_status.
	GlyphBlocked
	GlyphError
	GlyphDone
	GlyphWorking
	GlyphIdle
	// GlyphCurrent marks the session of the sidebar's own tab, GlyphSession a session with windows open.
	GlyphCurrent
	GlyphSession
	// GlyphBroken marks a space whose worktrees could not be listed.
	GlyphBroken
	// GlyphSpace, GlyphWorktree and GlyphOther head the rows of a space, a linked worktree and a session in
	// no space.
	GlyphSpace
	GlyphWorktree
	GlyphOther
	// GlyphSearch prompts for the picker's query.
	GlyphSearch
	// GlyphMoreAbove and GlyphMoreBelow mark entries of a list scrolled out of view.
	GlyphMoreAbove
	GlyphMoreBelow
	numGlyphs
)

// glyphSets are each glyphs' symbols, by role. Each is one cell wide, so a row's columns stay put
// whichever is drawn. Nerd Font icons are Font Awesome's and Codicons', at the code points Nerd Fonts
// keeps them at.
var glyphSets = map[Glyphs][numGlyphs]string{
	GlyphsASCII: {
		GlyphOpen:      "v",
		GlyphFolded:    ">",
		GlyphBlocked:   "!",
		GlyphError:     "x",
		GlyphDone:      "+",
		GlyphWorking:   "*",
		GlyphIdle:      "o",
		GlyphCurrent:   ">",
		GlyphSession:   "*",
		GlyphBroken:    "x",
		GlyphSpace:     "#",
		GlyphWorktree:  "-",
		GlyphOther:     "$",
		GlyphSearch:    "/",
		GlyphMoreAbove: "^",
		GlyphMoreBelow: "v",
	},
	GlyphsUnicode: {
		GlyphOpen:      "\u25bc", // black down-pointing triangle
		GlyphFolded:    "\u25b6", // black right-pointing triangle
		GlyphBlocked:   "!",
		GlyphError:     "\u2717", // ballot x
		GlyphDone:      "\u2713", // check mark
		GlyphWorking:   "\u2026", // horizontal ellipsis
		GlyphIdle:      "\u25cb", // white circle
		GlyphCurrent:   "\u203a", // single right angle quote
		GlyphSession:   "\u2022", // bullet
		GlyphBroken:    "\u2717",
		GlyphSpace:     "\u25a0", // black square
		GlyphWorktree:  "\u2514", // box drawings up and right
		GlyphOther:     "\u25aa", // black small square
		GlyphSearch:    "/",
		GlyphMoreAbove: "\u25b4", // small up triangle
		GlyphMoreBelow: "\u25be", // small down triangle
	},
	GlyphsNerd: {
		// The triangles, not Font Awesome's chevrons, which are drawn wider than their cell and thin.
		GlyphOpen:   "\u25bc",
		GlyphFolded: "\u25b6",
		// The ones kitty uses for tab.program_status.
		GlyphBlocked:  "\uf071", // warning
		GlyphError:    "\ue654", // error
		GlyphDone:     "\uf00c", // check
		GlyphWorking:  "\uf110", // spinner
		GlyphIdle:     "\uf10c", // circle outline
		GlyphCurrent:  "\uf054", // chevron-right
		GlyphSession:  "\uf444", // dot-fill
		GlyphBroken:   "\uf00d", // times
		GlyphSpace:    "\uf401", // repo
		GlyphWorktree: "\ue725", // git-branch
		GlyphOther:    "\uf120", // terminal
		GlyphSearch:   "\uf002", // search
		// The small triangles, for the same reason as the fold triangles.
		GlyphMoreAbove: "\u25b4",
		GlyphMoreBelow: "\u25be",
	},
}
