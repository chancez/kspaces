package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// Adding a space edits config.toml as text rather than encoding the config again, so its comments and
// layout survive: the file is often hand-written, and kept in dotfiles.

// AddSpace is the config text with root, an absolute and clean path, added to the end of spaces, written
// with ~/ for the home directory. The result is parsed back and must list the spaces it listed and then
// root, so a mistake in the edit is an error rather than a broken config.
func AddSpace(text, root string) (string, error) {
	before, err := Parse(text)
	if err != nil {
		return "", fmt.Errorf("the config has an error to fix first: %w", err)
	}
	if slices.Contains(before.Spaces, root) {
		return "", fmt.Errorf("%s is already a space", Tilde(root))
	}
	out, err := appendSpace(text, tomlString(Tilde(root)))
	if err != nil {
		return "", err
	}
	after, err := Parse(out)
	if err != nil {
		return "", fmt.Errorf("adding %s would break the config, so it was left as is: %w", Tilde(root), err)
	}
	if want := append(slices.Clone(before.Spaces), root); !slices.Equal(after.Spaces, want) {
		return "", fmt.Errorf("adding %s would list spaces %q, so the config was left as is", Tilde(root), after.Spaces)
	}
	return out, nil
}

// SaveSpace adds root to the spaces of the config file Load reads, see AddSpace. A symlinked config, as
// from dotfiles, has its target written, so it stays a symlink. The write is a rename from a temporary
// file beside it, so a sidebar refreshing meanwhile reads the old config or the new one, never half.
// A missing config is created.
func SaveSpace(root string) error {
	path, err := filePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return fmt.Errorf("reading the config: %w", err)
	}
	out, err := AddSpace(string(data), root)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	target, mode := path, fs.FileMode(0o644)
	if missing {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	} else {
		if target, err = filepath.EvalSymlinks(path); err != nil {
			return err
		}
		if info, err := os.Stat(target); err == nil {
			mode = info.Mode().Perm()
		}
	}
	return writeAtomic(target, []byte(out), mode)
}

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\u%04X", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// appendSpace is text with entry, a TOML value, added to the end of the top-level spaces array, in the
// array's own style: on a line of its own, indented as the item before it, in a multi-line array, and
// after a comma in a single-line one. Without a top-level spaces key, one is added before the first table,
// and the comments right above it, or at the end.
func appendSpace(text, entry string) (string, error) {
	sc := scanner{s: text}
	for {
		i := sc.skipBlank(sc.i)
		if i >= len(text) {
			sep := ""
			if text != "" && !strings.HasSuffix(text, "\n") {
				sep = "\n"
			}
			return text + sep + "spaces = [" + entry + "]\n", nil
		}
		if text[i] == '[' {
			at := commentsAbove(text, lineStart(text, i))
			return text[:at] + "spaces = [" + entry + "]\n\n" + text[at:], nil
		}
		key, j, err := sc.key(i)
		if err != nil {
			return "", err
		}
		j = sc.skipSpace(j)
		if j >= len(text) || text[j] != '=' {
			return "", fmt.Errorf("line %d: expected = after the key", sc.line(j))
		}
		j = sc.skipSpace(j + 1)
		if len(key) == 1 && key[0] == "spaces" {
			if j >= len(text) || text[j] != '[' {
				return "", fmt.Errorf("line %d: spaces is not a list", sc.line(j))
			}
			return sc.appendItem(j, entry)
		}
		if sc.i, err = sc.value(j); err != nil {
			return "", err
		}
	}
}

// scanner reads just enough TOML to find the top-level keys and step over their values: strings, arrays
// and inline tables, with the comments inside them.
type scanner struct {
	s string
	// Where the next top-level statement may start.
	i int
}

func (sc *scanner) line(i int) int { return strings.Count(sc.s[:min(i, len(sc.s))], "\n") + 1 }

// skipSpace skips spaces and tabs.
func (sc *scanner) skipSpace(i int) int {
	for i < len(sc.s) && (sc.s[i] == ' ' || sc.s[i] == '\t') {
		i++
	}
	return i
}

// skipBlank skips whitespace, newlines and comments.
func (sc *scanner) skipBlank(i int) int {
	for i < len(sc.s) {
		switch sc.s[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			i = sc.skipComment(i)
		default:
			return i
		}
	}
	return i
}

// skipComment skips to the end of the line, leaving the newline.
func (sc *scanner) skipComment(i int) int {
	if n := strings.IndexByte(sc.s[i:], '\n'); n >= 0 {
		return i + n
	}
	return len(sc.s)
}

// key reads a key, dotted or not, from i, and returns its parts unquoted.
func (sc *scanner) key(i int) ([]string, int, error) {
	var parts []string
	for {
		i = sc.skipSpace(i)
		switch {
		case i < len(sc.s) && (sc.s[i] == '"' || sc.s[i] == '\''):
			end, err := sc.str(i)
			if err != nil {
				return nil, 0, err
			}
			parts = append(parts, sc.s[i+1:end-1])
			i = end
		default:
			start := i
			for i < len(sc.s) && isBareKey(sc.s[i]) {
				i++
			}
			if i == start {
				return nil, 0, fmt.Errorf("line %d: expected a key", sc.line(i))
			}
			parts = append(parts, sc.s[start:i])
		}
		i = sc.skipSpace(i)
		if i >= len(sc.s) || sc.s[i] != '.' {
			return parts, i, nil
		}
		i++
	}
}

