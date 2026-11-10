package spaces

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// OpenWorktree switches to the worktree's session. When it has no windows, the session is restored from its
// saved file if there is one, and otherwise created with one tab in the worktree.
func OpenWorktree(cfg config.Config, st kitty.State, wt Worktree) error {
	if st.Home != 0 {
		// Hand this tab's focus back to its own window before leaving, so coming back to the tab lands
		// there rather than in the sidebar, which took focus when it was clicked or focused to pick.
		if err := kitty.FocusWindow(st.Home); err != nil {
			return err
		}
		// The sidebar's own session: stay in this tab rather than jumping to wherever it was last used.
		if wt.Session == st.Current {
			return nil
		}
	}
	path := sessionFile(cfg, wt.Session)
	if _, open := st.Sessions[wt.Session]; open {
		return gotoSession(path)
	}
	if _, err := os.Stat(path); err == nil && cfg.SessionsDir != "" {
		return gotoSession(path)
	}
	if wt.Path == "" {
		return fmt.Errorf("session %s has no windows and no saved file", wt.Session)
	}
	args := append([]string{}, cfg.NewTab...)
	args = append(args, "--cwd="+wt.Path, "--add-to-session="+wt.Session)
	if err := kitty.Action(args...); err != nil {
		return err
	}
	// The new tab is the session's only window, so this only records the switch, see gotoSession.
	return gotoSession(path)
}

// sessionFile is where a session would be saved. kitty names a session after its file, so this is also how
// goto_session is told which open session to switch to; without a sessions directory the file need not
// exist, as kitty reads it only for a session with no windows.
func sessionFile(cfg config.Config, session string) string {
	if cfg.SessionsDir == "" {
		return session + ".kitty-session"
	}
	return filepath.Join(cfg.SessionsDir, session+".kitty-session")
}

// gotoSession switches to a session with goto_session, which focuses its most recently used window. Not
// with focus-window: goto_session is what kitty counts as switching sessions, so the tab bar's
// `session:~` keeps showing this session's tabs while a window in no session, such as another OS window,
// has focus.
func gotoSession(path string) error {
	return kitty.Action("goto_session", path)
}

// OpenWindow focuses one window, handing this tab's focus back to its own window first, as OpenWorktree
// does, so coming back to the tab does not land in the sidebar. A window in a session is reached through
// its session first, for the same reason as gotoSession.
func OpenWindow(cfg config.Config, st kitty.State, id int, session string) error {
	if st.Home != 0 && st.Home != id {
		if err := kitty.FocusWindow(st.Home); err != nil {
			return err
		}
	}
	if _, open := st.Sessions[session]; open && session != st.Current {
		if err := gotoSession(sessionFile(cfg, session)); err != nil {
			return err
		}
	}
	return kitty.FocusWindow(id)
}

// OpenFromOverlay switches to the worktree's session from an overlay that has focus, such as the new
// worktree form. There is no window of the tab to hand back to: the tab's own window gets focus again
// when the overlay closes.
func OpenFromOverlay(cfg config.Config, wt Worktree) error {
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	st.Home = 0
	return OpenWorktree(cfg, st, wt)
}

// FindWorktree is the worktree at path and its space, from any of the configured spaces.
func FindWorktree(cfg config.Config, path string) (Space, Worktree, error) {
	// git reports worktree paths with symlinks resolved, for example /private/tmp for /tmp on macOS.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Space{}, Worktree{}, err
	}
	if path, err = filepath.Abs(path); err != nil {
		return Space{}, Worktree{}, err
	}
	for _, root := range cfg.Spaces {
		s := LoadSpace(root)
		for _, wt := range s.Worktrees {
			if wt.Path == path {
				return s, wt, nil
			}
		}
	}
	return Space{}, Worktree{}, fmt.Errorf("%s is not a worktree of any configured space", path)
}

// OpenPath switches to the session of the worktree at path, creating it if needed.
func OpenPath(cfg config.Config, path string) error {
	_, wt, err := FindWorktree(cfg, path)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return OpenWorktree(cfg, st, wt)
}

// Leave gives keyboard focus back to the window of the tab the sidebar took it from, if it is known.
func Leave(st kitty.State) error {
	if st.Home == 0 {
		return nil
	}
	return kitty.FocusWindow(st.Home)
}

// NewWorktree opens the form that creates a worktree in the space at root, in an overlay over the
// sidebar's tab: the dock is too narrow to type a name in and read back where it goes.
func NewWorktree(st kitty.State, root string) error {
	return kitty.LaunchForm(st, "new", root)
}

// AskRemoveWorktree opens the form that confirms removing the worktree at path, in an overlay as
// NewWorktree's.
func AskRemoveWorktree(st kitty.State, path string) error {
	return kitty.LaunchForm(st, "remove", path)
}

// AskCloseSession opens the form that confirms closing session, in an overlay as NewWorktree's.
func AskCloseSession(st kitty.State, session string) error {
	return kitty.LaunchForm(st, "close", session)
}

// closeWait is how long CloseSession waits for kitty to close a session's windows.
const closeWait = 10 * time.Second

// CloseSession closes every window of session and waits until kitty has: it closes them after answering,
// and what kitty.conf runs as each closes, such as ending a multiplexer session, runs then.
func CloseSession(session string) error {
	if err := kitty.CloseSession(session); err != nil {
		return err
	}
	for deadline := time.Now().Add(closeWait); ; time.Sleep(100 * time.Millisecond) {
		st, err := kitty.LoadState(0)
		if err != nil {
			return err
		}
		if st.Windows[session] == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("kitty still has %d windows of %s open", st.Windows[session], session)
		}
	}
}

// CanRemove says why git would refuse to remove wt, a worktree of space, or nil when it would not. Changes
// are not a reason: RemoveWorktree can force past them.
func CanRemove(space Space, wt Worktree) error {
	switch {
	case len(space.Worktrees) > 0 && space.Worktrees[0].Path == wt.Path:
		return fmt.Errorf("%s is the main worktree of %s, which cannot be removed", wt.Path, space.Name)
	case wt.Locked:
		return fmt.Errorf("%s is locked; git worktree unlock it first", wt.Path)
	}
	return nil
}

// RemoveWorktree ends the session of wt, a linked worktree of space, then removes the worktree, losing its
// changes if force. The session goes first so nothing still runs in the directory as it is deleted, and
// its saved file goes too, so it is not restored. The branch is kept.
func RemoveWorktree(cfg config.Config, space Space, wt Worktree, force bool) error {
	if err := CanRemove(space, wt); err != nil {
		return err
	}
	if err := CloseSession(wt.Session); err != nil {
		return err
	}
	if cfg.SessionsDir != "" {
		if err := os.Remove(sessionFile(cfg, wt.Session)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return git.RemoveWorktree(space.Root, wt.Path, force)
}

// RemoveInBackground runs `kspaces remove --yes` for the worktree at path as a kitty background process.
// Removing closes the worktree's windows, and the form that asked may be over one of them.
func RemoveInBackground(path string, force bool) error {
	args := []string{"remove", "--yes"}
	if force {
		args = append(args, "--force")
	}
	return kitty.LaunchBackground(append(args, path)...)
}
