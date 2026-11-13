package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) errorCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "error FILE",
		Short:  "Show the error in FILE in full, in a form, and delete FILE: what a click on the sidebar's error opens",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.RunShowError(cmd.Context(), args[0])
		},
	}
}
