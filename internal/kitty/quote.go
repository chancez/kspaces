package kitty

import "strings"

// Quote is s as one word of a kitty action's arguments, for a word to arrive as it is, such as a prompt.
// kitty splits an action's arguments as a shell would, then expands $VAR and a leading ~ in each word: $
// is doubled, kitty's escape for it, and anything a shell would split on or unquote goes in single quotes.
// A leading ~ has no escape and is expanded.
func Quote(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	if s != "" && !strings.ContainsAny(s, " \t\r\n'\"\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
