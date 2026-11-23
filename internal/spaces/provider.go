package spaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// The session provider is the program config's provider names, which owns what runs in windows and the
// sessions that run with no window, such as a multiplexer's. kspaces knows nothing of them but what it says:
// it runs the provider with a verb, writes a JSON request to its stdin, and reads a JSON response from its
// stdout. One that fails exits non-zero, its stderr the error. See Session providers in docs/kitty.md, which
// is the contract.

// The capabilities a provider can name, see ProviderCapabilities.
const (
	CapKeep   = "keep"
	CapList   = "list"
	CapOpen   = "open"
	CapKill   = "kill"
	CapShelve = "shelve"
	CapRemove = "remove"
	CapMove   = "move"
)

// Capabilities are what a provider does, by name.
type Capabilities map[string]bool

// ProviderWindow is a window as the provider is told of it, what it needs to say which of them shows
// which of its sessions, or which it keeps: its process, its program, a launch wrapper's user variable.
type ProviderWindow struct {
	ID  int `json:"id"`
	PID int `json:"pid"`
	// The window's program, as `kitten @ ls` reports it.
	Cmdline []string `json:"cmdline"`
	// The programs in the foreground of its terminal, each a command line.
	Foreground [][]string        `json:"foreground"`
	UserVars   map[string]string `json:"user_vars"`
	Cwd        string            `json:"cwd"`
}

// providerWindow is w as the provider is told of it.
func providerWindow(id, pid int, cmdline []string, fg []kitty.Process, vars map[string]string, cwd string) ProviderWindow {
	pw := ProviderWindow{ID: id, PID: pid, Cmdline: cmdline, Foreground: [][]string{}, UserVars: vars, Cwd: cwd}
	for _, p := range fg {
		pw.Foreground = append(pw.Foreground, p.Cmdline)
	}
	if pw.Cmdline == nil {
		pw.Cmdline = []string{}
	}
	if pw.UserVars == nil {
		pw.UserVars = map[string]string{}
	}
	return pw
}

// sessionWindows are the windows of session in tabs, kspaces' own left out, as the provider is told of them.
func sessionWindows(tabs []kitty.FullTab, session string) []ProviderWindow {
	windows := []ProviderWindow{}
	for _, tab := range tabs {
		for _, w := range tab.Windows {
			if w.SessionName == session && !w.Own() {
				windows = append(windows, providerWindow(w.ID, w.PID, w.Cmdline, w.ForegroundProcesses, w.UserVars, w.Cwd))
			}
		}
	}
	return windows
}

// allWindows are every window of data, kspaces' own left out, as the provider is told of them.
func allWindows(data []kitty.OSWindow) []ProviderWindow {
	windows := []ProviderWindow{}
	for _, osw := range data {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if !kitty.Own(w) {
					windows = append(windows, providerWindow(w.ID, w.PID, w.Cmdline, w.ForegroundProcesses, w.UserVars, w.Cwd))
				}
			}
		}
	}
	return windows
}

// fullWindows are every window of tabs, kspaces' own left out, as the provider is told of them.
func fullWindows(tabs []kitty.FullTab) []ProviderWindow {
	windows := []ProviderWindow{}
	for _, tab := range tabs {
		for _, w := range tab.Windows {
			if !w.Own() {
				windows = append(windows, providerWindow(w.ID, w.PID, w.Cmdline, w.ForegroundProcesses, w.UserVars, w.Cwd))
			}
		}
	}
	return windows
}

// runProvider runs provider with verb and req on its stdin, and is what it printed. Its error is the
// provider's stderr, as the sidebar shows it, after the provider and the verb.
func runProvider(provider []string, verb string, req any) ([]byte, error) {
	if len(provider) == 0 {
		return nil, errors.New("no session provider is configured")
	}
	in, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(provider[0], append(provider[1:], verb)...)
	cmd.Stdin = bytes.NewReader(in)
	out, err := logs.Output(cmd)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(bytes.TrimSpace(exit.Stderr)) > 0 {
			return nil, fmt.Errorf("%s %s: %s", provider[0], verb, bytes.TrimSpace(exit.Stderr))
		}
		return nil, fmt.Errorf("%s %s: %w", provider[0], verb, err)
	}
	return out, nil
}

// askProvider runs verb as runProvider does, and parses what the provider printed into resp, which may be
// nothing for a verb with no output.
func askProvider(provider []string, verb string, req, resp any) error {
	out, err := runProvider(provider, verb, req)
	if err != nil || resp == nil || len(bytes.TrimSpace(out)) == 0 {
		return err
	}
	if err := json.Unmarshal(out, resp); err != nil {
		return fmt.Errorf("parsing what %s %s printed: %w", provider[0], verb, err)
	}
	return nil
}

// ProviderCapabilities are what provider says it does. One that fails to say, or says nothing, is taken to
// keep alone, as a program written for keeper does; without a provider there are none.
func ProviderCapabilities(provider []string) Capabilities {
	if len(provider) == 0 {
		return Capabilities{}
	}
	var resp struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := askProvider(provider, "capabilities", struct{}{}, &resp); err != nil || resp.Capabilities == nil {
		return Capabilities{CapKeep: true}
	}
	caps := Capabilities{}
	for _, c := range resp.Capabilities {
		caps[c] = true
	}
	return caps
}

type keepRequest struct {
	Session string           `json:"session"`
	Mode    KeepMode         `json:"mode"`
	Windows []ProviderWindow `json:"windows"`
}

type keepResponse struct {
	Kept []string `json:"kept"`
}

type forgetRequest struct {
	Session string   `json:"session"`
	Kept    []string `json:"kept"`
}

