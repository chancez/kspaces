# Fitting kspaces into kitty

## Spaces and sessions

A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new ones
show up on their own. Each worktree is a kitty session: the main worktree's is named after the space
(`gr`), a linked worktree's `SPACE@DIR` (`gr@dev`).

Opening a worktree:

1. focuses its session's most recently focused window, if it has windows open;
2. otherwise loads `SESSION.kitty-session` from the `sessions` directory with `goto_session`, if there is one
   that opens any window (has a `launch` line);
3. otherwise runs the `new_tab` action with `--cwd=WORKTREE --add-to-session=SESSION`, in the home directory
   for a session in no space, such as `default`.

kspaces itself never writes session files. The session saver below, which it can install into kitty, does.

## Installing the kitty files

kitty needs a watcher to dock a sidebar in every tab, and a couple of kittens. They are built into kspaces:

```
kspaces kitty install
```

writes them to `kspaces/` in kitty's config directory (`--dir` for elsewhere), with a `kspaces.conf` that
loads them. Include that from kitty.conf, then restart kitty:

```conf
include kspaces/kspaces.conf
```

Installing another kspaces leaves the files as they were. A sidebar notices, and its last line says
`kitty files out of date`; `kspaces kitty check` names them, and exits 1, for a script to act on. With
`auto_install` the sidebar rewrites them itself, and says to restart kitty to load them:

```toml
[kitty]
# Rewrite the installed kitty files when they are not this kspaces' own. Off by default.
auto_install = true
# Install the session saver, below. Off by default.
save_sessions = true
```

Changing `[kitty]` changes what install writes, so it makes the files out of date too.

## kitty.conf

```conf
# Before your own mappings, so yours win.
include kspaces/kspaces.conf

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

# kspaces.conf's actions.
map f3 kspaces_focus
map f4 kspaces_toggle
map f5 kspaces_pick
map cmd+alt+[ kspaces_prev
map cmd+alt+] kspaces_next
```

The watcher and the actions find `kspaces` the way kitty's `launch` does: kitty's `exe_search_path`, its
own PATH, `~/.local/bin`, then the PATH of kitty.conf's `env` and your login shell. kitty started from the
Dock has a short PATH, so if kspaces is somewhere else, such as mise's shims, set
`env PATH=~/.local/share/mise/shims:${PATH}` or `exe_search_path`.

## What the files do

kitty can dock a window to one tab only, so a sidebar in every tab is one `kspaces sidebar` per tab.
`kspaces_watcher.py`:

- docks `kspaces sidebar` at the left of every tab that belongs to a session, with the `kspaces` user
  variable and `--keep-focus`;
- sizes the docks to `[sidebar] width`, and resizes them when it changes;
- notes when each program status record changed state, in the window's `program_status_at` user variable,
  which the status list's ages come from.

`kspaces_toggle.py` hides and shows every sidebar, and `kspaces_focus.py` moves the keyboard into the
current tab's sidebar and back.

`save_sessions.py`, with `save_sessions = true`, saves each session to its own file at quit and as its
windows close or take focus, and loads the ones that were open at startup. A space then reopens as it was
left, and afresh once its last window has closed: its file is removed, or, for the default session, which
is `startup_session`'s, emptied. Windows in no session are saved with the default session, at quit and on
demand only. The files go next to kitty's `startup_session`, so point kspaces' `sessions` at that
directory. `kspaces_save_sessions` saves them on demand. The docks, the forms kspaces opens and the picker
carry a `kspaces*` user variable, and are never saved.

## Keys without focusing the sidebar

The sidebar's keys can be sent from any window with `kspaces_key`, which kspaces.conf defines with
`send-key`. A keyboard mode makes that a set of mappings: `f6`, then `j`/`k` move the cursor and `enter`
switches.

```conf
map --new-mode spaces --on-unknown passthrough_and_end f6
map --mode spaces j kspaces_key j
map --mode spaces k kspaces_key k
map --mode spaces enter combine : kspaces_key enter : pop_keyboard_mode
map --mode spaces esc combine : kspaces_key esc : pop_keyboard_mode
```
