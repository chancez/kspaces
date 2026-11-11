package kitty

import (
	"reflect"
	"testing"
)

// Each word comes back from kitty's splitting and expanding as it was: plain words as they are, the rest
// quoted, with $ doubled.
func TestQuote(t *testing.T) {
	var got []string
	for _, s := range []string{"claude", "--model=opus", "", "fix the bug", "it's", `say "hi"`, `a\b`, "$HOME", "costs $5 now", "a\nb"} {
		got = append(got, Quote(s))
	}
	want := []string{"claude", "--model=opus", "''", "'fix the bug'", `'it'\''s'`, `'say "hi"'`, `'a\b'`, "$$HOME", "'costs $$5 now'", "'a\nb'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
