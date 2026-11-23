package tui

import (
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

// lines are the choice's, width cells wide: `sessions   < end >  left/right change`, then what the fate
// chosen does to sessions, in the error color when it ends a busy one. None when there is no choice.
func (c *fateChoice) lines(th *look.Theme, width int, sessions []spaces.ProviderSession) []string {
	if !c.offered() {
		return nil
	}
	line := func(spans ...look.Span) string { return th.Render(look.Plain, th.Cut(width, spans...)...) }
	out := []string{th.Draw(look.Plain, labelWidth, look.Label("sessions")) +
		line(look.Text("< "+fateNames[c.chosen()]+" >"), look.Muted("  left/right change"))}
	says := spaces.FateWords(c.chosen(), sessions)
	if says == "" {
		says = "None of the session provider's sessions shows in these windows"
		return append(out, th.Draw(look.Plain, labelWidth, look.Default(""))+line(look.Muted(says)))
	}
	words := look.Muted(says)
	if c.chosen() != spaces.FateLeave && busy(sessions) {
		words = look.Error(says)
	}
	return append(out, th.Draw(look.Plain, labelWidth, look.Default(""))+line(words))
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