func isBareKey(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// str is the end of the string that starts at i, after its closing quote: basic or literal, on one line
// or many.
func (sc *scanner) str(i int) (int, error) {
	q := sc.s[i]
	if strings.HasPrefix(sc.s[i:], strings.Repeat(string(q), 3)) {
		for j := i + 3; j < len(sc.s); j++ {
			if q == '"' && sc.s[j] == '\\' {
				j++
				continue
			}
			if strings.HasPrefix(sc.s[j:], strings.Repeat(string(q), 3)) {
				// Up to two more quotes can end the string's content.
				end := j + 3
				for n := 0; n < 2 && end < len(sc.s) && sc.s[end] == q; n++ {
					end++
				}
				return end, nil
			}
		}
		return 0, fmt.Errorf("line %d: a string that does not end", sc.line(i))
	}
	for j := i + 1; j < len(sc.s) && sc.s[j] != '\n'; j++ {
		switch {
		case q == '"' && sc.s[j] == '\\':
			j++
		case sc.s[j] == q:
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("line %d: a string that does not end", sc.line(i))
}

// value is the end of the value that starts at i.
func (sc *scanner) value(i int) (int, error) {
	if i >= len(sc.s) {
		return i, nil
	}
	switch sc.s[i] {
	case '"', '\'':
		return sc.str(i)
	case '[', '{':
		return sc.bracketed(i)
	}
	// A number, a boolean or a date, which can have a space in it: up to a comment or the line's end.
	for i < len(sc.s) && sc.s[i] != '\n' && sc.s[i] != '#' {
		i++
	}
	return i, nil
}

// bracketed is the end of the array or inline table that starts at i, after its closing bracket.
func (sc *scanner) bracketed(i int) (int, error) {
	depth := 0
	for j := i; j < len(sc.s); {
		switch sc.s[j] {
		case '"', '\'':
			end, err := sc.str(j)
			if err != nil {
				return 0, err
			}
			j = end
			continue
		case '#':
			j = sc.skipComment(j)
			continue
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return j + 1, nil
			}
		}
		j++
	}
	return 0, fmt.Errorf("line %d: a list that does not end", sc.line(i))
}

// appendItem adds entry to the end of the array that opens at open.
func (sc *scanner) appendItem(open int, entry string) (string, error) {
	s := sc.s
	// The last item's start and end, and the comma after it, if any.
	last, lastEnd, comma := -1, -1, -1
	closing := -1
	for j := open + 1; j < len(s) && closing < 0; {
		switch c := s[j]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			j++
		case c == '#':
			j = sc.skipComment(j)
		case c == ',':
			if last >= 0 && comma < 0 {
				comma = j
			}
			j++
		case c == ']':
			closing = j
		case c == '"' || c == '\'' || c == '[' || c == '{':
			end, err := sc.value(j)
			if err != nil {
				return "", err
			}
			last, lastEnd, comma, j = j, end, -1, end
		default:
			// A bare value, such as a number: up to what ends an item.
			end := j + strings.IndexAny(s[j:]+"\n", ",]#\n")
			last, lastEnd, comma, j = j, j+len(strings.TrimRight(s[j:end], " \t\r")), -1, end
		}
	}
	if closing < 0 {
		return "", fmt.Errorf("line %d: spaces does not end", sc.line(open))
	}
	multi := strings.Contains(s[open:closing], "\n")
	switch {
	case last < 0 && !multi:
		return s[:open] + "[" + entry + "]" + s[closing+1:], nil
	case last < 0:
		// Each on a line of its own, indented as the comments inside are, before the bracket's line.
		indent, at := "  ", lineStart(s, closing)
		if strings.TrimSpace(s[at:closing]) != "" {
			return s[:closing] + "\n" + indent + entry + ",\n" + s[closing:], nil
		}
		for _, l := range strings.Split(s[open:at], "\n")[1:] {
			if t := strings.TrimLeft(l, " \t"); strings.HasPrefix(t, "#") {
				indent = l[:len(l)-len(t)]
				break
			}
		}
		return s[:at] + indent + entry + ",\n" + s[at:], nil
	case !multi:
		return s[:lastEnd] + ", " + entry + s[lastEnd:], nil
	}
	ls := lineStart(s, last)
	indent := s[ls:last]
	switch {
	case strings.TrimSpace(indent) == "":
	case ls <= open:
		// The item is on the line of spaces = [, so line up under it.
		indent = strings.Repeat(" ", utf8.RuneCountInString(indent))
	default:
		indent = indent[:len(indent)-len(strings.TrimLeft(indent, " \t"))]
	}
	after := lastEnd
	if comma >= 0 {
		after = comma + 1
	}
	nl := strings.IndexByte(s[after:closing], '\n')
	switch {
	case nl < 0 && comma >= 0:
		// `"b",]`: a new line before the bracket, with a comma as the others have.
		return s[:after] + "\n" + indent + entry + "," + s[after:], nil
	case nl < 0:
		return s[:lastEnd] + ",\n" + indent + entry + s[lastEnd:], nil
	case comma >= 0:
		// After the comments on the last item's line, which belong to it.
		return s[:after+nl] + "\n" + indent + entry + "," + s[after+nl:], nil
	default:
		at := after + nl
		return s[:lastEnd] + "," + s[lastEnd:at] + "\n" + indent + entry + s[at:], nil
	}
}

// lineStart is where the line holding i starts.
func lineStart(s string, i int) int {
	return strings.LastIndexByte(s[:i], '\n') + 1
}

// commentsAbove is where the comment lines directly above the line at i start, so a table keeps its
// comments.
func commentsAbove(s string, i int) int {
	for i > 0 {
		prev := lineStart(s, i-1)
		if !strings.HasPrefix(strings.TrimLeft(s[prev:i-1], " \t"), "#") {
			break
		}
		i = prev
	}
	return i
}
