package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) newSpaceCommand() *cobra.Command {
	var dir string
	var stay bool
	cmd := &cobra.Command{
		Use:   "new-space [[HOST:]NAME]",
		Short: "Make an ad hoc space, on this machine or another host's, and switch to it",
		Long: `Make an ad hoc space: a name and a directory to start in, with no repository or config behind it,
kept by its host's kspaces server. HOST: makes it on one of [hosts]. Its one kitty session is named after
it, HOST:NAME for another host's, and the provider's sessions in it are filed under its name. Without a
NAME, or with HOST: alone, it opens a form that asks for them.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			var host, name string
			if len(args) > 0 {
				host, name = spaces.SplitCopy(args[0])
			}
			if name == "" {
				return tui.RunNewSpace(cmd.Context(), cfg, host, dir)
			}
			h, hangUp := spaces.ThisHost(), func() {}
			if host != "" {
				if h, hangUp, err = spaces.HostOf(cfg, host); err != nil {
					return err
				}
			} else if dir, err = spaces.AdHocDir(dir); err != nil {
				return err
			}
			defer hangUp()
			if err := h.NewSpace(name, dir); err != nil {
				return err
			}
			if stay {
				return nil
			}
			return spaces.OpenAdHoc(cfg, spaces.AdHocWorktree(host, name, dir))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "the directory its windows start in, the host's home directory by default; another host's as that host has it")
	cmd.Flags().BoolVar(&stay, "background", false, "make it without switching to it")
	return cmd
}
