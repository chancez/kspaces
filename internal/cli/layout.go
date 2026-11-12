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
	cmd.AddCommand(a.layoutSaveCommand())
	return cmd
}

func (a *app) layoutSaveCommand() *cobra.Command {
	var tab bool
	cmd := &cobra.Command{
		Use:   "save [NAME] [PATH]",
		Short: "Save the tabs of the session of the worktree at PATH, the current directory's by default, as the layout NAME in the config, after choosing what each pane runs in a form",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name, dir := "", "."
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				dir = args[1]
			}
			return tui.SaveLayoutForm(cmd.Context(), cfg, dir, name, tab)
		},
	}
	cmd.Flags().BoolVar(&tab, "tab", false, "save this tab alone: the one kspaces runs in, if the session's, else the session's most recently focused")
	return cmd
}
