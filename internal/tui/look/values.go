package look

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
)

// Constructors for each kind of value kspaces draws. Each takes the value itself, so the element and the
// format come from its type.

// A marker is the icon at the start of a row. kitty draws a Nerd Font symbol over two cells when a space
// follows it, so each is followed by two spaces: one it draws over and one to keep it off the text.
func marker(el element, g Glyph) []Span {
	return []Span{glyph(el, g, false), sp(el, "  ")}
}

// NoMarker is as wide as a marker, for a row without one.
func NoMarker() Span { return sp(elDefault, "   ") }

// StateMarker is the marker of a program status state.
func StateMarker(state string) []Span {
	el, g := stateLook(state)
	return marker(el, g)
}

func stateLook(state string) (element, Glyph) {
	switch state {
	case "blocked":
		return elStateBlocked, GlyphBlocked
	case "error":
		return elStateError, GlyphError
	case "done":
		return elStateDone, GlyphDone
	case "idle":
		return elStateIdle, GlyphIdle
	}
	return elStateWorking, GlyphWorking
}

// CurrentMarker marks the session of the sidebar's own tab.
func CurrentMarker() []Span { return marker(elSession, GlyphCurrent) }

// SessionMarker marks a session with windows open.
func SessionMarker() []Span { return marker(elSession, GlyphSession) }

// BrokenMarker marks a space whose worktrees could not be listed.
func BrokenMarker() []Span { return marker(elBroken, GlyphBroken) }

// Caret heads a space with linked worktrees: pointing down while they are shown, right when folded.
func Caret(folded bool) []Span {
	g := GlyphOpen
	if folded {
		g = GlyphFolded
	}
	return []Span{glyph(elCaret, g, false), sp(elCaret, " ")}
}

// NoCaret is as wide as a caret, for a row without one, and indents linked worktrees under their space.
func NoCaret() Span { return sp(elCaret, "  ") }

// Space is a space's name, on its own row.
func Space(s spaces.Space) Span { return sp(elSpace, s.Name) }

// OtherSession is a session that belongs to no space, by its name.
func OtherSession(s spaces.Space) Span { return sp(elOther, s.Name) }

// Worktree is a linked worktree, by its directory, after its icon.
func Worktree(wt spaces.Worktree) []Span {
	return append(marker(elWorktree, GlyphWorktree), sp(elWorktree, filepath.Base(wt.Path)))
}

// Branch is the branch a worktree has checked out.
func Branch(wt spaces.Worktree) Span { return sp(elBranch, wt.Branch) }

// stateGroups name the groups of status entries in a state, in the order the list sorts them.
var stateGroups = map[string]string{"blocked": "Blocked", "error": "Failed", "done": "Done", "working": "Working", "idle": "Idle"}

// StateGroup titles the group of status entries in state, with how many it has.
func StateGroup(state string, n int) []Span {
	el, _ := stateLook(state)
	return []Span{sp(el, stateGroups[state]), sp(elMuted, fmt.Sprintf(" %d", n))}
}

// StatusLabel names the window of a status entry: by its session, or its title for a window in no
// session. A linked worktree's session is SPACE@DIR; the worktree's name alone fits the narrow dock and is
// what its row in the spaces list shows.
func StatusLabel(s status.Status) Span {
	label := s.Session
	if _, dir, ok := strings.Cut(label, "@"); ok {
		label = dir
	}
	if label == "" {
		label = s.WindowTitle
	}
	return sp(elDefault, label)
}

// StatusAge is how many other records a status entry's window has and how long ago its record changed to
// its state, after a space, or nothing when neither is known.
func StatusAge(s status.Status, now time.Time) Span {
	right := status.Age(s.Since, now)
	if len(s.Others) > 0 {
		right = strings.TrimSpace(fmt.Sprintf("+%d %s", len(s.Others), right))
	}
	if right != "" {
		right = " " + right
	}
	return sp(elMuted, right)
}

// waitsFor says what a blocked program is waiting for, from the protocol's kind field.
var waitsFor = map[string]string{"permission": "wants permission", "question": "has a question", "auth": "needs you to sign in"}

// WaitsFor is what a blocked program waits for, empty for any other.
func WaitsFor(r status.Record) Span { return sp(elWaitsFor, waitsFor[r.Kind]) }

// Summary is the one line that best says what a program is up to, width cells at most before it is cut:
// its progress, its title, its message, or what it is blocked on, after the program's name.
func Summary(r status.Record, width int) Span {
	text := r.Title
	switch {
	case r.Progress != nil:
		text = ProgressBar(*r.Progress, max(width-len([]rune(r.App))-3, 6))
	case text == "" && r.Msg != "":
		text = r.Msg
	case text == "":
		text = waitsFor[r.Kind]
	}
	switch {
	case r.App == "":
	case text == "":
		text = r.App
	default:
		text = r.App + " \u00b7 " + text
	}
	return sp(elMuted, text)
}

// ProgressBar draws a percentage as a bar that fits width, followed by the number.
func ProgressBar(percent, width int) string {
	n := max(min(width-5, 16), 1)
	filled := n * min(max(percent, 0), 100) / 100
	return strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", n-filled) + fmt.Sprintf(" %d%%", percent)
}

// Rule is a horizontal line width cells long, in the accent when active.
func Rule(width int, active bool) Span {
	return Border(strings.Repeat("\u2500", max(width, 0)), active)
}

// Match is text a query matched.
func Match(s string) Span { return sp(elMatch, s) }

// SearchPrompt prompts for a query, as a marker.
func (t *Theme) SearchPrompt() string { return t.Glyph(GlyphSearch) + "  " }

// Free text, styled by what it is for.

// Default is text in the terminal's own foreground.
func Default(s string) Span { return sp(elDefault, s) }

// Text is text in the theme's foreground.
func Text(s string) Span { return sp(elText, s) }

// Muted is secondary text, such as notes and counts.
func Muted(s string) Span { return sp(elMuted, s) }

// Title heads a list.
func Title(s string) Span { return sp(elTitle, s) }

// Heading heads a screen.
func Heading(s string) Span { return sp(elHeading, s) }

// Label names a form's field.
func Label(s string) Span { return sp(elLabel, s) }

// Busy is something under way.
func Busy(s string) Span { return sp(elBusy, s) }

// Key is a key, as a hint shows it.
func Key(k string) Span { return sp(elKey, k) }

// KeyDesc is what a key does.
func KeyDesc(s string) Span { return sp(elKeyDesc, s) }

// Error is an error.
func Error(s string) Span { return sp(elError, s) }

// Border is a divider, in the accent when active.
func Border(s string, active bool) Span {
	if active {
		return sp(elBorderActive, s)
	}
	return sp(elBorder, s)
}
