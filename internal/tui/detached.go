package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A space's provider sessions that no kitty window shows are listed after its rows, see spaces.WithDetached: a
// head row with how many there are, which space or a click shows and hides them under it, and a row for
// each, which enter or a click opens in a tab of the space, see spaces.OpenSession. They start hidden, and are
// shown again only by hand, as a long-lived space collects many.

// toggleDetachedAt shows or hides the detached sessions under row i, their head: a worktree's, or on the
// hosts tab a worktree's on one host, see openKey.
func (m *sidebar) toggleDetachedAt(i int) {
	r := m.rows[i]
	m.toggleDetached(openKey(r.ByHost, r.Host, r.Detached.Worktree))
}

// openKey names the worktree whose detached sessions show in detachedOpen: its session, or on the hosts
// tab its session on host, which shows and hides them there alone.
func openKey(byHost bool, host, worktree string) string {
	if !byHost {
		return worktree
	}
	return "host:" + host + ":" + worktree
}

// toggleDetached shows or hides the detached sessions under the head of the worktree named by key, see
// openKey.
func (m *sidebar) toggleDetached(space string) {
	if m.detachedOpen == nil {
		m.detachedOpen = map[string]bool{}
	}
	if m.detachedOpen[space] {
		delete(m.detachedOpen, space)
	} else {
		m.detachedOpen[space] = true
	}
	if err := spaces.SaveDetachedShown(m.detachedOpen); err != nil {
		m.failed = err
	}
	m.setRows()
}

// openDetachedCmd opens r's session in a tab of its space, see spaces.OpenTarget.
func (m *sidebar) openDetachedCmd(r spaces.DetachedRow) tea.Cmd {
	cfg, st, self, p := m.cfg, m.kitty, m.self, m.poller
	space, s := *r.Of, *r.Session
	return func() tea.Msg {
		if err := spaces.OpenSession(cfg, st, spaces.OpenTarget(st, space, s), s); err != nil {
			return failedMsg{err}
		}
		return p.Poll().View(self)
	}
}

// detachedCount is how many sessions d has, on this machine alone, or on the hosts tab under the one host
// it lists, `14 detached`, else by host, `local 3, server 9`.
func detachedCount(d spaces.Detached, byHost bool) string {
	if len(d.Hosts) == 1 && (d.Hosts[0].Host == "" || byHost) {
		return fmt.Sprintf("%d detached", len(d.Hosts[0].Sessions))
	}
	var parts []string
	for _, h := range d.Hosts {
		parts = append(parts, fmt.Sprintf("%s %d", hostName(h.Host), len(h.Sessions)))
	}
	return strings.Join(parts, ", ")
}

// hostName names a host, local for this machine.
func hostName(host string) string {
	if host == "" {
		return "local"
	}
	return host
}

// renderDetached draws row i, one of a space's detached sessions' rows, marked as its entry, indented under the
// space's rows as a linked worktree's icon is: the head, its caret, then how many; a host's name; or a
// session, its icon, which is its program's reported state's when it reports one that is not idle, its
// name, what it is doing in a word, and its title, else its command. A session that is not running is
// faint, as a session with no windows is.
func (m *sidebar) renderDetached(i int) string {
	r, width, th := m.rows[i].Detached, m.contentWidth(), m.th
	indent := []look.Span{look.NoCaret()}
	var spans []look.Span
	switch {
	case r.Head():
		caret := look.Mark(fmt.Sprintf(caretZone, i), look.Caret(!r.Expanded)...)
		spans = append(append(indent, caret...), look.Muted(detachedCount(r.All, m.rows[i].ByHost)))
	case r.HostRow:
		spans = append(indent, look.NoCaret(), look.Muted(hostName(r.Host)))
	default:
		s := *r.Session
		state := s.Status
		if state == "idle" {
			state = ""
		}
		faint := s.State != "running"
		head := append(append(indent, look.NoCaret()), look.RowIcon(look.RowOther, state, false, false)...)
		rest := []look.Span{look.Text(s.Name), look.Muted(" " + spaces.StateWords(s))}
		if s.Stale {
			rest = append(rest, look.Muted(" stale"))
		}
		if what := cmp.Or(strings.TrimSpace(s.Title), strings.TrimSpace(s.Command)); what != "" {
			rest = append(rest, look.Muted(" "+what))
		}
		spans = append(head, look.DimIf(faint, th.Cut(max(width-th.Width(head...), 0), rest...)...)...)
	}
	surface := look.Plain
	if i == m.cursor && m.navigating() && !m.selecting {
		surface = look.Cursor
	}
	return th.DrawZone(fmt.Sprintf(entryZone, i), surface, width, th.Cut(width, spans...)...)
}

