// Package kitty talks to kitty with remote control, `kitten @`.
package kitty

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/status"
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

// Window is a window in `kitten @ ls`.
type Window struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	// The window kitty would focus in its tab: is_focused in `kitten @ ls` is true for that one in every
	// tab, so it does not say which window has the keyboard.
	IsActive      bool              `json:"is_active"`
	SessionName   string            `json:"session_name"`
	LastFocusedAt float64           `json:"last_focused_at"`
	UserVars      map[string]string `json:"user_vars"`
	ProgramStatus []status.Record   `json:"program_status"`
}

// Tab is a tab in `kitten @ ls`.
type Tab struct {
	ID       int      `json:"id"`
	IsActive bool     `json:"is_active"`
	Windows  []Window `json:"windows"`
}

// OSWindow is an OS window in `kitten @ ls`.
type OSWindow struct {
	IsFocused   bool  `json:"is_focused"`
	LastFocused bool  `json:"last_focused"`
	Tabs        []Tab `json:"tabs"`
}

// State is the part of `kitten @ ls` kspaces needs.
type State struct {
	// For each session with windows, the one focused most recently. Switching to a session focuses it.
	Sessions map[string]int
	// The session of the tab the sidebar runs in, empty when it belongs to none.
	Current string
	// The window focused most recently in the sidebar's tab, not counting docks. Leaving the sidebar returns
	// focus to it. Zero when unknown.
	Home int
	// Every window that reports a program status, grouped by state, most urgent first.
	Statuses []status.Status
}

// FocusedWindow stands for the window with keyboard focus, for a command run from a kitty mapping rather
// than from a window of its own.
const FocusedWindow = -1

// LoadState reads kitty's state as seen from the window selfWindowID, or from the focused window when that
// is FocusedWindow.
func LoadState(selfWindowID int) (State, error) {
	out, err := kitten("ls")
	if err != nil {
		return State{}, err
	}
	var data []OSWindow
	if err := json.Unmarshal(out, &data); err != nil {
		return State{}, fmt.Errorf("parsing kitten @ ls: %w", err)
	}
	if selfWindowID == FocusedWindow {
		selfWindowID = keyboardWindow(data)
	}
	return stateFrom(data, selfWindowID), nil
}

// keyboardWindow is the window that has the keyboard: the active window of the active tab of the focused
// OS window, or of the one focused last when kitty itself does not have focus. Zero when there is none.
func keyboardWindow(data []OSWindow) int {
	var osw *OSWindow
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

func stateFrom(data []OSWindow, selfWindowID int) State {
	st := State{Sessions: map[string]int{}}
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
				if s, ok := status.OfWindow(w.ID, w.SessionName, w.Title, w.UserVars, w.ProgramStatus); ok {
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
	status.Sort(st.Statuses)
	return st
}

// SelfWindowID is the kitty window kspaces runs in, zero when it runs in none.
func SelfWindowID() int {
	id, err := strconv.Atoi(os.Getenv("KITTY_WINDOW_ID"))
	if err != nil {
		return 0
	}
	return id
}

// LaunchNewWorktree opens the new worktree form for the space at root in an overlay over the window of
// the sidebar's tab. The var keeps the overlay out of saved sessions, whose match leaves out var:kspaces.
func LaunchNewWorktree(st State, root string) error {
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

// FocusWindow focuses the window id.
func FocusWindow(id int) error {
	_, err := kitten("focus-window", "--match", fmt.Sprintf("id:%d", id))
	return err
}

// Action runs a kitty action, such as goto_session PATH.
func Action(args ...string) error {
	_, err := kitten(append([]string{"action"}, args...)...)
	return err
}
