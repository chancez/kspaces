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

// The keeper is the program config's keeper names, which keeps what runs in a session's windows when
// KeepSession closes them, such as the multiplexer sessions a launch wrapper runs each window in. kspaces
// knows nothing of what it keeps: it runs the keeper with a verb, writes a JSON request to its stdin, and
// for keep reads a JSON response from its stdout. A keeper that fails exits non-zero, its stderr the error.
//
//	KEEPER keep    {"session": S, "mode": "close"|"suspend", "windows": [keeperWindow...]}  ->  {"kept": [HANDLE...]}
//	KEEPER forget  {"session": S, "kept": [HANDLE...]}
//
// The handles are the keeper's own, opaque to kspaces, which lists them in keptFile and gives them back to
// forget when the session is forgotten or its worktree removed. Opening the session again needs nothing of
// the keeper: its windows come back from the session's file, attached to whatever the keeper kept.

// keeperWindow is one of a session's windows as the keeper is told of it, what it needs to know which of
// them it keeps: a launch wrapper's user variable, or the program the window runs.
type keeperWindow struct {
	ID int `json:"id"`
	// The window's program, as `kitten @ ls` reports it.
	Cmdline []string `json:"cmdline"`
	// The programs in the foreground of its terminal, each a command line.
	Foreground [][]string        `json:"foreground"`
	UserVars   map[string]string `json:"user_vars"`
	Cwd        string            `json:"cwd"`
}

type keepRequest struct {
	Session string         `json:"session"`
	Mode    KeepMode       `json:"mode"`
	Windows []keeperWindow `json:"windows"`
}

type keepResponse struct {
	Kept []string `json:"kept"`
}

type forgetRequest struct {
	Session string   `json:"session"`
	Kept    []string `json:"kept"`
}

// keeperWindows are the windows of session in tabs, kspaces' own left out, as the keeper is told of them.
func keeperWindows(tabs []kitty.FullTab, session string) []keeperWindow {
	windows := []keeperWindow{}
	for _, tab := range tabs {
		for _, w := range tab.Windows {
			if w.SessionName != session || w.Own() {
				continue
			}
			kw := keeperWindow{ID: w.ID, Cmdline: w.Cmdline, Foreground: [][]string{}, UserVars: w.UserVars, Cwd: w.Cwd}
			for _, p := range w.ForegroundProcesses {
				kw.Foreground = append(kw.Foreground, p.Cmdline)
			}
			if kw.UserVars == nil {
				kw.UserVars = map[string]string{}
			}
			windows = append(windows, kw)
		}
	}
	return windows
}

// keeperKeep has keeper keep what runs in session's windows, as mode says, and is what it says it kept.
func keeperKeep(keeper []string, session string, mode KeepMode, windows []keeperWindow) ([]string, error) {
	out, err := runKeeper(keeper, "keep", keepRequest{Session: session, Mode: mode, Windows: windows})
	if err != nil {
		return nil, err
	}
	var resp keepResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parsing what keeper %s kept: %w", keeper[0], err)
	}
	return resp.Kept, nil
}

// keeperForget has keeper forget what it kept of session.
func keeperForget(keeper []string, session string, kept []string) error {
	_, err := runKeeper(keeper, "forget", forgetRequest{Session: session, Kept: kept})
	return err
}

func runKeeper(keeper []string, verb string, req any) ([]byte, error) {
	in, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(keeper[0], append(keeper[1:], verb)...)
	cmd.Stdin = bytes.NewReader(in)
	out, err := logs.Output(cmd)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(bytes.TrimSpace(exit.Stderr)) > 0 {
			err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
		}
		return nil, fmt.Errorf("keeper %s %s: %w", keeper[0], verb, err)
	}
	return out, nil
}
