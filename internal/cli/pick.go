package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) pickCommand() *cobra.Command {
	var from int
	cmd := &cobra.Command{
		Use:   "pick",
		Short: "Pick a space, worktree or status entry to switch to, meant for a kitty popup",
		Long: `Pick a space, worktree or status entry to switch to, meant for a kitty popup.

With --from it opens the picker for kitty window WINDOW, in an overlay over it or in a panel as the config's
[sidebar] popup says, rather than running it here: what kspaces.conf's kspaces_pick does for a panel, as only
kspaces can fall back to an overlay where kitty has no panels.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == 0 {
				// The picker loads the config itself, so an error shows in its UI.
				return tui.RunPicker(cmd.Context())
			}
			// A config that does not load opens the overlay, where the picker says what is wrong with it.
			cfg, _ := config.Load()
			err := spaces.LaunchPicker(cfg, from)
			// Run by kitty with no window, the note goes to kitty's log.
			if refused := (*kitty.PanelRefused)(nil); errors.As(err, &refused) {
				fmt.Fprintln(a.stderr, "kspaces:", err)
				return nil
			}
			return err
		},
	}
	cmd.Flags().IntVar(&from, "from", 0, "open the picker for kitty window `WINDOW`, in a popup")
	return cmd
}