// detachedKey names a row of a worktree's detached sessions, see rowKey: detached:WORKTREE for the head, by
// the worktree's session, detached:WORKTREE:HOST for a host, detached:SPACE:HOST:ID for a session, see
// sessionKey. A kitty session's
// name could be any of these, but a session kspaces names never starts detached:.
func detachedKey(r spaces.DetachedRow) string {
	switch {
	case r.HostRow:
		return "detached:" + r.Worktree + ":" + r.Host
	case r.Session != nil:
		return sessionKey(r.Of.Name, *r.Session)
	}
	return "detached:" + r.Worktree
}

// sessionKey names s, a detached session of space, as its row and the select mode's ticks do.
func sessionKey(space string, s spaces.ProviderSession) string {
	return "detached:" + space + ":" + s.Host + ":" + s.ID
}

// tickedDetached are the detached sessions ticked in the select mode, by space, then as the rows list
// them.
func (m *sidebar) tickedDetached() []spaces.ProviderSession {
	var out []spaces.ProviderSession
	for _, s := range m.tickable() {
		if m.ticked[sessionKey(s.SpaceName(), s)] {
			out = append(out, s)
		}
	}
	return out
}

// tickable are the sessions the select mode can tick, the detached ones, by space, then host, as the rows
// list them.
func (m *sidebar) tickable() []spaces.ProviderSession {
	var out []spaces.ProviderSession
	for _, space := range slices.Sorted(maps.Keys(m.detached)) {
		for _, h := range m.detached[space].Hosts {
			out = append(out, h.Sessions...)
		}
	}
	return out
}

// detachedTicks are the keys of the sessions r stands for in the select mode: a session's own, or its
// head's every session, shown or not; a host's row stands for none.
func detachedTicks(r spaces.DetachedRow) []string {
	switch {
	case r.Session != nil:
		return []string{sessionKey(r.Of.Name, *r.Session)}
	case r.Head():
		var out []string
		for _, h := range r.All.Hosts {
			for _, s := range h.Sessions {
				out = append(out, sessionKey(r.Of.Name, s))
			}
		}
		return out
	}
	return nil
}

// moveSelected opens the move form with the ticked detached sessions. Done once one of them is in another
// space or gone.
func (m *sidebar) moveSelected() tea.Cmd {
	sessions := m.tickedDetached()
	if len(sessions) == 0 {
		return m.refuse(noneToMove)
	}
	var ids, keys []string
	for _, s := range sessions {
		ids = append(ids, s.ID)
		keys = append(keys, sessionKey(s.SpaceName(), s))
	}
	return m.launch(func(cfg config.Config, st kitty.State) error { return spaces.AskMoveProviderSessions(cfg, st, ids...) },
		func(m *sidebar) bool {
			listed := map[string]bool{}
			for _, s := range m.tickable() {
				listed[sessionKey(s.SpaceName(), s)] = true
			}
			return slices.ContainsFunc(keys, func(k string) bool { return !listed[k] })
		})
}

// askDetached opens a form with ask, which acts on the provider's sessions in a popup of its own; the next
// poll shows what it did.
func (m *sidebar) askDetached(ask func(cfg config.Config, st kitty.State) error) tea.Cmd {
	m.remote = false
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := ask(cfg, st); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}

// shelveDetachedCmd has the provider shelve s, which asks nothing first: it ends s only so it can be
// brought back as it was. What the provider says when it cannot goes on the sidebar's last line.
func (m *sidebar) shelveDetachedCmd(s spaces.ProviderSession) tea.Cmd {
	provider, self, p := m.cfg.Provider, m.self, m.poller
	return func() tea.Msg {
		if err := spaces.ShelveSession(provider, s); err != nil {
			return failedMsg{err}
		}
		return p.Poll().View(self)
	}
}
