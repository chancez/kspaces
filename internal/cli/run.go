package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) runCommand() *cobra.Command {
	var (
		yes        bool
		background bool
		open       string
		inputs     []string
		window     int
	)
	cmd := &cobra.Command{
		Use:   "run NAME [PATH...] | run [PATH...] -- ARGS...",
		Short: "Run the configured command NAME in the worktrees at PATH, the current directory's by default, after filling in its inputs in a form, or the command line ARGS without one",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				return a.runArgs(args[:dash], args[dash:], open, background, window != 0 || yes || len(inputs) > 0)
			}
			if window != 0 && len(args) > 1 {
				return fmt.Errorf("--from runs the command in the window's worktree; give no PATH")
			}
			if window != 0 {
				return a.popupForWorktree(window, func(cfg config.Config, st kitty.State, s spaces.Space, wt spaces.Worktree) error {
					if _, ok := s.ConfigOf(cfg).Command(args[0]); !ok {
						return fmt.Errorf("no command %s in the config", args[0])
					}
					return spaces.AskRunCommand(cfg, st, args[0], wt.Path)
				})
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dirs := args[1:]
			if len(dirs) == 0 {
				dirs = []string{"."}
			}
			values, err := parseInputs(inputs)
			if err != nil {
				return err
			}
			if yes {
				return tui.RunCommandNow(cfg, dirs, args[0], values, open, background)
			}
			if background {
				return fmt.Errorf("--background needs --yes")
			}
			return tui.RunCommandForm(cmd.Context(), cfg, dirs, args[0], values, open)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "run without the form; a required input not given is an error")
	cmd.Flags().BoolVar(&background, "background", false, "with --yes or a command after --, leave the focus where it is rather than switching to the session")
	cmd.Flags().StringVar(&open, "open", "", "where to open it in a session with windows, for this run: "+strings.Join(config.Opens, ", "))
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "an input's value, as NAME=VALUE; repeat for more")
	a.fromFlag(cmd, &window, "the worktree of its session")
	for _, f := range []string{"yes", "background", "open", "input"} {
		cmd.MarkFlagsMutuallyExclusive("from", f)
	}
	return cmd
}

// runArgs runs argv, the words after --, in the worktrees at dirs, the current directory's by default.
// It has no form and no inputs, so configured says one of those was asked for.
func (a *app) runArgs(dirs, argv []string, open string, background, configured bool) error {
	switch {
	case len(argv) == 0:
		return fmt.Errorf("no command after --")
	case configured:
		return fmt.Errorf("a command after -- takes no --from, --yes or --input")
	}
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return tui.RunArgsNow(cfg, dirs, argv, open, background)
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
