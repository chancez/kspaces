package spaces

import (
	"fmt"
	"slices"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// LayoutRun is a configured layout to open in a worktree.
type LayoutRun struct {
	Layout config.Layout
	// The inputs' values by name, as config.Layout.Values gives them.
	Inputs map[string]string
	// The worktree's PR, 0 when not known.
	PR int
}

// NewLayoutRun is l to open with inputs, the defaults filled in. A required input left empty, or one l
// does not have, is an error.
func NewLayoutRun(l config.Layout, inputs map[string]string) (LayoutRun, error) {
	values, err := l.Values(inputs)
	if err != nil {
		return LayoutRun{}, err
	}
	return LayoutRun{Layout: l, Inputs: values}, nil
}

// locations are the launch --location for each of config.Splits: kitty's splits layout puts the new
// window after the one split, to the right for vsplit and below for hsplit.
var locations = map[string]string{"right": "vsplit", "below": "hsplit"}

// layoutKitty is what building a layout asks of kitty, a fake in tests.
type layoutKitty interface {
	// Launch runs a launch action and says the id of the window it opened.
	Launch(args []string) (int, error)
	GotoLayout(id int, layout string) error
	FocusWindow(id int) error
	Action(args ...string) error
}

// liveKitty is kitty itself, for the windows of session. An action says nothing back, and a kitten that
// stands in for launch need not either, so the window a launch opened is the one of the session that was
// not there before it.
type liveKitty struct {
	session string
	known   map[int]bool
}

func (k *liveKitty) windows() ([]int, error) {
	data, err := kitty.ListWindows()
	if err != nil {
		return nil, err
	}
	return kitty.SessionWindows(data, k.session), nil
}

func (k *liveKitty) Launch(args []string) (int, error) {
	if k.known == nil {
		ids, err := k.windows()
		if err != nil {
			return 0, err
		}
		k.known = map[int]bool{}
		for _, id := range ids {
			k.known[id] = true
		}
	}
	if err := kitty.Action(args...); err != nil {
		return 0, err
	}
	ids, err := k.windows()
	if err != nil {
		return 0, err
	}
	var opened []int
	for _, id := range ids {
		if !k.known[id] {
			opened = append(opened, id)
			k.known[id] = true
		}
	}
	if len(opened) != 1 {
		return 0, fmt.Errorf("launching a pane opened %d windows in %s, not one", len(opened), k.session)
	}
	return opened[0], nil
}

func (*liveKitty) GotoLayout(id int, layout string) error { return kitty.GotoLayout(id, layout) }
func (*liveKitty) FocusWindow(id int) error               { return kitty.FocusWindow(id) }
func (*liveKitty) Action(args ...string) error            { return kitty.Action(args...) }

// OpenLayout adds lr's tabs to wt's session, a worktree of space, and switches to it, see buildLayout; st
// says which sessions have windows. lr's PR is looked up with WorktreePR when not given.
func OpenLayout(cfg config.Config, st kitty.State, space Space, wt Worktree, lr LayoutRun) error {
	if lr.PR == 0 {
		lr.PR = WorktreePR(space, wt)
	}
	_, err := buildLayout(cfg, st, space, wt, lr, &liveKitty{session: wt.Session})
	return err
}

// buildLayout opens lr's tabs in wt's session with k, and says the window it leaves focused. Each tab is
// a launch action of its first pane with --type=tab, set to its kitty layout, then one of each other pane
// with --type=window: in splits the --location for its side and --bias for its size, on the window of
// the pane it splits; in another layout --location=after, on the pane before it, so they keep their
// order. The action runs on that window, which a kitten acts on too, and --next-to names it. Each pane has
// its --cwd, and one running a command its --env options and --hold, as a command's tab does. In a
// session with windows the tabs go by the window switching to it focuses, as a command's do; in one with
// none the first opens as OpenWorktree's first tab would, and the rest by its window. Each tab's focus
// pane is focused, the first tab's last, so the session is left on it.
func buildLayout(cfg config.Config, st kitty.State, space Space, wt Worktree, lr LayoutRun, k layoutKitty) (int, error) {
	vars := commandVars(space, wt, Run{Inputs: lr.Inputs, PR: lr.PR})
	goTo := []string{"goto_session", SessionFile(cfg, wt.Session)}
	anchor, open := st.Sessions[wt.Session]
	if open {
		if err := k.Action(goTo...); err != nil {
			return 0, err
		}
	}
	var focus []int
	for t, tab := range lr.Layout.Tabs {
		layout := tab.KittyLayout()
		ids := make([]int, len(tab.Panes))
		for i, p := range tab.Panes {
			var args []string
			at := []string{kitty.Quote("--cwd=" + p.CwdWith(vars)), "--add-to-session=" + wt.Session}
			switch {
			case i == 0:
				if anchor != 0 {
					args = append(args, fmt.Sprintf("--match=id:%d", anchor))
				}
				args = append(args, cfg.Launch...)
				args = append(append(args, "--type=tab"), at...)
				if tab.Title != "" {
					args = append(args, kitty.Quote("--tab-title="+tab.TitleWith(vars)))
				}
			case layout == config.SplitsLayout:
				of := ids[p.Of-1]
				args = append(args, fmt.Sprintf("--match=id:%d", of))
				args = append(args, cfg.Launch...)
				args = append(args, "--type=window", "--location="+locations[p.Split], fmt.Sprintf("--next-to=id:%d", of),
					fmt.Sprintf("--bias=%d", p.Size))
				args = append(args, at...)
			default:
				prev := ids[i-1]
				args = append(args, fmt.Sprintf("--match=id:%d", prev))
				args = append(args, cfg.Launch...)
				args = append(args, "--type=window", "--location=after", fmt.Sprintf("--next-to=id:%d", prev))
				args = append(args, at...)
			}
			if cmd, ok := cfg.PaneCommand(lr.Layout, p); ok {
				env, argv := commandWords(cmd, vars)
				args = append(append(append(args, env...), "--hold"), argv...)
			}
			id, err := k.Launch(args)
			if err != nil {
				return 0, err
			}
			ids[i] = id
			if i > 0 {
				continue
			}
			if anchor == 0 {
				anchor = id
			}
			if err := k.GotoLayout(id, layout); err != nil {
				return 0, fmt.Errorf("tab %d: setting kitty's %s layout, which has to be in enabled_layouts: %w", t+1, layout, err)
			}
		}
		focus = append(focus, ids[max(slices.IndexFunc(tab.Panes, func(p config.Pane) bool { return p.Focus }), 0)])
	}
	for _, id := range slices.Backward(focus) {
		if err := k.FocusWindow(id); err != nil {
			return 0, err
		}
	}
	if !open {
		// The tabs are the session's only windows, so going to it only records the switch, see gotoSession.
		if err := k.Action(goTo...); err != nil {
			return 0, err
		}
	}
	return focus[0], nil
}

// freshLayout is the layout wt's session opens with afresh, its default, see config.LayoutFor, for a
// worktree of a configured space, with that space, the worktree as git lists it and the config as the
// space sees it; ok false for none. A layout that needs an input it has no default for is not one to open
// without asking: err says so.
func freshLayout(global config.Config, wt Worktree) (cfg config.Config, space Space, found Worktree, lr LayoutRun, ok bool, err error) {
	if wt.Path == "" {
		return cfg, Space{}, Worktree{}, LayoutRun{}, false, nil
	}
	space, found, err = FindWorktree(global, wt.Path)
	if err != nil {
		// Not a worktree git lists, such as one being removed: the shell, as before layouts.
		return cfg, Space{}, Worktree{}, LayoutRun{}, false, nil
	}
	cfg = space.ConfigOf(global)
	name := cfg.LayoutFor(space.Name, space.Worktrees[0].Path != found.Path)
	l, ok := cfg.Layout(name)
	if !ok {
		return cfg, Space{}, Worktree{}, LayoutRun{}, false, nil
	}
	lr, err = NewLayoutRun(l, nil)
	if err != nil {
		return cfg, Space{}, Worktree{}, LayoutRun{}, false, err
	}
	return cfg, space, found, lr, true, nil
}

// OpenNew opens the session of wt, a worktree of space just created, so its session has no windows:
// with lr's tabs when lr is not nil, else with one tab, then runs r in it when r is not nil, by the
// window the layout left focused, else as its first tab. No layout is a shell, whatever the default.
func OpenNew(cfg config.Config, space Space, wt Worktree, lr *LayoutRun, r *Run) error {
	if lr == nil {
		if r == nil {
			for _, args := range [][]string{firstTab(cfg, wt.Path, wt.Session), {"goto_session", SessionFile(cfg, wt.Session)}} {
				if err := kitty.Action(args...); err != nil {
					return err
				}
			}
			return nil
		}
		return RunCommand(cfg, kitty.State{}, space, wt, *r)
	}
	if lr.PR == 0 {
		lr.PR = WorktreePR(space, wt)
	}
	focus, err := buildLayout(cfg, kitty.State{}, space, wt, *lr, &liveKitty{session: wt.Session})
	if err != nil || r == nil {
		return err
	}
	return RunCommand(cfg, kitty.State{Sessions: map[string]int{wt.Session: focus}}, space, wt, *r)
}
