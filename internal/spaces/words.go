package spaces

import (
	"fmt"
	"strings"
)

// Command lines as a shell reads them, for a pane's command, which kitty and cm report as one line and a
// layout runs as words.

// SplitWords is line's words as a POSIX shell splits them, its quotes and backslashes taken out. A quote
// that does not end is an error.
func SplitWords(line string) ([]string, error) {
	var words []string
	var w strings.Builder
	in := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case ' ', '\t', '\n':
			if in {
				words, in = append(words, w.String()), false
				w.Reset()
			}
		case '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("%s has a ' that does not end", line)
			}
			w.WriteString(line[i+1 : i+1+end])
			i, in = i+1+end, true
		case '"':
			i++
			for ; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && strings.IndexByte("$`\"\\\n", line[i+1]) >= 0 {
					i++
				}
				w.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, fmt.Errorf("%s has a \" that does not end", line)
			}
			in = true
		case '\\':
			if i+1 < len(line) {
				i++
				w.WriteByte(line[i])
			}
			in = true
		default:
			w.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, w.String())
	}
	return words, nil
}

// JoinWords is words as one line a shell splits back into them, each quoted only when it has to be.
func JoinWords(words []string) string {
	out := make([]string, len(words))
	for i, w := range words {
		if w != "" && !strings.ContainsAny(w, " \t\n'\"\\$`|&;<>()*?[]#~=%{}!") {
			out[i] = w
			continue
		}
		out[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}
