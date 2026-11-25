package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
)

// fromFlag is the --from of a command whose form a kspaces.conf action opens, as pick's is, with the
// --size of its panel.
func (a *app) fromFlag(cmd *cobra.Command, from *int, what string) {
	cmd.Flags().IntVar(from, "from", 0, "open the form for kitty window `WINDOW`, in a popup, for "+what)
	a.sizeFlag(cmd)
}

// sizeFlag is the --size of the panel a command opens, which overrides [sidebar.popup_size].
func (a *app) sizeFlag(cmd *cobra.Command) {
	cmd.Flags().Var(&a.popupSize, "size", "with --from, open a panel of `COLUMNSxLINES` rather than [sidebar.popup_size]'s")
}

// sized is cfg with --size's panel size for every popup, when it was given.
func (a *app) sized(cfg config.Config) config.Config {
	if a.popupSize != (config.PopupSize{}) {
		cfg.Sidebar.PopupSize = config.AllPopups(a.popupSize)
	}
	return cfg
}

// popupFor runs open, which opens a form in a popup, for kitty window from, as a command's --from asks.
// Run by kitty with no window, it shows an error in kitty, and a note that the popup is an overlay goes
// to kitty's log.
func (a *app) popupFor(from int, open func(config.Config, kitty.State) error) error {
	err := func() error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		st, err := kitty.LoadState(from)
		if err != nil {
			return err
		}
		return open(a.sized(cfg), st)
	}()
	if refused := (*kitty.PanelRefused)(nil); errors.As(err, &refused) {
		fmt.Fprintln(a.stderr, "kspaces:", err)
		return nil
	}
	if err != nil {
		_ = kitty.ShowError("kspaces", err.Error())
	}
	return err
}

// popupForWorktree is popupFor for a form on the space and worktree of window from's session.
func (a *app) popupForWorktree(from int, open func(config.Config, kitty.State, spaces.Space, spaces.Worktree) error) error {
	return a.popupFor(from, func(cfg config.Config, st kitty.State) error {
		s, wt, err := spaces.Current(cfg, st)
		if err != nil {
			return err
		}
		return open(cfg, st, s, wt)
	})
}

// popupForSession is popupFor for a form on sessions, window from's when there are none.
func (a *app) popupForSession(from int, sessions []string, open func(config.Config, kitty.State, ...string) error) error {
	return a.popupFor(from, func(cfg config.Config, st kitty.State) error {
		if len(sessions) > 0 {
			return open(cfg, st, sessions...)
		}
		if st.Current == "" {
			return errors.New("the current window is in no session")
		}
		return open(cfg, st, st.Current)
	})
}
