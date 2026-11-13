package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Command is a command to run in a worktree, from the new worktree form or a worktree's Run menu item:
//
//	[[commands]]
//	name = "claude"
//	run = ["claude", "{prompt}"]
//	env = { TASK_PR = "{pr}" }
//	open = "tab"
//	inputs = [{ name = "prompt", label = "Prompt", required = true }]
//
// run and the values of env are templates: {NAME} is a built-in, see Builtins, or an input, substituted
// as text within the argument, and {{ and }} are literal braces. {NAME?} is substituted the same, but a
// word of run with one whose value is empty is left out.
type Command struct {
	Name string `toml:"name"`
	// The command line, its first word the program. Never run through a shell.
	Run []string `toml:"run"`
	// Environment variables to set, after the KSPACES_ ones Environ sets.
	Env map[string]string `toml:"env"`
	// Where the command opens in a session that has windows, one of Opens. tab by default.
	Open string `toml:"open"`
	// What is asked for before running it, in order.
	Inputs []Input `toml:"inputs"`
}

// Input is a value a command asks for, a placeholder of its templates.
type Input struct {
	// What its placeholder is, [a-z][a-z0-9_]*, and not a built-in's.
	Name string `toml:"name"`
	// InputString, the only type so far, and the default: text, which can take several lines.
	Type string `toml:"type"`
	// What the form calls it, the name by default.
	Label string `toml:"label"`
	// What it is when not given, a template of the built-ins, see DefaultWith.
	Default string `toml:"default"`
	// Required refuses to run the command with the input empty.
	Required bool `toml:"required"`
}

// DefaultWith is the input's default with vars, the built-ins, substituted.
func (in Input) DefaultWith(vars map[string]string) string {
	return expand(in.Default, vars)
}

// InputString is an input of text, which can take several lines.
const InputString = "string"

// OpenTab is where a command opens by default: a new tab in the session.
const OpenTab = "tab"

// Opens are the places a command can open in a session that has windows: a new tab, a split beside the
// session's window, the layout or the given axis choosing how, an overlay over it, or a new OS window.
var Opens = []string{OpenTab, "split", "hsplit", "vsplit", "overlay", "window"}

// Builtins are the placeholders every command has, from the worktree it runs in: the space's name, its
// root, the worktree's path, its branch, the main worktree's branch, the session, and the worktree's PR
// number, when known.
var Builtins = []string{"space", "root", "worktree", "branch", "base", "session", "pr"}

var inputName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// checkCommands fills in the commands' defaults and says what is wrong with the first that has a problem,
// naming it.
func checkCommands(cmds []Command) error {
	seen := map[string]bool{}
	for i := range cmds {
		c := &cmds[i]
		if c.Name == "" {
			return fmt.Errorf("commands: command %d has no name", i+1)
		}
		if seen[c.Name] {
			return fmt.Errorf("commands: %s is defined twice", c.Name)
		}
		seen[c.Name] = true
		if err := c.check(); err != nil {
			return fmt.Errorf("commands: %s: %w", c.Name, err)
		}
	}
	return nil
}

func (c *Command) check() error {
	if len(c.Run) == 0 {
		return fmt.Errorf("run cannot be empty")
	}
	if c.Open == "" {
		c.Open = OpenTab
	}
	if !slices.Contains(Opens, c.Open) {
		return fmt.Errorf("open is %q, it must be one of %s", c.Open, strings.Join(Opens, ", "))
	}
	known := map[string]bool{}
	for _, b := range Builtins {
		known[b] = true
	}
	for i := range c.Inputs {
		in := &c.Inputs[i]
		switch {
		case !inputName.MatchString(in.Name):
			return fmt.Errorf("input name %q must be lowercase letters, digits and _, starting with a letter", in.Name)
		case slices.Contains(Builtins, in.Name):
			return fmt.Errorf("input %s has the name of a built-in placeholder", in.Name)
		case known[in.Name]:
			return fmt.Errorf("input %s is defined twice", in.Name)
		}
		known[in.Name] = true
		if in.Type == "" {
			in.Type = InputString
		}
		if in.Type != InputString {
			return fmt.Errorf("input %s has type %q, the only type is %q", in.Name, in.Type, InputString)
		}
		if in.Label == "" {
			in.Label = in.Name
		}
		if err := checkDefault("input "+in.Name+" default", in.Default); err != nil {
			return err
		}
	}
	if err := checkRun(c.Run, known); err != nil {
		return err
	}
	for _, k := range slices.Sorted(maps.Keys(c.Env)) {
		if k == "" || strings.ContainsAny(k, "= \t\n") {
			return fmt.Errorf("env has a bad name %q", k)
		}
		if err := checkTemplate("env."+k, c.Env[k], known); err != nil {
			return err
		}
	}
	return nil
}

// checkTemplate says what is wrong with tmpl, found at where, whose placeholders must be in known.
func checkTemplate(where, tmpl string, known map[string]bool) error {
	parts, err := parseTemplate(tmpl)
	if err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	for _, p := range parts {
		if p.name != "" && !known[p.name] {
			return fmt.Errorf("%s: {%s} is neither a built-in nor an input", where, p.name)
		}
	}
	return nil
}

