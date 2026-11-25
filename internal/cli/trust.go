package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) trustCommand() *cobra.Command {
	var yes, revoke bool
	var window int
	cmd := &cobra.Command{
		Use:   "trust [PATH]",
		Short: "Trust the .kspaces.toml of the space PATH is in, the current directory's by default, after seeing what it adds in a form",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if window != 0 && len(args) > 0 {
				return errors.New("--from trusts the window's space; give no PATH")
			}
			if window != 0 {
				return a.popupForWorktree(window, func(cfg config.Config, st kitty.State, s spaces.Space, _ spaces.Worktree) error {
					return spaces.AskTrust(cfg, st, s.Root)
				})
			}
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
	a.fromFlag(cmd, &window, "the space of its session")
	cmd.MarkFlagsMutuallyExclusive("yes", "revoke")
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
	cmd.MarkFlagsMutuallyExclusive("from", "revoke")
	return cmd
}
