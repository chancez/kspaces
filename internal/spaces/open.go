package spaces

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
)

// OpenOptions are how OpenWorktree opens a session with no windows.
type OpenOptions struct {
	// NoLayout opens one tab in the worktree in place of its default layout. A saved file that opens
	// windows still restores the session: the default layout only ever stands in for its first tab.
	NoLayout bool
}

// OpenWorktree switches to the worktree's session. When it has no windows, the session is restored from its
// saved file if that opens any, and otherwise opened with its default layout, see freshLayout, unless
// opts.NoLayout, or with one tab in the worktree, or in the home directory for a session in no space. A
// default layout that needs inputs is not opened: the tab is, and the error says so.
func OpenWorktree(cfg config.Config, st kitty.State, wt Worktree, opts OpenOptions) error {
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
	actions, err := openActions(cfg, st, wt)
	if err != nil {
		return err
	}
	// More than the switch is a fresh session's first tab, which the default layout stands in for.
	var skipped error
	if len(actions) > 1 && !opts.NoLayout {
		spaceCfg, space, found, lr, ok, err := freshLayout(cfg, wt)
		if ok {
			return OpenLayout(spaceCfg, st, space, found, lr)
		}
		if err != nil {
			skipped = fmt.Errorf("opened %s with a shell, as %w", wt.Session, err)
		}
	}
	for _, args := range actions {
		if err := kitty.Action(args...); err != nil {
			return err
		}
	}
	return skipped
}

// openActions are the kitty actions that switch to wt's session, see OpenWorktree.
func openActions(cfg config.Config, st kitty.State, wt Worktree) ([][]string, error) {
	path := SessionFile(cfg, wt.Session)
	goTo := []string{"goto_session", path}
	if !opensAfresh(cfg, st, wt) {
		return [][]string{goTo}, nil
	}
	cwd := wt.Path
	if cwd == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cwd = home
	}
	// The new tab is the session's only window, so going to it only records the switch, see gotoSession.
	return [][]string{firstTab(cfg, cwd, wt.Session), goTo}, nil
}

// opensAfresh says whether wt's session has neither windows nor a saved file that opens any, so opening it
// makes its first tab, or its default layout.
func opensAfresh(cfg config.Config, st kitty.State, wt Worktree) bool {
	if _, open := st.Sessions[wt.Session]; open {
		return false
	}
	return cfg.SessionsDir == "" || !opensWindows(SessionFile(cfg, wt.Session))
}

// OpensDefaultLayout says whether OpenWorktree would open wt, a worktree of space as listed, with its
// default layout, so that OpenOptions.NoLayout changes what it opens. A default that needs inputs is not
// opened anyway. Its inputs' defaults are filled in without the worktree's PR, which git is not asked
// for here, as a menu asks this as it is drawn.
func OpensDefaultLayout(cfg config.Config, st kitty.State, space Space, wt Worktree) bool {
	if wt.Path == "" || len(space.Worktrees) == 0 || !opensAfresh(cfg, st, wt) {
		return false
	}
	cfg = space.ConfigOf(cfg)
	l, ok := cfg.Layout(cfg.LayoutFor(space.Name, space.Worktrees[0].Path != wt.Path))
	if !ok {
		return false
	}
	_, err := NewLayoutRun(l, nil, BuiltinVars(space, wt, 0))
	return err == nil
}

// firstTab is the launch action that opens a session's first tab, in cwd.
func firstTab(cfg config.Config, cwd, session string) []string {
	args := append([]string{}, cfg.Launch...)
	return append(args, "--type=tab", "--cwd="+cwd, "--add-to-session="+session)
}

// opensWindows says whether goto_session opens any window from the session file at path. Windows come only
// from launch lines, and kitty opens nothing, silently, for a file without one, such as the empty file
// save_sessions.py leaves for the default session once its last window has closed.
func opensWindows(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(data)) {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "launch" {
			return true
		}
	}
	return false
}

