package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

// sessionsCommand is the forms that act on the session provider's sessions no window shows, which the
// sidebar opens in popups. Sessions are given by the provider's ids.
func (a *app) sessionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Act on the session provider's sessions no window shows, in forms the sidebar opens",
	}
	end := func(use, short string) *cobra.Command {
		var yes bool
		c := &cobra.Command{
			Use:   use + " ID...",
			Short: short,
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return tui.RunEndSessions(cmd.Context(), cfg, args, yes)
			},
		}
		c.Flags().BoolVar(&yes, "yes", false, "without asking")
		return c
	}
	var to string
	move := &cobra.Command{
		Use:   "move ID...",
		Short: "Move sessions to another space, chosen in a form; --to SPACE without asking",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunMoveSessions(cmd.Context(), cfg, args, to)
		},
	}
	move.Flags().StringVar(&to, "to", "", "the space to move them to, which the provider creates if it must")
	cmd.AddCommand(
		move,
		end("kill", "Kill running sessions, after confirming in a form; an ended one is removed"),
		end("remove", "Remove ended sessions, after confirming in a form; a running one is killed"),
		&cobra.Command{
			Use:   "cleanup SPACE",
			Short: "Kill and remove the sessions of SPACE no window shows, chosen in a form",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				return tui.RunCleanupSessions(cmd.Context(), cfg, args[0])
			},
		},
	)
	return cmd
}
