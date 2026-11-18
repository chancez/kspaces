// Package logs is kspaces' log, $XDG_STATE_HOME/kspaces/kspaces.log, one file every kspaces process
// appends to, a line each: what was asked of it, as creating a worktree, with how long it took, and
// every program it ran that failed. With KSPACES_LOG=debug every program run is logged, with how long it
// took, so a slow sidebar says what it waited on: by default that is too many lines where every run is
// slow, as under a virus scanner. Until Init, as in tests, nothing is.
package logs

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxSize is how large the log grows before it is moved to kspaces.log.1, the one before it dropped.
const maxSize = 5 << 20

var logger = slog.New(slog.DiscardHandler)

// Init logs from here on to the log file, each line with the process's pid and role, what it runs as,
// such as sidebar or new. A log that cannot be opened logs nothing, which is no reason to fail.
func Init(role string) {
	path, err := Path()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("KSPACES_LOG"), "debug") {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(&file{path: path}, &slog.HandlerOptions{Level: level})
	logger = slog.New(h).With("pid", os.Getpid(), "role", role)
}

// Path is where the log is.
func Path() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "kspaces", "kspaces.log"), nil
}

// Action logs what was asked of kspaces, as creating a worktree, with how long it took since start, and
// the error it ended in, if any.
func Action(msg string, start time.Time, err error, attrs ...any) {
	attrs = append(attrs, "dur", time.Since(start).Round(time.Millisecond))
	if err != nil {
		logger.Error(msg, append(attrs, "err", err)...)
		return
	}
	logger.Info(msg, attrs...)
}

// Error logs an error shown to the user.
func Error(msg string, err error) { logger.Error(msg, "err", err) }

// Debug logs what is only worth seeing when KSPACES_LOG=debug.
func Debug(msg string, attrs ...any) { logger.Debug(msg, attrs...) }

// Took logs at debug what was done since start, as a refresh of the sidebar, with how long it took.
func Took(msg string, start time.Time, attrs ...any) {
	logger.Debug(msg, append(attrs, "dur", time.Since(start).Round(time.Millisecond))...)
}

// Output is cmd.Output, logged: as an error when it failed, with what it said on stderr, else at debug.
func Output(cmd *exec.Cmd) ([]byte, error) {
	start := time.Now()
	out, err := cmd.Output()
	logRun(cmd, start, err, false)
	return out, err
}

// CombinedOutput is cmd.CombinedOutput, logged as Output logs it, with what it printed as the error.
func CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	logRun(cmd, start, err, false)
	return out, err
}

// Query is cmd.Output for a program whose exit status is an answer, as git config --get says a key is
// not set: an exit status is logged as a success is, other failures, as one that cannot start, as errors.
func Query(cmd *exec.Cmd) ([]byte, error) {
	start := time.Now()
	out, err := cmd.Output()
	logRun(cmd, start, err, true)
	return out, err
}

// logRun logs cmd, run since start and ended in err; with answers, an exit status is no failure.
func logRun(cmd *exec.Cmd, start time.Time, err error, answers bool) {
	attrs := []any{"cmd", commandLine(cmd), "dur", time.Since(start).Round(time.Millisecond)}
	if cmd.Dir != "" {
		attrs = append(attrs, "dir", cmd.Dir)
	}
	var ee *exec.ExitError
	exited := errors.As(err, &ee)
	if err != nil && !(answers && exited) {
		if exited && len(ee.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		logger.Error("run", append(attrs, "err", err)...)
		return
	}
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	logger.Debug("run", attrs...)
}

// commandLine is cmd's program and arguments, an argument over 200 bytes, such as a prompt, cut short.
func commandLine(cmd *exec.Cmd) string {
	words := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		if i == 0 {
			a = filepath.Base(a)
		}
		if len(a) > 200 {
			a = a[:200] + "..."
		}
		words[i] = a
	}
	return strings.Join(words, " ")
}

// file appends to the log at path, opened at the first write, moving it aside once it is past maxSize.
// Several processes append to it at once, each line one write.
type file struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

func (w *file) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		// Another process may have moved it aside: follow the path, not the file.
		if info, err := os.Stat(w.path); err != nil || !os.SameFile(info, w.stat()) {
			w.f.Close()
			w.f = nil
		}
	}
	if w.f != nil {
		if info := w.stat(); info != nil && info.Size() > maxSize {
			os.Rename(w.path, w.path+".1")
			w.f.Close()
			w.f = nil
		}
	}
	if w.f == nil {
		f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		w.f = f
	}
	return w.f.Write(p)
}

// stat is the open file's, nil when it cannot say.
func (w *file) stat() os.FileInfo {
	info, err := w.f.Stat()
	if err != nil {
		return nil
	}
	return info
}
