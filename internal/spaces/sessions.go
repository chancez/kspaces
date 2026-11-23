package spaces

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// The provider's sessions with no window, see provider.go, as the sidebar lists them: by space, then by
// host, opened in a tab of their space, and the provider's spaces that are no configured space's.

// Detached are a space's sessions no kitty window shows, by the host they run on, this machine's first,
// then by name; each host's by name, then id.
type Detached struct {
	Hosts []HostSessions
}

// HostSessions are one host's sessions of a Detached.
type HostSessions struct {
	// Host is empty for this machine.
	Host     string
	Sessions []ProviderSession
}

// Count is how many sessions d has, on every host.
func (d Detached) Count() int {
	n := 0
	for _, h := range d.Hosts {
		n += len(h.Sessions)
	}
	return n
}

// detachedBySpace are the provider's sessions no window shows, by space: those it lists with no windows.
func detachedBySpace(st *ProviderState) map[string]Detached {
	if st == nil {
		return nil
	}
	by := map[string]map[string][]ProviderSession{}
	for _, s := range st.Sessions {
		if len(s.Windows) > 0 {
			continue
		}
		space := s.SpaceName()
		if by[space] == nil {
			by[space] = map[string][]ProviderSession{}
		}
		by[space][s.Host] = append(by[space][s.Host], s)
	}
	out := map[string]Detached{}
	for space, hosts := range by {
		var d Detached
		for _, host := range slices.Sorted(maps.Keys(hosts)) {
			sessions := hosts[host]
			slices.SortFunc(sessions, func(a, b ProviderSession) int {
				return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
			})
			d.Hosts = append(d.Hosts, HostSessions{Host: host, Sessions: sessions})
		}
		out[space] = d
	}
	return out
}

// topicSpaces are the provider's spaces that are no configured space's, each a topic space: a kitty
// session of its name, in no space, see placeSessions. default is one too, kitty's default session.
func topicSpaces(st *ProviderState, spaces []Space) []string {
	if st == nil {
		return nil
	}
	var out []string
	for _, name := range st.Spaces {
		if !slices.ContainsFunc(spaces, func(s Space) bool { return s.Name == name }) {
			out = append(out, name)
		}
	}
	return out
}

// OpenTarget is the worktree of space whose kitty session a new tab opening s, one of the space's sessions
// no window shows, goes in: the current session, when it is the space's; else the linked worktree s's
// worktree tag names, when the space lists it; else the space's main worktree, or a session in no space's
// own.
func OpenTarget(st kitty.State, space Space, s ProviderSession) Worktree {
	wts := append(append([]Worktree{}, space.Worktrees...), space.Missing...)
	for _, wt := range wts {
		if st.Current != "" && wt.Session == st.Current {
			return wt
		}
	}
	// Not a missing one: opening its session with no windows would make it in a directory that is gone.
	if dir := s.Tags["worktree"]; dir != "" {
		for _, wt := range space.Worktrees {
			if wt.Session == space.Name+"@"+dir {
				return wt
			}
		}
	}
	if len(space.Worktrees) > 0 {
		return space.Worktrees[0]
	}
	return Worktree{Session: space.Name}
}

// OpenSession switches to wt's session and opens a tab in it running what the provider says opens s, which
// may bring back one that has ended. The tab goes by the session's window switching to it focuses. A
// session with no windows is restored from its saved file first, if that opens any, and gets the tab after;
// else the tab is its first, without the session's default layout, which is for starting work rather than
// going back to it. When the command ends, as at a detach, the tab closes.
func OpenSession(cfg config.Config, st kitty.State, wt Worktree, s ProviderSession) error {
	argv, cwd, err := openCommand(cfg.Provider, s)
	if err != nil {
		return err
	}
	if st.Home != 0 {
		// As OpenWorktree, so coming back to this tab lands on its own window rather than the sidebar.
		if err := kitty.FocusWindow(st.Home); err != nil {
			return err
		}
	}
	for _, args := range openSessionActions(cfg, st, wt, argv, cmp.Or(cwd, s.Cwd)) {
		if err := kitty.Action(args...); err != nil {
			return fmt.Errorf("opening %s: %w", s.Name, err)
		}
	}
	return nil
}

// openSessionActions are the kitty actions OpenSession runs for argv, starting in cwd, else the
// worktree's or the home directory, so a split from it opens there.
func openSessionActions(cfg config.Config, st kitty.State, wt Worktree, argv []string, cwd string) [][]string {
	goTo := []string{"goto_session", SessionFile(cfg, wt.Session)}
	cwd = cmp.Or(cwd, wt.Path)
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	tab := append(append([]string{}, cfg.Launch...), "--type=tab")
	if cwd != "" {
		tab = append(tab, kitty.Quote("--cwd="+cwd))
	}
	tab = append(tab, inSession(wt.Session)...)
	for _, a := range argv {
		tab = append(tab, kitty.Quote(a))
	}
	anchor, open := st.Sessions[wt.Session]
	switch {
	case open && anchor != 0:
		return [][]string{goTo, append([]string{fmt.Sprintf("--match=id:%d", anchor)}, tab...)}
	case open, !opensAfresh(cfg, st, wt):
		// Restored from its file, the session's window has focus, which launch goes by.
		return [][]string{goTo, tab}
	}
	// The new tab is the session's only window, so going to it only records the switch, see gotoSession.
	return [][]string{tab, goTo}
}

// StateWords says what a session is doing, in a word: busy while a command runs, else its state,
// running, exited, shelved or dead.
func StateWords(s ProviderSession) string {
	if s.State == "running" && s.Busy {
		return "busy"
	}
	return s.State
}

// AskKillProviderSessions opens the form that confirms killing sessions, by id, in a popup sized as
// AskCloseSession's; an ended one is removed instead.
func AskKillProviderSessions(cfg config.Config, st kitty.State, ids ...string) error {
	return launchForm(cfg, st, panelFor(len(ids)), append([]string{"sessions", "kill", "--"}, ids...)...)
}

// AskRemoveProviderSessions opens the form that confirms removing ended sessions, as AskKillProviderSessions's.
func AskRemoveProviderSessions(cfg config.Config, st kitty.State, ids ...string) error {
	return launchForm(cfg, st, panelFor(len(ids)), append([]string{"sessions", "remove", "--"}, ids...)...)
}

// AskCleanupProviderSessions opens the form that kills and removes the sessions of space no window shows, in a
// popup of a list's size, as AskCleanup's.
func AskCleanupProviderSessions(cfg config.Config, st kitty.State, space string) error {
	return launchForm(cfg, st, listPanel, "sessions", "cleanup", space)
}

// AskMoveProviderSessions opens the form that moves sessions to another space, in a popup sized as
// AskOpenLayout's.
func AskMoveProviderSessions(cfg config.Config, st kitty.State, ids ...string) error {
	return launchForm(cfg, st, formPanel, append([]string{"sessions", "move", "--"}, ids...)...)
}
