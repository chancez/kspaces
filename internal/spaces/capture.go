package spaces

import (
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// Saving a session as a layout reads its tabs from kitty and writes each as a layout's tab that builds
// back the same arrangement: the splits tree as panes split off one another, or in another layout the
// windows in order. What runs in each pane is detected for the save form to offer, which chooses what
// the layout runs there.

// CapturedTab is a tab of a session as kitty has it, as a layout's tab: its kitty layout, its title, and
// its panes, where each goes, its cwd and which has focus, each running the shell until chosen otherwise.
type CapturedTab struct {
	Tab config.LayoutTab
	// What runs in each pane, by its index: a command line as a shell reads it, empty for the shell at its
	// prompt.
	Programs []string
}

// CaptureSession is wt's session's tabs, and the index among them of this tab: the one window self is
// in, when it is one of them, such as the sidebar's tab for a form for it, else the one the session
// focused most recently. A session with no windows is an error.
func CaptureSession(cfg config.Config, wt Worktree, self int) ([]CapturedTab, int, error) {
	all, err := kitty.ListTabs()
	if err != nil {
		return nil, 0, err
	}
	tabs, this := sessionTabs(all, wt.Session, self)
	if len(tabs) == 0 {
		return nil, 0, fmt.Errorf("%s has no windows to save", wt.Session)
	}
	shows := providerShows(cfg.Provider, all)
	out := make([]CapturedTab, len(tabs))
	for i, tab := range tabs {
		out[i] = captureTab(tab, wt, shows)
	}
	return out, this, nil
}

// sessionTabs are the tabs of tabs with a window of session's, not counting kspaces' own, and the index
// among them of the one self is in, else of the one with the session's most recently focused window.
func sessionTabs(tabs []kitty.FullTab, session string, self int) ([]kitty.FullTab, int) {
	var out []kitty.FullTab
	this, recent, at := -1, 0, -1.0
	for _, tab := range tabs {
		mine := false
		for _, w := range tab.Windows {
			if w.SessionName == session && !w.Own() {
				mine = true
				if w.LastFocusedAt > at {
					recent, at = len(out), w.LastFocusedAt
				}
			}
		}
		if !mine {
			continue
		}
		if slices.ContainsFunc(tab.Windows, func(w kitty.FullWindow) bool { return w.ID == self }) {
			this = len(out)
		}
		out = append(out, tab)
	}
	if this < 0 {
		this = recent
	}
	return out, this
}

// providerShows are the provider's sessions by the windows that show them, for windows that are its
// clients, such as a multiplexer's attach, whose own program says nothing of what runs. None when the
// provider lists no sessions or cannot now, which saves the layout from what kitty knows alone.
func providerShows(provider []string, tabs []kitty.FullTab) map[int]ProviderSession {
	if !ProviderCapabilities(provider)[CapList] {
		return nil
	}
	st, err := listSessions(provider, fullWindows(tabs))
	if err != nil {
		logs.Error("listing the session provider's sessions", err)
		return nil
	}
	shows := map[int]ProviderSession{}
	for _, s := range st.Sessions {
		for _, id := range s.Windows {
			shows[id] = s
		}
	}
	return shows
}

// captureTab is tab as a layout's tab in wt, with the provider's sessions by the windows that show them.
func captureTab(tab kitty.FullTab, wt Worktree, shows map[int]ProviderSession) CapturedTab {
	windows := map[int]kitty.FullWindow{}
	for _, w := range tab.Windows {
		windows[w.ID] = w
	}
	// Each group's window is the first that is not kspaces' own: an overlay comes after the window it is
	// over. A group of only kspaces' own, a form or a picker, is no pane.
	groups := map[int]kitty.FullWindow{}
	var order []int
	for _, g := range tab.LayoutState.AllWindows.WindowGroups {
		for _, id := range g.WindowIDs {
			if w, ok := windows[id]; ok && !w.Own() {
				groups[g.ID] = w
				order = append(order, g.ID)
				break
			}
		}
	}
	var out CapturedTab
	var panes []int
	if tab.Layout == config.SplitsLayout && tab.LayoutState.Pairs != nil {
		root := unmaximize(tab.LayoutState)
		root = prune(root, func(g int) bool { _, ok := groups[g]; return ok })
		panes, out.Tab.Panes = splitPanes(root)
	} else {
		if tab.Layout != config.SplitsLayout {
			out.Tab.Layout = tab.Layout
		}
		panes = order
		out.Tab.Panes = make([]config.Pane, len(order))
	}
	if tab.TitleOverridden {
		out.Tab.Title = escapeBraces(tab.Title)
		if wt.Branch != "" {
			out.Tab.Title = strings.ReplaceAll(out.Tab.Title, escapeBraces(wt.Branch), "{branch}")
		}
	}
	if ag := tab.LayoutState.AllWindows.ActiveGroupIdx; ag >= 0 && ag < len(tab.LayoutState.AllWindows.WindowGroups) {
		if i := slices.Index(panes, tab.LayoutState.AllWindows.WindowGroups[ag].ID); i > 0 {
			out.Tab.Panes[i].Focus = true
		}
	}
	// kitty reports a cwd with symlinks resolved, as /private/tmp for /tmp on macOS.
	roots := []string{wt.Path}
	if real, err := filepath.EvalSymlinks(wt.Path); err == nil && real != wt.Path {
		roots = append(roots, real)
	}
	out.Programs = make([]string, len(panes))
	for i, g := range panes {
		var shown *ProviderSession
		if s, ok := shows[groups[g].ID]; ok {
			shown = &s
		}
		prog, cwd := detect(groups[g], shown)
		out.Programs[i] = prog
		out.Tab.Panes[i].Cwd = cwdTemplate(cwd, roots...)
	}
	return out
}

// unmaximize is st's splits tree with the biases a maximized window's splits had put back, as kitty puts
// them back when it is unmaximized, so the tree is the arrangement rather than one window filling the tab.
func unmaximize(st kitty.LayoutState) *kitty.Split {
	root := clone(st.Pairs)
	for _, m := range st.Maximized {
		for _, sb := range m.SavedBiases {
			n := root
			for _, step := range sb.Path {
				switch {
				case n == nil:
				case step == "one":
					n = n.One
				default:
					n = n.Two
				}
			}
			if n != nil && !n.IsGroup() {
				n.Bias = sb.Bias
			}
		}
	}
	return root
}

func clone(s *kitty.Split) *kitty.Split {
	if s == nil {
		return nil
	}
	c := *s
	c.One, c.Two = clone(s.One), clone(s.Two)
	return &c
}

// prune is s without the groups keep says no to, a pair left with one side being that side.
func prune(s *kitty.Split, keep func(group int) bool) *kitty.Split {
	switch {
	case s == nil:
		return nil
	case s.IsGroup():
		if keep(s.Group) {
			return s
		}
		return nil
	}
	one, two := prune(s.One, keep), prune(s.Two, keep)
	switch {
	case one == nil:
		return two
	case two == nil:
		return one
	}
	p := *s
	p.One, p.Two = one, two
	return &p
}

// splitPanes are the panes that build the tree root, top down, and the group each stands for: a pair's
// first window is the first of its One, which the pane for the first window of its Two splits, right of
// it when the pair is side by side, else below, taking 1-bias of the room; then each side is built the same
// from its first window. kitty's --bias on a split sets the pair's bias, so the tree comes back as it was.
func splitPanes(root *kitty.Split) ([]int, []config.Pane) {
	if root == nil {
		return nil, nil
	}
	first := func(s *kitty.Split) int {
		for !s.IsGroup() {
			s = s.One
		}
		return s.Group
	}
	groups, panes := []int{first(root)}, []config.Pane{{}}
	var build func(s *kitty.Split, at int)
	build = func(s *kitty.Split, at int) {
		if s.IsGroup() {
			return
		}
		split := "below"
		if s.Horizontal {
			split = "right"
		}
		size := min(max(int(math.Round((1-s.Bias)*100)), 1), 99)
		groups = append(groups, first(s.Two))
		panes = append(panes, config.Pane{Split: split, Of: at + 1, Size: size})
		two := len(panes) - 1
		build(s.One, at)
		build(s.Two, two)
	}
	build(root, 0)
	return groups, panes
}

// shells are the programs a pane runs when it runs nothing in particular.
var shells = []string{"sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "tcsh", "csh", "nu", "xonsh", "elvish", "pwsh"}

func isShell(argv []string) bool {
	if len(argv) == 0 {
		return true
	}
	name := strings.TrimPrefix(filepath.Base(argv[0]), "-")
	// kitty's --hold runs a command under kitten run-shell, which starts the shell after it.
	return slices.Contains(shells, name) || name == "kitten" && len(argv) > 1 && argv[1] == "run-shell"
}

// detect is what runs in w, a command line, empty for the shell at its prompt, and where: the command its
// shell reports running, with shell integration; else, for a window that shows one of the provider's
// sessions, shown, what the provider says it runs, and where; else the program in its foreground, when
// that is not a shell.
func detect(w kitty.FullWindow, shown *ProviderSession) (string, string) {
	cwd := w.Cwd
	if shown != nil {
		cwd = cmp.Or(shown.Cwd, cwd)
		if !w.AtPrompt && w.LastReportedCmdline != "" {
			return w.LastReportedCmdline, cwd
		}
		return shown.Command, cwd
	}
	if !w.AtPrompt && w.LastReportedCmdline != "" {
		return w.LastReportedCmdline, cwd
	}
	for _, p := range w.ForegroundProcesses {
		if !isShell(p.Cmdline) {
			return JoinWords(p.Cmdline), cwd
		}
	}
	return "", cwd
}

// cwdTemplate is cwd as a pane's cwd template: empty for the worktree, the default, {worktree}/SUB inside
// it, else the path. The worktree is any of worktrees, its path and the same with symlinks resolved.
func cwdTemplate(cwd string, worktrees ...string) string {
	for _, wt := range worktrees {
		wt = strings.TrimSuffix(wt, "/")
		switch {
		case cwd == "" || cwd == wt:
			return ""
		case wt != "" && strings.HasPrefix(cwd, wt+"/"):
			return "{worktree}/" + escapeBraces(cwd[len(wt)+1:])
		}
	}
	return escapeBraces(cwd)
}

// escapeBraces is s as template text: its braces doubled.
func escapeBraces(s string) string {
	return strings.NewReplacer("{", "{{", "}", "}}").Replace(s)
}
