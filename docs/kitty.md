# Fitting kspaces into kitty

## Spaces and sessions

A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new ones
show up on their own. Each worktree is a kitty session: the main worktree's is named after the space
(`gr`), a linked worktree's `SPACE@DIR` (`gr@dev`).

Opening a worktree:

1. focuses its session's most recently focused window, if it has windows open;
2. otherwise loads `SESSION.kitty-session` from the `sessions` directory with `goto_session`, if there is one
   that opens any window (has a `launch` line);
3. otherwise opens it with its [default layout](#layouts), if it has one, unless opened without it;
4. otherwise runs the `launch` action with `--type=tab --cwd=WORKTREE --add-to-session=SESSION`, in the home
   directory for a session in no space, such as `default`.

`launch` in the config is that action, `["launch"]` by default. Every window kspaces opens in a session
goes through it, with kspaces' options appended, so a kitten that passes them on to kitty's `launch` can
stand in for it, such as one that runs each window in a multiplexer session. It has to take options as
`--name=value` and treat the first argument that does not start with `-` as the start of the command.

kspaces itself never writes session files. The session saver below, which it can install into kitty, does.

## Running commands

Commands are what the new worktree form and a worktree's **Run** can start in a session, from config.toml
or a space's [own config](#per-space-config):

```toml
[[commands]]
name = "claude"              # shown in the forms; unique
run = ["claude", "{prompt}"]  # the command line, never run through a shell
env = { TASK_PR = "{pr}" }    # optional
open = "tab"                  # optional: tab, split, hsplit, vsplit, overlay or window
inputs = [{ name = "prompt", label = "Prompt", required = true }]
```

`inputs` are what the forms ask for, a text box each, in order, which takes several lines, `shift+enter`
starting a new one, as the [forms](sidebar.md#the-menu) say. An input has:

- `name`: lowercase letters, digits and `_`, used as `{name}` in templates;
- `type`: optional, `string`, text, of one line or several, the only type so far;
- `label`: optional, the name by default;
- `default`: optional; a multi-line TOML string, `"""` to `"""`, for several lines;
- `required`: optional; an empty required input refuses to run.

A command with several inputs or long ones can list them as tables of their own instead, each
`[[commands.inputs]]` belonging to the `[[commands]]` above it.

`run` and the values of `env` are templates. `{NAME}` is replaced, as text within the argument, by an
input's value or a built-in: `{space}`, `{root}`, `{worktree}` (its path), `{branch}`, `{base}` (the main
worktree's branch), `{session}` and `{pr}`, the PR's number when the worktree was created from one or its
branch pulls from a PR's head, else empty. Values are never split or expanded, whatever is in them: a
value's newlines stay in its word, so `claude {prompt}` gets a prompt of several lines as one argument. `{{`
and `}}` are a literal brace. Anything else in braces, a bad input or an unknown `open` is an error when the
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
`--input NAME=VALUE` filling inputs in, a value's newlines kept, as in `--input "prompt=$(cat task.md)"`,
and `--open=WHERE` overriding `open` once. `--yes` runs it without the form, refusing a required input
that is empty.

## Layouts

A layout is tabs split into panes, each running a command, a command line or your shell, which kspaces
builds window by window through the `launch` action:

```toml
[[layouts]]
name = "agent"                 # shown in the forms; unique
[[layouts.tabs]]
title = "{branch}"             # optional, a template; kitty's title by default
panes = [
  { command = "claude" },                               # pane 1, the tab's first window
  { command = "review", split = "right", size = 35 },   # right of pane 1, 35% of the width pane 1 had
  { split = "below", of = 1, size = 30 },               # below pane 1, 30% of its height: your shell
]
```

A pane has one of `command`, a configured command with its env and inputs, or `run`, a command line as a
command's, a template too, or neither, for your shell. `cwd` is where it starts, a template,
`{worktree}` by default, such as `{worktree}/docs`. A pane after the first has `split`, `right` or
`below`, and splits pane `of`, from 1, the one before it by default; `size` is its share in percent of
what that pane had, 50 by default. `focus = true` on one pane of a tab leaves it focused rather than the
first. More `[[layouts.tabs]]` are more tabs. Anything wrong with a layout is an error when the config
loads.

There is no `left` or `above`: kitty's splits layout puts a new window after the one it splits. A pane on
the left of or above another is the earlier one, and the other splits it `right` or `below`; any
arrangement of splits can be built that way, the top left pane first.

A tab is in kitty's `splits` layout unless its `layout` names another of kitty's: `tall`, `fat`, `grid`,
`horizontal`, `vertical` or `stack`. In those the layout arranges the windows, so the panes are listed in
order, with no `split`, `of` or `size`, which are an error there:

```toml
[[layouts.tabs]]
layout = "tall"
panes = [{ run = ["make", "watch"] }, {}, { cwd = "{worktree}/docs" }]
```

kitty only switches a tab to a layout in `enabled_layouts`, so opening a layout whose tab names one that
is not there fails, saying so.

A layout's inputs are those of its panes' commands, in order, one input for a name two commands share, as
long as they define it the same. `run`, `cwd` and `title` can use them, and the built-ins. A tab's title is
one line, so a newline in a value there is a space.

Each tab is, for the agent layout:

```
launch --type=tab --cwd=WORKTREE --add-to-session=SESSION --tab-title=BRANCH --env=K=V... --hold claude 'the prompt'
kitten @ goto-layout --match=window_id:N splits
launch --type=window --location=vsplit --next-to=id:N --bias=35 --cwd=... --add-to-session=... --env=... --hold gr
launch --type=window --location=hsplit --next-to=id:N --bias=30 --cwd=... --add-to-session=...
```

each split run with `kitten @ action --match=id:N` on window N of the pane it splits, so a kitten acts on
it too. A pane running a command has its `--env` options and `--hold`, as a command's tab does. kitty says
nothing back from an action, and a kitten standing in for launch need not either, so N, the window a
launch opened, is the one of the session's windows `kitten @ ls` did not list before it. The tab is set to
its layout, `splits` by default, which has to be in `enabled_layouts`; `--bias` is a percentage in it. In
any other layout each pane after the first is `--location=after --next-to=id:N` of the pane before it,
so they keep their order. In a session with windows the tabs go after the session's tabs, by the window
switching to it focuses, as a command's do.

A worktree whose session has no windows and no saved file to restore opens with its default layout: for a
linked worktree its space's `worktree_layout`, else the space's `layout`, else the top-level `layout`; for
the main worktree the space's `layout`, else the top-level one; else the one tab it always had. A space's
[own config](#per-space-config) can set its defaults too, which come first.

```toml
layout = "agent"            # before any table

[space.gr]                  # by the space's name
layout = "plain"
worktree_layout = "agent"
```

A default layout with an input that is required and has no default is not opened without asking: the
session opens with its one tab, and the sidebar's last line says why. The new worktree form, **Open layout**
and `kspaces layout NAME [PATH]`, with `--input NAME=VALUE` and `--yes` as `kspaces run`, ask for it.

**Open without layout** (`o`) on a row of the sidebar, or `kspaces open --no-layout [PATH]`, opens the one
tab instead of the default layout. It replaces only the default layout, so a session with windows is
switched to, and one with a saved file that opens windows is restored from it, as ever; the sidebar
offers it only where the default layout would open.

### Saving a layout

A session's **Save as layout**, in the sidebar's menu, and `kspaces layout save [NAME] [--tab] [PATH]`
write a layout from the session's tabs as kitty has them, read from `kitten @ ls`, after a
[form](sidebar.md#the-menu) chooses what each pane runs. Each tab becomes a `[[layouts.tabs]]`:

- In `splits`, kitty's tree of splits becomes panes built top down. A pair of splits has its first window,
  the first of its first side; the first window of its second side splits that `right` when the two are
  side by side, else `below`, with `size` the second side's share, `round((1 - bias) * 100)`, from 1 to 99;
  then each side is built the same way from its first window. kitty's `--bias` on a split sets that
  pair's bias, so building the layout makes the same tree, each pair at its share to the percent. A window
  that is maximized is saved at the sizes it had before.
- In any other layout, `layout` is its name and the panes are its windows in order.
- kspaces' own windows are left out: the docks, the forms and the picker, which carry a `kspaces*` user
  variable. An overlay is left out for the window it is over.
- `title` is the tab's only when it was set, with `--tab-title` or `set_tab_title`, with the worktree's
  branch as `{branch}`; otherwise kitty titles it as it does any tab.
- `cwd` is left out for the worktree itself, is `{worktree}/SUB` inside it, else the path.
- `focus = true` marks the tab's active window, unless it is pane 1.

What runs in a pane, which the form offers, is the command its shell reports running, with kitty's shell
integration; else for a window running `cm attach NAME`, what `cm list --json` says session NAME runs,
and its cwd is cm's; else the program in the window's foreground, when that is not a shell; else the shell
at its prompt.

The layout goes at the end of the file it is saved to, a space's [own config](#per-space-config) or
config.toml, as a `[[layouts]]` block, its panes inline a line each, and the rest of the file is kept byte
for byte. Replacing a layout puts the block where the old one's lines were, from its `[[layouts]]` to the
next table that is not one of its own, keeping the comments right above that table; that needs the
layouts to be `[[layouts]]` tables, not an inline `layouts = [...]`. The result is parsed back and must
have the layout as saved and nothing else changed, or the file is left as it was. A symlinked file has its
target written, by a rename from a file beside it.

Not everything kitty arranges can be saved: the biases of `tall`, `fat` and the other layouts, and a
`splits` tab's sizes finer than a percent, come back at the layout's defaults and to the percent; a
session's tabs in another OS window come back as tabs of the one it opens in; a pane's scrollback, and what
a program does when it starts again, are not kept.

## Per-space config

Some layouts and commands belong to one repository, and a work repository's should not end up in
config.toml, which is often in dotfiles. A space can have its own, in `.kspaces.toml` at the top of its
main worktree; its linked worktrees use the main worktree's, as it is on disk. Commit it, or keep it to
yourself in `.git/info/exclude`.

```toml
layout = "tests"            # the space's default layouts, in place of its [space.NAME]'s
worktree_layout = "agent"

[worktrees]                 # in place of config.toml's
branch_prefix = "pr/me"

[[commands]]
name = "test"
run = ["make", "test"]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "test" }, { command = "claude", split = "right" }] }]
```

It can have `[[layouts]]`, `[[commands]]`, `layout`, `worktree_layout` and `[worktrees]`'s `dir` and
`branch_prefix`, checked as config.toml's are; anything else is an error naming the key, `spaces`,
`launch`, `sessions`, `[sidebar]`, `[kitty]` and `[space.NAME]` included, which only config.toml has. For
that space alone, in its forms, its rows' menus, opening its worktrees and `kspaces layout` and `kspaces
run` in them, its layouts and commands are added to config.toml's, one called as one of config.toml's in
that one's place, and a layout of either can run a command of either. Its `layout` and `worktree_layout`
come first, then the space's `[space.NAME]`'s, then the top-level `layout`; a linked worktree's falls back
on the file's own `layout` before `[space.NAME]`'s. Its worktree settings are used in place of
config.toml's they name. Other spaces never see any of it.

A file with an error is left out, and only for its space: the space's row says `config error`, and
`kspaces list` what it is.

A repository can bring a `.kspaces.toml` with a clone, a pull or a branch checked out, and its commands
run, so a file is used only once trusted, and only with the content trusted: kspaces keeps the sha256 of
each file's trusted content, by its path, in `$XDG_STATE_HOME/kspaces/trust`. Until then, and again
whenever the file changes, it is left out and the space's row has a lock and `untrusted config` where its
git state goes. **Trust space config**, in the menu of the space's rows, and `kspaces trust [PATH]`, for the
space PATH is in, or the current directory's, open a form showing what the file adds: each command and the
line it runs, each layout and what each of its panes runs, and the settings it sets, saying which take
the place of config.toml's. Trust trusts that content. `--yes` trusts the file as it is without asking,
and `--revoke` forgets it. A file kspaces writes itself, saving a layout to it, is trusted as written.

Saving a layout to a space's file, as [Save as layout](sidebar.md#the-menu) does at first, edits it as
text the way config.toml is edited, checked merged with config.toml. A file that is not there is made; a
file that is there but not trusted is refused until trusted, as saving would trust what is in it unseen.
Making the file adds `/.kspaces.toml` to the repository's `info/exclude`, in its common git directory, so
git leaves it out, unless the form's box says not to; the line is added once.

The sidebars' leader reads every space's file, as it lists their worktrees, and shares what it read, so
each sidebar sees the same and trusting or editing a file shows in all of them at the next poll. A file
that has not changed, by its size, time and inode, is not read again: every two seconds is a stat.

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

Changing `[kitty]`, or `[sidebar] popup`, changes what install writes, so it makes the files out of date too.

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
- hides the tab bar of the panels kspaces opens, marked with the `kspaces_popup` user variable: kitty has
  no option for one OS window's tab bar, and `tab_bar_filter` always shows the active tab, so it sets
  kitty's own flag for it, which a config reload resets and its next check, within two seconds, sets again;
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

## Popups: overlays and panels

The forms and the picker open in a popup, by `[sidebar] popup`:

```toml
[sidebar]
# "overlay", the default, or "panel".
popup = "panel"
```

- `overlay` opens each over a window, as `launch --type=overlay`: a form over the window beside the
  sidebar, the picker over the window `kspaces_pick` was pressed in. It is that window's size, which over
  a small split is cramped, and closing it gives the keyboard back to that window, as kitty does.
- `panel` opens each as a kitty panel, an OS window of its own of a fixed size, centered on the screen,
  above other windows, which takes the keyboard while it is open:

  ```
  launch --type=os-panel --os-panel edge=center-sized --os-panel columns=90 --os-panel lines=24
         --os-panel focus-policy=exclusive --os-panel layer=overlay
         --var=kspaces_form=1 --var=kspaces_popup=1 --env KSPACES_FROM=ID ...
  ```

  A form is 90 by 24 cells; Save as layout, clean up and Trust space config, which list what they act on,
  100 by 32; the picker 120 by 32, room for its preview beside the list. A panel is drawn in a rounded
  border in the accent, `+`, `-` and `|` in ASCII, a form's heading set into its top and its buttons and
  keys on the row above its bottom; the watcher hides its tab bar. An overlay is drawn as it always was.
  A panel closes when it loses focus, so none is left behind, unseen, in another window's way; one that is
  working, such as switching sessions, which moves the keyboard itself, finishes first, and one that then
  fails takes the keyboard back to say why. Closing one any other way, with `esc` or by finishing without
  switching anywhere, gives the keyboard back to the window it was opened from.

A panel is in no session's tab, so it is told the window it was opened from in `KSPACES_FROM`, and works
as an overlay over that window would: the current session is that window's, Save as layout's "this tab"
is its tab, and kitty actions run on it, so a tab a form opens goes in that window's OS window rather than
the panel's. kitty loads a session restored from its file into the focused OS window, so before that a
panel hands the keyboard back to the window it was opened from, and waits up to a second for kitty to
have moved its focus; if kitty does not, it refuses with an error rather than fill the panel.

The picker's panel is opened by kspaces, not kitty: with `popup = "panel"` kspaces.conf's `kspaces_pick`
is `launch --type=background kspaces pick --from @active-kitty-window-id`, which opens the panel for that
window. Run `kspaces kitty install`, and restart kitty, after changing `popup`; until then `kspaces kitty
check` reports kspaces.conf out of date, and `kspaces_pick` keeps the old kind.

Panels need kitty 0.42 or later, on a platform kitty can make them on: macOS; Wayland with a compositor
that has the layer shell protocol, which GNOME's does not; X11 with a window manager that has the EWMH
window types and states. Where kitty refuses, a popup opens as an overlay instead, and says so once: in the
sidebar's last line for a form, in kitty's log for the picker. A panel is in no session, so the watcher
docks no sidebar in it and the session saver never saves it, and it carries the `kspaces_form` or
`kspaces_picker` user variable as an overlay does. While one has focus no window in a session has, as for
any other OS window: `tab_bar_filter`'s `session:~` above keeps the tab bar on the session last switched
to.

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
