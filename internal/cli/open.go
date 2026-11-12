package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) openCommand() *cobra.Command {
	var noLayout bool
	cmd := &cobra.Command{
		Use:   "open [PATH]",
		Short: "Switch to the session of the worktree at PATH, the current directory's by default, creating it if needed",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return spaces.OpenPath(cfg, dir, spaces.OpenOptions{NoLayout: noLayout})
		},
	}
	cmd.Flags().BoolVar(&noLayout, "no-layout", false, "open a session that has no windows and nothing saved to restore with one tab, not its default layout")
	return cmd
}
