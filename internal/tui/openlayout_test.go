package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
)

// testLayouts are a plain layout and the one I use, claude with gr on its right and a shell below it,
// the default for gr's linked worktrees.
func testLayouts(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Parse(`
spaces = ["/src/gr"]

[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", required = true }]

[[commands]]
name = "review"
run = ["gr"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]

[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch}"
panes = [
  { command = "claude" },
  { command = "review", split = "right", size = 35 },
  { split = "below", of = 1, size = 30 },
]

[space.gr]
worktree_layout = "agent"
`)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// fakeKittenWindows puts a kitten on PATH whose `kitten @ ls` lists session's windows, at first those in
// ids, and whose every other call with a --type= option opens a window, 11 and on. It records each call
// but ls, its arguments one a line and -- after each, and returns those calls.
func fakeKittenWindows(t *testing.T, session string, ids ...int) func() []string {
	t.Helper()
	dir := t.TempDir()
	var lines []string
	for _, id := range ids {
		lines = append(lines, fmt.Sprint(id)+"\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "ids"), []byte(strings.Join(lines, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "next"), []byte("10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
d=%s
if [ "$2" = ls ]; then
  printf '[{"is_focused":true,"tabs":[{"id":1,"is_active":true,"windows":['
  sep=
  while read -r id; do
    printf '%%s{"id":%%s,"is_active":true,"session_name":"%s","last_focused_at":%%s}' "$sep" "$id" "$id"
    sep=,
  done < "$d/ids"
  printf ']}]}]'
  exit 0
fi
{ for a; do printf '%%s\n' "$a"; done; echo --; } >> "$d/calls"
case " $* " in
*" --type="*)
  n=$(( $(cat "$d/next") + 1 ))
  echo "$n" > "$d/next"
  echo "$n" >> "$d/ids"
  ;;
esac
`, dir, session)
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	return func() []string {
		data, err := os.ReadFile(filepath.Join(dir, "calls"))
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// envOf is the --env options of a command run in wt, a worktree of the space at root, then extra.
func envOf(root string, wt spaces.Worktree, extra ...string) []string {
	return append([]string{
		"--env=KSPACES_SPACE=" + filepath.Base(root), "--env=KSPACES_ROOT=" + root, "--env=KSPACES_WORKTREE=" + wt.Path,
		"--env=KSPACES_BRANCH=" + wt.Branch, "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=" + wt.Session, "--env=KSPACES_PR=",
	}, extra...)
}

// agentCalls are the kitten calls that build the agent layout in wt, a worktree of the space at root, its
// prompt "fix it", its tab launched with opts first, windows 11 to 13 opening.
func agentCalls(root string, wt spaces.Worktree, opts ...string) []string {
	at := []string{"--cwd=" + wt.Path, "--add-to-session=" + wt.Session}
	return slices.Concat(
		[]string{"@", "action"}, opts, []string{"launch", "--type=tab"}, at, []string{"--tab-title=" + wt.Branch},
		envOf(root, wt, "'--env=KSPACES_INPUT_PROMPT=fix it'"), []string{"--hold", "claude", "'fix it'", "--"},
		[]string{"@", "goto-layout", "--match=window_id:11", "splits", "--"},
		[]string{"@", "action", "--match=id:11", "launch", "--type=window", "--location=vsplit", "--next-to=id:11", "--bias=35"}, at,
		envOf(root, wt), []string{"--hold", "gr", "--"},
		[]string{"@", "action", "--match=id:11", "launch", "--type=window", "--location=hsplit", "--next-to=id:11", "--bias=30"}, at,
		[]string{"--"},
		[]string{"@", "focus-window", "--match", "id:11", "--"},
	)
}

// Open layout is on a worktree's menu, and L its key, only with layouts configured; it opens the layout
// form in an overlay, with the worktree's default layout chosen, else the first.
func TestLayoutKey(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	e.m.cursor = e.row("gr@dev")
	if _, ok := runOn(e.m, keyMsg("L"), e.m.chain()...); ok {
		t.Error("L ran something with no layouts")
	}
	snap := fixture(time.Now())
	cfg := testLayouts(t)
	snap.Config = &cfg
	e.m.Update(snap)
	e.m.render()
	dev := e.row("gr@dev")
	e.click(fmt.Sprintf(entryZone, dev), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, dev))
	want := []string{
		menuRule("Session"), " Open", " Run", " Open layout", " Save as layout", " Close session",
		menuRule("Worktree"), " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+14]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	e.press("esc")

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ session, layout, path string }{
		{"gr@dev", "agent", "/src/gr/.worktrees/dev"},
		{"gr", "plain", "/src/gr"},
	} {
		e.m.cursor = e.row(c.session)
		cmd, ok := runOn(e.m, keyMsg("L"), e.m.chain()...)
		if !ok || cmd == nil {
			t.Fatalf("%s: L ran nothing", c.session)
		}
		if msg := cmd(); msg != nil {
			t.Errorf("%s: L said %v", c.session, msg)
		}
		if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "layout", c.layout, c.path}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: L launched %q, want %q", c.session, got, want)
		}
	}
}

// The layout form chooses between the layouts, with a field for each input of the one chosen, and says
// what its tabs hold and whether they go in the session or open it. A required input left empty refuses
// to open. Open goes to the session and adds the tabs by its window.
func TestLayoutForm(t *testing.T) {
	// Opening looks up the worktree's PR with git, in a repository that is not there.
	gittest.Isolate(t)
	calls := fakeKittenWindows(t, "gr@fix", 7)
	cfg := testLayouts(t)
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	open := kitty.State{Sessions: map[string]int{"gr@fix": 7}}
	screen := func(fields []string, says ...string) []string {
		lines := append([]string{"", "  Open a layout in gr@fix", ""}, fields...)
		lines = append(lines, "")
		for _, s := range says {
			if s != "" {
				s = "  " + s
			}
			lines = append(lines, s)
		}
		return lines
	}
	sized := func(f *layoutForm) *layoutForm {
		f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
		return f
	}
	f := sized(newLayoutForm(cfg, space, wt, open, "agent", nil))
	focused := "  layout     < agent >  left/right change  enter list"
	want := screen([]string{focused, "  Prompt     required"}, "A tab of claude, review, a shell", "Adds its tabs to gr@fix", "",
		" Open    Cancel esc   tab next field  left/right layout  enter/space list", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent:\n%q\nwant\n%q", got, want)
	}
	// enter on the layout opens the list, so it opens the layout from the prompt.
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("enter"))
	want = screen([]string{"  layout     < agent >", "  Prompt     required"}, "A tab of claude, review, a shell", "Adds its tabs to gr@fix", "",
		"agent needs Prompt", "", " Open enter    Cancel esc   tab next field  shift+enter newline", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent, its prompt empty:\n%q\nwant\n%q", got, want)
	}
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	f.err = nil
	want = screen([]string{"  layout     < plain >  left/right change  enter list"}, "A tab of a shell", "Adds its tabs to gr@fix", "",
		" Open    Cancel esc   left/right layout  enter/space list", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("plain:\n%q\nwant\n%q", got, want)
	}
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	// Prefilled, and with the session closed.
	if got, want := formScreen(sized(newLayoutForm(cfg, space, wt, kitty.State{}, "agent", map[string]string{"prompt": "hi"})))[3:9],
		[]string{focused, "  Prompt     hi", "", "  A tab of claude, review, a shell", "  Opens gr@fix with its tabs, as it has no windows", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("prefilled:\n%q\nwant\n%q", got, want)
	}
	// With one layout there is nothing to choose.
	one := cfg
	one.Layouts = cfg.Layouts[:1]
	if got, want := formScreen(sized(newLayoutForm(one, space, wt, open, "plain", nil)))[1:6], []string{"  Open a layout in gr@fix", "", "  A tab of a shell", "  Adds its tabs to gr@fix", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("one layout:\n%q\nwant\n%q", got, want)
	}

	f.Update(keyMsg("tab"))
	typeInto(f, "fix it")
	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter ran nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	want = slices.Concat([]string{"@", "action", "goto_session", "gr@fix.kitty-session", "--"}, agentCalls("/src/gr", wt, "--match=id:7"))
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// listRule is a rule of a choice field's list in a form 70 cells wide, with what is typed on it, if any.
func listRule(typed string) string {
	if typed == "" {
		return "             " + strings.Repeat("\u2500", 55)
	}
	return "             \u2500 " + typed + " " + strings.Repeat("\u2500", 52-len(typed))
}

// listRow is a row of a choice field's list in a form 70 cells wide, marked as the one chosen or not.
func listRow(chosen bool, s string) string {
	if chosen {
		return "             \uf00c " + s
	}
	return "               " + s
}

// enter, space or a click on the layout opens a list of the layouts, the one chosen marked and selected,
// with how many tabs and panes each has. j, k and the arrows move; typing filters; enter or a click
// chooses; esc or a click outside it closes it as it was. While it is open the form's keys are its.
func TestLayoutFormList(t *testing.T) {
	cfg := testLayouts(t)
	cfg.Layouts = append(cfg.Layouts, config.Layout{Name: "review", Tabs: []config.LayoutTab{
		{Panes: []config.Pane{{Command: "review"}, {Split: "right"}}}, {Panes: []config.Pane{{}}},
	}})
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	f := newLayoutForm(cfg, space, wt, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, "plain", nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	open := func(field string, rows ...string) []string {
		lines := append([]string{"", "  Open a layout in gr@fix", "", "  layout     < " + field + " >  left/right change  enter list"}, rows...)
		return append(lines, "  up/down move  type filter  enter choose  esc close", "")
	}
	selected := func() string { l := f.layouts.list; return l.names[l.chosen()] }
	check := func(what string, want []string, sel string) {
		t.Helper()
		if got := formScreen(f); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%q\nwant\n%q", what, got, want)
		}
		if sel != "" && selected() != sel {
			t.Errorf("%s: %s selected, want %s", what, selected(), sel)
		}
	}
	all := open("plain", listRule(""), listRow(true, "plain  1 tab, 1 pane"), listRow(false, "agent  1 tab, 3 panes"),
		listRow(false, "review  2 tabs, 3 panes"), listRule(""))
	f.Update(keyMsg("enter"))
	check("open", all, "plain")

	// The form's keys do nothing while it is open: tab stays, esc closes the list, not the form.
	f.Update(keyMsg("tab"))
	check("tab", all, "plain")
	if _, cmd := f.Update(keyMsg("esc")); cmd != nil || f.layouts.listOpen() || f.focus != 0 {
		t.Errorf("esc: cmd %v, open %v, focus %d; want the list closed", cmd != nil, f.layouts.listOpen(), f.focus)
	}
	f.Update(keyMsg("space"))
	f.Update(keyMsg("j"))
	f.Update(keyMsg("down"))
	f.Update(keyMsg("k"))
	check("moved", all, "agent")
	f.Update(keyMsg("esc"))
	if c, _ := f.layouts.chosen(); c.Name != "plain" {
		t.Errorf("esc chose %s, want plain", c.Name)
	}

	// Typed, j and k filter like any letter.
	f.Update(keyMsg("enter"))
	typeInto(f, "e")
	check("filtered", open("plain", listRule("e"), listRow(false, "agent  1 tab, 3 panes"), listRow(false, "review  2 tabs, 3 panes"), listRule("")), "agent")
	typeInto(f, "j")
	f.Update(keyMsg("enter"))
	check("no match", open("plain", listRule("ej"), "              no matches", listRule(""), ""), "")
	f.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	f.Update(keyMsg("down"))
	// enter chooses rather than opening the layout.
	if f.Update(keyMsg("enter")); f.busy || f.layouts.listOpen() {
		t.Errorf("enter: busy %v, open %v; want review chosen", f.busy, f.layouts.listOpen())
	}
	check("review", []string{"", "  Open a layout in gr@fix", "", "  layout     < review >  left/right change  enter list", "",
		"  A tab of review, a shell", "  A tab of a shell", "  Adds its tabs to gr@fix", "",
		"   Open    Cancel esc   left/right layout  enter/space list", ""}, "")

	// A click on the field opens it, outside closes it, on a rule leaves it, on a layout chooses it.
	clickOn(t, f, "< review >")
	clickOn(t, f, "Open a layout")
	if c, _ := f.layouts.chosen(); f.layouts.listOpen() || c.Name != "review" {
		t.Errorf("a click outside: open %v, %s chosen; want it closed on review", f.layouts.listOpen(), c.Name)
	}
	clickOn(t, f, "< review >")
	clickOn(t, f, "\u2500")
	if !f.layouts.listOpen() {
		t.Error("a click on the rule closed the list")
	}
	clickOn(t, f, "plain")
	if c, _ := f.layouts.chosen(); f.layouts.listOpen() || c.Name != "plain" {
		t.Errorf("a click on plain: open %v, %s chosen", f.layouts.listOpen(), c.Name)
	}
}

// A list longer than the screen has room for scrolls, marking how many are out of view either side.
func TestLayoutFormListScrolls(t *testing.T) {
	cfg := testLayouts(t)
	cfg.Layouts = nil
	for i := range 6 {
		cfg.Layouts = append(cfg.Layouts, config.Layout{Name: fmt.Sprintf("l%d", i+1), Tabs: []config.LayoutTab{{Panes: []config.Pane{{}}}}})
	}
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	f := newLayoutForm(cfg, space, wt, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, "l1", nil)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 12})
	f.Update(keyMsg("enter"))
	screen := func(rows ...string) []string {
		lines := append([]string{"", "  Open a layout in gr@fix", "", "  layout     < l1 >  left/right change  enter list", listRule("")}, rows...)
		return append(lines, listRule(""), "  up/down move  type filter  enter choose  esc close", "")
	}
	want := screen("", listRow(true, "l1  1 tab, 1 pane"), listRow(false, "l2  1 tab, 1 pane"), "              \u25be 4 more")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("top:\n%q\nwant\n%q", got, want)
	}
	for range 3 {
		f.Update(keyMsg("down"))
	}
	want = screen("              \u25b4 2 more", listRow(false, "l3  1 tab, 1 pane"), listRow(false, "l4  1 tab, 1 pane"), "              \u25be 2 more")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("scrolled:\n%q\nwant\n%q", got, want)
	}
}

// A layout's detail in the list is how many tabs and panes it has, a command's its command line.
func TestChoiceDetails(t *testing.T) {
	cfg := testLayouts(t)
	var got []string
	for _, l := range cfg.Layouts {
		got = append(got, layoutDetail(l))
	}
	for _, c := range cfg.Commands {
		got = append(got, commandDetail(c))
	}
	if want := []string{"1 tab, 1 pane", "1 tab, 3 panes", "claude {prompt}", "gr"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// kspaces layout --yes opens the layout in the worktree a directory is in, refusing a required input not
// given, an input the layout does not have and a layout that is not configured. A session with no windows
// gets the tabs as its first, then the switch.
func TestOpenLayoutNow(t *testing.T) {
	root := mainRepo(t)
	cfg := testLayouts(t)
	cfg.Spaces = []string{root}
	space := spaces.LoadSpace(root)
	wt, err := spaces.CreateWorktree(space, "fix", filepath.Join(root, ".worktrees", "fix"))
	if err != nil {
		t.Fatal(err)
	}
	calls := fakeKittenWindows(t, wt.Session)
	sub := filepath.Join(wt.Path, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range []struct {
		name   string
		inputs map[string]string
	}{
		{"agent", nil},
		{"agent", map[string]string{"model": "opus"}},
		{"nope", nil},
		{"agent", map[string]string{"prompt": "fix it"}},
	} {
		got = append(got, errString(OpenLayoutNow(cfg, sub, c.name, c.inputs)))
	}
	want := []string{"agent needs Prompt", "agent has no input model", "no layout nope in the config", "<nil>"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if got, want := calls(), append(agentCalls(root, wt), "@", "action", "goto_session", wt.Session+".kitty-session", "--"); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// With layouts configured the new worktree form chooses one, the space's worktree layout at first, with a
// field for each of its inputs, before the choice of command. Created, the session opens with the layout,
// then the command runs where it opens, by the window the layout left focused.
func TestNewWorktreeFormLayout(t *testing.T) {
	root := mainRepo(t)
	calls := fakeKittenWindows(t, filepath.Base(root)+"@fix")
	cfg := testLayouts(t)
	cfg.Spaces = []string{root}
	cfg.Space = map[string]config.SpaceSettings{filepath.Base(root): {WorktreeLayout: "agent"}}
	space := spaces.LoadSpace(root)
	f := newForm(cfg, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	keys := "   Create enter    Cancel esc   tab next field"
	screen := func(fields []string, keys string, says ...string) []string {
		lines := append([]string{"", "  New worktree in " + space.Name, "", "  branch     fix", "  directory  .worktrees/fix"}, fields...)
		lines = append(lines, "", "  Creates fix from main")
		for _, s := range says {
			lines = append(lines, "  "+s)
		}
		return append(lines, "", keys, "")
	}
	typeInto(f, "fix")
	agent := []string{"  layout     < agent >", "  Prompt     required"}
	if got, want := formScreen(f), screen(append(agent, "  command    < none >"), keys, "Opens with the agent layout"); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("tab"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got, want := formScreen(f), screen([]string{"  layout     < none >  left/right change  enter list", "  command    < none >"},
		"   Create    Cancel esc   tab next field  left/right layout  enter/space list"); !reflect.DeepEqual(got, want) {
		t.Errorf("none:\n%q\nwant\n%q", got, want)
	}
	// With no room below, the command's list opens above it.
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 12})
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("enter"))
	if got, want := formScreen(f), []string{"", listRule(""), listRow(true, "none"), listRow(false, "claude  claude {prompt}"), listRow(false, "review  gr"), listRule(""),
		"  command    < none >  left/right change  enter list", "", "  Creates fix from main", "",
		"  up/down move  type filter  enter choose  esc close", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("above:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("esc"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	// A click opens the list, none first as it is a choice, and a click on one chooses it.
	clickOn(t, f, "< none >")
	rule := "             " + strings.Repeat("\u2500", 55)
	want := []string{"", "  New worktree in " + space.Name, "", "  branch     fix", "  directory  .worktrees/fix",
		"  layout     < none >  left/right change  enter list",
		rule, "             \uf00c none", "               plain  1 tab, 1 pane", "               agent  1 tab, 3 panes", rule,
		"  up/down move  type filter  enter choose  esc close", ""}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("the list:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "agent")
	f.Update(keyMsg("tab"))
	typeInto(f, "fix it")
	f.Update(keyMsg("tab"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	want = screen(append(agent[:1:1], "  Prompt     fix it", "  command    < review >  left/right change  enter list"),
		"   Create    Cancel esc   tab next field  left/right command  enter/space list",
		"Opens with the agent layout", "Then runs review in a new tab")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent and review:\n%q\nwant\n%q", got, want)
	}
	// enter on a choice opens its list, so create from the branch.
	f.Update(keyMsg("tab"))
	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter created nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	wt := spaces.LoadSpace(root).Worktrees[1]
	goTo := []string{"@", "action", "goto_session", wt.Session + ".kitty-session", "--"}
	want = slices.Concat(agentCalls(root, wt), goTo, goTo,
		[]string{"@", "action", "--match=id:11", "launch", "--type=tab", "--cwd=" + wt.Path, "--add-to-session=" + wt.Session},
		envOf(root, wt), []string{"--hold", "gr", "--"})
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// Opening a worktree afresh opens its default layout; a default that needs an input it has no default for
// opens the shell tab instead, and says so.
func TestOpenWorktreeLayout(t *testing.T) {
	root := mainRepo(t)
	space := spaces.LoadSpace(root)
	wt, err := spaces.CreateWorktree(space, "fix", filepath.Join(root, ".worktrees", "fix"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := testLayouts(t)
	cfg.Spaces = []string{root}
	cfg.Space = map[string]config.SpaceSettings{space.Name: {Layout: "plain", WorktreeLayout: "agent"}}
	type result struct {
		Err   string
		Calls []string
	}
	var got []result
	for _, w := range []spaces.Worktree{space.Worktrees[0], wt} {
		calls := fakeKittenWindows(t, w.Session)
		err := spaces.OpenPath(cfg, w.Path, spaces.OpenOptions{})
		got = append(got, result{errString(err), calls()})
	}
	at := func(wt spaces.Worktree) []string {
		return []string{"--cwd=" + wt.Path, "--add-to-session=" + wt.Session}
	}
	main := space.Worktrees[0]
	want := []result{
		{"<nil>", slices.Concat([]string{"@", "action", "launch", "--type=tab"}, at(main), []string{"--",
			"@", "goto-layout", "--match=window_id:11", "splits", "--",
			"@", "focus-window", "--match", "id:11", "--",
			"@", "action", "goto_session", main.Session + ".kitty-session", "--"})},
		{"opened " + wt.Session + " with a shell, as agent needs Prompt", slices.Concat([]string{"@", "action", "launch", "--type=tab"}, at(wt), []string{"--",
			"@", "action", "goto_session", wt.Session + ".kitty-session", "--"})},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// Opening without the layout opens the one tab where the default layout would open, and changes nothing
// where a saved file restores the session or there is no default.
func TestOpenWithoutLayout(t *testing.T) {
	root := mainRepo(t)
	space := spaces.LoadSpace(root)
	wt := space.Worktrees[0]
	saved := t.TempDir()
	if err := os.WriteFile(filepath.Join(saved, wt.Session+".kitty-session"), []byte("launch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withDefault := testLayouts(t)
	withDefault.Spaces = []string{root}
	withDefault.Space = map[string]config.SpaceSettings{space.Name: {Layout: "plain"}}
	restores := withDefault
	restores.SessionsDir = saved
	noDefault := withDefault
	noDefault.Space = nil
	type result struct {
		Err   string
		Calls []string
	}
	var got []result
	for _, cfg := range []config.Config{withDefault, restores, noDefault} {
		for _, opts := range []spaces.OpenOptions{{}, {NoLayout: true}} {
			calls := fakeKittenWindows(t, wt.Session)
			err := spaces.OpenPath(cfg, wt.Path, opts)
			got = append(got, result{errString(err), calls()})
		}
	}
	tab := []string{"@", "action", "launch", "--type=tab", "--cwd=" + wt.Path, "--add-to-session=" + wt.Session, "--"}
	goTo := []string{"@", "action", "goto_session", wt.Session + ".kitty-session", "--"}
	plain := result{"<nil>", slices.Concat(tab, goTo)}
	restored := result{"<nil>", []string{"@", "action", "goto_session", filepath.Join(saved, wt.Session+".kitty-session"), "--"}}
	want := []result{
		{"<nil>", slices.Concat(tab, []string{"@", "goto-layout", "--match=window_id:11", "splits", "--",
			"@", "focus-window", "--match", "id:11", "--"}, goTo)},
		plain,
		restored, restored,
		plain, plain,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// A space's own layouts, once its file is trusted, are its alone: its worktrees' layout form lists them
// after the global ones, and another space's does not, nor does its own before the file is trusted.
func TestSpaceFileLayouts(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mine, other := mainRepo(t), mainRepo(t)
	global := testLayouts(t)
	global.Spaces, global.Space = []string{mine, other}, nil
	file := filepath.Join(mine, config.SpaceFileName)
	data := []byte("[[commands]]\nname = \"test\"\nrun = [\"make\", \"test\"]\n\n[[layouts]]\nname = \"tests\"\ntabs = [{ panes = [{ command = \"test\" }, { command = \"review\", split = \"right\" }] }]\n")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	list := func(root string) []string {
		t.Helper()
		cfg, space, wt, _, err := findLayout(global, root, "plain", nil)
		if err != nil {
			t.Fatal(err)
		}
		f := newLayoutForm(cfg, space, wt, kitty.State{}, "plain", nil)
		f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
		f.Update(keyMsg("enter"))
		return formScreen(f)
	}
	screen := func(root string, rows ...string) []string {
		lines := append([]string{"", "  Open a layout in " + filepath.Base(root), "", "  layout     < plain >  left/right change  enter list", listRule(""),
			listRow(true, "plain  1 tab, 1 pane"), listRow(false, "agent  1 tab, 3 panes")}, rows...)
		return append(lines, listRule(""), "  up/down move  type filter  enter choose  esc close", "")
	}
	if got, want := list(mine), screen(mine); !reflect.DeepEqual(got, want) {
		t.Errorf("untrusted:\n%q\nwant\n%q", got, want)
	}
	if err := spaces.Trust(file, data); err != nil {
		t.Fatal(err)
	}
	if got, want := list(mine), screen(mine, listRow(false, "tests  1 tab, 2 panes")); !reflect.DeepEqual(got, want) {
		t.Errorf("trusted:\n%q\nwant\n%q", got, want)
	}
	if got, want := list(other), screen(other); !reflect.DeepEqual(got, want) {
		t.Errorf("the other space:\n%q\nwant\n%q", got, want)
	}
	if _, _, _, _, err := findLayout(global, other, "tests", nil); errString(err) != "no layout tests in the config" {
		t.Errorf("tests in the other space: %v", err)
	}
}
