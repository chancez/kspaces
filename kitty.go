package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// dockVar is the user variable that marks a sidebar dock, so it is never mistaken for one of the
// session's own windows.
const dockVar = "kspaces"

// kittenPath prefers the kitten next to the kitty that started us, so remote control talks to a kitten of
// the same version. That is only known on macOS, where the installation directory sits inside the app
// bundle; elsewhere kitty puts its own directory on PATH.
func kittenPath() string {
	if dir := os.Getenv("KITTY_INSTALLATION_DIR"); dir != "" {
		p := filepath.Join(dir, "..", "..", "MacOS", "kitten")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "kitten"
}

func kitten(args ...string) ([]byte, error) {
	cmd := exec.Command(kittenPath(), append([]string{"@"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("kitten @ %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("kitten @ %s: %w", args[0], err)
	}
	return out, nil
}

type lsWindow struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	// The window kitty would focus in its tab: is_focused in `kitten @ ls` is true for that one in every
	// tab, so it does not say which window has the keyboard.
	IsActive      bool              `json:"is_active"`
	SessionName   string            `json:"session_name"`
	LastFocusedAt float64           `json:"last_focused_at"`
	UserVars      map[string]string `json:"user_vars"`
	ProgramStatus []programRecord   `json:"program_status"`
}

type lsTab struct {
	ID       int        `json:"id"`
	IsActive bool       `json:"is_active"`
	Windows  []lsWindow `json:"windows"`
}

type lsOSWindow struct {
	IsFocused   bool    `json:"is_focused"`
	LastFocused bool    `json:"last_focused"`
	Tabs        []lsTab `json:"tabs"`
}

// KittyState is the part of `kitten @ ls` the sidebar needs.
type KittyState struct {
	// For each session with windows, the one focused most recently. Switching to a session focuses it.
	Sessions map[string]int
	// The session of the tab the sidebar runs in, empty when it belongs to none.
	Current string
	// The window focused most recently in the sidebar's tab, not counting docks. Leaving the sidebar returns
	// focus to it. Zero when unknown.
	Home int
	// Every window that reports a program status, grouped by state, most urgent first.
	Statuses []Status
}

// focusedWindow stands for the window with keyboard focus, for a command run from a kitty mapping rather
// than from a window of its own.
const focusedWindow = -1

// loadKittyState reads kitty's state as seen from the window selfWindowID, or from the focused window
// when that is focusedWindow.
func loadKittyState(selfWindowID int) (KittyState, error) {
	out, err := kitten("ls")
	if err != nil {
		return KittyState{}, err
	}
	var data []lsOSWindow
	if err := json.Unmarshal(out, &data); err != nil {
		return KittyState{}, fmt.Errorf("parsing kitten @ ls: %w", err)
	}
	if selfWindowID == focusedWindow {
		selfWindowID = keyboardWindow(data)
	}
	return kittyStateFrom(data, selfWindowID), nil
}

// keyboardWindow is the window that has the keyboard: the active window of the active tab of the focused
// OS window, or of the one focused last when kitty itself does not have focus. Zero when there is none.
func keyboardWindow(data []lsOSWindow) int {
	var osw *lsOSWindow
	for i := range data {
		if data[i].IsFocused || (osw == nil && data[i].LastFocused) {
			osw = &data[i]
		}
	}
	if osw == nil {
		return 0
	}
	for _, tab := range osw.Tabs {
		if !tab.IsActive {
			continue
		}
		for _, w := range tab.Windows {
			if w.IsActive {
				return w.ID
			}
		}
	}
	return 0
}

func kittyStateFrom(data []lsOSWindow, selfWindowID int) KittyState {
	st := KittyState{Sessions: map[string]int{}}
	lastFocused := map[string]float64{}
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			hasSelf, session := false, ""
			home, homeAt := 0, 0.0
			for _, w := range tab.Windows {
				if w.ID == selfWindowID {
					hasSelf = true
				}
				if _, isDock := w.UserVars[dockVar]; isDock {
					continue
				}
				if home == 0 || w.LastFocusedAt > homeAt {
					home, homeAt = w.ID, w.LastFocusedAt
				}
				if s, ok := windowStatus(w); ok {
					st.Statuses = append(st.Statuses, s)
				}
				if w.SessionName == "" {
					continue
				}
				session = w.SessionName
				if t, seen := lastFocused[w.SessionName]; !seen || w.LastFocusedAt > t {
					lastFocused[w.SessionName] = w.LastFocusedAt
					st.Sessions[w.SessionName] = w.ID
				}
			}
			if hasSelf {
				st.Current, st.Home = session, home
			}
		}
	}
	sortStatuses(st.Statuses)
	return st
}

func selfWindowID() int {
	id, err := strconv.Atoi(os.Getenv("KITTY_WINDOW_ID"))
	if err != nil {
		return 0
	}
	return id
}

// launchNewWorktree opens the new worktree form for the space at root in an overlay over the window of
// the sidebar's tab. The var keeps the overlay out of saved sessions, whose match leaves out var:kspaces.
func launchNewWorktree(st KittyState, root string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"launch", "--type=overlay", "--var=kspaces_form=1"}
	if st.Home != 0 {
		args = append(args, "--next-to", fmt.Sprintf("id:%d", st.Home))
	}
	_, err = kitten(append(args, exe, "new", root)...)
	return err
}

func focusWindow(id int) error {
	_, err := kitten("focus-window", "--match", fmt.Sprintf("id:%d", id))
	return err
}

// openWindow focuses one window, handing this tab's focus back to its own window first, as openWorktree
// does, so coming back to the tab does not land in the sidebar.
func openWindow(st KittyState, id int) error {
	if st.Home != 0 && st.Home != id {
		if err := focusWindow(st.Home); err != nil {
			return err
		}
	}
	return focusWindow(id)
}

// openWorktree switches to the worktree's session. When it has no windows, the session is restored from its
// saved file if there is one, and otherwise created with one tab in the worktree.
func openWorktree(cfg Config, st KittyState, wt Worktree) error {
	if st.Home != 0 {
		// Hand this tab's focus back to its own window before leaving, so coming back to the tab lands
		// there rather than in the sidebar, which took focus when it was clicked or focused to pick.
		if err := focusWindow(st.Home); err != nil {
			return err
		}
		// The sidebar's own session: stay in this tab rather than jumping to wherever it was last used.
		if wt.Session == st.Current {
			return nil
		}
	}
	if id, ok := st.Sessions[wt.Session]; ok {
		return focusWindow(id)
	}
	if cfg.SessionsDir != "" {
		path := filepath.Join(cfg.SessionsDir, wt.Session+".kitty-session")
		if _, err := os.Stat(path); err == nil {
			_, err := kitten("action", "goto_session", path)
			return err
		}
	}
	if wt.Path == "" {
		return fmt.Errorf("session %s has no windows and no saved file", wt.Session)
	}
	args := append([]string{"action"}, cfg.NewTab...)
	args = append(args, "--cwd="+wt.Path, "--add-to-session="+wt.Session)
	_, err := kitten(args...)
	return err
}
