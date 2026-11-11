package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) runCommand() *cobra.Command {
	var (
		yes    bool
		open   string
		inputs []string
	)
	cmd := &cobra.Command{
		Use:   "run NAME [PATH]",
		Short: "Run the configured command NAME in the worktree at PATH, the current directory's by default, after filling in its inputs in a form",
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
				return tui.RunCommandNow(cfg, dir, args[0], values, open)
			}
			return tui.RunCommandForm(cmd.Context(), cfg, dir, args[0], values, open)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "run without the form; a required input not given is an error")
	cmd.Flags().StringVar(&open, "open", "", "where to open it in a session with windows, for this run: "+strings.Join(config.Opens, ", "))
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "an input's value, as NAME=VALUE; repeat for more")
	return cmd
}

// parseInputs is the --input flags, NAME=VALUE each, by name.
func parseInputs(flags []string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range flags {
		name, value, ok := strings.Cut(f, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--input %q is not NAME=VALUE", f)
		}
		out[name] = value
	}
	return out, nil
}
