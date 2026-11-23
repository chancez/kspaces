package config

import (
	"testing"
)

// Removing a space takes out its entry with its own comments, and its [space.NAME] tables, and keeps the
// rest of the file as it was.
func TestRemoveSpaceText(t *testing.T) {
	text := `# The spaces.
spaces = [
  # gr, for review
  { path = "/src/gr", group = "work" }, # trailing
  "/src/kitty",
  "/src/docs",
]

layout = "plain"

# gr's own.
[space.gr]
worktree_layout = "agent"

# kitty's.
[space.kitty]
layout = "plain"
`
	for _, c := range []struct {
		root, want string
	}{
		{"/src/gr", `# The spaces.
spaces = [
  "/src/kitty",
  "/src/docs",
]

layout = "plain"

# kitty's.
[space.kitty]
layout = "plain"
`},
		{"/src/kitty", `# The spaces.
spaces = [
  # gr, for review
  { path = "/src/gr", group = "work" }, # trailing
  "/src/docs",
]

layout = "plain"

# gr's own.
[space.gr]
worktree_layout = "agent"
`},
	} {
		got, err := RemoveSpaceText(text, c.root)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v\n%s\nwant\n%s", c.root, err, got, c.want)
		}
	}
	inline := `spaces = ["/src/gr", "/src/kitty", "/src/docs"]` + "\n"
	for root, want := range map[string]string{
		"/src/gr":    `spaces = ["/src/kitty", "/src/docs"]` + "\n",
		"/src/docs":  `spaces = ["/src/gr", "/src/kitty"]` + "\n",
		"/src/kitty": `spaces = ["/src/gr", "/src/docs"]` + "\n",
	} {
		if got, err := RemoveSpaceText(inline, root); err != nil || got != want {
			t.Errorf("inline %s: got %v %q, want %q", root, err, got, want)
		}
	}
}
