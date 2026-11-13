package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) replyCommand() *cobra.Command {
	var noEnter bool
	cmd := &cobra.Command{
		Use:   "reply WINDOW_ID [TEXT]",
		Short: "Type TEXT and Enter into a kitty window without switching to it, or write it in a form that shows the window's screen",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			window, err := strconv.Atoi(args[0])
			if err != nil || window <= 0 {
				return fmt.Errorf("window id %q is not a number above 0", args[0])
			}
			var text *string
			if len(args) > 1 {
				text = &args[1]
			}
			return tui.RunReply(cmd.Context(), window, text, noEnter)
		},
	}
	cmd.Flags().BoolVar(&noEnter, "no-enter", false, "send TEXT as it is, with no Enter after it, as for a single key")
	return cmd
}