// providerKeep has provider keep what runs in session's windows, as mode says, and is what it says it
// kept.
func providerKeep(provider []string, session string, mode KeepMode, windows []ProviderWindow) ([]string, error) {
	var resp keepResponse
	if err := askProvider(provider, "keep", keepRequest{Session: session, Mode: mode, Windows: windows}, &resp); err != nil {
		return nil, err
	}
	return resp.Kept, nil
}

// providerForget has provider forget what it kept of session.
func providerForget(provider []string, session string, kept []string) error {
	return askProvider(provider, "forget", forgetRequest{Session: session, Kept: kept}, nil)
}

// ProviderSession is one of the provider's sessions, as list says it.
type ProviderSession struct {
	// ID is the provider's own reference, opaque to kspaces, given back to every other verb.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Space is the space it is in, empty for default.
	Space string `json:"space"`
	// Host is the machine it runs on, empty for this one.
	Host string `json:"host,omitempty"`
	// State is running, exited, shelved or dead.
	State string `json:"state"`
	// Busy is a command running rather than the shell at its prompt.
	Busy    bool   `json:"busy"`
	Title   string `json:"title"`
	Command string `json:"command"`
	// Cwd is where it is, which a layout saved from a window showing it starts in; empty when unknown.
	Cwd string `json:"cwd,omitempty"`
	// Status is what a program in it says of itself, OSC 7501's states: idle, working, done, blocked or
	// error, empty for nothing.
	Status string            `json:"status"`
	Tags   map[string]string `json:"tags"`
	// Windows are the ids of the windows that show it; none is detached.
	Windows []int `json:"windows"`
}

// SpaceName is the space the session is in, default for none.
func (s ProviderSession) SpaceName() string {
	if s.Space == "" {
		return defaultSpace
	}
	return s.Space
}

// Ended reports whether s has ended: exited, dead or shelved, which only removing is for.
func (s ProviderSession) Ended() bool {
	return s.State != "running"
}

// defaultSpace is the space of a session the provider puts in none, and kitty's default session's.
const defaultSpace = "default"

// ProviderState is what the provider says: its sessions, its spaces' names, and what it does.
type ProviderState struct {
	Sessions []ProviderSession `json:"sessions,omitempty"`
	Spaces   []string          `json:"spaces,omitempty"`
	Caps     Capabilities      `json:"caps,omitempty"`
}

type listRequest struct {
	Windows []ProviderWindow `json:"windows"`
}

// listSessions is provider's list for windows.
func listSessions(provider []string, windows []ProviderWindow) (*ProviderState, error) {
	var resp struct {
		Spaces   []string          `json:"spaces"`
		Sessions []ProviderSession `json:"sessions"`
	}
	if err := askProvider(provider, "list", listRequest{Windows: windows}, &resp); err != nil {
		return nil, err
	}
	return &ProviderState{Sessions: resp.Sessions, Spaces: resp.Spaces}, nil
}

// sessionRequest names one of the provider's sessions to a verb.
type sessionRequest struct {
	Session string `json:"session"`
}

// KillSession ends s, which is running.
func KillSession(provider []string, s ProviderSession) error {
	return askProvider(provider, "kill", sessionRequest{s.ID}, nil)
}

// RemoveSession discards s, which has ended.
func RemoveSession(provider []string, s ProviderSession) error {
	return askProvider(provider, "remove", sessionRequest{s.ID}, nil)
}

// openCommand is what provider says opens s in a new tab: the command, and where it starts, empty for
// kspaces to choose.
func openCommand(provider []string, s ProviderSession) ([]string, string, error) {
	var resp struct {
		Argv []string `json:"argv"`
		Cwd  string   `json:"cwd"`
	}
	if err := askProvider(provider, "open", sessionRequest{s.ID}, &resp); err != nil {
		return nil, "", err
	}
	if len(resp.Argv) == 0 {
		return nil, "", fmt.Errorf("%s open printed no command for %s", provider[0], s.Name)
	}
	return resp.Argv, resp.Cwd, nil
}

// ListNow is what provider says now, with kitty's windows as they are, for a form, which polls nothing.
func ListNow(provider []string) (*ProviderState, error) {
	caps := ProviderCapabilities(provider)
	if !caps[CapList] {
		return nil, errors.New("the session provider lists no sessions")
	}
	data, err := kitty.ListWindows()
	if err != nil {
		return nil, err
	}
	st, err := listSessions(provider, allWindows(data))
	if err != nil {
		return nil, err
	}
	st.Caps = caps
	return st, nil
}

// FindSessions are the provider's sessions by ids, in their order; one it does not list is an error.
func FindSessions(provider []string, ids []string) ([]ProviderSession, error) {
	st, err := ListNow(provider)
	if err != nil {
		return nil, err
	}
	return st.Find(ids)
}

// Find are st's sessions by ids, in their order; one st does not list is an error.
func (st *ProviderState) Find(ids []string) ([]ProviderSession, error) {
	var out []ProviderSession
	for _, id := range ids {
		i := slices.IndexFunc(st.Sessions, func(s ProviderSession) bool { return s.ID == id })
		if i < 0 {
			return nil, fmt.Errorf("the session provider has no session %s", id)
		}
		out = append(out, st.Sessions[i])
	}
	return out, nil
}

// DetachedOf are the sessions of space no window shows, on this machine, by name, as the sidebar lists
// them.
func DetachedOf(provider []string, space string) ([]ProviderSession, error) {
	st, err := ListNow(provider)
	if err != nil {
		return nil, err
	}
	for _, h := range detachedBySpace(st)[space].Hosts {
		if h.Host == "" {
			return h.Sessions, nil
		}
	}
	return nil, nil
}
