package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// appendSpace adds the entry in the array's own style, and touches nothing else.
func TestAppendSpace(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{
			name: "multi-line with a trailing comma",
			in:   "spaces = [\n    \"~/a\",\n    \"~/b\",\n]\nnew_tab = [\"launch\"]\n",
			want: "spaces = [\n    \"~/a\",\n    \"~/b\",\n    \"NEW\",\n]\nnew_tab = [\"launch\"]\n",
		},
		{
			name: "multi-line without a trailing comma",
			in:   "spaces = [\n  \"~/a\",\n  \"~/b\"\n]\n",
			want: "spaces = [\n  \"~/a\",\n  \"~/b\",\n  \"NEW\"\n]\n",
		},
		{
			name: "comments inside and after",
			in:   "# Repositories.\nspaces = [ # in order\n  # the first\n  \"~/a\", # mine\n  # \"~/old\",\n] # done\n",
			want: "# Repositories.\nspaces = [ # in order\n  # the first\n  \"~/a\", # mine\n  \"NEW\",\n  # \"~/old\",\n] # done\n",
		},
		{
			name: "a comment after the last item without a comma",
			in:   "spaces = [\n\t\"~/a\" # mine\n]\n",
			want: "spaces = [\n\t\"~/a\", # mine\n\t\"NEW\"\n]\n",
		},
		{
			name: "the bracket on the last item's line",
			in:   "spaces = [\"~/a\",\n          \"~/b\"]\n",
			want: "spaces = [\"~/a\",\n          \"~/b\",\n          \"NEW\"]\n",
		},
		{
			name: "the bracket after the last item's comma",
			in:   "spaces = [\n  \"~/a\",]\n",
			want: "spaces = [\n  \"~/a\",\n  \"NEW\",]\n",
		},
		{
			name: "the last item on the line of the bracket that opens",
			in:   "spaces = [\"~/a\", # mine\n]\n",
			want: "spaces = [\"~/a\", # mine\n          \"NEW\",\n]\n",
		},
		{
			name: "single-line",
			in:   "spaces = [\"~/a\", '/src/b']  # mine\nsessions = \"~/s\"\n",
			want: "spaces = [\"~/a\", '/src/b', \"NEW\"]  # mine\nsessions = \"~/s\"\n",
		},
		{
			name: "single-line with a trailing comma",
			in:   "spaces = [\"~/a\",]\n",
			want: "spaces = [\"~/a\", \"NEW\",]\n",
		},
		{
			name: "empty",
			in:   "spaces = [ ]\n[sidebar]\nwidth = 30\n",
			want: "spaces = [\"NEW\"]\n[sidebar]\nwidth = 30\n",
		},
		{
			name: "empty over lines, with a comment",
			in:   "spaces = [\n    # none yet\n]\n",
			want: "spaces = [\n    # none yet\n    \"NEW\",\n]\n",
		},
		{
			name: "missing, before the first table and its comments",
			in:   "# kspaces\nsessions = \"~/s\"\n\n# Where worktrees go.\n[worktrees]\ndir = \"wt\"\n",
			want: "# kspaces\nsessions = \"~/s\"\n\nspaces = [\"NEW\"]\n\n# Where worktrees go.\n[worktrees]\ndir = \"wt\"\n",
		},
		{
			name: "missing, with no table",
			in:   "sessions = \"~/s\"",
			want: "sessions = \"~/s\"\nspaces = [\"NEW\"]\n",
		},
		{
			name: "an empty file",
			in:   "",
			want: "spaces = [\"NEW\"]\n",
		},
		{
			name: "spaces inside a table is not the top-level key",
			in:   "new_tab = [\"launch\"]\n[other]\nspaces = [\"~/a\"]\n",
			want: "new_tab = [\"launch\"]\nspaces = [\"NEW\"]\n\n[other]\nspaces = [\"~/a\"]\n",
		},
		{
			name: "strings and comments with brackets and hashes",
			in:   "new_tab = [\"spaces = [\", '#]', \"\\\"]\"] # spaces = [\nspaces = [\"~/a]#b\", '~/c]', \"~/d\\\"]#\"] # ] \"\n",
			want: "new_tab = [\"spaces = [\", '#]', \"\\\"]\"] # spaces = [\nspaces = [\"~/a]#b\", '~/c]', \"~/d\\\"]#\", \"NEW\"] # ] \"\n",
		},
		{
			name: "multi-line strings and a quoted key",
			in:   "sessions = \"\"\"\nspaces = [\n]\"\"\"\n\"spaces\" = [\"~/a\"]\n",
			want: "sessions = \"\"\"\nspaces = [\n]\"\"\"\n\"spaces\" = [\"~/a\", \"NEW\"]\n",
		},
		{
			name: "a nested array before spaces",
			in:   "x = [[1, 2], [\n  3]]\nspaces = []\n",
			want: "x = [[1, 2], [\n  3]]\nspaces = [\"NEW\"]\n",
		},
	} {
		got, err := appendSpace(c.in, `"NEW"`)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\n%s\nwant\n%s", c.name, got, c.want)
		}
	}

	for _, bad := range []string{
		`spaces = "~/a"`,
		"spaces = [\"~/a\"",
		"spaces = [\"~/a]\n",
		"sessions \"~/s\"",
	} {
		if got, err := appendSpace(bad, `"NEW"`); err == nil {
			t.Errorf("%q was edited, to %q", bad, got)
		}
	}
}

