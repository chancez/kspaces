package git

import (
	"context"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/logs"
)

// DefaultTimeout is how long git may take to answer what kspaces asks when the config sets no timeout.
const DefaultTimeout = 30 * time.Second

// killDelay is how long git has to exit after it is asked to stop before it is killed, and its output
// given up on, as when a process it started still holds it open.
var killDelay = 5 * time.Second

var timeout atomic.Int64

func init() { timeout.Store(int64(DefaultTimeout)) }

// SetTimeout sets how long git may take to answer what kspaces asks, 0 for no limit.
func SetTimeout(d time.Duration) { timeout.Store(int64(d)) }

// Ask runs git with args, a question that changes nothing, such as git status, logged as run does. git is
// stopped once it runs past the timeout, see SetTimeout: asked to with SIGTERM, so it removes the lock
// files it holds, then killed killDelay later.
func Ask(run func(*exec.Cmd) ([]byte, error), args ...string) ([]byte, error) {
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if d := time.Duration(timeout.Load()); d > 0 {
		ctx, cancel = context.WithTimeout(ctx, d)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = killDelay
	return run(cmd)
}

// query is Ask with an exit status taken for an answer, see logs.Query.
func query(args ...string) ([]byte, error) { return Ask(logs.Query, args...) }
