package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
)

// testCommands are a command with a required input and one with none.
func testCommands(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Parse(`
[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
[[commands.inputs]]
name = "prompt"
label = "Prompt"
required = true
[[commands]]
name = "gr"
run = ["gr"]
open = "split"
`)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// fakeKittenLs puts a kitten on PATH that answers `kitten @ ls` with ls and records every other call, its
// arguments one a line and -- after each, and returns those calls.
func fakeKittenLs(t *testing.T, ls string) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	if err := os.WriteFile(filepath.Join(dir, "ls.json"), []byte(ls), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$2\" = ls ] && exec cat " + filepath.Join(dir, "ls.json") + "\n" +
		"{ for a; do printf '%s\\n' \"$a\"; done; echo --; } >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// lsWith is `kitten @ ls` with one window, 7, in session.
func lsWith(session string) string {
	return fmt.Sprintf(`[{"is_focused":true,"tabs":[{"id":1,"is_active":true,"windows":[{"id":7,"is_active":true,"session_name":%q,"last_focused_at":1}]}]}]`, session)
}

// Run is on a worktree's menu, and R its key, only with commands configured; it opens the run form in
// an overlay for the first command.
func TestRunKey(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	e.m.cursor = e.row("gr@dev")
	if _, ok := runOn(e.m, keyMsg("R"), e.m.chain()...); ok {
		t.Error("R ran something with no commands")
	}
	snap := fixture(time.Now())
	cfg := testCommands(t)
	snap.Config = &cfg
	e.m.Update(snap)
	e.m.render()
	dev := e.row("gr@dev")
	e.click(fmt.Sprintf(entryZone, dev), tea.MouseRight)
	_, y := e.zone(fmt.Sprintf(entryZone, dev))
	want := []string{
		menuRule("Session"), " Open", " Run", " Close session",
		menuRule("Worktree"), " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Add space",
		menuRule(""),
	}
	if got := e.screen()[y+1 : y+12]; !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n%q\nwant\n%q", got, want)
	}
	e.press("esc")

	e.m.cursor = dev
	cmd, ok := runOn(e.m, keyMsg("R"), e.m.chain()...)
	if !ok || cmd == nil {
		t.Fatal("R ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("R said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "run", "claude", "/src/gr/.worktrees/dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("R launched %q, want %q", got, want)
	}
}

// The run form chooses between the commands, with a field for each input, and says where the command
// opens: where configured, or as --open says, or as the first tab of a session with no windows. A required
// input left empty refuses to run. Run goes to the session and opens the command by its window.
func TestRunForm(t *testing.T) {
	// Running looks up the worktree's PR with git, in a repository that is not there.
	gittest.Isolate(t)
	calls := fakeKittenLs(t, lsWith("gr@fix"))
	cfg := testCommands(t)
	cfg.Launch = []string{"launch"}
	space := spaces.Space{Name: "gr", Root: "/src/gr", Worktrees: []spaces.Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := spaces.Worktree{Path: "/src/gr/.worktrees/fix", Branch: "fix", Session: "gr@fix"}
	open := kitty.State{Sessions: map[string]int{"gr@fix": 7}}
	screen := func(fields []string, says ...string) []string {
		lines := append([]string{"", "  Run in gr@fix", ""}, fields...)
		lines = append(lines, "")
		for _, s := range says {
			if s != "" {
				s = "  " + s
			}
			lines = append(lines, s)
		}
		return lines
	}
	sized := func(f *runForm) *runForm {
		f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
		return f
	}
	f := sized(newRunForm(cfg, space, wt, open, "gr", nil, ""))
	want := screen([]string{"  command    < gr >"}, "Runs gr", "Opens in a split beside gr@fix's window", "",
		" Run enter    Cancel esc   left/right command", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("gr:\n%q\nwant\n%q", got, want)
	}
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	f.Update(keyMsg("enter"))
	want = screen([]string{"  command    < claude >", "  Prompt     required"}, "Runs claude", "Opens in a new tab in gr@fix", "",
		"claude needs Prompt", "", " Run enter    Cancel esc   tab next field  left/right command", "")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("claude, its prompt empty:\n%q\nwant\n%q", got, want)
	}

	// Prefilled, for one run in an overlay, and with the session closed.
	if got, want := formScreen(sized(newRunForm(cfg, space, wt, open, "claude", map[string]string{"prompt": "hi"}, "overlay")))[3:8],
		[]string{"  command    < claude >", "  Prompt     hi", "", "  Runs claude", "  Opens over gr@fix's window, closing back to it"}; !reflect.DeepEqual(got, want) {
		t.Errorf("prefilled:\n%q\nwant\n%q", got, want)
	}
	if got, want := formScreen(sized(newRunForm(cfg, space, wt, kitty.State{}, "gr", nil, "")))[6], "  Opens as the first tab of gr@fix, which has no windows"; got != want {
		t.Errorf("closed: %q, want %q", got, want)
	}
	// With one command there is nothing to choose.
	one := cfg
	one.Commands = cfg.Commands[1:]
	if got, want := formScreen(sized(newRunForm(one, space, wt, open, "gr", nil, "")))[1:6], []string{"  Run in gr@fix", "", "  Runs gr", "  Opens in a split beside gr@fix's window", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("one command:\n%q\nwant\n%q", got, want)
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
	if got, want := calls(), []string{
		"@", "action", "goto_session", "gr@fix.kitty-session", "--",
		"@", "action", "--match=id:7", "launch", "--type=tab", "--cwd=/src/gr/.worktrees/fix", "--add-to-session=gr@fix",
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=/src/gr", "--env=KSPACES_WORKTREE=/src/gr/.worktrees/fix",
		"--env=KSPACES_BRANCH=fix", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=",
		"'--env=KSPACES_INPUT_PROMPT=fix it'", "--hold", "claude", "'fix it'", "--",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// kspaces run --yes runs the command in the worktree a directory is in, refusing a required input not
// given, an input the command does not have and an --open that is no place.
func TestRunCommandNow(t *testing.T) {
	root := mainRepo(t)
	cfg := testCommands(t)
	cfg.Spaces = []string{root}
	space := spaces.LoadSpace(root)
	wt, err := spaces.CreateWorktree(space, "fix", filepath.Join(root, ".worktrees", "fix"))
	if err != nil {
		t.Fatal(err)
	}
	calls := fakeKittenLs(t, lsWith(wt.Session))
	sub := filepath.Join(wt.Path, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range []struct {
		name   string
		inputs map[string]string
		open   string
	}{
		{"claude", nil, ""},
		{"claude", map[string]string{"model": "opus"}, ""},
		{"gr", nil, "sideways"},
		{"nope", nil, ""},
		{"gr", nil, "overlay"},
	} {
		got = append(got, errString(RunCommandNow(cfg, sub, c.name, c.inputs, c.open)))
	}
	want := []string{
		"claude needs Prompt",
		"claude has no input model",
		`--open is "sideways", it must be one of tab, split, hsplit, vsplit, overlay, window`,
		"no command nope in the config",
		"<nil>",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if got, want := calls(), []string{
		"@", "action", "goto_session", wt.Session + ".kitty-session", "--",
		"@", "action", "--match=id:7", "launch", "--type=overlay", "--next-to=id:7", "--cwd=" + wt.Path, "--add-to-session=" + wt.Session,
		"--env=KSPACES_SPACE=" + filepath.Base(root), "--env=KSPACES_ROOT=" + root, "--env=KSPACES_WORKTREE=" + wt.Path,
		"--env=KSPACES_BRANCH=fix", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=" + wt.Session, "--env=KSPACES_PR=",
		"gr", "--",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
