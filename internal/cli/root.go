// Package cli is kspaces' command line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/server"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

// app holds what the commands write to.
type app struct {
	stdout io.Writer
	stderr io.Writer
	// The --size of the panel the command run opens, zero when not given.
	popupSize config.PopupSize
	// version is kspaces' own, see versionOf.
	version string
}

// Execute runs kspaces with os.Args and returns the exit code.
func Execute(ctx context.Context) int {
	a := newApp(os.Stdout, os.Stderr)
	cmd := a.rootCommand()
	cmd.SetArgs(os.Args[1:])
	// Here, not in rootCommand, so tests running commands log nothing.
	role := "sidebar"
	if sub, _, err := cmd.Find(os.Args[1:]); err == nil && sub != cmd {
		role = sub.Name()
	}
	logs.Init(role)
	logs.Debug("start", "args", os.Args[1:])
	// Every command but the server's own asks it for the host data, starting it if need be.
	if role != "server" {
		if err := useServer(a.version); err != nil {
			logs.Error("finding the kspaces server", err)
		}
	}
	// A config that cannot load leaves git's default; the command loading it says why.
	if cfg, err := config.Load(); err == nil {
		git.SetTimeouts(cfg.Git.Timeout, cfg.Git.FetchTimeout)
	}
	if err := cmd.ExecuteContext(ctx); err != nil {
		logs.Error("exit", err)
		fmt.Fprintln(a.stderr, "kspaces:", err)
		return 1
	}
	return 0
}

// newApp is kspaces writing to stdout and stderr.
func newApp(stdout, stderr io.Writer) *app {
	info, ok := debug.ReadBuildInfo()
	return &app{stdout: stdout, stderr: stderr, version: versionOf(version, info, ok)}
}

// useServer has this process ask this host's server for the host data.
func useServer(version string) error {
	path, err := server.SocketPath()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	spaces.UseHost(server.NewLocal(path, exe, version))
	return nil
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kspaces",
		Short: "A kitty sidebar for repositories and their git worktrees",
		Long: `kspaces shows repositories ("spaces") and their git worktrees in a kitty sidebar, and switches
kitty to a worktree's session, creating the session the first time.

Run without a command to show the sidebar. The config is $XDG_CONFIG_HOME/kspaces/kspaces.toml, by
default ~/.config/kspaces/kspaces.toml, then every .toml file in conf.d beside it, by name, then
kspaces.ENV.toml beside it for each of $KSPACES_ENV, by default the host's name, and kspaces.local.toml.`,
		Args:          cobra.NoArgs,
		Version:       a.version,
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
		a.recentCommand(),
		a.newCommand(),
		a.runCommand(),
		a.layoutCommand(),
		a.removeCommand(),
		a.removeSpaceCommand(),
		a.cleanupCommand(),
		a.closeCommand(),
		a.suspendCommand(),
		a.forgetCommand(),
		a.resumeCommand(),
		a.sessionsCommand(),
		a.trustCommand(),
		a.replyCommand(),
		a.errorCommand(),
		a.addCommand(),
		a.kittyCommand(),
		a.serverCommand(),
	)
	return root
}
