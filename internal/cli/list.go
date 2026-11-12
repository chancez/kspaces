package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Print every space, its worktrees and their session names, and a space's own config when it is untrusted or broken",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			for _, root := range cfg.Spaces {
				s := spaces.ReadSpace(cfg, root)
				if s.Err != nil {
					fmt.Fprintf(a.stdout, "%s\t%s\terror: %v\n", s.Name, s.Root, s.Err)
					continue
				}
				for _, wt := range s.Worktrees {
					fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", wt.Session, wt.Path, wt.Branch)
				}
				switch {
				case s.Untrusted:
					fmt.Fprintf(a.stdout, "%s\t%s\tuntrusted config: kspaces trust %s\n", s.Name, s.ConfigPath(), s.Root)
				case s.ConfigErr != nil:
					fmt.Fprintf(a.stdout, "%s\t%s\terror: %v\n", s.Name, s.Root, s.ConfigErr)
				}
			}
			return nil
		},
	}
}
