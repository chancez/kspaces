package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// configDir writes files into a fresh config directory, by name, and points Load at it.
func configDir(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("HOME", "/home/me")
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	dir := filepath.Join(base, "kspaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const mainConfig = `groups = ["work", "tools"]
spaces = [{ path = "~/gr", group = "tools" }]
layout = "review"

[worktrees]
dir = ".wt"

[[commands]]
name = "gr"
run = ["gr"]
`

const workConfig = `spaces = ["~/work/api", { path = "~/work/web", group = "work" }]

[space.api]
group = "work"
worktree_layout = "review"

[worktrees]
branch_prefix = "pr/me"

[[layouts]]
name = "review"
tabs = [{ panes = [{ command = "gr" }] }]
`

// The other .toml files beside config.toml add up with it, by name after it, as one file saying all of
// it would: lists one after another, settings each from the file that sets them, and what one file
// names from another, a layout running another's command, found. Hidden and other files are left out.
func TestLoadFiles(t *testing.T) {
	configDir(t, map[string]string{
		"config.toml":      mainConfig,
		"work.toml":        workConfig,
		"extra.toml":       "groups = [\"extra\"]\n",
		".config.toml.123": "nope = 1\n",
		".hidden.toml":     "nope = 1\n",
		"notes.txt":        "nope",
	})
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse(`groups = ["work", "tools", "extra"]
spaces = [{ path = "~/gr", group = "tools" }, "~/work/api", { path = "~/work/web", group = "work" }]
layout = "review"

[worktrees]
dir = ".wt"
branch_prefix = "pr/me"

[space.api]
group = "work"
worktree_layout = "review"

[[commands]]
name = "gr"
run = ["gr"]

[[layouts]]
name = "review"
tabs = [{ panes = [{ command = "gr" }] }]
`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// Without config.toml the other files are the config; without any file, it is missing.
func TestLoadFilesWithoutMain(t *testing.T) {
	configDir(t, map[string]string{"work.toml": "spaces = [\"~/api\"]\n"})
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Parse("spaces = [\"~/api\"]\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	configDir(t, nil)
	if _, err := Load(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no files: %v", err)
	}
}

// A setting, a key of a space, a command, a layout or a group in two files is an error naming both; an
// error in one file names it, and one of them all the directory.
func TestLoadFilesErrors(t *testing.T) {
	main := "groups = [\"tools\"]\nspaces = [{ path = \"~/gr\", group = \"tools\" }]\n\n[worktrees]\ndir = \".wt\"\n\n[[commands]]\nname = \"gr\"\nrun = [\"gr\"]\n"
	for _, c := range []struct {
		work string
		// Whether the error is work.toml's alone rather than the files'.
		own  bool
		want string
	}{
		{"launch = [\"x\"]\n[worktrees]\ndir = \"w\"\n", false, "worktrees.dir is set both in config.toml and in work.toml; set it in one"},
		{"[space.gr]\ngroup = \"work\"\n", false, "gr's group is set both in config.toml and in work.toml; set it in one"},
		{"spaces = [{ path = \"~/other/gr\", group = \"work\" }]\n", false, "gr's group is set both in config.toml and in work.toml; set it in one"},
		{"[[commands]]\nname = \"gr\"\nrun = [\"x\"]\n", false, "commands: gr is defined both in config.toml and in work.toml; define it in one"},
		{"groups = [\"tools\"]\n", false, "groups: tools is listed both in config.toml and in work.toml; list it in one"},
		{"[[layouts]]\nname = \"review\"\ntabs = [{ panes = [{ command = \"nope\" }] }]\n", false,
			`layouts: review: tab 1 pane 1: command is "nope", no command has that name`},
		{"[space.vim]\ngroup = \"work\"\n", false, "space.vim: no space is named vim"},
		{"nope = 1\n", true, "unknown keys: nope"},
	} {
		dir := configDir(t, map[string]string{"config.toml": main, "work.toml": c.work})
		where := dir
		if c.own {
			where = filepath.Join(dir, "work.toml")
		}
		if _, err := Load(); err == nil || err.Error() != where+": "+c.want {
			t.Errorf("%q: %v", c.work, err)
		}
	}
}

// Moving spaces moves each file's among its own places; a group's spaces in two files keep the files'
// order, so moving one before a space of an earlier file is an error that leaves both as they were.
func TestSaveSpacesOrderFiles(t *testing.T) {
	main := "groups = [\"work\"]\nspaces = [\"~/a\", { path = \"~/b\", group = \"work\" }]\n"
	work := "spaces = [{ path = \"~/c\", group = \"work\" }, \"~/d\", { path = \"~/e\", group = \"work\" }]\n"
	dir := configDir(t, map[string]string{"config.toml": main, "work.toml": work})
	read := func() []string {
		a, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
		b, _ := os.ReadFile(filepath.Join(dir, "work.toml"))
		return []string{string(a), string(b)}
	}
	// The sidebar's order: the spaces in no group, then work's, e moved before c.
	err := SaveSpacesOrder([]string{"/home/me/a", "/home/me/d", "/home/me/b", "/home/me/e", "/home/me/c"})
	want := []string{main, "spaces = [\"~/d\", { path = \"~/e\", group = \"work\" }, { path = \"~/c\", group = \"work\" }]\n"}
	if got := read(); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("within files: %v\n%q\nwant %q", err, got, want)
	}
	err = SaveSpacesOrder([]string{"/home/me/a", "/home/me/d", "/home/me/e", "/home/me/b", "/home/me/c"})
	if err == nil || err.Error() != "e cannot go before b: it is in work.toml, whose spaces come after config.toml's" {
		t.Errorf("across files: %v", err)
	}
	if got := read(); !reflect.DeepEqual(got, want) {
		t.Errorf("across files changed them: %q", got)
	}
}

// A layout is saved in the file that has it, and a new one in config.toml, each checked with the other
// files: config.toml's layout can run work.toml's command.
func TestSaveLayoutFiles(t *testing.T) {
	dir := configDir(t, map[string]string{
		"config.toml": "spaces = [\"~/a\"]\n",
		"work.toml":   "[[commands]]\nname = \"gr\"\nrun = [\"gr\"]\n\n[[layouts]]\nname = \"review\"\ntabs = [{ panes = [{}] }]\n",
	})
	l := Layout{Name: "review", Tabs: []LayoutTab{{Panes: []Pane{{Command: "gr"}}}}}
	if err := SaveLayout(l, true); err != nil {
		t.Fatal(err)
	}
	l.Name = "new"
	if err := SaveLayout(l, false); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	b, _ := os.ReadFile(filepath.Join(dir, "work.toml"))
	got := []string{string(a), string(b)}
	want := []string{
		"spaces = [\"~/a\"]\n\n[[layouts]]\nname = \"new\"\n[[layouts.tabs]]\npanes = [\n  { command = \"gr\" },\n]\n",
		"[[commands]]\nname = \"gr\"\nrun = [\"gr\"]\n\n[[layouts]]\nname = \"review\"\n[[layouts.tabs]]\npanes = [\n  { command = \"gr\" },\n]\n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// A space is added to config.toml, checked with the other files, which can say it is a space already.
func TestSaveSpaceFiles(t *testing.T) {
	dir := configDir(t, map[string]string{
		"config.toml": "layout = \"review\"\nspaces = [\"~/a\"]\n",
		"work.toml":   "spaces = [\"~/b\"]\n\n[[layouts]]\nname = \"review\"\ntabs = [{ panes = [{}] }]\n",
	})
	if err := SaveSpace("/home/me/b"); err == nil || err.Error() != filepath.Join(dir, "config.toml")+": ~/b is already a space" {
		t.Errorf("listed by work.toml: %v", err)
	}
	if err := SaveSpace("/home/me/c"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if want := "layout = \"review\"\nspaces = [\"~/a\", \"~/c\"]\n"; string(got) != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
