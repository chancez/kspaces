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
- See each worktree's uncommitted changes, how far it is ahead of and behind its upstream, and its PR.
- See which programs are blocked, failed, done or working, from kitty's program status protocol (OSC 7501).
- Switch with a fuzzy picker, or to the next and previous session.
- Filter the list as you type, and jump to the windows whose programs are blocked, failed or done.

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

kitty needs a watcher that docks a sidebar in every tab, which kspaces carries and installs:

```
kspaces kitty install    # -> ~/.config/kitty/kspaces/
```

Include its `kspaces.conf` from kitty.conf, with remote control on and the tab bar filtered to the current
session, then restart kitty:

```conf
include kspaces/kspaces.conf
allow_remote_control socket-only
listen_on unix:/tmp/kitty
tab_bar_filter session:~ or session:^$
map f4 kspaces_toggle
```

[docs/kitty.md](docs/kitty.md) has the rest: the actions to map, keeping the files up to date, the session
saver, commands and layouts.

Then list your spaces in `~/.config/kspaces/config.toml`:

```toml
# Repositories, in the sidebar's manual order, which moving a space there rewrites. ~/ is expanded.
spaces = ["~/projects/gr", "~/projects/kitty"]

# Where kitty session files are kept, as SESSION.kitty-session, to reopen a worktree's session from.
# Unset by default.
sessions = "~/.local/share/kitty/sessions"

# The kitty action every window kspaces opens goes through, its options appended: --type, --cwd and the
# like, then any command. A kitten that passes them on to launch can stand in for it.
launch = ["launch"]

# Where new worktrees go, under the space's root, and what new branch names start with.
[worktrees]
dir = ".worktrees"
branch_prefix = "pr/me"

# Commands to run in a worktree, from the new worktree form, a worktree's Run or kspaces run. run and
# env are templates: {prompt} is the input, {worktree}, {branch}, {pr} and the like are built in.
[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", required = false }] # Inputs can be marked required, and default to false.

# Where it opens in a session with windows: tab, split, hsplit, vsplit, overlay or window.
[[commands]]
name = "gr"
run = ["gr"]
open = "split"

# Layouts: tabs split into panes, each running a command, a command line, or your shell. A pane splits one
# before it, right or below, at a size in percent of what that pane had. A session's Save as layout, or
# kspaces layout save, writes one from the tabs as they are.
[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "{branch}"
panes = [
  { command = "claude" },
  { command = "gr", split = "right", size = 35 },
  { split = "below", of = 1, size = 30 },
]

# Per space, by name: the layout its linked worktrees' sessions open with afresh, when there is no saved
# session to restore, and the new worktree form's at first. `layout` is the main worktree's, and a
# top-level `layout`, before any table, the default for every space. A layout needing an input with no
# default, as claude's prompt, is opened afresh as one shell tab, and the sidebar says so.
[space.gr]
worktree_layout = "agent"

# The sidebar's width in columns, 12 to 120, and where its forms and the picker open: "overlay", over the
# window they are for, or "panel", a centered OS window of a fixed size (macOS, Wayland with layer shell,
# X11), which needs kspaces kitty install again. See docs/kitty.md.
[sidebar]
width = 28
popup = "overlay"

# Rewrite the installed kitty files when another kspaces is installed, and install the session saver.
[kitty]
auto_install = true
save_sessions = true
```

Every key is optional. An unknown key is an error.

A repository's own layouts and commands can live with it, rather than in your dotfiles, in `.kspaces.toml`
at the top of its main worktree, which its linked worktrees share. They are added to config.toml's for that
space alone, a name it has taking config.toml's one's place, and it can set the space's `layout`,
`worktree_layout` and `[worktrees]`:

```toml
layout = "tests"

[[commands]]
name = "test"
run = ["make", "test"]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "test" }, { command = "claude", split = "right" }] }]
```

A repository can bring one with a clone or a pull, so it is used only once trusted, and again after each
change: the space's row says `untrusted config` until **Trust space config** in its menu, or `kspaces
trust`, shows what it adds and you trust it. Save as layout saves to it at first, making it and adding it to
`.git/info/exclude`. See [Per-space config](docs/kitty.md#per-space-config).

## Usage

[docs/sidebar.md](docs/sidebar.md) covers the sidebar: its rows and their order, the menu and its forms,
the status list and the keys.

```
kspaces [sidebar]        the sidebar, run in a kitty dock
kspaces pick             a fuzzy picker over sessions, worktrees and status entries, with a preview
kspaces list             every space's worktrees and their sessions, and a space's own config not in use
kspaces open PATH        switch to the session of the worktree at PATH
kspaces next, prev       switch to the next or previous session with windows open
  --attention            or to the next or previous window that is blocked, failed or done
kspaces new [SPACE]      create a worktree, in a form
kspaces run NAME [PATH]  run a configured command in the worktree at PATH or the current directory,
  --input K=V, --yes     in a form, with inputs filled in; --yes without asking
kspaces layout NAME [PATH]  open a configured layout's tabs in the worktree's session, in a form, as run
kspaces layout save [NAME] [PATH]  save the session's tabs as a layout, choosing what each pane runs in a
  --tab                  form; --tab saves only this tab
kspaces close [SESSION]  close every window of a session, after confirming
kspaces forget SESSION   delete the saved file of a session with no windows, after confirming
kspaces trust [PATH]     trust the .kspaces.toml of the space PATH is in, after seeing what it adds in a
  --yes, --revoke        form; --yes without asking, --revoke to forget it
kspaces remove PATH      close a linked worktree's session and remove the worktree, after confirming
  --delete-branch        and then its branch, if merged; --force-branch even if not, losing its commits
kspaces cleanup SPACE    remove the worktrees whose PR is merged, upstream gone or branch merged, in a form
kspaces add [PATH]       add the repository at PATH to the spaces, in a form; --yes without asking
kspaces kitty install    install the files kitty needs; kspaces kitty check says if they are current
```

## Development

```
mise run build    # bin/kspaces
mise run check    # go fmt, go vet, go test
mise run dist     # release archives for macOS and Linux in dist/
```

Pushing a `v*` tag runs `.github/workflows/release.yml`: check, `mise run dist`, and a GitHub release with
the archives and their checksums.
