package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) addCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add [PATH]",
		Short: "Add the git repository PATH is in to the spaces, in a form with PATH filled in",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			if !yes {
				return tui.RunAddSpace(cmd.Context(), path)
			}
			if path == "" {
				return errors.New("--yes needs a PATH")
			}
			return tui.AddSpace(path)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "add without asking")
	return cmd
}
