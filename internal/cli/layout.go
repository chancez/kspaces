package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) layoutCommand() *cobra.Command {
	var (
		yes    bool
		inputs []string
	)
	cmd := &cobra.Command{
		Use:   "layout NAME [PATH]",
		Short: "Open the configured layout NAME's tabs in the session of the worktree at PATH, the current directory's by default, after filling in its inputs in a form",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 1 {
				dir = args[1]
			}
			values, err := parseInputs(inputs)
			if err != nil {
				return err
			}
			if yes {
				return tui.OpenLayoutNow(cfg, dir, args[0], values)
			}
			return tui.OpenLayoutForm(cmd.Context(), cfg, dir, args[0], values)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "open without the form; a required input not given is an error")
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "an input's value, as NAME=VALUE; repeat for more")
	return cmd
}
