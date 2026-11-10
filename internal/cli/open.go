package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) openCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open PATH",
		Short: "Switch to the session of the worktree at PATH, creating it if needed",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return spaces.OpenPath(cfg, args[0])
		},
	}
}
