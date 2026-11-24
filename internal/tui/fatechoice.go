package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// fateChoice is what becomes of the session provider's sessions the windows a form closes show, chosen in
// the form: End, its first, Shelve or Leave running, as the provider allows, see spaces.Fates. With no
// choice to make, without a provider or with one that can only leave them, it is not drawn, and the form
// closes as it did before there were providers.
type fateChoice struct {
	fates []spaces.Fate
	at    int
}

// offered says whether there is a choice to make.
func (c *fateChoice) offered() bool { return len(c.fates) > 1 }

// shown says whether there is a fate to show: a choice, or one fate alone.
func (c *fateChoice) shown() bool { return len(c.fates) > 0 }

// chosen is the fate chosen, empty when there is no choice.
func (c *fateChoice) chosen() spaces.Fate {
	if !c.offered() {
		return ""
	}
	return c.fates[c.at]
}

// cycle chooses the next fate, or the one before, round.
func (c *fateChoice) cycle(delta int) {
	if c.offered() {
		c.at = (c.at + delta + len(c.fates)) % len(c.fates)
	}
}

// fateNames are the fates as the choice shows them.
var fateNames = map[spaces.Fate]string{spaces.FateEnd: "end", spaces.FateShelve: "shelve", spaces.FateLeave: "leave running"}

// Zones of the choice: the fate before, and the fate after, which a click goes to.
const (
	fatePrevZone = "fate:prev"
	fateNextZone = "fate:next"
)

// lines are the choice's, width cells wide: `sessions   < end >  left/right change`, the arrows and the name
// marked for a click, then what the fate chosen does to sessions, in the error color when it ends a busy
// one. A fate alone, as a topic space's End, is said as it is, with nothing to change. None without fates.
func (c *fateChoice) lines(th *look.Theme, width int, sessions []spaces.ProviderSession) []string {
	if len(c.fates) == 0 {
		return nil
	}
	fate := c.fates[c.at]
	line := func(spans ...look.Span) string { return th.Render(look.Plain, th.Cut(width, spans...)...) }
	choice := []look.Span{look.Text(fateNames[fate])}
	if c.offered() {
		choice = slices.Concat(look.Mark(fatePrevZone, look.Text("< ")), look.Mark(fateNextZone, look.Text(fateNames[fate]+" >")),
			[]look.Span{look.Muted("  left/right change")})
	}
	out := []string{th.Draw(look.Plain, labelWidth, look.Label("sessions")) + line(choice...)}
	says := spaces.FateWords(fate, sessions)
	if says == "" {
		says = "None of the session provider's sessions shows in these windows"
		return append(out, th.Draw(look.Plain, labelWidth, look.Default(""))+line(look.Muted(says)))
	}
	words := look.Muted(says)
	if fate != spaces.FateLeave && busy(sessions) {
		words = look.Error(says)
	}
	return append(out, th.Draw(look.Plain, labelWidth, look.Default(""))+line(words))
}

// click goes to the fate before or after on a left click on the choice's arrows or name, in the frame
// zones were scanned from, and says whether it was one.
func (c *fateChoice) click(zones map[string][]look.Zone, msg tea.MouseClickMsg) bool {
	switch {
	case !c.offered():
		return false
	case clickedChoice(zones, fatePrevZone, msg):
		c.cycle(-1)
	case clickedChoice(zones, fateNextZone, msg):
		c.cycle(1)
	default:
		return false
	}
	return true
}

// busy says whether any of sessions runs a command, which ending or shelving it stops.
func busy(sessions []spaces.ProviderSession) bool {
	for _, s := range sessions {
		if s.Busy && !s.Ended() {
			return true
		}
	}
	return false
}
