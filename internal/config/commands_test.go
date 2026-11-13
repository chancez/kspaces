package config

import (
	"reflect"
	"testing"
)

// Commands parse with their defaults filled in: open is a tab, an input's type is string and its label
// its name. An input's default can be a string of several lines.
func TestParseCommands(t *testing.T) {
	got, err := Parse(`
[[commands]]
name = "claude"
run = ["claude", "--name={space}-{branch}", "{prompt}"]
env = { TASK_PR = "{pr}", LITERAL = "{{x}}" }
open = "split"

[[commands.inputs]]
name = "prompt"
label = "Prompt"
required = true
default = """
fix it
and test"""

[[commands.inputs]]
name = "model"
type = "string"
default = "opus"

[[commands]]
name = "gr"
run = ["gr"]
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Command{
		{
			Name: "claude",
			Run:  []string{"claude", "--name={space}-{branch}", "{prompt}"},
			Env:  map[string]string{"TASK_PR": "{pr}", "LITERAL": "{{x}}"},
			Open: "split",
			Inputs: []Input{
				{Name: "prompt", Type: "string", Label: "Prompt", Default: "fix it\nand test", Required: true},
				{Name: "model", Type: "string", Label: "model", Default: "opus"},
			},
		},
		{Name: "gr", Run: []string{"gr"}, Open: "tab"},
	}
	if !reflect.DeepEqual(got.Commands, want) {
		t.Errorf("got %#v\nwant %#v", got.Commands, want)
	}
}

// Each problem with a command is an error that names it.
func TestParseBadCommands(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`[[commands]]
run = ["x"]`, "commands: command 1 has no name"},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands]]
name = "a"
run = ["y"]`, "commands: a is defined twice"},
		{`[[commands]]
name = "a"
run = []`, "commands: a: run cannot be empty"},
		{`[[commands]]
name = "a"
run = ["x"]
open = "tabs"`, `commands: a: open is "tabs", it must be one of tab, split, hsplit, vsplit, overlay, window`},
		{`[[commands]]
name = "a"
run = ["x", "{nope}"]`, "commands: a: run[1]: {nope} is neither a built-in nor an input"},
		{`[[commands]]
name = "a"
run = ["x"]
env = { A = "{nope}" }`, "commands: a: env.A: {nope} is neither a built-in nor an input"},
		{`[[commands]]
name = "a"
run = ["x"]
env = { "A=B" = "" }`, `commands: a: env has a bad name "A=B"`},
		{`[[commands]]
name = "a"
run = ["x {"]`, `commands: a: run[0]: "x {" has a { with no }; write {{ for a brace`},
		{`[[commands]]
name = "a"
run = ["x }"]`, `commands: a: run[0]: "x }" has a } with no {; write }} for a brace`},
		{`[[commands]]
name = "a"
run = ["{}"]`, `commands: a: run[0]: "{}" has an empty {}`},
		{`[[commands]]
name = "a"
run = ["{?}"]`, `commands: a: run[0]: "{?}" has an empty {}`},
		{`[[commands]]
name = "a"
run = ["x", "{nope?}"]`, "commands: a: run[1]: {nope} is neither a built-in nor an input"},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands.inputs]]
name = "Prompt"`, `commands: a: input name "Prompt" must be lowercase letters, digits and _, starting with a letter`},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands.inputs]]
name = "branch"`, "commands: a: input branch has the name of a built-in placeholder"},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands.inputs]]
name = "p"
[[commands.inputs]]
name = "p"`, "commands: a: input p is defined twice"},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands.inputs]]
name = "p"
type = "bool"`, `commands: a: input p has type "bool", the only type is "string"`},
		{`[[commands]]
name = "a"
run = ["x"]
[[commands.inputs]]
name = "p"
lable = "P"`, "unknown keys: commands.inputs.lable"},
		{`[[commands]]
name = "a"
run = ["x", "{p}", "{q}"]
inputs = [{ name = "p" }, { name = "q", default = "{p}" }]`, "commands: a: input q default: {p} is not a built-in, the only placeholders a default can have"},
		{`[[commands]]
name = "a"
run = ["x"]
inputs = [{ name = "p", default = "{" }]`, `commands: a: input p default: "{" has a { with no }; write {{ for a brace`},
	} {
		_, err := Parse(c.in)
		if got := errString(err); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.in, got, c.want)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// Placeholders are substituted within each argument as text, never split or expanded, and doubled
// braces are literal. A word with an optional placeholder whose value is empty or only spaces is left out.
// The environment has the built-ins and inputs, then env, which can override them, optional placeholders
// substituted like any other.
func TestCommandExpand(t *testing.T) {
	cfg, err := Parse(`
[[commands]]
name = "claude"
run = ["claude", "--name={space}-{branch}", "{prompt}", "{{literal}}", "{pr}", "--pr={pr?}", "{prompt?}", "{model?}", "-m={model?}{pr}"]
env = { TASK_PR = "#{pr?}", KSPACES_PR = "overridden" }
[[commands.inputs]]
name = "prompt"
[[commands.inputs]]
name = "model"
`)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cfg.Command("claude")
	if !ok {
		t.Fatal("no command claude")
	}
	vars := map[string]string{
		"space": "gr", "root": "/src/gr", "worktree": "/src/gr/.worktrees/fix", "branch": "pr/me/fix", "base": "main",
		"session": "gr@fix", "pr": "", "prompt": `fix "it" in $HOME; rm -rf {x}`, "model": " \n",
	}
	type result struct{ Argv, Env []string }
	got := result{c.Argv(vars), c.Environ(vars)}
	want := result{
		Argv: []string{"claude", "--name=gr-pr/me/fix", `fix "it" in $HOME; rm -rf {x}`, "{literal}", "", `fix "it" in $HOME; rm -rf {x}`},
		Env: []string{
			"KSPACES_SPACE=gr", "KSPACES_ROOT=/src/gr", "KSPACES_WORKTREE=/src/gr/.worktrees/fix",
			"KSPACES_BRANCH=pr/me/fix", "KSPACES_BASE=main", "KSPACES_SESSION=gr@fix", "KSPACES_PR=",
			`KSPACES_INPUT_PROMPT=fix "it" in $HOME; rm -rf {x}`, "KSPACES_INPUT_MODEL= \n",
			"KSPACES_PR=overridden", "TASK_PR=#",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

// Values fills in defaults, their built-ins substituted, and refuses an input the command lacks or a
// required one left empty.
func TestCommandValues(t *testing.T) {
	c := Command{Name: "claude", Inputs: []Input{
		{Name: "prompt", Label: "Prompt", Required: true},
		{Name: "model", Label: "model", Default: "opus"},
		{Name: "task", Label: "task", Default: "Review PR #{pr} on {branch}, {{as is}}"},
	}}
	vars := map[string]string{"pr": "3", "branch": "feat"}
	type result struct {
		Values map[string]string
		Err    string
	}
	var got []result
	for _, given := range []map[string]string{
		{"prompt": "fix it"},
		{"prompt": "fix it", "model": "", "task": "{pr}"},
		{"prompt": "  "},
		{},
		{"prompt": "x", "temperature": "1"},
	} {
		v, err := c.Values(given, vars)
		got = append(got, result{v, errString(err)})
	}
	want := []result{
		{map[string]string{"prompt": "fix it", "model": "opus", "task": "Review PR #3 on feat, {as is}"}, "<nil>"},
		{map[string]string{"prompt": "fix it", "model": "", "task": "{pr}"}, "<nil>"},
		{nil, "claude needs Prompt"},
		{nil, "claude needs Prompt"},
		{nil, "claude has no input temperature"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}
