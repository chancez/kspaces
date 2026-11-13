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
	// GlyphGone marks a missing worktree: a session whose worktree directory is gone.
	GlyphGone
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
	// GlyphAdd is the button that adds a space.
	GlyphAdd
	// GlyphSortManual, GlyphSortActivity and GlyphSortStatus are the sort button in each mode.
	GlyphSortManual
	GlyphSortActivity
	GlyphSortStatus
	// GlyphFilter is the attention filter's button.
	GlyphFilter
	// GlyphChosen marks the menu item chosen now, as the sort mode.
	GlyphChosen
	// GlyphChanges marks how many files a worktree has not committed, GlyphAhead and GlyphBehind how many
	// commits it is ahead of and behind its upstream.
	GlyphChanges
	GlyphAhead
	GlyphBehind
	// GlyphChecksPassing, GlyphChecksFailing and GlyphChecksPending are what a PR's CI checks come to,
	// after its number; GlyphApproved and GlyphChangesRequested its review decision.
	GlyphChecksPassing
	GlyphChecksFailing
	GlyphChecksPending
	GlyphApproved
	GlyphChangesRequested
	// GlyphUntrusted marks a space whose own config is not trusted, so not in use.
	GlyphUntrusted
	// The corners and sides of the border a popup in a panel is drawn in.
	GlyphFrameTopLeft
	GlyphFrameTopRight
	GlyphFrameBottomLeft
	GlyphFrameBottomRight
	GlyphFrameHorizontal
	GlyphFrameVertical
	// GlyphCut marks where text was cut to fit.
	GlyphCut
	numGlyphs
)

// glyphSets are each glyphs' symbols, by role. Each is one cell wide, so a row's columns stay put
// whichever is drawn; only ASCII's cut mark is wider, as ASCII has no one-cell ellipsis. Nerd Font icons
// are Font Awesome's and Codicons', at the code points Nerd Fonts
// keeps them at.
var glyphSets = map[Glyphs][numGlyphs]string{
	GlyphsASCII: {
		GlyphOpen:             "v",
		GlyphFolded:           ">",
		GlyphBlocked:          "!",
		GlyphError:            "x",
		GlyphDone:             "+",
		GlyphWorking:          "*",
		GlyphIdle:             "o",
		GlyphCurrent:          ">",
		GlyphSession:          "*",
		GlyphBroken:           "x",
		GlyphGone:             "x",
		GlyphSpace:            "#",
		GlyphWorktree:         "-",
		GlyphOther:            "$",
		GlyphSearch:           "/",
		GlyphMoreAbove:        "^",
		GlyphMoreBelow:        "v",
		GlyphAdd:              "+",
		GlyphSortManual:       "=",
		GlyphSortActivity:     "@",
		GlyphSortStatus:       "!",
		GlyphFilter:           "Y",
		GlyphChosen:           "*",
		GlyphChanges:          "*",
		GlyphAhead:            "+",
		GlyphBehind:           "-",
		GlyphChecksPassing:    "v",
		GlyphChecksFailing:    "x",
		GlyphChecksPending:    "~",
		GlyphApproved:         "A",
		GlyphChangesRequested: "R",
		GlyphUntrusted:        "?",
		GlyphFrameTopLeft:     "+",
		GlyphFrameTopRight:    "+",
		GlyphFrameBottomLeft:  "+",
		GlyphFrameBottomRight: "+",
		GlyphFrameHorizontal:  "-",
		GlyphFrameVertical:    "|",
		GlyphCut:              "...",
	},
	GlyphsUnicode: {
		GlyphOpen:             "\u25bc", // black down-pointing triangle
		GlyphFolded:           "\u25b6", // black right-pointing triangle
		GlyphBlocked:          "!",
		GlyphError:            "\u2717", // ballot x
		GlyphDone:             "\u2713", // check mark
		GlyphWorking:          "\u2026", // horizontal ellipsis
		GlyphIdle:             "\u25cb", // white circle
		GlyphCurrent:          "\u203a", // single right angle quote
		GlyphSession:          "\u2022", // bullet
		GlyphBroken:           "\u2717",
		GlyphGone:             "\u2715", // multiplication x
		GlyphSpace:            "\u25a0", // black square
		GlyphWorktree:         "\u2514", // box drawings up and right
		GlyphOther:            "\u25aa", // black small square
		GlyphSearch:           "/",
		GlyphMoreAbove:        "\u25b4", // small up triangle
		GlyphMoreBelow:        "\u25be", // small down triangle
		GlyphAdd:              "+",
		GlyphSortManual:       "\u2261", // identical to
		GlyphSortActivity:     "\u25f7", // white circle with upper right quadrant, a clock face
		GlyphSortStatus:       "!",
		GlyphFilter:           "\u25bd", // white down-pointing triangle, a funnel
		GlyphChosen:           "\u2713", // check mark
		GlyphChanges:          "\u25cf", // black circle
		GlyphAhead:            "\u2191", // upwards arrow
		GlyphBehind:           "\u2193", // downwards arrow
		GlyphChecksPassing:    "\u2713", // check mark
		GlyphChecksFailing:    "\u2717", // ballot x
		GlyphChecksPending:    "\u25cb", // white circle
		GlyphApproved:         "\u2605", // black star
		GlyphChangesRequested: "\u00b1", // plus-minus sign, changes
		GlyphUntrusted:        "?",
		GlyphFrameTopLeft:     "\u256d", // box drawings light arc down and right
		GlyphFrameTopRight:    "\u256e", // box drawings light arc down and left
		GlyphFrameBottomLeft:  "\u2570", // box drawings light arc up and right
		GlyphFrameBottomRight: "\u256f", // box drawings light arc up and left
		GlyphFrameHorizontal:  "\u2500", // box drawings light horizontal
		GlyphFrameVertical:    "\u2502", // box drawings light vertical
		GlyphCut:              "\u2026", // horizontal ellipsis
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
		GlyphGone:     "\uf127", // chain-broken
		GlyphSpace:    "\uf401", // repo
		GlyphWorktree: "\ue725", // git-branch
		GlyphOther:    "\uf120", // terminal
		GlyphSearch:   "\uf002", // search
		// The small triangles, for the same reason as the fold triangles.
		GlyphMoreAbove:    "\u25b4",
		GlyphMoreBelow:    "\u25be",
		GlyphAdd:          "\uf067", // plus
		GlyphSortManual:   "\uf0c9", // bars
		GlyphSortActivity: "\uf017", // clock
		GlyphSortStatus:   "\uf071", // warning
		GlyphFilter:       "\uf0b0", // filter
		GlyphChosen:       "\uf00c", // check
		// Unicode rather than Nerd Font icons: they are followed by a number, not a space, or end the row.
		GlyphChanges:          "\u25cf",
		GlyphAhead:            "\u2191",
		GlyphBehind:           "\u2193",
		GlyphChecksPassing:    "\u2713",
		GlyphChecksFailing:    "\u2717",
		GlyphChecksPending:    "\u25cb",
		GlyphApproved:         "\u2605",
		GlyphChangesRequested: "\u00b1",
		// Followed by two spaces, as a marker is, since kitty draws it over the first.
		GlyphUntrusted: "\uf023", // lock
		// Box drawings, as Unicode's: kitty draws them itself, joined.
		GlyphFrameTopLeft:     "\u256d",
		GlyphFrameTopRight:    "\u256e",
		GlyphFrameBottomLeft:  "\u2570",
		GlyphFrameBottomRight: "\u256f",
		GlyphFrameHorizontal:  "\u2500",
		GlyphFrameVertical:    "\u2502",
		GlyphCut:              "\u2026",
	},
}
