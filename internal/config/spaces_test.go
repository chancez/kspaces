package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// An entry of spaces is a path, or an inline table of a path and what [space.NAME] says, which is folded
// into Space by the space's name, with any [space.NAME] table's other keys; Spaces is the paths in order.
func TestParseSpaceEntries(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(`spaces = [
  { path = "~/projects/work/hubble-timescape", group = "work" }, # at work
  { path = "~/projects/gr", group = "tools", worktree_layout = "plain" },
  "~/.dotfiles",
  { path = "/src/kitty" },
]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]

[space.gr]
layout = "plain"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := mustParse(t, `spaces = ["~/projects/work/hubble-timescape", "~/projects/gr", "~/.dotfiles", "/src/kitty"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]

[space.hubble-timescape]
group = "work"

[space.gr]
group = "tools"
layout = "plain"
worktree_layout = "plain"
`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	if paths := []string{filepath.Join(home, "projects/work/hubble-timescape"), filepath.Join(home, "projects/gr"), filepath.Join(home, ".dotfiles"), "/src/kitty"}; !reflect.DeepEqual(got.Spaces, paths) {
		t.Fatalf("spaces %q, want %q", got.Spaces, paths)
	}
}

// An inline entry has a path and only what [space.NAME] takes, and says nothing [space.NAME] or another
// entry of the same name says too.
func TestParseBadSpaceEntries(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`spaces = [{ group = "work" }]`, `toml: line 1 (last key "spaces"): spaces: item 1: it has no path`},
		{`spaces = ["/src/gr", { path = "" }]`, `toml: line 1 (last key "spaces"): spaces: item 2: path is empty`},
		{`spaces = [{ path = 1 }]`, `toml: line 1 (last key "spaces"): spaces: item 1: path must be a string`},
		{`spaces = [{ path = "/src/gr", grup = "work", nope = 1 }]`, `toml: line 1 (last key "spaces"): spaces: item 1: /src/gr: unknown keys: grup, nope`},
		{`spaces = [["/src/gr"]]`, `toml: line 1 (last key "spaces"): spaces: item 1 is neither a path nor a { path = ... } table`},
		{`spaces = [{ path = "/src/gr", group = 1 }]`, `toml: line 1 (last key "spaces"): spaces: item 1: /src/gr: group must be a string`},
		{`spaces = "/src/gr"`, `toml: line 1 (last key "spaces"): spaces must be a list of paths, or of { path = ... } tables`},
		{"spaces = [{ path = \"/src/gr\", group = \"work\" }]\n[space.gr]\ngroup = \"tools\"", "gr's group is set both in its spaces entry and in [space.gr]; set it in one"},
		{`spaces = [{ path = "/src/gr", group = "work" }, { path = "/x/gr", group = "tools" }]`, "gr's group is set by two entries of spaces"},
		{`spaces = [{ path = "/src/gr", layout = "nope" }]`, `space.gr.layout is "nope", no layout has that name`},
	} {
		_, err := Parse(c.in)
		if got := errString(err); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.in, got, c.want)
		}
	}
}