// SessionFile is where a session would be saved. kitty names a session after its file, so this is also how
// goto_session is told which open session to switch to; without a sessions directory the file need not
// exist, as kitty reads it only for a session with no windows.
func SessionFile(cfg config.Config, session string) string {
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
		if err := gotoSession(SessionFile(cfg, session)); err != nil {
			return err
		}
	}
	return kitty.FocusWindow(id)
}

// OpenFromOverlay switches to the worktree's session from an overlay that has focus, such as the new
// worktree form. There is no window of the tab to hand back to: the tab's own window gets focus again
// when the overlay closes.
func OpenFromOverlay(cfg config.Config, wt Worktree, opts OpenOptions) error {
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	st.Home = 0
	return OpenWorktree(cfg, st, wt, opts)
}

// FindWorktree is the worktree at path and its space, from any of the configured spaces, its file read as
// cfg's, see ReadSpace.
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
				s.readConfig(cfg)
				return s, wt, nil
			}
		}
	}
	return Space{}, Worktree{}, fmt.Errorf("%s is not a worktree of any configured space", path)
}

// OpenPath switches to the session of the worktree at path, creating it if needed, as opts says.
func OpenPath(cfg config.Config, path string, opts OpenOptions) error {
	_, wt, err := FindWorktree(cfg, path)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return OpenWorktree(cfg, st, wt, opts)
}

// Leave gives keyboard focus back to the window of the tab the sidebar took it from, if it is known.
func Leave(st kitty.State) error {
	if st.Home == 0 {
		return nil
	}
	return kitty.FocusWindow(st.Home)
}

// NewWorktree opens the form that creates a worktree in the space at root, in a popup for the sidebar's
// tab, as cfg says: the dock is too narrow to type a name in and read back where it goes.
func NewWorktree(cfg config.Config, st kitty.State, root string) error {
	return launchForm(cfg, st, formPanel, "new", root)
}

// AskRemoveWorktree opens the form that confirms removing the worktree at path, in a popup as
// NewWorktree's.
func AskRemoveWorktree(cfg config.Config, st kitty.State, path string) error {
	return launchForm(cfg, st, formPanel, "remove", path)
}

// AskRunCommand opens the form that runs the command called name in the worktrees at paths, in a popup
// sized as AskCloseSession's. For several it offers the commands they all have, name or else the first.
func AskRunCommand(cfg config.Config, st kitty.State, name string, paths ...string) error {
	return launchForm(cfg, st, panelFor(len(paths)), append([]string{"run", name}, paths...)...)
}

// AskOpenLayout opens the form that opens the layout called name in the worktree at path, in a popup as
// NewWorktree's.
func AskOpenLayout(cfg config.Config, st kitty.State, name, path string) error {
	return launchForm(cfg, st, formPanel, "layout", name, path)
}

// AskSaveLayout opens the form that saves the session of the worktree at path as a layout, its name left
// to fill in, in a popup as NewWorktree's. The form is for the window of the tab st is seen from, the
// sidebar's, so that is this tab when the session is the sidebar's.
func AskSaveLayout(cfg config.Config, st kitty.State, path string) error {
	return launchForm(cfg, st, listPanel, "layout", "save", "", path)
}

// AskCloseSession opens the form that confirms closing sessions, in a popup as NewWorktree's, or as
// AskCleanup's for several, which the form lists.
func AskCloseSession(cfg config.Config, st kitty.State, sessions ...string) error {
	return launchForm(cfg, st, panelFor(len(sessions)), append([]string{"close"}, sessions...)...)
}

// panelFor is the size of the popup of a form for n worktrees or sessions: a list's for more than one.
func panelFor(n int) panelSize {
	if n > 1 {
		return listPanel
	}
	return formPanel
}

