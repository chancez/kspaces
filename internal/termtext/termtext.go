// Package termtext makes text from repositories, reviews and other terminals safe to write to a terminal.
//
// File contents, commit messages, names, and comments can hold control characters, and a terminal
// acts on them: an escape sequence can recolor the screen, move the cursor, or set the window
// title. They are shown instead in caret notation, as cat -v does: ESC as ^[, BEL as ^G, DEL as ^?.
// C1 controls and bytes that are not UTF-8 are shown as escapes, such as \u009b and \xff.
package termtext

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Line makes s safe as one line: every control character is shown, newlines and tabs included.
func Line(s string) string {
	return clean(s, false)
}

// Block makes s safe as text of several lines: newlines and tabs are kept, and every other control
// character is shown.
func Block(s string) string {
	return clean(s, true)
}

func clean(s string, block bool) string {
	if safe(s, block) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case !control(r, block):
			b.WriteString(s[i : i+n])
		case r < 0x20:
			b.WriteByte('^')
			b.WriteByte(byte(r) + '@')
		case r == 0x7f:
			b.WriteString("^?")
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
		i += n
	}
	return b.String()
}

// safe reports whether s has nothing to show differently, the common case, so it is returned as is.
func safe(s string, block bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if control(r, block) {
			return false
		}
	}
	return true
}

// control reports whether a terminal could act on r: C0 controls, DEL, and C1 controls. In a block,
// newlines and tabs are text.
func control(r rune, block bool) bool {
	if block && (r == '\n' || r == '\t') {
		return false
	}
	return r < 0x20 || r >= 0x7f && r < 0xa0
}

// tabStop is how far apart tab stops are, as terminals set them by default.
const tabStop = 8

// StyledLines splits s, text a terminal drew with its colors and styles as SGR sequences, such as kitty's
// get-text --ansi, into lines safe to write each on its own. Each starts with the styles earlier lines
// left set, as the text carries them over. SGR sequences are kept; every other escape sequence and control
// character is dropped, so nothing can move the cursor, set a title or open a link. Tabs become spaces
// to the next stop.
func StyledLines(s string) []string {
	var lines []string
	var line, carry strings.Builder
	var state byte
	col := 0
	p := ansi.NewParser()
	for len(s) > 0 {
		seq, width, n, next := ansi.DecodeSequence(s, state, p)
		s, state = s[n:], next
		r, size := utf8.DecodeRuneInString(seq)
		switch {
		case seq == "\n":
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(carry.String())
			col = 0
		case seq == "\t":
			pad := tabStop - col%tabStop
			line.WriteString(strings.Repeat(" ", pad))
			col += pad
		case isSGR(seq):
			line.WriteString(seq)
			// Nothing set before a reset carries over it, and a bare reset leaves nothing to carry.
			params := seq[2 : len(seq)-1]
			if resets(params) {
				carry.Reset()
			}
			if strings.Trim(params, "0") != "" {
				carry.WriteString(seq)
			}
		case r == utf8.RuneError && size <= 1, control(r, false):
			// Escape sequences start with ESC, a control, and are dropped whole.
		default:
			line.WriteString(seq)
			col += width
		}
	}
	return append(lines, line.String())
}

// isSGR reports whether seq is a CSI sequence that sets colors and styles, and nothing else.
func isSGR(seq string) bool {
	params, ok := strings.CutPrefix(seq, "\x1b[")
	if !ok {
		return false
	}
	params, ok = strings.CutSuffix(params, "m")
	return ok && strings.Trim(params, "0123456789;:") == ""
}

// resets reports whether SGR params start by resetting every style.
func resets(params string) bool {
	first, _, _ := strings.Cut(params, ";")
	return strings.Trim(first, "0") == ""
}

// NewWriter returns a writer that shows control characters in what it writes to w, as Block
// does. Each write is cleaned on its own, so a rune split across writes shows as bytes; fmt and
// encoders write whole strings.
func NewWriter(w io.Writer) io.Writer {
	return writer{w}
}

type writer struct {
	w io.Writer
}

func (w writer) Write(p []byte) (int, error) {
	s := string(p)
	if clean := Block(s); clean != s {
		if _, err := io.WriteString(w.w, clean); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return w.w.Write(p)
}
