package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// savedCommands are what the saved layouts below run.
const savedCommands = `# mine
spaces = ["/src/gr"] # the one

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt" }]
`

// saved is a layout as the save form captures it: claude with gr on its right and a shell below it, from
// a tab whose title is its branch, then a tall tab of make watch and a shell elsewhere, focused.
var saved = Layout{Name: "agent", Tabs: []LayoutTab{
	{Title: "{branch} {{x}}", Panes: []Pane{
		{Command: "claude"},
		{Run: []string{"gr", "--say", `"hi"`}, Split: "right", Of: 1, Size: 35},
		{Split: "below", Of: 1, Size: 30, Cwd: "{worktree}/docs"},
		{Split: "right", Of: 3, Size: 50, Focus: true},
	}},
	{Layout: "tall", Panes: []Pane{{Run: []string{"make", "watch"}}, {Cwd: "/tmp"}}},
}}

// savedBlock is saved as AddLayout writes it.
const savedBlock = `[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch} {{x}}"
panes = [
  { command = "claude" },
  { run = ["gr", "--say", "\"hi\""], split = "right", size = 35 },
  { split = "below", of = 1, size = 30, cwd = "{worktree}/docs" },
  { split = "right", focus = true },
]
[[layouts.tabs]]
layout = "tall"
panes = [
  { run = ["make", "watch"] },
  { cwd = "/tmp" },
]
`

// A layout goes at the end of the config, a blank line after what is there, however that ends; replacing
// one puts it in place of the old one's lines, from its [[layouts]] to the next table that is not one of
// its own, keeping the comments above either and everything else as it was.
func TestAddLayout(t *testing.T) {
	old := `# the old agent
[[layouts]]
name = "agent" # first
[[layouts.tabs]]
# its panes
panes = [{}, { split = "right" }]

`
	plain := Layout{Name: "plain", Tabs: []LayoutTab{{Panes: []Pane{{}}}}}
	plainBlock := "[[layouts]]\nname = \"plain\"\n[[layouts.tabs]]\npanes = [\n  {},\n]\n"
	for _, c := range []struct {
		name, in string
		l        Layout
		replace  bool
		want     string
	}{
		{"empty", "", plain, false, plainBlock},
		{"blank", "\n", plain, false, "\n" + plainBlock},
		{"newline", savedCommands, saved, false, savedCommands + "\n" + savedBlock},
		{"blank line", savedCommands + "\n", saved, false, savedCommands + "\n" + savedBlock},
		{"no newline", savedCommands + "# last", saved, false, savedCommands + "# last\n\n" + savedBlock},
		{"replace the last", savedCommands + "\n" + old, saved, true, savedCommands + "\n# the old agent\n" + savedBlock + "\n"},
		{"replace at the end with no newline", savedCommands + "[[layouts]]\nname = \"agent\"\ntabs = [{ panes = [{}] }]", saved, true,
			savedCommands + savedBlock},
		{
			"replace between", savedCommands + "\n[[layouts]]\nname = \"plain\"\ntabs = [{ panes = [{}] }]\n\n" + old +
				"# not the agent's\n[space.gr]\nlayout = \"plain\"\n",
			saved, true,
			savedCommands + "\n[[layouts]]\nname = \"plain\"\ntabs = [{ panes = [{}] }]\n\n# the old agent\n" + savedBlock +
				"\n# not the agent's\n[space.gr]\nlayout = \"plain\"\n",
		},
	} {
		got, err := AddLayout(c.in, c.l, c.replace)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

// A layout that exists is replaced only when asked to, and only when it is a [[layouts]] table; a layout
// that would not parse, or a config that does not, is left as it is.
func TestAddLayoutRefused(t *testing.T) {
	broken := saved
	broken.Tabs = []LayoutTab{{Panes: []Pane{{Command: "nope"}}}}
	for _, c := range []struct {
		in      string
		l       Layout
		replace bool
		want    string
	}{
		{savedCommands + "[[layouts]]\nname = \"agent\"\ntabs = [{ panes = [{}] }]\n", saved, false, "a layout called agent exists"},
		{"layouts = [{ name = \"agent\", tabs = [{ panes = [{}] }] }]\n" + savedCommands, saved, true,
			"the config's layouts are not all [[layouts]] tables, so one cannot be replaced in place"},
		{savedCommands, broken, false,
			`saving layout agent would break the config, so it was left as is: layouts: agent: tab 1 pane 1: command is "nope", no command has that name`},
		{"spaces = 1", saved, false, "the config has an error to fix first: toml: line 1 (last key \"spaces\"): spaces must be a list of paths, or of { path = ... } tables"},
	} {
		got, err := AddLayout(c.in, c.l, c.replace)
		if errString(err) != c.want {
			t.Errorf("%s:\ngot  %q, %s\nwant %s", c.in, got, errString(err), c.want)
		}
	}
}

// SaveLayout writes through a symlinked config, as SaveSpace does, and parses back to the layout saved.
func TestSaveLayout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	target := filepath.Join(dir, "kspaces.toml")
	if err := os.WriteFile(target, []byte(savedCommands), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "kspaces", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveLayout(saved, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	dest, _ := os.Readlink(link)
	info, _ := os.Stat(target)
	data, _ := os.ReadFile(target)
	withInputs := saved
	withInputs.Inputs = []Input{{Name: "prompt", Type: "string", Label: "Prompt"}}
	got := []any{dest, info.Mode().Perm().String(), string(data), cfg.Layouts}
	want := []any{target, "-rw-------", savedCommands + "\n" + savedBlock, []Layout{withInputs}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
