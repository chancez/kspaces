package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) addCommand() *cobra.Command {
	var yes bool
	var group string
	var window int
	cmd := &cobra.Command{
		Use:   "add [PATH]",
		Short: "Add the git repository PATH is in to the spaces, in a form with PATH filled in",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			if window != 0 && path != "" {
				return errors.New("--from adds the repository of the window's directory; give no PATH")
			}
			if window != 0 {
				return a.popupFor(window, func(cfg config.Config, st kitty.State) error {
					return spaces.AskAddSpace(cfg, st, group)
				})
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
	fromFlag(cmd, &window, "the repository of its directory")
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
	return cmd
}
