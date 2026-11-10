package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) sidebarCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sidebar",
		Short: "Show the spaces sidebar (the default), meant to run in a kitty dock",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return tui.RunSidebar(cmd.Context()) },
	}
}