// AddSpace writes a root under the home directory with ~/, checks the edit parses to the spaces it had
// and then the new one, and refuses a root already listed or a config that does not parse.
func TestAddSpace(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	in := "# Mine.\nspaces = [\n  \"~/a\", # first\n]\n\n[sidebar]\nwidth = 30\n"
	got, err := AddSpace(in, "/home/me/src/b")
	if want := "# Mine.\nspaces = [\n  \"~/a\", # first\n  \"~/src/b\",\n]\n\n[sidebar]\nwidth = 30\n"; err != nil || got != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}
	got, err = AddSpace("", "/src/with \"quote\"")
	if want := "spaces = [\"/src/with \\\"quote\\\"\"]\n"; err != nil || got != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}

	for _, c := range []struct{ in, root string }{
		{in, "/home/me/a"},
		{"spaces = [\"~/a\"]\nspace = 1\n", "/src/b"},
	} {
		if got, err := AddSpace(c.in, c.root); err == nil {
			t.Errorf("adding %s to %q gave %q", c.root, c.in, got)
		}
	}
}

// SaveSpace writes through a symlinked config, keeping the link and the file's mode, and creates a
// missing one.
func TestSaveSpace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", "/home/me")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	dotfiles := filepath.Join(dir, "dotfiles")
	if err := os.MkdirAll(dotfiles, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dotfiles, "kspaces.toml")
	if err := os.WriteFile(target, []byte("spaces = [\"~/a\"] # mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "kspaces", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveSpace("/src/b"); err != nil {
		t.Fatal(err)
	}
	type file struct {
		Link string
		Mode os.FileMode
		Text string
		// What else the directories hold, such as a temporary file left behind.
		Dotfiles, Config []string
	}
	names := func(dir string) []string {
		entries, _ := os.ReadDir(dir)
		var out []string
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return out
	}
	read := func() file {
		dest, _ := os.Readlink(link)
		info, _ := os.Stat(target)
		data, _ := os.ReadFile(target)
		return file{dest, info.Mode().Perm(), string(data), names(dotfiles), names(filepath.Dir(link))}
	}
	want := file{target, 0o600, "spaces = [\"~/a\", \"/src/b\"] # mine\n", []string{"kspaces.toml"}, []string{"config.toml"}}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if err := SaveSpace("/src/b"); err == nil || !strings.Contains(err.Error(), "already a space") {
		t.Errorf("adding it again: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "fresh"))
	if err := SaveSpace("/home/me/c"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "fresh", "kspaces", "config.toml")); err != nil || string(data) != "spaces = [\"~/c\"]\n" {
		t.Errorf("a new config: %q, %v", data, err)
	}
}
