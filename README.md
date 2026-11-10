# kspaces

A sidebar for kitty that lists your repositories ("spaces") and their git worktrees, with each worktree in
its own kitty session, and the state of the programs running in them.

```
SPACES

  default

v gr main
    dev pr/me/dev
    fix fix

  kitty master

---------------------------
STATUS
Blocked 1
! dev                 5m
  claude - approve
```

- Click a worktree to switch to its session, created with a tab in the worktree the first time.
- Create, close and remove worktrees and sessions from a right-click menu.
- See which programs are blocked, failed, done or working, from kitty's program status protocol (OSC 7501).
- Switch with a fuzzy picker, or to the next and previous session.

## Install

### Via mise

```
mise use -g github:chancez/kspaces
```

### With go install

```
go install github.com/chancez/kspaces/cmd/kspaces@latest
```

It needs Go 1.27 or newer, which `go` fetches itself unless `GOTOOLCHAIN=local`.

### Manually

Download the archive for your OS and architecture from the
[releases](https://github.com/chancez/kspaces/releases), check it against `checksums.txt`, and put the
`kspaces` in it on your PATH. Or, for the latest release into `~/.local/bin`:

```
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -fsSL "https://github.com/chancez/kspaces/releases/latest/download/kspaces_${os}_${arch}.tar.gz" |
  tar -xz -C ~/.local/bin kspaces
```

### From source

```
mise install
mise run install    # -> ~/.local/bin/kspaces
```

## Setup

kspaces needs kitty's remote control, a tab bar filtered to the current session, and a kitty watcher that
docks a sidebar in every tab. [docs/kitty.md](docs/kitty.md) has the kitty.conf and what the watcher does.

Then list your spaces in `~/.config/kspaces/config.toml`:

```toml
# Repositories, in display order. ~/ is expanded.
spaces = ["~/projects/gr", "~/projects/kitty"]

# Where kitty session files are kept, as SESSION.kitty-session, to reopen a worktree's session from.
# Unset by default.
sessions = "~/.local/share/kitty/sessions"

# The kitty action that opens a worktree's first tab.
new_tab = ["launch", "--type=tab"]

# Where new worktrees go, under the space's root, and what new branch names start with.
[worktrees]
dir = ".worktrees"
branch_prefix = "pr/me"

# The sidebar's width in columns, 12 to 120.
[sidebar]
width = 28
```

Every key is optional. An unknown key is an error.

## Usage

[docs/sidebar.md](docs/sidebar.md) covers the sidebar: its rows, the menu and its forms, the status list
and the keys.

```
kspaces [sidebar]        the sidebar, run in a kitty dock
kspaces pick             a fuzzy picker over sessions, worktrees and status entries, for an overlay
kspaces list             every space's worktrees and their sessions
kspaces open PATH        switch to the session of the worktree at PATH
kspaces next, prev       switch to the next or previous session with windows open
kspaces new [SPACE]      create a worktree, in a form
kspaces close [SESSION]  close every window of a session, after confirming
kspaces remove PATH      close a linked worktree's session and remove the worktree, after confirming
```

## Development

```
mise run build    # bin/kspaces
mise run check    # go fmt, go vet, go test
mise run dist     # release archives for macOS and Linux in dist/
```

Pushing a `v*` tag runs `.github/workflows/release.yml`: check, `mise run dist`, and a GitHub release with
the archives and their checksums.
