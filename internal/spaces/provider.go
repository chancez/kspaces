package spaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

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
