package config

import (
	"reflect"
	"testing"
)

// layoutCommands are the commands the layouts below use: two asking for a prompt the same way, and one
// with none.
const layoutCommands = `
spaces = ["/src/gr", "/src/kitty"]

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", required = true }]

[[commands]]
name = "codex"
run = ["codex", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", required = true }, { name = "model", default = "o3" }]

[[commands]]
name = "review"
run = ["gr"]
`

// Layouts parse with where each pane splits filled in, and the inputs of their panes' commands merged by
// name; the defaults name them globally and per space.
func TestParseLayouts(t *testing.T) {
	got, err := Parse(`layout = "agent"
` + layoutCommands + `
[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch}"
panes = [
  { command = "claude" },
  { command = "review", split = "right", size = 35 },
  { split = "below", of = 1, size = 30 },
]
[[layouts.tabs]]
panes = [{ command = "codex", focus = true }, { run = ["tail", "-f", "{model}.log"], split = "below" }, { command = "claude", split = "right", cwd = "{worktree}/docs" }]
[[layouts.tabs]]
layout = "tall"
panes = [{ cwd = "/tmp" }, { command = "review" }, {}]

[[layouts]]
name = "plain"
[[layouts.tabs]]
panes = [{}]

[space.gr]
layout = "plain"
worktree_layout = "agent"
`)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		Layouts       []Layout
		DefaultLayout string
		Space         map[string]SpaceSettings
	}
	prompt := Input{Name: "prompt", Type: "string", Label: "Prompt", Required: true}
	want := result{
		Layouts: []Layout{
			{
				Name: "agent",
				Tabs: []LayoutTab{
					{Title: "{branch}", Panes: []Pane{
						{Command: "claude"},
						{Command: "review", Split: "right", Of: 1, Size: 35},
						{Split: "below", Of: 1, Size: 30},
					}},
					{Panes: []Pane{
						{Command: "codex", Focus: true},
						{Run: []string{"tail", "-f", "{model}.log"}, Split: "below", Of: 1, Size: 50},
						{Command: "claude", Split: "right", Of: 2, Size: 50, Cwd: "{worktree}/docs"},
					}},
					{Layout: "tall", Panes: []Pane{{Cwd: "/tmp"}, {Command: "review"}, {}}},
				},
				Inputs: []Input{prompt, {Name: "model", Type: "string", Label: "model", Default: "o3"}},
			},
			{Name: "plain", Tabs: []LayoutTab{{Panes: []Pane{{}}}}},
		},
		DefaultLayout: "agent",
		Space:         map[string]SpaceSettings{"gr": {Layout: "plain", WorktreeLayout: "agent"}},
	}
	if g := (result{got.Layouts, got.DefaultLayout, got.Space}); !reflect.DeepEqual(g, want) {
		t.Errorf("got %#v\nwant %#v", g, want)
	}
}

