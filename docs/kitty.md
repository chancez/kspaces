# Fitting kspaces into kitty

## Spaces and sessions

A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new ones
show up on their own. Each worktree is a kitty session: the main worktree's is named after the space
(`gr`), a linked worktree's `SPACE@DIR` (`gr@dev`).

Opening a worktree:

1. focuses its session's most recently focused window, if it has windows open;
2. otherwise loads `SESSION.kitty-session` from the `sessions` directory with `goto_session`, if there is one;
3. otherwise runs the `new_tab` action with `--cwd=WORKTREE --add-to-session=SESSION`.

kspaces never writes session files. Saving them, as the watcher below can, is up to kitty.

## kitty.conf

```conf
# kspaces talks to kitty over remote control.
allow_remote_control socket-only
listen_on unix:/tmp/kitty

# kitty places docks inside the tab bar, so keep the tab bar at the top or bottom.
tab_bar_edge top

# Show the current session's tabs, and tabs in no session. `~` also shows the session last switched to
# with goto_session, which kspaces uses, so another OS window's focus does not change this one's tabs.
tab_bar_filter session:~ or session:^$

# Closing a tab focuses its neighbor in the session rather than the last used tab.
tab_switch_strategy left

# The picker.
map f5 launch --type=overlay --var=kspaces_picker=1 kspaces pick

# The next and previous session with windows open, in sidebar order. A background process needs
# --allow-remote-control to reach kitty.
map cmd+alt+[ launch --type=background --allow-remote-control kspaces prev
map cmd+alt+] launch --type=background --allow-remote-control kspaces next
```

## The watcher

kitty can dock a window to one tab only, so a sidebar in every tab is one `kspaces sidebar` per tab, docked
by a global watcher in kitty.conf (`watcher kspaces_watcher.py`). It:

- docks `kspaces sidebar` at the left of every tab that belongs to a session, with the `kspaces` user
  variable and `--keep-focus`;
- sizes the docks to `[sidebar] width` from the config, and resizes them when it changes;
- notes when each program status record changed state, in the window's `program_status_at` user variable,
  which the status list's ages come from.

The watcher starts `~/.local/bin/kspaces`. The docks, the forms kspaces opens and the picker's overlay carry
a `kspaces*` user variable, so a session saver can leave them out with `not var:kspaces`.

A session saver watcher can save each session to its own file in the `sessions` directory, at quit and as
windows close, and load them again at startup. Then a space reopens as it was left, and starts afresh once
its last window has closed.

## Keys without focusing the sidebar

The sidebar's keys can be sent from any window with `send-key`. A keyboard mode makes that a set of
mappings: `f6`, then `j`/`k` move the cursor and `enter` switches.

```conf
action_alias spaces_key remote_control send-key --match "var:^kspaces$ and state:parent_focused"
map --new-mode spaces --on-unknown passthrough_and_end f6
map --mode spaces j spaces_key j
map --mode spaces k spaces_key k
map --mode spaces enter combine : spaces_key enter : pop_keyboard_mode
map --mode spaces esc combine : spaces_key esc : pop_keyboard_mode
```

`^kspaces$` rather than `kspaces`, so the picker's overlay, whose variable is `kspaces_picker`, is not sent
the keys too.
