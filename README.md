# kspaces

A sidebar for kitty that lists repositories ("spaces") and their git worktrees. Each worktree is a kitty
session. Clicking a worktree switches to its session, or creates it with one tab in the worktree the first
time.

```
SPACES

* gr pr/chancez/dev
> docs-reference main
*   release-caveats pr/ch...
```

`>` marks the session of the tab the sidebar is in, `*` a session with windows open, and faint rows
worktrees with no session yet. `!` means the space's worktrees could not be listed.

## How it fits into kitty

- A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new
  ones show up without configuring them.
- A worktree's session is named after the space for the main worktree (`gr`), and `SPACE@DIR` for the
  others (`gr@dev`), so two repositories with a worktree of the same name do not share a session.
- Opening a worktree whose session has windows focuses the most recently focused one. Otherwise kspaces
  runs the configured `new_tab` action with `--cwd=WORKTREE --add-to-session=SESSION` appended.
- Set `tab_bar_filter session:~ or session:^$` in kitty.conf so the tab bar only shows the current
  session's tabs.
- kitty can only dock a window to a single tab, so a sidebar in every tab is one `kspaces sidebar` dock per
  tab. A global watcher in kitty.conf adds them. Docks are marked with the `kspaces` user variable.
- kitty places docks inside the tab bar, so keep the tab bar at the top or bottom.

## Install

```sh
go build -o ~/.local/bin/kspaces .
```

## Config

`~/.config/kspaces/config`, or `$XDG_CONFIG_HOME/kspaces/config`:

```
# One repository per line, in display order. ~ is expanded.
space ~/projects/gr
space ~/projects/kitty

# The kitty action that opens a worktree's first tab, split on spaces. Defaults to `launch --type=tab`.
new_tab launch --type=tab
```

## Commands

```
kspaces [sidebar]   the sidebar, meant to run in a kitty dock
kspaces list        every space's worktrees with their session names
kspaces open PATH   switch to the session of the worktree at PATH, creating it if needed
```

The sidebar is driven by the mouse when the dock is started with `--dock-skip-focus`. When it has focus,
`j`/`k` move, `enter` opens and `r` refreshes. It refreshes every two seconds on its own.

Remote control has to be enabled in kitty (`allow_remote_control` and `listen_on`).
