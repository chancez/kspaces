package spaces

import (
	"cmp"
	"slices"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// Pane is a kitty window as the picker lists it: where it is and what runs in it.
type Pane struct {
	Window int
	// Its session, empty for a window in none.
	Session string
	Title   string
	Tab     string
	// What runs in it, as a layout's capture detects it: empty for the shell at its prompt.
	Program string
	Cwd     string
	// kitty's last_focused_at, in seconds.
	FocusedAt float64
}

// Panes are kitty's windows but kspaces' own, the most recently focused first.
func Panes(cfg config.Config) ([]Pane, error) {
	tabs, err := kitty.ListTabs()
	if err != nil {
		return nil, err
	}
	return panesOf(tabs, providerShows(cfg.Provider, tabs)), nil
}

// panesOf is Panes from tabs, with the provider's sessions by the windows that show them.
func panesOf(tabs []kitty.FullTab, shows map[int]ProviderSession) []Pane {
	var out []Pane
	for _, tab := range tabs {
		for _, w := range tab.Windows {
			if w.Own() {
				continue
			}
			var shown *ProviderSession
			if s, ok := shows[w.ID]; ok {
				shown = &s
			}
			prog, cwd := detect(w, shown)
			out = append(out, Pane{Window: w.ID, Session: w.SessionName, Title: w.Title, Tab: tab.Title, Program: prog, Cwd: cwd, FocusedAt: w.LastFocusedAt})
		}
	}
	slices.SortStableFunc(out, func(a, b Pane) int { return cmp.Compare(b.FocusedAt, a.FocusedAt) })
	return out
}
