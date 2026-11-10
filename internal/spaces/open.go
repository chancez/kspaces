package spaces

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chancez/kspaces/internal/config"
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

// OpenPath switches to the session of the worktree at path, creating it if needed.
func OpenPath(cfg config.Config, path string) error {
	// git reports worktree paths with symlinks resolved, for example /private/tmp for /tmp on macOS.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if path, err = filepath.Abs(path); err != nil {
		return err
	}
	for _, root := range cfg.Spaces {
		for _, wt := range LoadSpace(root).Worktrees {
			if wt.Path != path {
				continue
			}
			st, err := kitty.LoadState(kitty.SelfWindowID())
			if err != nil {
				return err
			}
			return OpenWorktree(cfg, st, wt)
		}
	}
	return fmt.Errorf("%s is not a worktree of any configured space", path)
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
	return kitty.LaunchNewWorktree(st, root)
}
