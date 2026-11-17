package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) addCommand() *cobra.Command {
	var yes bool
	var group string
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
				return tui.RunAddSpace(cmd.Context(), path, group)
			}
			if path == "" {
				return errors.New("--yes needs a PATH")
			}
			return tui.AddSpace(path, group)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "add without asking")
	cmd.Flags().StringVar(&group, "group", "", "put the space in this group, in the config file that lists its spaces")
	return cmd
}