// Each problem with a layout is an error that names it, and so is a default naming no layout and a space
// table naming no space.
func TestParseBadLayouts(t *testing.T) {
	one := func(panes string) string {
		return layoutCommands + "[[layouts]]\nname = \"a\"\n[[layouts.tabs]]\npanes = " + panes + "\n"
	}
	for _, c := range []struct{ in, want string }{
		{"[[layouts]]\ntabs = [{ panes = [{}] }]", "layouts: layout 1 has no name"},
		{one("[{}]") + "[[layouts]]\nname = \"a\"\ntabs = [{ panes = [{}] }]", "layouts: a is defined twice"},
		{"[[layouts]]\nname = \"a\"", "layouts: a: it has no tabs"},
		{one("[]"), "layouts: a: tab 1 has no panes"},
		{one(`[{ command = "nope" }]`), `layouts: a: tab 1 pane 1: command is "nope", no command has that name`},
		{one(`[{ command = "review", run = ["x"] }]`), "layouts: a: tab 1 pane 1: it has both command and run, it can have one"},
		{one(`[{ run = [] }]`), "layouts: a: tab 1 pane 1: run cannot be empty"},
		{one(`[{ run = ["{nope}"] }]`), "layouts: a: tab 1 pane 1: run[0]: {nope} is neither a built-in nor an input"},
		{one(`[{ split = "right" }]`), "layouts: a: tab 1 pane 1: the first pane is the tab's first window, it takes no split, of or size"},
		{one(`[{}, {}]`), `layouts: a: tab 1 pane 2: split is "", it must be one of right, below; a pane left of or above another goes before it`},
		{one(`[{}, { split = "left" }]`), `layouts: a: tab 1 pane 2: split is "left", it must be one of right, below; a pane left of or above another goes before it`},
		{one(`[{}, { split = "right", of = 2 }]`), "layouts: a: tab 1 pane 2: of is 2, it must be a pane before this one, from 1 to 1"},
		{one(`[{}, { split = "right", size = 100 }]`), "layouts: a: tab 1 pane 2: size is 100, it must be from 1 to 99"},
		{one(`[{ focus = true }, { split = "right", focus = true }]`), "layouts: a: tab 1 has 2 panes with focus, it can have one"},
		{one(`[{}]`) + "title = \"{nope}\"", "layouts: a: tab 1 title: {nope} is neither a built-in nor an input"},
		{one(`[{ cwd = "{worktree}/{nope}" }]`), `layouts: a: tab 1 pane 1: cwd: {nope} is neither a built-in nor an input`},
		{one(`[{}]`) + `layout = "columns"`, `layouts: a: tab 1 layout is "columns", it must be one of kitty's fat, grid, horizontal, splits, stack, tall, vertical, and in its enabled_layouts`},
		{one(`[{}, { split = "right" }]`) + `layout = "tall"`, "layouts: a: tab 1 pane 2: the tab's layout is tall, which places its panes in order: split, of and size are only for splits"},
		{one(`[{}, { size = 30 }]`) + `layout = "stack"`, "layouts: a: tab 1 pane 2: the tab's layout is stack, which places its panes in order: split, of and size are only for splits"},
		{one(`[{ command = "claude" }, { split = "right", sise = 3 }]`), "unknown keys: layouts.tabs.panes.sise"},
		{layoutCommands + `[[commands]]
name = "other"
run = ["x", "{prompt}"]
inputs = [{ name = "prompt" }]
[[layouts]]
name = "a"
tabs = [{ panes = [{ command = "claude" }, { command = "other", split = "right" }] }]`, "layouts: a: input prompt is defined differently by claude and other"},
		{`layout = "nope"`, `layout is "nope", no layout has that name`},
		{one("[{}]") + "[space.gr]\nworktree_layout = \"nope\"", `space.gr.worktree_layout is "nope", no layout has that name`},
		{one("[{}]") + "[space.gr]\nlayout = \"nope\"", `space.gr.layout is "nope", no layout has that name`},
		{one("[{}]") + "[space.vim]\nlayout = \"a\"", "space.vim: no space is named vim"},
		{`space = ["~/x"]`, "space must be [space.NAME] tables; the list of spaces is spaces"},
		{"space = 1", "space must be [space.NAME] tables; the list of spaces is spaces"},
	} {
		_, err := Parse(c.in)
		if got := errString(err); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.in, got, c.want)
		}
	}
}

// A linked worktree opens with its space's worktree_layout, else the space's layout, else the global
// one; the main worktree with the space's layout, else the global one.
func TestLayoutFor(t *testing.T) {
	cfg := Config{
		DefaultLayout: "global",
		Space: map[string]SpaceSettings{
			"both":   {Layout: "main", WorktreeLayout: "linked"},
			"main":   {Layout: "main"},
			"linked": {WorktreeLayout: "linked"},
		},
	}
	type result struct{ Main, Linked string }
	got := map[string]result{}
	for _, space := range []string{"both", "main", "linked", "none"} {
		got[space] = result{cfg.LayoutFor(space, false), cfg.LayoutFor(space, true)}
	}
	want := map[string]result{
		"both":   {"main", "linked"},
		"main":   {"main", "main"},
		"linked": {"global", "linked"},
		"none":   {"global", "global"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if got := (Config{}).LayoutFor("none", true); got != "" {
		t.Errorf("with no default: %q", got)
	}
}

// A layout's values are its merged inputs', defaults filled in; a pane's command is its configured one or
// its run, with the layout's inputs, and the shell has none.
func TestLayoutValues(t *testing.T) {
	cfg, err := Parse(layoutCommands + `
[[layouts]]
name = "a"
[[layouts.tabs]]
title = "{prompt} {{x}}"
panes = [{ command = "claude" }, { command = "codex", split = "right" }, { run = ["{model}"], split = "below" }, { split = "below" }]
`)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := cfg.Layout("a")
	type result struct {
		Values map[string]string
		Err    string
	}
	var got []result
	for _, given := range []map[string]string{{"prompt": "hi"}, {}, {"prompt": "hi", "x": "1"}} {
		v, err := l.Values(given)
		got = append(got, result{v, errString(err)})
	}
	want := []result{
		{map[string]string{"prompt": "hi", "model": "o3"}, "<nil>"},
		{nil, "a needs Prompt"},
		{nil, "a has no input x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
	type pane struct {
		Command Command
		OK      bool
	}
	var panes []pane
	for _, p := range l.Tabs[0].Panes {
		c, ok := cfg.PaneCommand(l, p)
		panes = append(panes, pane{c, ok})
	}
	claude, _ := cfg.Command("claude")
	codex, _ := cfg.Command("codex")
	wantPanes := []pane{{claude, true}, {codex, true}, {Command{Name: "a", Run: []string{"{model}"}, Inputs: l.Inputs}, true}, {}}
	if !reflect.DeepEqual(panes, wantPanes) {
		t.Errorf("panes %#v\nwant %#v", panes, wantPanes)
	}
	if got, want := l.Tabs[0].TitleWith(map[string]string{"prompt": "fix"}), "fix {x}"; got != want {
		t.Errorf("title %q, want %q", got, want)
	}
}
