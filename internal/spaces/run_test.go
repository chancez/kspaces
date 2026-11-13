package spaces

import (
	"reflect"
	"slices"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A command opens as the first tab of a session with no windows, wherever it is set to open. In a session
// with windows, it goes to the session, then opens where it is set to, on the session's window 7: the
// action runs on it, and a split or an overlay is next to it. Each word is quoted for kitty, and the
// --env values' $ doubled once more for launch. Through a kitten the same, after its own words.
func TestRunActions(t *testing.T) {
	space := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}}}
	wt := Worktree{Path: "/src/gr/.worktrees/fix", Branch: "pr/me/fix", Session: "gr@fix"}
	cmd := config.Command{
		Name:   "claude",
		Run:    []string{"claude", "--name={space}", "{prompt}"},
		Env:    map[string]string{"TASK_PR": "#{pr}"},
		Inputs: []config.Input{{Name: "prompt", Label: "prompt"}},
	}
	env := []string{
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=/src/gr", "--env=KSPACES_WORKTREE=/src/gr/.worktrees/fix",
		"--env=KSPACES_BRANCH=pr/me/fix", "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=12",
		`'--env=KSPACES_INPUT_PROMPT=fix it'\''s $$$$HOME'`, "--env=TASK_PR=#12",
	}
	argv := []string{"claude", "--name=gr", `'fix it'\''s $$HOME'`}
	at := []string{"--cwd=/src/gr/.worktrees/fix", "--add-to-session=gr@fix"}
	goTo := []string{"goto_session", "gr@fix.kitty-session"}
	// launch is a launch action: the launcher, the options, at, env, maybe --hold, then argv.
	launch := func(launcher []string, hold bool, opts ...string) []string {
		out := slices.Concat(launcher, opts, at, env)
		if hold {
			out = append(out, "--hold")
		}
		return append(out, argv...)
	}
	type result struct{ Open, Closed [][]string }
	for _, launcher := range [][]string{{"launch"}, {"kitten", "my_launch.py"}} {
		cfg := config.Config{Launch: launcher}
		on7 := slices.Concat([]string{"--match=id:7"}, launcher)
		first := [][]string{launch(launcher, true, "--type=tab"), goTo}
		want := map[string]result{
			"tab":     {[][]string{goTo, launch(on7, true, "--type=tab")}, first},
			"split":   {[][]string{goTo, launch(on7, true, "--type=window", "--location=split", "--next-to=id:7")}, first},
			"hsplit":  {[][]string{goTo, launch(on7, true, "--type=window", "--location=hsplit", "--next-to=id:7")}, first},
			"vsplit":  {[][]string{goTo, launch(on7, true, "--type=window", "--location=vsplit", "--next-to=id:7")}, first},
			"overlay": {[][]string{goTo, launch(on7, false, "--type=overlay", "--next-to=id:7")}, first},
			"window":  {[][]string{goTo, launch(on7, true, "--type=os-window")}, first},
		}
		got := map[string]result{}
		for _, open := range config.Opens {
			r := Run{Command: cmd, Inputs: map[string]string{"prompt": "fix it's $HOME"}, Open: open, PR: 12}
			got[open] = result{
				Open:   runActions(cfg, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, space, wt, r),
				Closed: runActions(cfg, kitty.State{}, space, wt, r),
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\ngot  %q\nwant %q", launcher[0], got, want)
		}
	}

	// Staying, there is no switch, and the launch keeps the focus.
	r := Run{Command: cmd, Inputs: map[string]string{"prompt": "fix it's $HOME"}, Open: "split", PR: 12, Stay: true}
	cfg := config.Config{Launch: []string{"launch"}}
	got := result{
		Open:   runActions(cfg, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, space, wt, r),
		Closed: runActions(cfg, kitty.State{}, space, wt, r),
	}
	at = append(at, "--keep-focus")
	want := result{
		Open:   [][]string{launch([]string{"--match=id:7", "launch"}, true, "--type=window", "--location=split", "--next-to=id:7")},
		Closed: [][]string{launch([]string{"launch"}, true, "--type=tab")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("staying:\ngot  %q\nwant %q", got, want)
	}
}

// A worktree's PR is known from a branch that pulls from a PR's head, as one from a fork is set up.
func TestWorktreePR(t *testing.T) {
	r, _ := prRepos(t)
	r.Git("branch", "-q", "fork")
	r.Git("config", "branch.fork.merge", "refs/pull/7/head")
	space := Space{Name: "space", Root: r.Dir}
	var got []int
	for _, branch := range []string{"fork", "main", "123", ""} {
		got = append(got, WorktreePR(space, Worktree{Branch: branch}))
	}
	if want := []int{7, 0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
