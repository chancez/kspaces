package spaces

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A copy is a space or a worktree as one host has it, see docs/stories.md. This machine's are the spaces
// tab's; another host's is opened in a kitty session named HOST:SESSION, SESSION its session's name there,
// its windows running on that host, in its directory there.

// CopyName is the kitty session of host's copy of the worktree whose session there is session.
func CopyName(host, session string) string {
	return host + ":" + session
}

// SplitCopy is the host and the session there of a kitty session named as CopyName names one, and "" and
// name for one of this machine's.
func SplitCopy(name string) (host, session string) {
	host, session, ok := strings.Cut(name, ":")
	if !ok {
		return "", name
	}
	return host, session
}

// remoteTab is the launch action that opens the first tab of wt, another host's copy, in its directory
// there: a shell a launch wrapper puts on the host, when the provider says one does, see CapHosts, and
// else kitten ssh to it, whose windows kitty opens on the host too. kitty starts the window here in the
// home directory, as the copy's is the host's.
func remoteTab(cfg config.Config, caps Capabilities, wt Worktree) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	args := append(append([]string{}, cfg.Launch...), "--type=tab", kitty.Quote("--cwd="+home))
	args = append(args, inSession(wt.Session)...)
	if wt.Path != "" {
		args = append(args, kitty.Quote("--var=kspaces_dir="+wt.Path))
	}
	if caps[CapHosts] {
		return args, nil
	}
	ssh := []string{kitty.KittenPath(), "ssh"}
	if wt.Path != "" {
		ssh = append(ssh, "--kitten", "cwd="+wt.Path)
	}
	ssh = append(ssh, cfg.Hosts[wt.Host].Destination(wt.Host))
	for _, a := range ssh {
		args = append(args, kitty.Quote(a))
	}
	return args, nil
}

// ErrNoHost is a copy of a host the config does not name.
var ErrNoHost = errors.New("no such host in [hosts]")

// HostOf is the server of host, which the config names, and what hangs up on it.
func HostOf(cfg config.Config, host string) (Host, func(), error) {
	settings, ok := cfg.Hosts[host]
	dial := remoteDialer()
	if !ok || dial == nil {
		return nil, nil, fmt.Errorf("%s: %w", host, ErrNoHost)
	}
	h := dial(host, settings)
	return h, func() {
		if c, ok := h.(io.Closer); ok {
			c.Close()
		}
	}, nil
}

// FindCopy is host's copy whose session there is session, as host's server says, its main worktree's for
// a space's name. An empty session is host's default: a shell in its home directory.
func FindCopy(cfg config.Config, host, session string) (Worktree, error) {
	h, hangUp, err := HostOf(cfg, host)
	if err != nil {
		return Worktree{}, err
	}
	defer hangUp()
	session = cmp.Or(session, "default")
	if session == "default" {
		return Worktree{Session: CopyName(host, session), Host: host}, nil
	}
	hd, err := h.Load(false)
	if err != nil {
		return Worktree{}, fmt.Errorf("asking %s: %w", host, err)
	}
	for _, s := range hd.Spaces {
		for _, wt := range remoteSpace(host, &hd, s).Worktrees {
			if wt.Session == CopyName(host, session) {
				return wt, nil
			}
		}
	}
	return Worktree{}, fmt.Errorf("%s has no worktree whose session is %s", host, session)
}

// OpenCopy switches to host's copy whose session there is session, see FindCopy, opening it if need be.
func OpenCopy(cfg config.Config, host, session string, opts OpenOptions) error {
	wt, err := FindCopy(cfg, host, session)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return OpenWorktree(cfg, st, wt, opts)
}

// isCopy says whether name is the kitty session of a copy of a host cfg names, which the hosts tab lists
// under its host rather than as a session in no space.
func isCopy(cfg *config.Config, name string) bool {
	host, _ := SplitCopy(name)
	_, ok := cfg.Hosts[host]
	return host != "" && ok
}

// AskNewCopyWorktree opens the form that creates a worktree in host's copy of the space at root, there,
// in a popup as NewWorktree's.
func AskNewCopyWorktree(cfg config.Config, st kitty.State, host, root string) error {
	return launchForm(cfg, st, formPanel, "new", "--host", host, root)
}

// AskRemoveCopyWorktree opens the form that removes host's worktree at path, in a popup as
// AskRemoveWorktree's.
func AskRemoveCopyWorktree(cfg config.Config, st kitty.State, host, path string) error {
	return launchForm(cfg, st, formPanel, "remove", "--host", host, path)
}

// copyOf is session as kitty names host's copy of it, see CopyName, and session itself for this machine's.
func copyOf(host, session string) string {
	if host == "" {
		return session
	}
	return CopyName(host, session)
}
