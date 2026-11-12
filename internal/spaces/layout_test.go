package spaces

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/kitty"
)

// fakeLayoutKitty records what building a layout asks of kitty, as the arguments of kitten @, and opens
// windows 11, 12 and on. Its enabled layouts are all of kitty's but those in disabled.
type fakeLayoutKitty struct {
	calls    [][]string
	next     int
	disabled []string
}

func (k *fakeLayoutKitty) Launch(args []string) (int, error) {
	k.calls = append(k.calls, append([]string{"action"}, args...))
	k.next++
	return 10 + k.next, nil
}

func (k *fakeLayoutKitty) GotoLayout(id int, layout string) error {
	k.calls = append(k.calls, []string{"goto-layout", fmt.Sprintf("--match=window_id:%d", id), layout})
	if slices.Contains(k.disabled, layout) {
		return fmt.Errorf("kitten @ goto-layout: The layout %s is unknown or disabled or the name is ambiguous", layout)
	}
	return nil
}

func (k *fakeLayoutKitty) FocusWindow(id int) error {
	k.calls = append(k.calls, []string{"focus-window", "--match", fmt.Sprintf("id:%d", id)})
	return nil
}

func (k *fakeLayoutKitty) Action(args ...string) error {
	k.calls = append(k.calls, append([]string{"action"}, args...))
	return nil
}

// agentConfig has the layout I use: claude, gr on its right at 35%, a shell below claude at 30%, and a
// second tab with a log below a shell, focused.
const agentConfig = `
[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", required = true }]

[[commands]]
name = "review"
run = ["gr"]

[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch}: {prompt}"
panes = [
  { command = "claude" },
  { command = "review", split = "right", size = 35 },
  { split = "below", of = 1, size = 30 },
]
[[layouts.tabs]]
panes = [{}, { run = ["tail", "-f", "{worktree}/log"], split = "below", focus = true }]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]
`

// A layout opens each tab with its first pane, sets it to splits, then splits each other pane off the
// window of the one it names, on its side at its size; commands have their env and --hold. Each tab's
// focus pane is focused, the first tab's last. A session with no windows gets the first tab as its first
// window and the second by it, then the switch; one with windows is switched to first, and gets them by
// its window 7. Through a kitten the same, after its own words.
func TestBuildLayout(t *testing.T) {
	cfg, err := config.Parse(agentConfig)
	if err != nil {
		t.Fatal(err)
	}
	space := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := Worktree{Path: "/src/gr/.worktrees/fix", Branch: "pr/me/fix", Session: "gr@fix"}
	l, _ := cfg.Layout("agent")
	lr, err := NewLayoutRun(l, map[string]string{"prompt": "fix it"})
	if err != nil {
		t.Fatal(err)
	}
	lr.PR = 12
	builtins := []string{
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=/src/gr", "--env=KSPACES_WORKTREE=/src/gr/.worktrees/fix",
		"--env=KSPACES_BRANCH=pr/me/fix", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=12",
	}
	withPrompt := append(slices.Clone(builtins), "'--env=KSPACES_INPUT_PROMPT=fix it'")
	at := []string{"--cwd=/src/gr/.worktrees/fix", "--add-to-session=gr@fix"}
	goTo := []string{"action", "goto_session", "gr@fix.kitty-session"}
	type result struct {
		Calls [][]string
		Focus int
	}
	for _, launcher := range [][]string{{"launch"}, {"kitten", "my_launch.py"}} {
		cfg.Launch = launcher
		// on is an action on window id, or on none for 0, of the launcher with opts, at, then words.
		on := func(id int, opts []string, words ...string) []string {
			return slices.Concat([]string{"action"}, matchOf(id), launcher, opts, at, words)
		}
		split := func(of int, loc string, bias int) []string {
			return []string{"--type=window", "--location=" + loc, fmt.Sprintf("--next-to=id:%d", of), fmt.Sprintf("--bias=%d", bias)}
		}
		// tabs are the calls for both tabs, the first tab's launch on window first, the second's on second.
		tabs := func(first, second int) [][]string {
			return [][]string{
				on(first, []string{"--type=tab"}, slices.Concat([]string{"'--tab-title=pr/me/fix: fix it'"}, withPrompt, []string{"--hold", "claude", "'fix it'"})...),
				{"goto-layout", "--match=window_id:11", "splits"},
				on(11, split(11, "vsplit", 35), slices.Concat(builtins, []string{"--hold", "gr"})...),
				on(11, split(11, "hsplit", 30)),
				on(second, []string{"--type=tab"}),
				{"goto-layout", "--match=window_id:14", "splits"},
				on(14, split(14, "hsplit", 50), slices.Concat(withPrompt, []string{"--hold", "tail", "-f", "/src/gr/.worktrees/fix/log"})...),
				{"focus-window", "--match", "id:15"},
				{"focus-window", "--match", "id:11"},
			}
		}
		k := &fakeLayoutKitty{}
		focus, err := buildLayout(cfg, kitty.State{}, space, wt, lr, k)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]result{"closed": {k.calls, focus}}
		k = &fakeLayoutKitty{}
		focus, err = buildLayout(cfg, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, space, wt, lr, k)
		if err != nil {
			t.Fatal(err)
		}
		got["open"] = result{k.calls, focus}
		want := map[string]result{
			"closed": {append(tabs(0, 11), goTo), 11},
			"open":   {append([][]string{goTo}, tabs(7, 7)...), 11},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\ngot  %v\nwant %v", launcher[0], got, want)
		}
	}
}

