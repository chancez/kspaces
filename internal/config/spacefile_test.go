package config

import (
	"reflect"
	"testing"
)

// spaceGlobal is a global config with a command and a layout for a space file to add to and override, and
// defaults for it to take the place of.
const spaceGlobal = `spaces = ["/src/gr", "/src/kitty"]
layout = "plain"

[worktrees]
dir = ".wt"
branch_prefix = "pr/me"

[[commands]]
name = "review"
run = ["gr"]

[[commands]]
name = "claude"
run = ["claude"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]

[[layouts]]
name = "agent"
tabs = [{ panes = [{ command = "claude" }, { command = "review", split = "right" }] }]

[space.gr]
layout = "agent"
`

func mustParse(t *testing.T, text string) Config {
	t.Helper()
	cfg, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func withSpace(t *testing.T, global, file string) (Config, error) {
	t.Helper()
	f, err := ParseSpaceFile(file)
	if err != nil {
		return Config{}, err
	}
	return mustParse(t, global).WithSpace("gr", f)
}

// A space file's commands and layouts are added to the global ones, one of the same name in its place, and
// a global layout running an overridden command takes its inputs; its worktree settings take the place of
// the global ones it sets. The merged config is the global config as if it said all that itself.
func TestWithSpaceMerges(t *testing.T) {
	global := mustParse(t, spaceGlobal)
	f, err := ParseSpaceFile(`
[worktrees]
branch_prefix = ""

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt" }]

[[commands]]
name = "test"
run = ["make", "test"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{ command = "test" }] }]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "review" }, { command = "test", split = "below" }] }]
`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := global.WithSpace("gr", f)
	if err != nil {
		t.Fatal(err)
	}
	want := mustParse(t, `spaces = ["/src/gr", "/src/kitty"]
layout = "plain"

[worktrees]
dir = ".wt"
branch_prefix = ""

[[commands]]
name = "review"
run = ["gr"]

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt" }]

[[commands]]
name = "test"
run = ["make", "test"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{ command = "test" }] }]

[[layouts]]
name = "agent"
tabs = [{ panes = [{ command = "claude" }, { command = "review", split = "right" }] }]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "review" }, { command = "test", split = "below" }] }]

[space.gr]
layout = "agent"
`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged:\n%#v\nwant\n%#v", got, want)
	}
	// The global config is as it was.
	if !reflect.DeepEqual(global, mustParse(t, spaceGlobal)) {
		t.Errorf("the global config changed: %#v", global)
	}
}

// The space file's defaults come first, then its space table's, then the global layout; its linked
// worktrees' default falls back on its main one before the table's.
func TestWithSpaceDefaults(t *testing.T) {
	table := spaceGlobal + "worktree_layout = \"plain\"\n"
	type result struct{ Main, Linked string }
	var got []result
	for _, c := range []struct{ global, file string }{
		{table, ""},
		{table, `layout = "mine"`},
		{table, `worktree_layout = "mine"`},
		{table, "layout = \"plain\"\nworktree_layout = \"mine\""},
		{`spaces = ["/src/gr"]` + "\nlayout = \"g\"\n[[layouts]]\nname = \"g\"\ntabs = [{ panes = [{}] }]\n", ""},
	} {
		cfg, err := withSpace(t, c.global, c.file+"\n[[layouts]]\nname = \"mine\"\ntabs = [{ panes = [{}] }]\n")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, result{cfg.LayoutFor("gr", false), cfg.LayoutFor("gr", true)})
	}
	want := []result{
		{"agent", "plain"},
		{"mine", "mine"},
		{"agent", "mine"},
		{"plain", "mine"},
		{"g", "g"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// A space file is checked as the global config is, with the global config's commands and layouts to
// refer to, and has none of what only the global config says.
func TestSpaceFileErrors(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"nope = 1\n[worktrees]\ndirr = \"x\"", "unknown keys: nope, worktrees.dirr"},
		{`spaces = ["/x"]` + "\n[sidebar]\nwidth = 30", "only the global config can have spaces, sidebar"},
		{"[space.gr]\nlayout = \"agent\"", "space tables are for the global config; a space's file sets its own layout and worktree_layout at the top"},
		{"layout = \"nope\"", `layout is "nope", no layout has that name`},
		{"worktree_layout = \"nope\"", `worktree_layout is "nope", no layout has that name`},
		{"[[layouts]]\nname = \"a\"\ntabs = [{ panes = [{ command = \"nope\" }] }]", `layouts: a: tab 1 pane 1: command is "nope", no command has that name`},
		{"[[layouts]]\nname = \"a\"\ntabs = [{ panes = [{}] }]\n[[layouts]]\nname = \"a\"\ntabs = [{ panes = [{}] }]", "layouts: a is defined twice"},
		{"[[commands]]\nname = \"x\"\nrun = []", "commands: x: run cannot be empty"},
		// An override that breaks a global layout, which now asks for prompt twice over.
		{"[[commands]]\nname = \"review\"\nrun = [\"gr\", \"{prompt}\"]\ninputs = [{ name = \"prompt\", required = true }]\n" +
			"[[commands]]\nname = \"claude\"\nrun = [\"claude\", \"{prompt}\"]\ninputs = [{ name = \"prompt\" }]", "layouts: agent: input prompt is defined differently by claude and review"},
		{"layout = ", `toml: line 1 (last key "layout"): unexpected EOF; expected value`},
	} {
		if _, err := withSpace(t, spaceGlobal, c.in); err == nil || err.Error() != c.want {
			t.Errorf("%q: got %v\nwant %s", c.in, err, c.want)
		}
	}
}
