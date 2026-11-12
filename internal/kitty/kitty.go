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
	"sync/atomic"
	"time"

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
	// For each session with windows, when one of them, not counting docks, was last focused: kitty's
	// last_focused_at, in seconds. Zero when kitty gives no times.
	Focused map[string]float64
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
// that is FocusedWindow. In a panel its tab is the one of the window the panel was opened from, see
// PanelFrom: the panel's own is in no session.
func StateOf(data []OSWindow, selfWindowID int) State {
	if selfWindowID == FocusedWindow {
		selfWindowID = keyboardWindow(data)
	}
	return stateFrom(data, selfWindowID, PanelFrom())
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

// stateFrom is StateOf for the window self, its tab the one of the window from, or self's when from is
// zero.
func stateFrom(data []OSWindow, self, from int) State {
	if from == 0 {
		from = self
	}
	st := State{Sessions: map[string]int{}, Windows: map[string]int{}, Screens: map[string]int{}, Own: map[int]bool{}}
	lastFocused, screenAt, activeScreen := map[string]float64{}, map[string]float64{}, map[string]int{}
	st.Focused = lastFocused
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			hasFrom, session := false, ""
			home, homeAt, homeDir := 0, 0.0, ""
			for _, w := range tab.Windows {
				if w.ID == from {
					hasFrom = true
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
				if own || w.ID == self {
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
			if hasFrom {
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

// FromEnv tells kspaces in a panel which window it was opened from. A panel is an OS window of its own,
// its tab in no session, so it cannot learn the session it is for from its own tab as an overlay does.
const FromEnv = "KSPACES_FROM"

// PopupVar marks every panel kspaces opens, beside kspaces_form or kspaces_picker, so kitty.conf's watcher
// can hide its tab bar.
const PopupVar = "kspaces_popup"

// PanelFrom is the window the panel kspaces runs in was opened from, zero when it runs in no panel.
func PanelFrom() int {
	id, err := strconv.Atoi(os.Getenv(FromEnv))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// ThisWindowID is the window kspaces works for: in a panel the one it was opened from, else the one it
// runs in.
func ThisWindowID() int {
	if from := PanelFrom(); from != 0 {
		return from
	}
	return SelfWindowID()
}

// Popup is where a form or the picker opens.
type Popup struct {
	// Panel opens it as a kitty panel, an OS window of its own centered on the screen, Columns by Lines
	// cells, rather than as an overlay over a window.
	Panel          bool
	Columns, Lines int
}

// LaunchPopup runs this kspaces with args, such as `new ROOT`, in a popup for the window from, marked
// with the user variable v, kspaces_form or kspaces_picker, which keeps it out of saved sessions, whose
// match leaves out var:kspaces. An overlay goes over from, or over the active window when from is zero.
// A panel is told from in FromEnv. When kitty cannot open a panel, as on GNOME's Wayland or before kitty
// had them, it opens as an overlay, and the error says so.
func LaunchPopup(p Popup, from int, v string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p.Panel && from != 0 {
		_, perr := kitten(append(append(panelLaunch(p, from, v), exe), args...)...)
		if perr == nil {
			return nil
		}
		if _, err := kitten(append(append(overlayLaunch(from, v), exe), args...)...); err != nil {
			return err
		}
		return &PanelRefused{perr}
	}
	_, err = kitten(append(append(overlayLaunch(from, v), exe), args...)...)
	return err
}

// PanelRefused is LaunchPopup's error when kitty could not open a panel, and the popup opened as an
// overlay instead.
type PanelRefused struct{ Err error }

func (e *PanelRefused) Error() string {
	return "opened an overlay, as kitty could not open a panel: " + e.Err.Error()
}

func (e *PanelRefused) Unwrap() error { return e.Err }

// overlayLaunch is the launch command of an overlay over from, see LaunchPopup.
func overlayLaunch(from int, v string) []string {
	launch := []string{"launch", "--type=overlay", "--var=" + v + "=1"}
	if from != 0 {
		launch = append(launch, "--next-to", fmt.Sprintf("id:%d", from))
	}
	return launch
}

// panelLaunch is the launch command of p's panel for from, see LaunchPopup. It takes the keyboard while
// it is open, and sits above other windows, over a full screen one too on Wayland. Nothing puts it in a
// session, so the watcher docks no sidebar in it, and it saves with none.
func panelLaunch(p Popup, from int, v string) []string {
	return []string{
		"launch", "--type=os-panel",
		"--os-panel", "edge=center-sized",
		"--os-panel", fmt.Sprintf("columns=%d", p.Columns),
		"--os-panel", fmt.Sprintf("lines=%d", p.Lines),
		"--os-panel", "focus-policy=exclusive",
		"--os-panel", "layer=overlay",
		"--var=" + v + "=1", "--var=" + PopupVar + "=1",
		"--env", fmt.Sprintf("%s=%d", FromEnv, from),
	}
}

// LaunchForm runs this kspaces with args, such as `new ROOT`, in a popup for the window of the sidebar's
// tab, for a form the sidebar is too narrow for, see LaunchPopup.
func LaunchForm(p Popup, st State, args ...string) error {
	return LaunchPopup(p, st.Home, "kspaces_form", args...)
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
	acted.Store(true)
	_, err := kitten("focus-window", "--match", fmt.Sprintf("id:%d", id))
	return err
}

// acted is whether this process has run a kitty action or focused a window, either of which leaves the
// keyboard where it wants it.
var acted atomic.Bool

// Acted says whether this process has run an action or focused a window, see Action and FocusWindow.
func Acted() bool { return acted.Load() }

// leaveWait is how long LeavePanel waits for kitty to move its focus, a var for tests.
var leaveWait = time.Second

// left is whether LeavePanel has moved the focus out of the panel, which it does once.
var left atomic.Bool

// LeavePanel gives the keyboard back to the window the panel kspaces runs in was opened from, and waits
// until kitty has moved its focus to that window's OS window. Without it kitty acts on the panel's OS
// window, the focused one: goto_session loads a session with no windows into it whatever window the
// action runs on, and a kitten launcher's remote control launch places tabs and windows by the active tab,
// whatever --match or --next-to say, as kitty's own launch action does not. Once per process; nothing
// outside a panel.
func LeavePanel() error {
	from := PanelFrom()
	if from == 0 || left.Load() {
		return nil
	}
	if err := FocusWindow(from); err != nil {
		return err
	}
	for deadline := time.Now().Add(leaveWait); ; time.Sleep(20 * time.Millisecond) {
		data, err := ListWindows()
		if err != nil {
			return err
		}
		if osWindowFocused(data, from) {
			left.Store(true)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("kitty did not move its focus to window %d, so this panel would have taken the session", from)
		}
	}
}

// osWindowFocused says whether the OS window that has the window id has kitty's focus.
func osWindowFocused(data []OSWindow, id int) bool {
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == id {
					return osw.IsFocused
				}
			}
		}
	}
	return false
}

// GotoLayout sets the layout of the tab window id is in. Remote control rather than the goto_layout
// action, which only logs a layout that is not in enabled_layouts.
func GotoLayout(id int, layout string) error {
	_, err := kitten("goto-layout", fmt.Sprintf("--match=window_id:%d", id), layout)
	return err
}

// SessionWindows are the ids of session's windows in data, leaving out kspaces' own.
func SessionWindows(data []OSWindow, session string) []int {
	var out []int
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.SessionName == session && !ownWindow(w) {
					out = append(out, w.ID)
				}
			}
		}
	}
	return out
}

// ScreenText is what window id has on its screen, its colors and styles as SGR sequences.
func ScreenText(id int) (string, error) {
	out, err := kitten("get-text", "--match", fmt.Sprintf("id:%d", id), "--ansi", "--extent", "screen")
	return string(out), err
}

// Action runs a kitty action, such as goto_session PATH. In a panel it first leaves the panel, see
// LeavePanel, and runs on the window the panel was opened from, unless args start with a --match of their
// own, as it would run on an overlay over that window.
func Action(args ...string) error {
	if from := PanelFrom(); from != 0 {
		if err := LeavePanel(); err != nil {
			return err
		}
		if !strings.HasPrefix(args[0], "--match") {
			args = append([]string{fmt.Sprintf("--match=id:%d", from)}, args...)
		}
	}
	acted.Store(true)
	_, err := kitten(append([]string{"action"}, args...)...)
	return err
}
