package spaces

import (
	"strings"

	"github.com/chancez/kspaces/internal/kitty"
)

// inSession are the launch options that put a window in session: --add-to-session, and the user
// variables a launch wrapper reads to file the window's multiplexer session, see launchVars.
func inSession(session string) []string {
	return append([]string{"--add-to-session=" + session}, launchVars(session)...)
}

// launchVars are the user variables a window opened in session carries for a launch wrapper:
// kspaces_space, the space part of the session's name, and for a linked worktree's session, SPACE@DIR,
// kspaces_worktree, the DIR. The wrapper cannot see which kitty session a window goes in, as it runs before
// kitty adds the window to one, but it sees the window's --var options. A session in no space, such as
// default or a topic space's, is a space of its own name. See kitty.LaunchVars, which are not kspaces' own
// windows' marks.
func launchVars(session string) []string {
	space, dir, linked := strings.Cut(session, "@")
	if space == "" {
		return nil
	}
	vars := []string{kitty.Quote("--var=kspaces_space=" + space)}
	if linked && dir != "" {
		vars = append(vars, kitty.Quote("--var=kspaces_worktree="+dir))
	}
	return vars
}
