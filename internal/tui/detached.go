package tui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
)

// openSessionCmd opens s, a provider session of space no window shows, in a tab of its space, see
// spaces.OpenTarget.
func (m *sidebar) openSessionCmd(space spaces.Space, s spaces.ProviderSession) tea.Cmd {
	cfg, st, self, p := m.cfg, m.kitty, m.self, m.poller
	return func() tea.Msg {
		if err := spaces.OpenSession(cfg, st, spaces.OpenTarget(st, space, s), s); err != nil {
			return failedMsg{err}
		}
		return p.Poll().View(self)
	}
}

// hostName names a host, local for this machine.
func hostName(host string) string {
	if host == "" {
		return localName
	}
	return host
}

// localName is this machine's name, its short hostname, as cm names it by default.
var localName = func() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "local"
	}
	name, _, _ = strings.Cut(name, ".")
	return strings.ToLower(name)
}()
