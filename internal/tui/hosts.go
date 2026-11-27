package tui

import (
	"fmt"
	"slices"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// With other hosts the config names, the spaces list is by host: each host's row, then what the list
// would have of that host, see spaces.HostViews, this machine first. Each host's spaces are its copies, see
// docs/stories.md, and its detached sessions are under them alone.

// byHost is whether the spaces list is by host: when the config names other hosts.
func (m *sidebar) byHost() bool {
	return len(m.remotes) > 0
}

// hostRows are the rows by host: for each host, its row, then the rows the list's filters keep of what the
// host has, see listRows, left out while the host is folded. A host whose rows a filter leaves none of is
// left out whole, and a blank row goes between hosts.
func (m *sidebar) hostRows() []spaces.Row {
	var out []spaces.Row
	for _, v := range spaces.HostViews(m.others, m.listed(), m.sessions, m.detached, m.remotes) {
		rows := m.listRows(v.Host, v.Others, v.Spaces, v.Detached)
		if len(rows) == 0 && m.narrowed() {
			continue
		}
		if len(out) > 0 {
			out = append(out, spaces.Row{})
		}
		folded := m.hostsFolded[v.Host] && !m.filtering
		out = append(out, spaces.Row{ByHost: true, Host: v.Host, HostHeader: true, Folded: folded})
		if folded {
			continue
		}
		for _, r := range rows {
			if !r.Blank() {
				r.ByHost, r.Host = true, v.Host
			}
			out = append(out, r)
		}
	}
	return out
}

// toggleHost folds or unfolds host's part of the hosts tab.
func (m *sidebar) toggleHost(host string) {
	if m.hostsFolded == nil {
		m.hostsFolded = map[string]bool{}
	}
	if m.hostsFolded[host] {
		delete(m.hostsFolded, host)
	} else {
		m.hostsFolded[host] = true
	}
	if err := spaces.SaveHostsFolded(m.hostsFolded); err != nil {
		m.failed = err
	}
	m.setRows()
}

// hostSessionsOf are host's sessions in the worktree whose kitty session is session, of space: those
// tagged with its directory. The ones no window shows come first.
func (m *sidebar) hostSessionsOf(host string, space spaces.Space, session string) []spaces.ProviderSession {
	var out []spaces.ProviderSession
	for _, s := range m.sessions {
		if s.Host == host && s.SpaceName() == space.Name && space.Name+"@"+s.Tags["worktree"] == session {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b spaces.ProviderSession) int { return min(len(a.Windows), 1) - min(len(b.Windows), 1) })
	return out
}

// tabTitle is the list's title.
func (m *sidebar) tabTitle() []look.Span {
	return []look.Span{look.Title("SPACES")}
}

// renderHostHeader draws row i, a host's row on the hosts tab, marked as its entry: its caret, its name,
// local for this machine, unreachable when its server could not load, and a rule across the rest, as it is
// the outermost of what the tab lists.
func (m *sidebar) renderHostHeader(i int) string {
	r, width, th := m.rows[i], m.contentWidth(), m.th
	spans := append(look.Mark(fmt.Sprintf(caretZone, i), look.Caret(r.Folded)...), look.Title(hostName(r.Host)))
	if m.remotes[r.Host].Err != "" {
		spans = append(spans, look.Muted(" "), look.Unreachable())
	}
	if rest := width - th.Width(spans...) - 1; rest > 0 {
		spans = append(spans, look.Muted(" "), th.GroupRule(rest))
	}
	surface := look.Plain
	if i == m.cursor && m.navigating() && !m.selecting {
		surface = look.Cursor
	}
	return th.DrawZone(fmt.Sprintf(entryZone, i), surface, width, th.Cut(width, spans...)...)
}
