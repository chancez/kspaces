package spaces

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// A session's space changing moves the windows that show it: a session moved with the provider, as `cm space
// move` does from a shell, belongs to the other space now, and a window left in the old one says otherwise.
// kitty keeps a window in the session it was made in, so moving one is a new tab in the other space's session
// showing the same provider session, opened as a detached row opens, and the old window closed.

// spaceWatch remembers the space of each session a window shows, so a change of it can be told from a
// session that has always been elsewhere. Only changes move a window: one that disagrees with its session's
// space when the sidebar starts is left where it is, so starting cannot rearrange the tabs.
type spaceWatch struct {
	seen map[string]string
}

// changed records the space of each session st lists with windows and returns those whose space is not the
// one it had at the last call. A session that had no windows then is recorded, not moved.
func (w *spaceWatch) changed(st *ProviderState) []ProviderSession {
	next := make(map[string]string, len(st.Sessions))
	var out []ProviderSession
	for _, s := range st.Sessions {
		if len(s.Windows) == 0 {
			continue
		}
		space := s.SpaceName()
		if was, ok := w.seen[s.ID]; ok && was != space {
			out = append(out, s)
		}
		next[s.ID] = space
	}
	w.seen = next
	return out
}

// spaceMove is a window to move: the provider session it shows, where to open it instead, and whether it has
// the keyboard, which goes with it.
type spaceMove struct {
	Window  int
	Session ProviderSession
	Target  Worktree
	Focused bool
}

// sessionSpace is the space of a kitty session, SPACE or SPACE@DIR, default for a window in none.
func sessionSpace(session string) string {
	space, _, _ := strings.Cut(session, "@")
	return cmp.Or(space, defaultSpace)
}

// planMoves are the moves for the sessions whose space changed, of their windows in the OS window of self:
// that sidebar's, so each window has one sidebar to move it when several run. A window already in its
// session's space stays, as does one of kspaces' own.
func planMoves(changed []ProviderSession, spaces []Space, st kitty.State, data []kitty.OSWindow, self int) []spaceMove {
	mine := kitty.OSWindowOf(data, self)
	keyboard := kitty.KeyboardWindow(data)
	var out []spaceMove
	for _, s := range changed {
		space := Space{Name: s.SpaceName()}
		for _, sp := range spaces {
			if sp.Name == space.Name {
				space = sp
			}
		}
		for _, id := range s.Windows {
			w, ok := kitty.FindWindow(data, id)
			if !ok || kitty.Own(w) || mine < 0 || kitty.OSWindowOf(data, id) != mine {
				continue
			}
			if sessionSpace(w.SessionName) == space.Name {
				continue
			}
			out = append(out, spaceMove{Window: id, Session: s, Target: OpenTarget(st, space, s), Focused: id == keyboard})
		}
	}
	return out
}

// errLeftForRestore is a move not made because its space's session is not open and opens from its file: a
// tab added now would be all the session had, and the file would not be restored when it is gone to.
var errLeftForRestore = errors.New("its space's session is not open and is restored from its file, so it is left")

// moveWindow opens m's session in its target and closes the window it was in. The keyboard goes to the new
// tab only when the old window had it, as opening a detached row switches there; otherwise the tab opens
// behind, and nothing moves under the user. The old window is marked kept before it closes, so a watcher
// ending a window's multiplexer session on close leaves this one, which the new tab shows.
func moveWindow(cfg config.Config, st kitty.State, m spaceMove) error {
	if m.Focused {
		if err := OpenSession(cfg, st, m.Target, m.Session); err != nil {
			return err
		}
	} else {
		args, err := behindActions(cfg, st, m.Target, m.Session)
		if err != nil {
			return err
		}
		if err := kitty.Action(args...); err != nil {
			return fmt.Errorf("opening %s: %w", m.Session.Name, err)
		}
	}
	if err := kitty.SetWindowVar(m.Window, kitty.KeepVar, string(KeepClosed)); err != nil {
		return err
	}
	return kitty.CloseWindow(m.Window)
}

// behindActions is the launch that opens s in wt's session without switching to it: a tab with --keep-focus,
// by the session's window when it has windows, else on its own. A session not open but restored from its
// file is errLeftForRestore.
func behindActions(cfg config.Config, st kitty.State, wt Worktree, s ProviderSession) ([]string, error) {
	anchor, open := st.Sessions[wt.Session]
	if !open && !opensAfresh(cfg, st, wt) {
		return nil, errLeftForRestore
	}
	argv, cwd, err := openCommand(cfg.Provider, s)
	if err != nil {
		return nil, err
	}
	return behindLaunch(cfg, wt, anchor, argv, cmp.Or(cwd, s.Cwd)), nil
}

// behindLaunch is behindActions' launch, given what the provider says opens the session.
func behindLaunch(cfg config.Config, wt Worktree, anchor int, argv []string, cwd string) []string {
	cwd = cmp.Or(cwd, wt.Path)
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	var tab []string
	if anchor != 0 {
		tab = append(tab, fmt.Sprintf("--match=id:%d", anchor))
	}
	tab = append(append(tab, cfg.Launch...), "--type=tab", "--keep-focus")
	if cwd != "" {
		tab = append(tab, kitty.Quote("--cwd="+cwd))
	}
	tab = append(tab, inSession(wt.Session)...)
	for _, a := range argv {
		tab = append(tab, kitty.Quote(a))
	}
	return tab
}

// followSpaces moves the windows of sessions whose space changed since the last poll, see spaceWatch. A move
// that fails is logged and not tried again: the session's new space is recorded either way.
func followSpaces(w *spaceWatch, sh Shared, self int) {
	if sh.Config == nil || sh.Provider == nil {
		return
	}
	changed := w.changed(sh.Provider)
	if len(changed) == 0 {
		return
	}
	snap := sh.View(self)
	for _, m := range planMoves(changed, snap.Spaces, snap.Kitty, sh.Windows, self) {
		if err := moveWindow(*sh.Config, snap.Kitty, m); err != nil {
			logs.Error(fmt.Sprintf("moving window %d of %s to %s", m.Window, m.Session.Name, m.Target.Session), err)
		}
	}
}
