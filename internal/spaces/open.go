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
	if id, ok := st.Sessions[wt.Session]; ok {
		return kitty.FocusWindow(id)
	}
	if cfg.SessionsDir != "" {
		path := filepath.Join(cfg.SessionsDir, wt.Session+".kitty-session")
		if _, err := os.Stat(path); err == nil {
			return kitty.Action("goto_session", path)
		}
	}
	if wt.Path == "" {
		return fmt.Errorf("session %s has no windows and no saved file", wt.Session)
	}
	args := append([]string{}, cfg.NewTab...)
	args = append(args, "--cwd="+wt.Path, "--add-to-session="+wt.Session)
	return kitty.Action(args...)
}

// OpenWindow focuses one window, handing this tab's focus back to its own window first, as OpenWorktree
// does, so coming back to the tab does not land in the sidebar.
func OpenWindow(st kitty.State, id int) error {
	if st.Home != 0 && st.Home != id {
		if err := kitty.FocusWindow(st.Home); err != nil {
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
