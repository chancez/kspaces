package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) openCommand() *cobra.Command {
	var noLayout bool
	cmd := &cobra.Command{
		Use:   "open [PATH | HOST:[SESSION]]",
		Short: "Switch to the session of the worktree at PATH, the current directory's by default, creating it if needed",
		Long: `Switch to the session of the worktree at PATH, the current directory's by default, creating it if needed.

HOST:SESSION opens another host's copy of the worktree whose session there is SESSION, such as white:gr@dev
or white:homelab, in a kitty session of that name, its windows on that host; HOST: alone is a shell there.
HOST is one of [hosts] in the config.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			opts := spaces.OpenOptions{NoLayout: noLayout}
			if host, session := spaces.SplitCopy(dir); host != "" {
				if _, ok := cfg.Hosts[host]; ok {
					return spaces.OpenCopy(cfg, host, session, opts)
				}
			}
			return spaces.OpenPath(cfg, dir, opts)
		},
	}
	cmd.Flags().BoolVar(&noLayout, "no-layout", false, "open a session that has no windows and nothing saved to restore with one tab, not its default layout")
	return cmd
}
