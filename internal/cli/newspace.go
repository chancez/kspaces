package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) newSpaceCommand() *cobra.Command {
	var dir string
	var stay bool
	cmd := &cobra.Command{
		Use:   "new-space [HOST:]NAME",
		Short: "Make an ad hoc space, on this machine or another host's, and switch to it",
		Long: `Make an ad hoc space: a name and a directory to start in, with no repository or config behind it,
kept by its host's kspaces server. HOST: makes it on one of [hosts]. Its one kitty session is named after
it, HOST:NAME for another host's, and the provider's sessions in it are filed under its name.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			host, name := spaces.SplitCopy(args[0])
			h, hangUp := spaces.ThisHost(), func() {}
			if host != "" {
				if h, hangUp, err = spaces.HostOf(cfg, host); err != nil {
					return err
				}
			} else if dir != "" {
				// This machine's directory, as kitty opens the space's windows here in it.
				if dir, err = filepath.Abs(spaces.ExpandHome(dir)); err != nil {
					return err
				}
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					return fmt.Errorf("%s is not a directory", dir)
				}
			}
			defer hangUp()
			if err := h.NewSpace(name, dir); err != nil {
				return err
			}
			if stay {
				return nil
			}
			wt := spaces.Worktree{Session: name, Path: dir}
			if host != "" {
				wt = spaces.Worktree{Session: spaces.CopyName(host, name), Host: host, Path: dir}
			}
			return spaces.OpenAdHoc(cfg, wt)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "the directory its windows start in, the host's home directory by default; another host's as that host has it")
	cmd.Flags().BoolVar(&stay, "background", false, "make it without switching to it")
	return cmd
}
