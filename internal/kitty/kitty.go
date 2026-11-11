// Package kitty talks to kitty with remote control, `kitten @`.
package kitty

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chancez/kspaces/internal/status"
)

// dockVar is the user variable that marks a sidebar dock, so it is never mistaken for one of the
// session's own windows.
const dockVar = "kspaces"

// ownWindow reports whether w is one of kspaces' own, a dock, form or picker, which mark themselves with a
// user variable starting kspaces, as the saved sessions leave them out.
func ownWindow(w Window) bool {
	for k := range w.UserVars {
		if strings.HasPrefix(k, dockVar) {
			return true
		}
	}
	return false
}

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
	Cwd           string            `json:"cwd"`
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
	// How many windows each session with windows has, not counting docks.
	Windows map[string]int
	// The session of the tab the sidebar runs in, empty when it belongs to none.
	Current string
	// The window focused most recently in the sidebar's tab, not counting docks. Leaving the sidebar returns
	// focus to it. Zero when unknown.
	Home int
	// The working directory of Home, empty when unknown.
	HomeDir string
	// Every window that reports a program status, grouped by state, most urgent first.
	Statuses []status.Status
	// For each session with windows, the one whose screen shows what the session is up to: the one
	// switching to it focuses, as goto_session picks it, leaving out kspaces' own windows and the one
	// kspaces runs in.
	Screens map[string]int
	// kspaces' own windows, docks, forms and the picker, whose screens show nothing of a session.
	Own map[int]bool
}

// FocusedWindow stands for the window with keyboard focus, for a command run from a kitty mapping rather
// than from a window of its own.
const FocusedWindow = -1

// LoadState reads kitty's state as seen from the window selfWindowID, or from the focused window when that
// is FocusedWindow.
func LoadState(selfWindowID int) (State, error) {
	data, err := ListWindows()
	if err != nil {
		return State{}, err
	}
	return StateOf(data, selfWindowID), nil
}

// ListWindows is `kitten @ ls`, the part of it kspaces reads. It is the same for every window, so one
// process can read it for all the sidebars, each working out its own State with StateOf.
func ListWindows() ([]OSWindow, error) {
	out, err := kitten("ls")
	if err != nil {
		return nil, err
	}
	var data []OSWindow
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("parsing kitten @ ls: %w", err)
	}
	return data, nil
}

// StateOf is kitty's state in data as seen from the window selfWindowID, or from the focused window when
// that is FocusedWindow.
func StateOf(data []OSWindow, selfWindowID int) State {
	if selfWindowID == FocusedWindow {
		selfWindowID = keyboardWindow(data)
	}
	return stateFrom(data, selfWindowID)
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
	st := State{Sessions: map[string]int{}, Windows: map[string]int{}, Screens: map[string]int{}, Own: map[int]bool{}}
	lastFocused, screenAt, activeScreen := map[string]float64{}, map[string]float64{}, map[string]int{}
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			hasSelf, session := false, ""
			home, homeAt, homeDir := 0, 0.0, ""
			for _, w := range tab.Windows {
				if w.ID == selfWindowID {
					hasSelf = true
				}
				own := ownWindow(w)
				if own {
					st.Own[w.ID] = true
				}
				if _, isDock := w.UserVars[dockVar]; isDock {
					continue
				}
				if home == 0 || w.LastFocusedAt > homeAt {
					home, homeAt, homeDir = w.ID, w.LastFocusedAt, w.Cwd
				}
				if s, ok := status.OfWindow(w.ID, w.SessionName, w.Title, w.UserVars, w.ProgramStatus); ok {
					st.Statuses = append(st.Statuses, s)
				}
				if w.SessionName == "" {
					continue
				}
				session = w.SessionName
				st.Windows[w.SessionName]++
				if t, seen := lastFocused[w.SessionName]; !seen || w.LastFocusedAt > t {
					lastFocused[w.SessionName] = w.LastFocusedAt
					st.Sessions[w.SessionName] = w.ID
				}
				if own || w.ID == selfWindowID {
					continue
				}
				// goto_session takes the most recently focused, the first of equals. A kitty that gives no
				// times leaves the active window of the session's active tab, else its first window.
				if _, seen := st.Screens[w.SessionName]; !seen || w.LastFocusedAt > screenAt[w.SessionName] {
					st.Screens[w.SessionName], screenAt[w.SessionName] = w.ID, w.LastFocusedAt
				}
				if _, seen := activeScreen[w.SessionName]; !seen && tab.IsActive && w.IsActive {
					activeScreen[w.SessionName] = w.ID
				}
			}
			if hasSelf {
				st.Current, st.Home, st.HomeDir = session, home, homeDir
			}
		}
	}
	for session, id := range activeScreen {
		if screenAt[session] == 0 {
			st.Screens[session] = id
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

// LaunchForm runs this kspaces with args, such as `new ROOT`, in an overlay over the window of the
// sidebar's tab, for a form the sidebar is too narrow for. The var keeps the overlay out of saved
// sessions, whose match leaves out var:kspaces.
func LaunchForm(st State, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	launch := []string{"launch", "--type=overlay", "--var=kspaces_form=1"}
	if st.Home != 0 {
		launch = append(launch, "--next-to", fmt.Sprintf("id:%d", st.Home))
	}
	_, err = kitten(append(append(launch, exe), args...)...)
	return err
}

// LaunchBackground runs this kspaces with args as a kitty background process, with no window, so it
// outlives the window that started it. It may use remote control.
func LaunchBackground(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = kitten(append([]string{"launch", "--type=background", "--allow-remote-control", exe}, args...)...)
	return err
}

// ShowError shows an error in kitty, for a process with no window of its own to show it in.
func ShowError(title, msg string) error {
	return Action("show_error", title, msg)
}

// CloseSession closes every window of session, as closing them by hand would, so whatever kitty.conf runs
// when a window closes runs for each, such as ending a multiplexer session.
func CloseSession(session string) error {
	_, err := kitten("close-window", "--ignore-no-match", "--match", "session:^"+regexp.QuoteMeta(session)+"$")
	return err
}

// FocusWindow focuses the window id.
func FocusWindow(id int) error {
	_, err := kitten("focus-window", "--match", fmt.Sprintf("id:%d", id))
	return err
}

// ScreenText is what window id has on its screen, its colors and styles as SGR sequences.
func ScreenText(id int) (string, error) {
	out, err := kitten("get-text", "--match", fmt.Sprintf("id:%d", id), "--ansi", "--extent", "screen")
	return string(out), err
}

// Action runs a kitty action, such as goto_session PATH.
func Action(args ...string) error {
	_, err := kitten(append([]string{"action"}, args...)...)
	return err
}
