package spaces

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// Run is a configured command to run in a worktree.
type Run struct {
	Command config.Command
	// The inputs' values by name, as config.Command.Values gives them.
	Inputs map[string]string
	// Where it opens in a session with windows, one of config.Opens.
	Open string
	// The worktree's PR, 0 when not known.
	PR int
}

// NewRun is cmd to run with inputs, the defaults filled in, where it is configured to open. A required
// input left empty, or one cmd does not have, is an error.
func NewRun(cmd config.Command, inputs map[string]string) (Run, error) {
	values, err := cmd.Values(inputs)
	if err != nil {
		return Run{}, err
	}
	return Run{Command: cmd, Inputs: values, Open: cmd.Open}, nil
}

// WorktreePR is the PR whose head wt's branch pulls from, as CreatePRWorktree sets up a PR from a fork,
// or 0. Only what git config says, so a PR from the repository itself is not known.
func WorktreePR(space Space, wt Worktree) int {
	if wt.Branch == "" {
		return 0
	}
	merge := git.BranchMerge(space.Root, wt.Branch)
	n, ok := strings.CutPrefix(merge, "refs/pull/")
	if !ok {
		return 0
	}
	n, ok = strings.CutSuffix(n, "/head")
	if !ok {
		return 0
	}
	pr, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	return pr
}

// RunCommand runs r in wt, a worktree of space, and switches to its session, see runActions; st says
// which sessions have windows. r's PR is looked up with WorktreePR when not given.
func RunCommand(cfg config.Config, st kitty.State, space Space, wt Worktree, r Run) error {
	if r.PR == 0 {
		r.PR = WorktreePR(space, wt)
	}
	for _, args := range runActions(cfg, st, space, wt, r) {
		if err := kitty.Action(args...); err != nil {
			return err
		}
	}
	return nil
}

// commandVars are the values of the placeholders of a command run in wt: the built-ins, then the inputs.
func commandVars(space Space, wt Worktree, r Run) map[string]string {
	vars := map[string]string{
		"space":    space.Name,
		"root":     space.Root,
		"worktree": wt.Path,
		"branch":   wt.Branch,
		"session":  wt.Session,
		"base":     "",
		"pr":       "",
	}
	if len(space.Worktrees) > 0 {
		vars["base"] = space.Worktrees[0].Branch
	}
	if r.PR > 0 {
		vars["pr"] = strconv.Itoa(r.PR)
	}
	for k, v := range r.Inputs {
		vars[k] = v
	}
	return vars
}

// runActions are the kitty actions that run r in wt and switch to wt's session. The launch action opens
// it, with options for where, --cwd and --add-to-session, the --env options, --hold, then the command,
// each word quoted for kitty to pass on as it is. A session with no windows gets it as its first tab, as
// OpenWorktree would open one. In a session with windows it opens where r.Open says, by the session's
// window that goto_session focuses: kitty's action runs on that window, which launch acts next to, through
// a kitten as well as plain launch, and --next-to names it for a split or an overlay. --hold has kitty
// start the shell once the command exits, so the window stays usable; not for an overlay, which closes
// back to the window it was over.
func runActions(cfg config.Config, st kitty.State, space Space, wt Worktree, r Run) [][]string {
	vars := commandVars(space, wt, r)
	var env, argv []string
	for _, e := range r.Command.Environ(vars) {
		// launch expands $VAR in --env values itself, after kitty has in the action's words.
		env = append(env, kitty.Quote("--env="+strings.ReplaceAll(e, "$", "$$")))
	}
	for _, arg := range r.Command.Argv(vars) {
		argv = append(argv, kitty.Quote(arg))
	}
	goTo := []string{"goto_session", SessionFile(cfg, wt.Session)}
	id, open := st.Sessions[wt.Session]
	if !open {
		launch := append(firstTab(cfg, wt.Path, wt.Session), env...)
		return [][]string{append(append(launch, "--hold"), argv...), goTo}
	}
	next := fmt.Sprintf("--next-to=id:%d", id)
	launch := append([]string{fmt.Sprintf("--match=id:%d", id)}, cfg.Launch...)
	switch r.Open {
	case "split", "hsplit", "vsplit":
		launch = append(launch, "--type=window", "--location="+r.Open, next)
	case "overlay":
		launch = append(launch, "--type=overlay", next)
	case "window":
		launch = append(launch, "--type=os-window")
	default:
		launch = append(launch, "--type=tab")
	}
	launch = append(launch, "--cwd="+wt.Path, "--add-to-session="+wt.Session)
	launch = append(launch, env...)
	if r.Open != "overlay" {
		launch = append(launch, "--hold")
	}
	return [][]string{goTo, append(launch, argv...)}
}
