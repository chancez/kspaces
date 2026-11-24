package git

import (
	"context"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/logs"
)

// DefaultTimeout is how long git may take to answer what kspaces asks when the config sets no timeout,
// and DefaultFetchTimeout how long a fetch may take.
const (
	DefaultTimeout      = 30 * time.Second
	DefaultFetchTimeout = 2 * time.Minute
)

// killDelay is how long git has to exit after it is asked to stop before it is killed, and its output
// given up on, as when a process it started still holds it open.
var killDelay = 5 * time.Second

var timeout, fetchTimeout atomic.Int64

func init() { SetTimeouts(DefaultTimeout, DefaultFetchTimeout) }

// SetTimeouts sets how long git may take to answer what kspaces asks, and to fetch, each 0 for no limit.
func SetTimeouts(ask, fetch time.Duration) {
	timeout.Store(int64(ask))
	fetchTimeout.Store(int64(fetch))
}

// Ask runs git with args, a question that changes nothing, such as git status, logged as run does, and
// stopped once it runs past the timeout, see SetTimeouts and limited.
func Ask(run func(*exec.Cmd) ([]byte, error), args ...string) ([]byte, error) {
	cmd, cancel := limited(time.Duration(timeout.Load()), args...)
	defer cancel()
	return run(cmd)
}

// query is Ask with an exit status taken for an answer, see logs.Query.
func query(args ...string) ([]byte, error) { return Ask(logs.Query, args...) }

// limited is git with args, stopped once it runs for longer than d, 0 for no limit: asked to with
// SIGTERM, so it removes the lock files it holds, then killed killDelay later. cancel is to be called once
// it has run.
func limited(d time.Duration, args ...string) (cmd *exec.Cmd, cancel context.CancelFunc) {
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if d > 0 {
		ctx, cancel = context.WithTimeout(ctx, d)
	}
	cmd = exec.CommandContext(ctx, "git", args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = killDelay
	return cmd, cancel
}
