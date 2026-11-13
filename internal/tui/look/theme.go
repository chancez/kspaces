// Package look is the TUI's registry of how things are drawn, as gr's ADR 8 has it. A theme assigns
// colors to roles, and styles every element and surface from the roles.
//
// Code that draws builds lines of spans and says which surface they sit on. A span is made only by this
// package's constructors, each for one kind of value, such as a space or a branch, so a value is always
// drawn as its own element. Elements are unexported, so nothing else can pair a value with the wrong one.
package look

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// roles are what a theme's colors are for. Elements and surfaces are styled from roles, so a theme that
// changes what danger looks like changes it everywhere.
//
// kspaces' colors are terminal palette indexes rather than fixed colors, so the sidebar follows the kitty
// theme. A control's chip is the exception: no palette index is a quiet step off the terminal's
// background, so it takes gr's, from palette.go, for a light or a dark one.
type roles struct {
	muted  color.Color
	accent color.Color
	// other is for sessions that belong to no space.
	other color.Color
	// success, danger, and attention are for states: open or done, errors, and things waiting on you.
	success   color.Color
	danger    color.Color
	attention color.Color
	// merged is for a merged pull request, as GitHub draws one.
	merged color.Color
	// onSolid is text on a solid background, such as a button that confirms losing work.
	onSolid color.Color
	// controlBg is a control's chip, and onControl what is quiet on it, a key or a greyed label, which
	// the palette's muted would all but hide on a grey chip.
	controlBg color.Color
	onControl color.Color
}

func newRoles(isDark bool) roles {
	ld := lipgloss.LightDark(isDark)
	return roles{
		muted:     lipgloss.Color("8"),
		accent:    lipgloss.Color("4"),
		other:     lipgloss.Color("6"),
		success:   lipgloss.Color("2"),
		danger:    lipgloss.Color("1"),
		attention: lipgloss.Color("3"),
		merged:    lipgloss.Color("5"),
		onSolid:   lipgloss.Color("15"),
		controlBg: ld(paleGray, charcoal),
		onControl: ld(slate, ash),
	}
}

// element is a kind of text on screen. Its style sets a foreground and attributes, never a background,
// which comes from the surface the text sits on.
type element int

const (
	// elDefault is text in the terminal's own foreground.
	elDefault element = iota
	elText
	elMuted
	// elTitle heads a list, as SPACES and STATUS.
	elTitle
	// elGroup heads a group of spaces in the spaces list.
	elGroup
	// elHeading heads a screen, as the new worktree form's.
	elHeading
	// elCurrent marks the sidebar's own session's row: its edge, or its caret.
	elCurrent
	elSpace
	elOther
	elWorktree
	elBranch
	// elCaret folds a space's linked worktrees.
	elCaret
	// elTick is a row's box in the select mode while it is ticked, or partly.
	elTick
	// elSession marks a session that is current or open.
	elSession
	elBroken
	// elGone marks a missing worktree whose session still has windows: running, but in a directory that
	// is gone.
	elGone
	elStateBlocked
	elStateError
	elStateDone
	elStateWorking
	elStateIdle
	// elWaitsFor is what a blocked program waits for.
	elWaitsFor
	// elMatch is what a query matched.
	elMatch
	// elLabel names a form's field.
	elLabel
	// elBusy is something under way, as creating a worktree.
	elBusy
	elKey
	elKeyDesc
	elError
	elBorder
	elBorderActive
	// elButton is a control's label, elButtonKey its key and the label of one greyed.
	elButton
	elButtonKey
	// elButtonOn is a toggle's label while it is on.
	elButtonOn
	// elChanges is how many files a worktree has not committed.
	elChanges
	// elPR* are a pull request's number, by its state.
	elPROpen
	elPRDraft
	elPRMerged
	elPRClosed
	// elChecks* are what a PR's checks come to, elReview* its review decision.
	elChecksPassing
	elChecksFailing
	elChecksPending
	elReviewApproved
	elReviewChangesRequested
	numElements
)

// elementNames are the names a config file would override elements by.
var elementNames = [numElements]string{
	elDefault:                "default",
	elText:                   "text",
	elMuted:                  "muted",
	elTitle:                  "title",
	elCurrent:                "current",
	elGroup:                  "group",
	elHeading:                "heading",
	elSpace:                  "space",
	elOther:                  "other-session",
	elWorktree:               "worktree",
	elBranch:                 "branch",
	elCaret:                  "caret",
	elTick:                   "tick",
	elSession:                "session",
	elBroken:                 "broken",
	elGone:                   "gone",
	elStateBlocked:           "state.blocked",
	elStateError:             "state.error",
	elStateDone:              "state.done",
	elStateWorking:           "state.working",
	elStateIdle:              "state.idle",
	elWaitsFor:               "waits-for",
	elMatch:                  "match",
	elLabel:                  "label",
	elBusy:                   "busy",
	elKey:                    "key",
	elKeyDesc:                "key-desc",
	elError:                  "error",
	elBorder:                 "border",
	elBorderActive:           "border.active",
	elButton:                 "button",
	elButtonKey:              "button.key",
	elButtonOn:               "button.on",
	elChanges:                "git.changes",
	elPROpen:                 "pr.open",
	elPRDraft:                "pr.draft",
	elPRMerged:               "pr.merged",
	elPRClosed:               "pr.closed",
	elChecksPassing:          "pr.checks.passing",
	elChecksFailing:          "pr.checks.failing",
	elChecksPending:          "pr.checks.pending",
	elReviewApproved:         "pr.review.approved",
	elReviewChangesRequested: "pr.review.changes-requested",
}

// Prop is an option a place asks an element for. The theme defines what each does.
type Prop uint8

