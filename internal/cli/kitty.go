package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kittyglue"
)

func (a *app) kittyCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "kitty",
		Short: "Install the files kitty needs to run kspaces: the watcher that docks the sidebars, and more",
		Args:  cobra.NoArgs,
	}
	cmd.PersistentFlags().StringVar(&dir, "dir", "", "where the files go; defaults to kspaces in kitty's config directory")
	resolve := func() (config.Config, string, error) {
		cfg, err := config.Load()
		if errors.Is(err, fs.ErrNotExist) {
			// The files do not need spaces, so they can go in before kspaces is set up.
			cfg, err = config.Parse("")
		}
		if err != nil {
			return config.Config{}, "", err
		}
		if dir != "" {
			abs, err := filepath.Abs(dir)
			return cfg, abs, err
		}
		d, err := kittyglue.DefaultDir()
		return cfg, d, err
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Write the files to the directory, replacing what is there, and say how to load them",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, d, err := resolve()
			if err != nil {
				return err
			}
			if err := kittyglue.Install(cfg, d); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "installed %s\n", d)
			fmt.Fprintf(a.stdout, "load it with this line in kitty.conf, then restart kitty:\n\n  include %s\n", filepath.Join(d, kittyglue.ConfName))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Exit 1 when the installed files are not what install would write, naming them",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, d, err := resolve()
			if err != nil {
				return err
			}
			stale, err := kittyglue.Stale(cfg, d)
			if err != nil {
				return err
			}
			if len(stale) > 0 {
				return fmt.Errorf("%s is out of date (%s): run kspaces kitty install", d, strings.Join(stale, ", "))
			}
			fmt.Fprintf(a.stdout, "%s is up to date\n", d)
			return nil
		},
	})
	return cmd
}