// checkDefault says what is wrong with tmpl, an input's default found at where, which can only use the
// built-ins: an input's value is not known before the inputs are filled in.
func checkDefault(where, tmpl string) error {
	parts, err := parseTemplate(tmpl)
	if err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	for _, p := range parts {
		if p.name != "" && !slices.Contains(Builtins, p.name) {
			return fmt.Errorf("%s: {%s} is not a built-in, the only placeholders a default can have", where, p.name)
		}
	}
	return nil
}

// checkRun checks each word of run with checkTemplate.
func checkRun(run []string, known map[string]bool) error {
	for i, arg := range run {
		if err := checkTemplate(fmt.Sprintf("run[%d]", i), arg, known); err != nil {
			return err
		}
	}
	return nil
}

// part is a piece of a template: literal text, or the placeholder name, optional when written {name?}.
type part struct {
	text     string
	name     string
	optional bool
}

// parseTemplate splits tmpl into text and placeholders. {{ and }} are a literal brace; a brace that is not
// doubled and closes no placeholder is an error.
func parseTemplate(tmpl string) ([]part, error) {
	var parts []part
	var text strings.Builder
	for i := 0; i < len(tmpl); i++ {
		switch c := tmpl[i]; {
		case strings.HasPrefix(tmpl[i:], "{{"), strings.HasPrefix(tmpl[i:], "}}"):
			text.WriteByte(c)
			i++
		case c == '{':
			end := strings.IndexByte(tmpl[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("%q has a { with no }; write {{ for a brace", tmpl)
			}
			name, optional := strings.CutSuffix(tmpl[i+1:i+end], "?")
			if name == "" {
				return nil, fmt.Errorf("%q has an empty {}", tmpl)
			}
			if text.Len() > 0 {
				parts = append(parts, part{text: text.String()})
				text.Reset()
			}
			parts = append(parts, part{name: name, optional: optional})
			i += end
		case c == '}':
			return nil, fmt.Errorf("%q has a } with no {; write }} for a brace", tmpl)
		default:
			text.WriteByte(c)
		}
	}
	if text.Len() > 0 {
		parts = append(parts, part{text: text.String()})
	}
	return parts, nil
}

// expand is tmpl, checked by Parse, with each placeholder replaced by its value in vars.
func expand(tmpl string, vars map[string]string) string {
	parts, _ := parseTemplate(tmpl)
	var b strings.Builder
	for _, p := range parts {
		if p.name != "" {
			b.WriteString(vars[p.name])
		} else {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// Argv is the command's run with vars, the built-ins and the inputs by name, substituted, leaving out a
// word with an optional placeholder whose value is empty or only spaces.
func (c Command) Argv(vars map[string]string) []string {
	var out []string
	for _, arg := range c.Run {
		if !omitted(arg, vars) {
			out = append(out, expand(arg, vars))
		}
	}
	return out
}

// omitted is whether tmpl has an optional placeholder whose value in vars is empty or only spaces.
func omitted(tmpl string, vars map[string]string) bool {
	parts, _ := parseTemplate(tmpl)
	return slices.ContainsFunc(parts, func(p part) bool {
		return p.optional && strings.TrimSpace(vars[p.name]) == ""
	})
}

// Environ is the environment the command runs with, as NAME=VALUE: KSPACES_ and the built-in's name in
// capitals for each built-in, KSPACES_INPUT_ and the input's for each input, then env, by name, expanded
// with vars, overriding them.
func (c Command) Environ(vars map[string]string) []string {
	var out []string
	for _, b := range Builtins {
		out = append(out, "KSPACES_"+strings.ToUpper(b)+"="+vars[b])
	}
	for _, in := range c.Inputs {
		out = append(out, "KSPACES_INPUT_"+strings.ToUpper(in.Name)+"="+vars[in.Name])
	}
	for _, k := range slices.Sorted(maps.Keys(c.Env)) {
		out = append(out, k+"="+expand(c.Env[k], vars))
	}
	return out
}

// Values are the command's inputs as given, by name, the default with vars, the built-ins, for any not
// given. An input it does not have, or a required one left empty, is an error.
func (c Command) Values(given, vars map[string]string) (map[string]string, error) {
	return inputValues(c.Name, c.Inputs, given, vars)
}

// inputValues are the values of inputs, those of owner, as given, the default with vars for any not given.
func inputValues(owner string, inputs []Input, given, vars map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for name := range given {
		if !slices.ContainsFunc(inputs, func(in Input) bool { return in.Name == name }) {
			return nil, fmt.Errorf("%s has no input %s", owner, name)
		}
	}
	for _, in := range inputs {
		v, ok := given[in.Name]
		if !ok {
			v = in.DefaultWith(vars)
		}
		if in.Required && strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%s needs %s", owner, in.Label)
		}
		out[in.Name] = v
	}
	return out, nil
}

// Command is the configured command called name.
func (cfg Config) Command(name string) (Command, bool) {
	i := slices.IndexFunc(cfg.Commands, func(c Command) bool { return c.Name == name })
	if i < 0 {
		return Command{}, false
	}
	return cfg.Commands[i], true
}
