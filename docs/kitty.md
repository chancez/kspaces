# Fitting kspaces into kitty

## Spaces and sessions

A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new ones
show up on their own. Each worktree is a kitty session: the main worktree's is named after the space
(`gr`), a linked worktree's `SPACE@DIR` (`gr@dev`).

Opening a worktree:

1. focuses its session's most recently focused window, if it has windows open;
2. otherwise loads `SESSION.kitty-session` from the `sessions` directory with `goto_session`, if there is one
   that opens any window (has a `launch` line);
3. otherwise runs the `launch` action with `--type=tab --cwd=WORKTREE --add-to-session=SESSION`, in the home
   directory for a session in no space, such as `default`.

`launch` in the config is that action, `["launch"]` by default. Every window kspaces opens in a session
goes through it, with kspaces' options appended, so a kitten that passes them on to kitty's `launch` can
stand in for it, such as one that runs each window in a multiplexer session. It has to take options as
`--name=value` and treat the first argument that does not start with `-` as the start of the command.

kspaces itself never writes session files. The session saver below, which it can install into kitty, does.

## Running commands

Commands are what the new worktree form and a worktree's **Run...** can start in a session:

```toml
[[commands]]
name = "claude"              # shown in the forms; unique
run = ["claude", "{prompt}"]  # the command line, never run through a shell
env = { TASK_PR = "{pr}" }    # optional
open = "tab"                  # optional: tab, split, hsplit, vsplit, overlay or window

[[commands.inputs]]
name = "prompt"   # lowercase letters, digits and _
type = "string"   # optional; one line of text, the only type so far
label = "Prompt"  # optional, the name by default
default = ""      # optional
required = true   # optional; an empty required input refuses to run
```

`run` and the values of `env` are templates. `{NAME}` is replaced, as text within the argument, by an
input's value or a built-in: `{space}`, `{root}`, `{worktree}` (its path), `{branch}`, `{base}` (the main
worktree's branch), `{session}` and `{pr}`, the PR's number when the worktree was created from one or its
branch pulls from a PR's head, else empty. Values are never split or expanded, whatever is in them. `{{` and
`}}` are a literal brace. Anything else in braces, a bad input or an unknown `open` is an error when the
config loads.

The command also gets `KSPACES_SPACE`, `KSPACES_ROOT`, `KSPACES_WORKTREE`, `KSPACES_BRANCH`, `KSPACES_BASE`,
`KSPACES_SESSION`, `KSPACES_PR` and `KSPACES_INPUT_NAME` for each input, then `env`, which can override them.
kitty strips spaces from either end of an environment value.

In a session with no windows the command runs as its first tab, wherever `open` says:

```
launch --type=tab --cwd=WORKTREE --add-to-session=SESSION --env=K=V... --hold claude 'the prompt'
```

In a session with windows kspaces switches to it, then runs the `launch` action on the window that
switching focuses, the session's most recently focused, with `kitten @ action --match=id:N`, so a kitten
acts on it too:

| `open` | adds |
|---|---|
| `tab` | `--type=tab`: a new tab, after the session's last with `tab_bar_filter session:~` |
| `split`, `hsplit`, `vsplit` | `--type=window --location=split --next-to=id:N`, or `hsplit`, `vsplit`; the layout picks the axis for `split` |
| `overlay` | `--type=overlay --next-to=id:N`, without `--hold`, so it closes back to the window |
| `window` | `--type=os-window` |

Each word is quoted for kitty, `$` doubled, so it reaches the command as typed; only a leading `~` is still
expanded, as kitty has no escape for it. `--hold` has kitty start your shell once the command exits, so the
window stays usable. The window's program is the command rather than a shell, so a saved session records
it, and restoring the session runs it again with the same inputs.

`kspaces run NAME [PATH]` opens the same form for the worktree PATH, or the current directory, is in, with
`--input NAME=VALUE` filling inputs in and `--open=WHERE` overriding `open` once. `--yes` runs it without
the form, refusing a required input that is empty.

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
map cmd+alt+, kspaces_attention_prev
map cmd+alt+. kspaces_attention_next
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

The sidebars of one kitty share their work: the first to start leads, with a lock on a file in
`$XDG_RUNTIME_DIR/kspaces/`, or `kspaces-UID/` in the temporary directory, named after kitty's
`KITTY_LISTEN_ON` socket. Every two seconds the leader runs `kitten @ ls` and `git worktree list` for each
space, and in the background `git status` and `gh` for the [rows' git state](sidebar.md#spaces), and
writes what it found beside the lock; the others read that file when it changes, and each works out its
own tab's session from it. When the leading sidebar's tab closes, the next sidebar to look takes the lead,
carrying on from what the last one knew. `r` in any sidebar loads at once in that sidebar, and has the
leader ask git and gh again.

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

The filter takes keys sent this way too, but a keyboard mode only sends the keys mapped in it, and
`passthrough_and_end` hands any other to the window, so typing a query needs its keys mapped. `send-key`
takes several keys, so a mapping can filter by a fixed query and leave the cursor on the first match:

```conf
map --mode spaces / kspaces_key /
map --mode spaces up kspaces_key up
map --mode spaces down kspaces_key down
map --mode spaces f kspaces_key / f i x
```
