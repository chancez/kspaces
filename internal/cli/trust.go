package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) trustCommand() *cobra.Command {
	var yes, revoke bool
	cmd := &cobra.Command{
		Use:   "trust [PATH]",
		Short: "Trust the .kspaces.toml of the space PATH is in, the current directory's by default, after seeing what it adds in a form",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return tui.RunTrust(cmd.Context(), cfg, path, yes, revoke)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "trust it as it is without asking")
	cmd.Flags().BoolVar(&revoke, "revoke", false, "forget it, so it is untrusted until trusted again")
	cmd.MarkFlagsMutuallyExclusive("yes", "revoke")
	return cmd
}
