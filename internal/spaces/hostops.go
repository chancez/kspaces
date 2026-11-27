package spaces

import (
	"strings"

	"github.com/chancez/kspaces/internal/config"
)

// What a host's server does for another machine's copy of one of its spaces, see docs/server.md: in its
// directories, with its git and its provider, and no kitty, which is the other machine's.

// CreateOnHost creates a worktree in the space at root on branch, from base when not empty, where the
// space's config puts it, marked pending while git makes it.
func CreateOnHost(cfg config.Config, root, branch, base string) (Worktree, error) {
	space, err := Find(cfg, root)
	if err != nil {
		return Worktree{}, err
	}
	branch = strings.TrimSpace(branch)
	dir := WorktreeDir(space.ConfigOf(cfg), space.Root, branch, "")
	unmark, err := MarkPending(PendingCreate, space.Root, dir, branch)
	if err != nil {
		return Worktree{}, err
	}
	defer unmark()
	return CreateWorktreeFrom(space, branch, base, dir)
}

// RemoveOnHost removes the worktree at path as RemoveWorktree does, ending the provider's sessions tagged
// with it, which run in its directory; the windows showing it are the other machine's to close.
func RemoveOnHost(cfg config.Config, path string, opts RemoveOptions) error {
	space, wt, err := FindWorktree(cfg, path)
	if err != nil {
		return err
	}
	return removeWorktree(cfg, space, wt, opts, func() (ended, err error) { return endTagged(cfg, wt.Session), nil })
}

// endTagged ends the provider's sessions tagged with the worktree whose session is session, see
// ClosingSessions, as no window here shows any.
func endTagged(cfg config.Config, session string) error {
	if len(cfg.Provider) == 0 {
		return nil
	}
	caps := ProviderCapabilities(cfg.Provider)
	if !caps[CapList] {
		return nil
	}
	st, err := listSessions(cfg.Provider, nil)
	if err != nil {
		return err
	}
	return ApplyFate(cfg.Provider, caps, FateEnd, ClosingSessions(st, nil, session, true))
}
