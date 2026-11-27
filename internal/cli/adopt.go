package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) adoptCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "adopt SESSION",
		Short:  "Open the provider's sessions of kitty session SESSION's worktree that no window shows, each in a tab there: what opening it runs",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return spaces.AdoptOrphans(cfg, args[0])
		},
	}
}
