package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Moving a space into another group rewrites its entry with the group and moves it, with its comments, to
// the file that lists the group's spaces, then puts the spaces in the order given: a path becomes a table,
// a table's group changes, and a table left with its path alone is its path again. A group a [space.NAME]
// table sets is refused.
func TestMoveSpace(t *testing.T) {
	dir := configDir(t, map[string]string{
		"kspaces.toml":      "spaces = [\n  \"~/a\",\n  # the reviewer\n  { path = \"~/gr\", group = \"tools\" }, # mine\n  \"~/b\",\n]\n",
		"conf.d/local.toml": "spaces = [\n  { path = \"~/w1\", group = \"work\" },\n]\n",
	})
	read := func() []string {
		var out []string
		for _, name := range []string{"kspaces.toml", "conf.d/local.toml"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, string(data))
		}
		return out
	}
	type step struct {
		root, group string
		order       []string
		want        []string
	}
	for _, s := range []step{
		// a into work, before w1: to local.toml as a table.
		{"/home/me/a", "work", []string{"/home/me/b", "/home/me/gr", "/home/me/a", "/home/me/w1"}, []string{
			"spaces = [\n  \"~/b\",\n  # the reviewer\n  { path = \"~/gr\", group = \"tools\" }, # mine\n]\n",
			"spaces = [\n  { path = \"~/a\", group = \"work\" },\n  { path = \"~/w1\", group = \"work\" },\n]\n",
		}},
		// gr out of tools, staying in kspaces.toml with its comments, a path again.
		{"/home/me/gr", "", []string{"/home/me/gr", "/home/me/b", "/home/me/a", "/home/me/w1"}, []string{
			"spaces = [\n  # the reviewer\n  \"~/gr\", # mine\n  \"~/b\",\n]\n",
			"spaces = [\n  { path = \"~/a\", group = \"work\" },\n  { path = \"~/w1\", group = \"work\" },\n]\n",
		}},
		// w1 into tools, which no file lists a space of now: to kspaces.toml.
		{"/home/me/w1", "tools", []string{"/home/me/gr", "/home/me/b", "/home/me/a", "/home/me/w1"}, []string{
			"spaces = [\n  # the reviewer\n  \"~/gr\", # mine\n  \"~/b\",\n  { path = \"~/w1\", group = \"tools\" },\n]\n",
			"spaces = [\n  { path = \"~/a\", group = \"work\" },\n]\n",
		}},
	} {
		if err := MoveSpace(s.root, s.group, s.order); err != nil {
			t.Fatalf("%s into %q: %v", s.root, s.group, err)
		}
		if got := read(); !reflect.DeepEqual(got, s.want) {
			t.Errorf("%s into %q:\n%s\nwant\n%s", s.root, s.group, strings.Join(got, "---\n"), strings.Join(s.want, "---\n"))
		}
	}

	configDir(t, map[string]string{"kspaces.toml": "spaces = [\"~/a\", \"~/b\"]\n\n[space.a]\ngroup = \"work\"\n"})
	if err := MoveSpace("/home/me/a", "", []string{"/home/me/a", "/home/me/b"}); err == nil || err.Error() != "a's group is set in [space.a] in kspaces.toml; change it there" {
		t.Errorf("a group a table sets: %v", err)
	}
}

// An entry gets its group as a table with the path, or a table's group changed or added; out of every
// group a table loses it, and is its path when that is all it has left.
func TestWithGroup(t *testing.T) {
	var got []string
	for _, c := range []struct{ v, group string }{
		{`"~/a"`, "work"},
		{`"~/a"`, ""},
		{`{ path = "~/a", group = "tools" }`, "work"},
		{`{ path = "~/a", layout = "x" }`, "work"},
		{`{ path = "~/a", group = "tools" }`, ""},
		{`{ group = "tools", path = "~/a", layout = "x" }`, ""},
		{`{ path = "~/a", layout = "x", group = "tools" }`, ""},
	} {
		v, err := withGroup(c.v, c.group)
		if err != nil {
			v = err.Error()
		}
		got = append(got, v)
	}
	want := []string{
		`{ path = "~/a", group = "work" }`,
		`"~/a"`,
		`{ path = "~/a", group = "work" }`,
		`{ path = "~/a", layout = "x", group = "work" }`,
		`"~/a"`,
		`{ path = "~/a", layout = "x" }`,
		`{ path = "~/a", layout = "x" }`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// An item with a line of its own goes with it and the comments above it; one sharing its line goes with
// its comma.
func TestRemoveItem(t *testing.T) {
	var got []string
	for _, c := range []struct {
		text string
		k    int
	}{
		{"spaces = [\n  # a's\n  \"a\", # mine\n  \"b\",\n]\n", 0},
		{"spaces = [\n  \"a\",\n  \"b\"\n]\n", 1},
		{"spaces = [\"a\", \"b\", \"c\"]\n", 1},
		{"spaces = [\"a\", \"b\"]\n", 1},
		{"spaces = [\"a\"]\n", 0},
	} {
		out, err := removeItem(c.text, c.k)
		if err != nil {
			out = err.Error()
		}
		got = append(got, out)
	}
	want := []string{
		"spaces = [\n  \"b\",\n]\n",
		"spaces = [\n  \"a\",\n]\n",
		"spaces = [\"a\", \"c\"]\n",
		"spaces = [\"a\"]\n",
		"spaces = []\n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
