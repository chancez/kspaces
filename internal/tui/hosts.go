package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The sidebar has two tabs when the provider lists sessions: spaces, and hosts, which lists the same rows
// by host: each host's row, then what the spaces tab would list of that host, see spaces.HostViews. tab or
// a click on its title switches, and the choice is kept. A row acts as it does on the spaces tab, so
// opening a worktree goes to its kitty session whichever host it is listed under; one only another host
// has opens a tab of its sessions there.

// Zones of the tabs on the title.
const (
	spacesTabZone = "tab:spaces"
	hostsTabZone  = "tab:hosts"
)

// showingHosts is whether the hosts tab shows, which needs other hosts to show: a provider that lists
// sessions, or hosts the config names.
func (m *sidebar) showingHosts() bool {
	return m.hostsTab && m.hasHosts()
}

// hasHosts is whether there is a hosts tab.
func (m *sidebar) hasHosts() bool {
	return m.caps[spaces.CapList] || len(m.remotes) > 0
}

// switchTab shows the other tab, and keeps the choice.
func (m *sidebar) switchTab() {
	m.hostsTab = !m.showingHosts()
	if err := spaces.SaveHostsTab(m.hostsTab); err != nil {
		m.failed = err
	}
	m.cursor, m.offset = 0, 0
	m.rebuildRows(cursorPos{entry: -1})
	m.cursorToCurrent()
}

// hostRows are the hosts tab's rows: for each host, its row, then the rows the spaces tab's filters keep of
// what the host has, see listRows, left out while the host is folded. A host whose rows a filter leaves
// none of is left out whole, and a blank row goes between hosts.
func (m *sidebar) hostRows() []spaces.Row {
	var out []spaces.Row
	for _, v := range spaces.HostViews(m.others, m.listed(), m.sessions, m.detached, m.remotes) {
		open := map[string]bool{}
		for key := range m.detachedOpen {
			if wt, ok := strings.CutPrefix(key, openKey(true, v.Host, "")); ok {
				open[wt] = true
			}
		}
		rows := m.listRows(v.Others, v.Spaces, v.Detached, open)
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

// tabTitle is the title's tabs, SPACES and HOSTS, the one showing as the title and the other muted, or
// SPACES alone without other hosts, see hasHosts.
func (m *sidebar) tabTitle() []look.Span {
	if !m.hasHosts() {
		return []look.Span{look.Title("SPACES")}
	}
	tab := func(name string, on bool) look.Span {
		if on {
			return look.Title(name)
		}
		return look.Muted(name)
	}
	hosts := m.showingHosts()
	return slices.Concat(look.Mark(spacesTabZone, tab("SPACES", !hosts)), []look.Span{look.Default(" ")}, look.Mark(hostsTabZone, tab("HOSTS", hosts)))
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
