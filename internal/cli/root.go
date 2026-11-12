// Package cli is kspaces' command line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

// app holds what the commands write to.
type app struct {
	stdout io.Writer
	stderr io.Writer
}

// Execute runs kspaces with os.Args and returns the exit code.
func Execute(ctx context.Context) int {
	a := &app{stdout: os.Stdout, stderr: os.Stderr}
	cmd := a.rootCommand()
	cmd.SetArgs(os.Args[1:])
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(a.stderr, "kspaces:", err)
		return 1
	}
	return 0
}

func (a *app) rootCommand() *cobra.Command {
	info, ok := debug.ReadBuildInfo()
	root := &cobra.Command{
		Use:   "kspaces",
		Short: "A kitty sidebar for repositories and their git worktrees",
		Long: `kspaces shows repositories ("spaces") and their git worktrees in a kitty sidebar, and switches
kitty to a worktree's session, creating the session the first time.

Run without a command to show the sidebar. The config is $XDG_CONFIG_HOME/kspaces/config.toml, by
default ~/.config/kspaces/config.toml.`,
		Args:          cobra.NoArgs,
		Version:       versionOf(version, info, ok),
		SilenceUsage:  true,
		SilenceErrors: true,
		// The sidebar loads the config itself, so an error shows in its UI.
		RunE: func(cmd *cobra.Command, _ []string) error { return tui.RunSidebar(cmd.Context()) },
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.AddCommand(
		a.sidebarCommand(),
		a.pickCommand(),
		a.listCommand(),
		a.openCommand(),
		a.cycleCommand("next", "Switch to the next session with windows open, in sidebar order", 1),
		a.cycleCommand("prev", "Switch to the previous session with windows open, in sidebar order", -1),
		a.newCommand(),
		a.runCommand(),
		a.layoutCommand(),
		a.removeCommand(),
		a.cleanupCommand(),
		a.closeCommand(),
		a.forgetCommand(),
		a.trustCommand(),
		a.addCommand(),
		a.kittyCommand(),
	)
	return root
}