// AskForgetSession opens the form that confirms forgetting sessions, in a popup sized as AskCloseSession's.
func AskForgetSession(cfg config.Config, st kitty.State, sessions ...string) error {
	return launchForm(cfg, st, panelFor(len(sessions)), append([]string{"forget"}, sessions...)...)
}

// AskTrust opens the form that trusts the own config of the space at root, in a popup as NewWorktree's.
func AskTrust(cfg config.Config, st kitty.State, root string) error {
	return launchForm(cfg, st, listPanel, "trust", root)
}

// AskReply opens the form that replies to window, in a popup as NewWorktree's, of a list's size, as it
// shows the window's screen.
func AskReply(cfg config.Config, st kitty.State, window int) error {
	return launchForm(cfg, st, listPanel, "reply", strconv.Itoa(window))
}

// AskShowError opens the form that shows text, an error, in full, in a popup as NewWorktree's, of a list's
// size, as the text can be long. The text goes in a temporary file the form reads and deletes, rather than
// on its command line, as an error can be long and run over several lines. A form kitty launched but that
// never started leaves the file to the temporary directory's cleaning.
func AskShowError(cfg config.Config, st kitty.State, text string) error {
	f, err := os.CreateTemp("", "kspaces-error-*.txt")
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = launchForm(cfg, st, listPanel, "error", f.Name())
	}
	// A refused panel opened an overlay, whose form reads the file.
	var refused *kitty.PanelRefused
	if err != nil && !errors.As(err, &refused) {
		os.Remove(f.Name())
	}
	return err
}

// ForgetSession deletes the saved file of session, so kitty does not restore it: for a missing worktree,
// whose directory is gone, that is all that is left of it.
func ForgetSession(cfg config.Config, session string) error {
	if cfg.SessionsDir == "" {
		return fmt.Errorf("there is no sessions directory to forget %s from", session)
	}
	return os.Remove(SessionFile(cfg, session))
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

// EndSession closes every window of session, waiting as CloseSession does, and deletes its saved file, so
// it is not restored: all there is of a missing worktree, and what removing a worktree ends first. A
// session with no saved file is fine.
func EndSession(cfg config.Config, session string) error {
	if err := CloseSession(session); err != nil {
		return err
	}
	if cfg.SessionsDir == "" {
		return nil
	}
	if err := os.Remove(SessionFile(cfg, session)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
// changes if opts.Force. The session goes first so nothing still runs in the directory as it is deleted,
// and its saved file goes too, so it is not restored. With opts.DeleteBranch the branch goes last, with
// git branch -d when merged and -D only with opts.ForceBranch; a branch that cannot go keeps the worktree
// too, but one git refuses after the worktree is removed is a *BranchError.
func RemoveWorktree(cfg config.Config, space Space, wt Worktree, opts RemoveOptions) error {
	if err := CanRemove(space, wt); err != nil {
		return err
	}
	var branch Branch
	if opts.DeleteBranch {
		var err error
		if branch, err = branchToDelete(space, wt, opts); err != nil {
			return err
		}
	}
	if err := EndSession(cfg, wt.Session); err != nil {
		return err
	}
	if err := git.RemoveWorktree(space.Root, wt.Path, opts.Force); err != nil {
		return err
	}
	if !opts.DeleteBranch {
		return nil
	}
	if err := git.DeleteBranch(space.Root, branch.Name, branch.Unmerged > 0); err != nil {
		return &BranchError{Branch: branch.Name, Err: err}
	}
	return nil
}

// RemoveInBackground runs `kspaces remove --yes` for the worktree at path as a kitty background process.
// Removing closes the worktree's windows, and the form that asked may be over one of them.
func RemoveInBackground(path string, opts RemoveOptions) error {
	args := []string{"remove", "--yes"}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.DeleteBranch {
		args = append(args, "--delete-branch")
	}
	if opts.DeleteBranch && opts.ForceBranch {
		args = append(args, "--force-branch")
	}
	return kitty.LaunchBackground(append(args, path)...)
}