const (
	// Dim is for entries with nothing open: a worktree without a session, or an empty list's note.
	Dim Prop = 1 << iota
	// Lit marks a mark as where keys act, reversed, as the select mode's box that space ticks: the
	// cursor's mark rather than a bar across the row.
	Lit
	// Strong is bold, as the name on the sidebar's own session's row.
	Strong
)

// Surface is where a line sits. It owns the background.
type Surface int

const (
	Plain Surface = iota
	// Cursor is the entry keys act on, while keys are moving it.
	Cursor
	// Control is a control's own background, a chip on whatever it sits on.
	Control
	// ControlSolid is a one-cell control that has to stand out on any line.
	ControlSolid
	numSurfaces
)

var surfaceNames = [numSurfaces]string{
	Plain:        "surface.plain",
	Cursor:       "surface.cursor",
	Control:      "surface.control",
	ControlSolid: "surface.control-solid",
}

// surfaceStyle is how a surface draws. A soft surface keeps its elements' colors. A solid one draws them
// all in its own colors, bold when it is: a bar reads as one piece, and colored text on a solid color is
// unreadable. A reversed one swaps the terminal's own colors, so it shows in any theme.
type surfaceStyle struct {
	bg      color.Color
	solid   bool
	fg      color.Color
	bold    bool
	reverse bool
}

// Theme is the registry: the roles, and the style they give each element and surface.
type Theme struct {
	isDark   bool
	roles    roles
	elements [numElements]lipgloss.Style
	surfaces [numSurfaces]surfaceStyle
	glyphs   Glyphs
}

// New is kspaces' theme, drawing in Nerd Font icons, for a dark background or a light one. Its colors are
// the terminal's palette, which suits either alike, but for the controls' chips, see roles.
func New(isDark bool) *Theme {
	p := newRoles(isDark)
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	bold := func(c color.Color) lipgloss.Style { return fg(c).Bold(true) }
	t := &Theme{isDark: isDark, roles: p, glyphs: GlyphsNerd}
	t.elements = [numElements]lipgloss.Style{
		elDefault: lipgloss.NewStyle(),
		elText:    lipgloss.NewStyle(),
		elMuted:   fg(p.muted),
		// The terminal's own foreground, not muted, which read as disabled.
		elTitle: lipgloss.NewStyle().Bold(true),
		// Muted, and not bold, so it heads its spaces without competing with SPACES above them.
		elGroup:   fg(p.muted),
		elHeading: bold(p.accent),
		// In the accent, the full height of the row, rather than a bar across it, which would read as the
		// cursor's, or a chip's grey.
		elCurrent:                fg(p.accent),
		elSpace:                  bold(p.accent),
		elOther:                  bold(p.other),
		elWorktree:               lipgloss.NewStyle(),
		elBranch:                 fg(p.muted),
		elCaret:                  fg(p.muted),
		elTick:                   fg(p.accent),
		elSession:                fg(p.success),
		elBroken:                 fg(p.danger),
		elGone:                   fg(p.attention),
		elStateBlocked:           bold(p.attention),
		elStateError:             fg(p.danger),
		elStateDone:              fg(p.success),
		elStateWorking:           fg(p.accent),
		elStateIdle:              fg(p.muted),
		elWaitsFor:               bold(p.attention),
		elMatch:                  bold(p.attention),
		elLabel:                  fg(p.muted),
		elBusy:                   fg(p.accent),
		elKey:                    fg(p.muted),
		elKeyDesc:                fg(p.muted),
		elError:                  fg(p.danger),
		elBorder:                 fg(p.muted),
		elBorderActive:           fg(p.accent),
		elButton:                 lipgloss.NewStyle().Bold(true),
		elButtonKey:              fg(p.onControl),
		elButtonOn:               bold(p.attention),
		elChanges:                fg(p.attention),
		elPROpen:                 fg(p.success),
		elPRDraft:                fg(p.muted),
		elPRMerged:               fg(p.merged),
		elPRClosed:               fg(p.danger),
		elChecksPassing:          fg(p.success),
		elChecksFailing:          fg(p.danger),
		elChecksPending:          fg(p.attention),
		elReviewApproved:         fg(p.success),
		elReviewChangesRequested: fg(p.danger),
	}
	t.surfaces = [numSurfaces]surfaceStyle{
		Plain:  {},
		Cursor: {solid: true, reverse: true},
		// A chip, as gr's and GitHub's buttons: a quiet shade the label stays bold on, in the text's own
		// color, rather than reversed, which reads as a selected row.
		Control: {bg: p.controlBg},
		// kspaces' only solid controls confirm closing and removing, which lose what runs or is not
		// committed, so they are drawn in danger's color.
		ControlSolid: {bg: p.danger, solid: true, fg: p.onSolid, bold: true},
	}
	return t
}

// SetDark makes the theme the one for a dark background or a light one, as the terminal says it has once
// it answers, keeping its glyphs.
func (t *Theme) SetDark(isDark bool) {
	g := t.glyphs
	*t = *New(isDark)
	t.glyphs = g
}

// style is how el with props draws on surface s.
func (t *Theme) style(el element, p Prop, s Surface) lipgloss.Style {
	ss := t.surfaces[s]
	if ss.solid {
		st := lipgloss.NewStyle().Bold(ss.bold).Reverse(ss.reverse != (p&Lit != 0))
		if ss.fg != nil {
			st = st.Foreground(ss.fg)
		}
		if ss.bg != nil {
			st = st.Background(ss.bg)
		}
		return st
	}
	st := t.elements[el]
	if p&Dim != 0 {
		st = st.Faint(true)
	}
	if p&Strong != 0 {
		st = st.Bold(true)
	}
	if p&Lit != 0 {
		st = st.Reverse(true)
	}
	if ss.bg != nil {
		st = st.Background(ss.bg)
	}
	return st
}
