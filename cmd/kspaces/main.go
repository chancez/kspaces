// Command kspaces shows repositories ("spaces") and their git worktrees in a kitty sidebar, and switches
// kitty to a worktree's session, creating the session the first time.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/chancez/kspaces/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Execute(ctx)
	stop()
	os.Exit(code)
}
