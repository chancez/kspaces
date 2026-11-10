package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kittyglue"
	"github.com/chancez/kspaces/internal/tui/look"
)

// glueDirEnv names the directory the kitty watcher that started the sidebar was installed in. The watcher
// sets it, so a sidebar knows which installed files are its own.
const glueDirEnv = "KSPACES_KITTY_DIR"

// glueMsg is what the sidebar says about its kitty files.
type glueMsg struct{ notice look.Span }

// checkGlue compares the kitty files the watcher was installed with to what this kspaces would install, as
// installing another kspaces leaves them as they were. With [kitty] auto_install it rewrites them, and kitty
// loads them when it restarts; else it says to run the install. One sidebar per tab does this at once, which
// install's atomic writes allow. Short, for the last line of a narrow sidebar.
func checkGlue() tea.Msg {
	dir := os.Getenv(glueDirEnv)
	if dir == "" {
		return nil
	}
	// A config that does not load is the snapshot's to report.
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	stale, err := kittyglue.Stale(cfg, dir)
	switch {
	case err != nil:
		return glueMsg{look.Error("kitty files: " + err.Error())}
	case len(stale) == 0:
		return nil
	case !cfg.Kitty.AutoInstall:
		return glueMsg{look.Error("kitty files out of date")}
	}
	if err := kittyglue.Install(cfg, dir); err != nil {
		return glueMsg{look.Error("kitty files: " + err.Error())}
	}
	return glueMsg{look.Busy("restart kitty for new files")}
}
