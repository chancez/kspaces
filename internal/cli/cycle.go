package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/spaces"
)

// cycleCommand is next or prev: they act on the window that has the keyboard, so they suit a kitty
// mapping.
func (a *app) cycleCommand(name, short string, delta int) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return spaces.Cycle(delta) },
	}
}
