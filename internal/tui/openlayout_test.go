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
	agent := []string{"  layout     < agent >", "  Prompt     required"}
	want := screen(agent, "A tab of claude, review, a shell", "Adds its tabs to gr@fix", "",
		" Open enter    Cancel esc   tab next field  left/right layout", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("enter"))
	want = screen(agent, "A tab of claude, review, a shell", "Adds its tabs to gr@fix", "",
		"agent needs Prompt", "", " Open enter    Cancel esc   tab next field  left/right layout", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent, its prompt empty:\n%q\nwant\n%q", got, want)
	}
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	f.err = nil
	want = screen([]string{"  layout     < plain >"}, "A tab of a shell", "Adds its tabs to gr@fix", "",
		" Open enter    Cancel esc   left/right layout", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("plain:\n%q\nwant\n%q", got, want)
	}
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	// Prefilled, and with the session closed.
	if got, want := formScreen(sized(newLayoutForm(cfg, space, wt, kitty.State{}, "agent", map[string]string{"prompt": "hi"})))[3:9],
		[]string{"  layout     < agent >", "  Prompt     hi", "", "  A tab of claude, review, a shell", "  Opens gr@fix with its tabs, as it has no windows", ""}; !reflect.DeepEqual(got, want) {
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
	screen := func(fields []string, hint string, says ...string) []string {
		lines := append([]string{"", "  New worktree in " + space.Name, "", "  branch     fix", "  directory  .worktrees/fix"}, fields...)
		lines = append(lines, "", "  Creates fix from main")
		for _, s := range says {
			lines = append(lines, "  "+s)
		}
		return append(lines, "", "   Create enter    Cancel esc   tab next field"+hint, "")
	}
	typeInto(f, "fix")
	agent := []string{"  layout     < agent >", "  Prompt     required"}
	if got, want := formScreen(f), screen(append(agent, "  command    < none >"), "", "Opens with the agent layout"); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("tab"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got, want := formScreen(f), screen([]string{"  layout     < none >", "  command    < none >"}, "  left/right layout"); !reflect.DeepEqual(got, want) {
		t.Errorf("none:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "< none >")
	clickOn(t, f, "< plain >")
	f.Update(keyMsg("tab"))
	typeInto(f, "fix it")
	f.Update(keyMsg("tab"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	want := screen(append(agent[:1:1], "  Prompt     fix it", "  command    < review >"), "  left/right command",
		"Opens with the agent layout", "Then runs review in a new tab")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("agent and review:\n%q\nwant\n%q", got, want)
	}
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
		err := spaces.OpenPath(cfg, w.Path)
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