// matchOf is the --match option of an action on window id, none for 0.
func matchOf(id int) []string {
	if id == 0 {
		return nil
	}
	return []string{fmt.Sprintf("--match=id:%d", id)}
}

// A tab in a layout other than splits is set to it, and its panes go in order, each after the one before;
// a pane's cwd is a template, quoted for kitty, the worktree by default. A layout kitty has not enabled is
// an error that says it has to be.
func TestBuildLayoutInOrder(t *testing.T) {
	cfg, err := config.Parse(`
[[layouts]]
name = "watch"
[[layouts.tabs]]
layout = "tall"
panes = [
  { cwd = "{worktree}/docs" },
  { run = ["make", "watch"] },
  { cwd = "/tmp/a dir" },
]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	l, _ := cfg.Layout("watch")
	lr, err := NewLayoutRun(l, nil)
	if err != nil {
		t.Fatal(err)
	}
	lr.PR = 12
	env := []string{
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=/src/gr", "--env=KSPACES_WORKTREE=/src/gr",
		"--env=KSPACES_BRANCH=main", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr", "--env=KSPACES_PR=12",
	}
	type result struct {
		Calls [][]string
		Err   string
	}
	var got []result
	for _, disabled := range [][]string{nil, {"tall"}} {
		k := &fakeLayoutKitty{disabled: disabled}
		_, err := buildLayout(cfg, kitty.State{}, space, space.Worktrees[0], lr, k)
		got = append(got, result{k.calls, errString(err)})
	}
	first := []string{"action", "launch", "--type=tab", "--cwd=/src/gr/docs", "--add-to-session=gr"}
	want := []result{
		{[][]string{
			first,
			{"goto-layout", "--match=window_id:11", "tall"},
			slices.Concat([]string{"action", "--match=id:11", "launch", "--type=window", "--location=after", "--next-to=id:11", "--cwd=/src/gr", "--add-to-session=gr"},
				env, []string{"--hold", "make", "watch"}),
			{"action", "--match=id:12", "launch", "--type=window", "--location=after", "--next-to=id:12", "'--cwd=/tmp/a dir'", "--add-to-session=gr"},
			{"focus-window", "--match", "id:11"},
			{"action", "goto_session", "gr.kitty-session"},
		}, "<nil>"},
		{[][]string{first, {"goto-layout", "--match=window_id:11", "tall"}},
			"tab 1: setting kitty's tall layout, which has to be in enabled_layouts: kitten @ goto-layout: The layout tall is unknown or disabled or the name is ambiguous"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// A worktree opens afresh with its space's layout for it, found with git; a layout that needs an input
// with no default is not opened, and says why. A session in no space, or a config with no layouts, has
// none.
func TestFreshLayout(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	linked := r.Worktree("dev", ".worktrees/dev")
	name := filepath.Base(r.Dir)
	cfg, err := config.Parse(fmt.Sprintf("spaces = [%q]\n", r.Dir) + agentConfig +
		fmt.Sprintf("[space.%s]\nlayout = \"plain\"\nworktree_layout = \"agent\"\n", name))
	if err != nil {
		t.Fatal(err)
	}
	space := LoadSpace(r.Dir)
	type result struct {
		Space, Worktree, Layout, Err string
		OK                           bool
	}
	var got []result
	for _, c := range []struct {
		cfg config.Config
		wt  Worktree
	}{
		{cfg, space.Worktrees[0]},
		{cfg, Worktree{Path: linked.Dir, Session: space.Worktrees[1].Session}},
		{cfg, Worktree{Session: "default"}},
		{config.Config{Spaces: cfg.Spaces}, space.Worktrees[0]},
	} {
		s, wt, lr, ok, err := freshLayout(c.cfg, c.wt)
		got = append(got, result{s.Name, wt.Path, lr.Layout.Name, errString(err), ok})
	}
	want := []result{
		{name, r.Dir, "plain", "<nil>", true},
		{"", "", "", "agent needs Prompt", false},
		{"", "", "", "<nil>", false},
		{"", "", "", "<nil>", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
