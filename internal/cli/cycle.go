package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/spaces"
)

// cycleCommand is next or prev: they act on the window that has the keyboard, so they suit a kitty
// mapping.
func (a *app) cycleCommand(name, short string, delta int) *cobra.Command {
	var attention bool
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if attention {
				return spaces.Attend(delta)
			}
			return spaces.Cycle(delta)
		},
	}
	cmd.Flags().BoolVar(&attention, "attention", false,
		"step through the windows whose program is blocked, failed or done instead, most urgent first")
	return cmd
}
