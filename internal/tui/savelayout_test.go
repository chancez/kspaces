package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// saveConfig has the commands a saved layout's panes can run, claude and gr as review, and the agent
// layout, which saving under its name replaces.
const saveConfig = `spaces = ["/src/gr"]

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt" }]

[[commands]]
name = "review"
run = ["/usr/local/bin/gr"]

# mine
[[layouts]]
name = "agent"
tabs = [{ panes = [{ command = "claude" }] }]

[space.gr]
layout = "agent"
`

// capturedTabs are gr@fix's tabs as kitty has them: claude resumed, gr right of it, focused, and a shell
// below claude in docs; then a tall tab running make watch, this tab.
var capturedTabs = []spaces.CapturedTab{
	{
		Tab: config.LayoutTab{Title: "{branch}", Panes: []config.Pane{
			{}, {Split: "right", Of: 1, Size: 35, Focus: true}, {Split: "below", Of: 1, Size: 30, Cwd: "{worktree}/docs"},
		}},
		Programs: []string{"claude --resume abc", "gr --watch", ""},
	},
	{Tab: config.LayoutTab{Layout: "tall", Panes: []config.Pane{{}}}, Programs: []string{"make watch {x}"}},
}

// Save as layout is on the menu of a worktree whose session has windows, and opens the save form in an
// overlay, its name left to fill in.
func TestSaveLayoutMenu(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	offered := map[string]bool{}
	for _, session := range []string{"gr", "gr@dev", "gr@fix", "default"} {
		e.m.cursor = e.row(session)
		target, _ := e.m.entryTarget(e.m.cursor)
		for _, g := range target.Groups(e.m) {
			for _, a := range g.Actions {
				if a.Is(actSaveAsLayout) {
					offered[session] = true
				}
			}
		}
	}
	if want := map[string]bool{"gr": true, "gr@dev": true}; !reflect.DeepEqual(offered, want) {
		t.Errorf("offered on %v, want %v", offered, want)
	}
	e.m.cursor = e.row("gr@dev")
	cmd := e.press("m", "j", "enter")
	if cmd == nil {
		t.Fatal("Save as layout ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("Save as layout said %v", msg)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "layout", "save", "", "/src/gr/.worktrees/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("launched %q, want %q", got, want)
	}
}

// saveEnv is the save form over capturedTabs, with saveConfig as the config file.
func saveEnv(t *testing.T, name string, only bool) (*saveLayoutForm, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "kspaces", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(saveConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	f := newSaveLayoutForm(cfg, wt, capturedTabs, 1, name, only)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	return f, path
}

func right(f tea.Model) { f.Update(tea.KeyPressMsg{Code: tea.KeyRight}) }

// The form has the name, then whether to save this tab or every tab, then what each pane runs, chosen as
// a command is: a configured command when what runs there is its program, the command line as run, its
// braces doubled, else the shell. Left and right change the choice that has focus, as a click does.
func TestSaveLayoutFormChoices(t *testing.T) {
	f, _ := saveEnv(t, "", false)
	keys := "   Save enter    Cancel esc   tab next field"
	want := []string{
		"", "  Save gr@fix as a layout", "",
		"  name       required",
		"  save       < every tab >",
		"  tab 1, pane 1: claude --resume abc",
		"  runs       < claude >",
		"  tab 1, pane 2: gr --watch",
		"  runs       < review >",
		"  tab 1, pane 3: a shell",
		"  runs       < shell >",
		"  tab 2, pane 1: make watch {x}",
		"  runs       < run >",
		"  run        make watch {{x}}",
		"", "  Adds 2 tabs to the config's layouts", "",
		keys, "",
	}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("every tab:\n%q\nwant\n%q", got, want)
	}
	// This tab alone, the second.
	f.Update(keyMsg("tab"))
	right(f)
	thisTab := []string{
		"", "  Save gr@fix as a layout", "",
		"  name       required",
		"  save       < this tab >",
		"  tab 1, pane 1: make watch {x}",
		"  runs       < run >",
		"  run        make watch {{x}}",
		"", "  Adds 1 tab to the config's layouts", "",
		keys + "  left/right choose", "",
	}
	if got := formScreen(f); !reflect.DeepEqual(got, thisTab) {
		t.Errorf("this tab:\n%q\nwant\n%q", got, thisTab)
	}
	// Its pane to the shell, past every choice: run, shell, then the commands.
	f.Update(keyMsg("tab"))
	right(f)
	if got := formScreen(f)[6]; got != "  runs       < shell >" {
		t.Errorf("after run: %q", got)
	}
	right(f)
	right(f)
	if got := formScreen(f)[6:8]; !reflect.DeepEqual(got, []string{"  runs       < review >", ""}) {
		t.Errorf("round to the commands: %q", got)
	}
	// A click cycles a choice too.
	f.View()
	x, y := f.zones["pane:1:0"][0].X, f.zones["pane:1:0"][0].Y
	f.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if got := formScreen(f)[6:8]; !reflect.DeepEqual(got, []string{"  runs       < run >", "  run        make watch {{x}}"}) {
		t.Errorf("after a click: %q", got)
	}
	// Prefilled from the command line, this tab first.
	g, _ := saveEnv(t, "watch", true)
	if got := formScreen(g)[3:5]; !reflect.DeepEqual(got, []string{"  name       watch", "  save       < this tab >"}) {
		t.Errorf("prefilled: %q", got)
	}
}

// Saving needs a name and a command line for a run pane. A name the config has asks first, and n goes
// back to the form; y replaces that layout in place, its comments and the rest of the file kept, and
// the form leaves.
func TestSaveLayoutFormSave(t *testing.T) {
	f, path := saveEnv(t, "", false)
	f.Update(keyMsg("enter"))
	if got := formScreen(f)[17:19]; !reflect.DeepEqual(got, []string{"  the layout needs a name", ""}) {
		t.Errorf("no name: %q", got)
	}
	typeInto(f, "agent")
	// Tab 2's run box emptied.
	for range 6 {
		f.Update(keyMsg("tab"))
	}
	for range len("make watch {{x}}") {
		f.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	f.Update(keyMsg("enter"))
	if got := formScreen(f)[17:19]; !reflect.DeepEqual(got, []string{"  tab 2, pane 1: run is empty", ""}) {
		t.Errorf("empty run: %q", got)
	}
	typeInto(f, "make 'watch it'")
	f.Update(keyMsg("enter"))
	confirm := []string{"  A layout called agent exists. Replace it?", "", "   Replace y    Keep it n", ""}
	if got := formScreen(f)[17:]; !reflect.DeepEqual(got, confirm) {
		t.Errorf("asking:\n%q\nwant\n%q", got, confirm)
	}
	// Typing does not reach the boxes while it asks.
	typeInto(f, "x")
	f.Update(keyMsg("n"))
	if got := formScreen(f)[13]; f.confirm || got != "  run        make 'watch it'" {
		t.Errorf("after n: asking %v, run box %q", f.confirm, got)
	}
	f.Update(keyMsg("enter"))
	_, cmd := f.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("y ran nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `spaces = ["/src/gr"]

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt" }]

[[commands]]
name = "review"
run = ["/usr/local/bin/gr"]

# mine
[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch}"
panes = [
  { command = "claude" },
  { command = "review", split = "right", size = 35, focus = true },
  { split = "below", of = 1, size = 30, cwd = "{worktree}/docs" },
]
[[layouts.tabs]]
layout = "tall"
panes = [
  { run = ["make", "watch it"] },
]

[space.gr]
layout = "agent"
`
	if string(data) != want {
		t.Errorf("config:\n%s\nwant\n%s", data, want)
	}
}

// A name the config does not have saves at once, at the end.
func TestSaveLayoutFormNew(t *testing.T) {
	f, path := saveEnv(t, "watch", true)
	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter ran nothing")
	}
	f.Update(cmd())
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := saveConfig + "\n" + `[[layouts]]
name = "watch"
[[layouts.tabs]]
layout = "tall"
panes = [
  { run = ["make", "watch", "{{x}}"] },
]
`; string(data) != want {
		t.Errorf("config:\n%s\nwant\n%s", data, want)
	}
	if f.err != nil {
		t.Error(fmt.Errorf("saving: %w", f.err))
	}
}
