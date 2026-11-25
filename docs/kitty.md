# Fitting kspaces into kitty

## Spaces and sessions

A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new ones
show up on their own. Each worktree is a kitty session: the main worktree's is named after the space
(`gr`), a linked worktree's `SPACE@DIR` (`gr@dev`).

An entry of `spaces` is the root's path, or an inline table of its `path` and any of the keys a
`[space.NAME]` table takes, `group`, `layout`, `worktree_layout`, `pr_layout` and `issue_layout`, meaning
the same:

```toml
spaces = [
  { path = "~/projects/work/hubble-timescape", group = "work" },
  { path = "~/projects/gr", group = "tools" },
  "~/.dotfiles",
]

[space.gr]                     # by the space's name, its directory's
worktree_layout = "agent"
```

The inline form suits a setting or two, kept on the space's line; a `[space.NAME]` table suits more, or
values too long for one line, as TOML's inline tables cannot span lines. The key is `path` rather than
`name`: a space's name is its directory's, `hubble-timescape` here, and `name =` would read as a name
to show it by. A space can use both, for different keys: the two are merged. A key set in both, or by
two entries whose directories have one name, is an error naming them, rather than one silently winning.
An inline table needs a non-empty `path`, and a key a `[space.NAME]` table does not take is an error.
Moving a space in the sidebar moves its inline table whole, with its comments; adding one appends its
path.

Spaces can be put in groups, such as work and personal, which the [sidebar](sidebar.md#groups) lists each
under a header of its own and can fold or hide:

```toml
groups = ["work", "personal"]  # the order of the groups; any other after them, by name
spaces = [{ path = "~/projects/gr", group = "work" }]
```

A space with no `group` is in none, and is listed before the groups, under no header. Only config.toml
says a space's group: a group is about how every space is shown, so a space's
[own config](#per-space-config) setting `group` is an error. `groups` cannot name a group twice; a group
it names that no space is in is left out.

Opening a worktree:

1. focuses its session's most recently focused window, if it has windows open;
2. otherwise loads `SESSION.kitty-session` from the `sessions` directory with `goto_session`, if there is one
   that opens any window (has a `launch` line);
3. otherwise opens it with its [default layout](#layouts), if it has one, unless opened without it;
4. otherwise runs the `launch` action with `--type=tab --cwd=WORKTREE --add-to-session=SESSION`, in the home
   directory for a session in no space, such as `default`.

`launch` in the config is that action, `["launch"]` by default. Every window kspaces opens in a session
goes through it, with kspaces' options appended. To run every window in a multiplexer session or a
container, use kitty's [`launch_wrapper.py`](https://sw.kovidgoyal.net/kitty/launch/#launch-wrapper),
which wraps kspaces' windows too. Change `launch` only for what kspaces' windows alone should go
through: a kitten that passes the options on to kitty's `launch`, taking them as `--name=value` and the
first argument that does not start with `-` as the start of the command.

Every window kspaces opens in a session, a first tab, a layout's tabs and panes, a command's window, also
carries `--var=kspaces_space=SPACE`, and in a linked worktree's session `SPACE@DIR`
`--var=kspaces_worktree=DIR`, for a launch wrapper that files each window's multiplexer session by space:
the wrapper runs before kitty adds the window to its session, so it cannot see the session, only the
window's variables. A session in no space, `default` or a [topic space](#session-providers)'s, is a space of
its own name. A wrapper that does not read them loses nothing.

These two describe a window that runs the user's programs. Every other `kspaces_` variable marks one of
kspaces' own windows, its sidebar, forms, pickers and popups, which a wrapper should leave unwrapped.

kspaces itself never writes session files. The session saver below, which it can install into kitty, does.

## Closing and suspending sessions

With the [session saver](#what-the-files-do), closing a session keeps it to open again, and suspending it
keeps what ran in it too. Both need `[kitty] save_sessions = true` and `sessions`; without the saver,
closing a session closes its windows and keeps nothing.

**Close session**, `kspaces close`, saves the session's file, then closes its windows. The file stays, so
opening the session again restores its tabs and splits from it rather than its default layout; **Forget**
deletes it. With a provider that can, its form chooses what becomes of the provider's sessions its windows
show, as [the sidebar](sidebar.md#the-menu) says: ended, the first, shelved, which suspends the session as
Suspend session below does, or left running. Ended or left, the provider keeps nothing, and the file alone is kept. **Suspend session**, `z` in the sidebar or `kspaces suspend [SESSION...]`, is the same, and
its row shows the suspended marker until it is opened. Opening it from the sidebar asks first, in the
form `kspaces resume SESSION` opens. While the provider keeps it, the row shows the suspended
marker dimmed, or for Close the working marker dimmed.

What runs in the windows is the [session provider](#session-providers)'s. kspaces knows only kitty's side;
a program the config names keeps the rest, such as the multiplexer session a launch wrapper runs each
window in. Without one, Close and Suspend keep the layout alone, and the windows open again with fresh programs. With
one, opening the session again is opening it as ever: each window comes back from the file attached to
whatever the provider kept, which brings back what ran there as the multiplexer does. A provider decides what
each mode means, such as keeping a closed session's for a week and a suspended one's until it is opened or
forgotten. Suspending frees what the session's programs held; what lived only in a program's memory, an
unsaved buffer or a shell's variables, is gone, unless the program saves it itself.

Closing and suspending use the provider's `keep` and `forget`, below. A provider that fails `keep` exits
non-zero, its stderr the error, and then kspaces closes nothing and takes back what it did. The handles it
prints are its own: kspaces lists them with the session and gives them back to `forget` when the session is
forgotten or its worktree removed. A provider that keeps nothing prints `{"kept": []}`.

Before running `keep` kspaces lists the session in `.kept-sessions` in the `sessions` directory, a line
each of its mode, its name and the handles, tab-separated, and sets the `kspaces_keep` user variable on its
windows, to the mode. The saver keeps the file of a listed session when its last window closes, and drops
the session from the list once it has windows again. A watcher that ends a window's multiplexer session as
the window closes should leave a window with `kspaces_keep` alone: ending it would lose what the provider
kept. Every close kspaces makes sets it, whatever was chosen for the sessions: ended, they are gone before
the windows close; left running, it is what keeps the watcher from shelving or detaching them. Removing a
worktree does not set it: it ends them.

A provider's `keep` shelves the sessions, and the windows marked then are left out of what `list` is told,
before `keep` says what it kept. A list in between would show those sessions as detached rows of their
space for a moment, then the session as suspended. So keeping runs in this order:

1. kspaces asks `capabilities`, then `list`, and notes the ids and names of the sessions the session's
   windows show.
2. It lists the session as `suspending` or `closing`, followed by `:` and the time in Unix seconds, with
   those ids and names as its handles. The sidebar treats them as the session's, as it does a kept
   session's handles, and marks the row as suspending or closing.
3. It sets `kspaces_keep` on the windows, to the mode, `suspend` or `close`.
4. It runs `keep`, then replaces the line, written to a temporary file and renamed over the list, with the
   mode and the handles `keep` printed.
5. It closes the windows.

If `keep` fails, kspaces removes the line and clears `kspaces_keep`. A kspaces that stops part way leaves the
line as it is. The sidebar settles any line still suspending or closing a minute after it was written, which
is far longer than a keep takes:

- If the provider lists one of the noted sessions as shelved, kspaces keeps the session with the ids of
  the shelved sessions and closes what is left of its windows.
- If the provider lists none of them as shelved, kspaces removes the line and clears `kspaces_keep`.
- If the provider does not list, or there were no noted sessions, the session is kept when none of its
  windows is left, and the line is removed otherwise.

The saver reads only the session's name from each line, so the suspending and closing modes do not change how it treats a listed session.

## Session providers

kspaces manages kitty's layout: spaces, kitty sessions, tabs, windows and their session files. What runs
in the windows, and the sessions that run with no window at all, a multiplexer's on this machine or on
another, are a session provider's: a program the config names, which kspaces runs with a verb and a JSON
request on its stdin, and which prints JSON on its stdout.

```toml
provider = ["kspaces-provider-cm"]   # config.toml only, not a space's own config
```

`keeper` is deprecated: it is `provider`'s earlier name, still read as `provider` so an older config keeps
working, and setting both is an error. Rename it. kspaces assumes
nothing about how the provider's sessions relate to windows: one session per window, as a launch wrapper
running each window in cm does, or one window running a multiplexer that holds many, as tmux does, are both
the provider's to describe.

```
PROVIDER capabilities  {}                                                  prints {"capabilities": [C...]}
PROVIDER keep          {"session": S, "mode": "close" | "suspend", "windows": [W...]}  prints {"kept": [H...]}
PROVIDER forget        {"session": S, "kept": [H...]}
PROVIDER list          {"windows": [W...]}                                 prints {"spaces": [N...], "sessions": [P...]}
PROVIDER open          {"session": ID}                                     prints {"argv": [A...], "cwd": D}
PROVIDER kill          {"session": ID}
PROVIDER shelve        {"session": ID}
PROVIDER remove        {"session": ID}
PROVIDER move          {"session": ID, "space": N}
PROVIDER remove-space  {"space": N}
```

Every verb exits non-zero on failure, its stderr the error, which kspaces shows. A verb with no output
prints nothing or `{}`.

**Capabilities.** `capabilities` names what the provider does: `keep` (for `keep` and `forget`), `list`,
`open`, `kill`, `shelve`, `remove`, `move` and `remove-space`. kspaces offers only what is named: a provider without
`shelve` never shows Shelve. It is asked once per sidebar, and again at `r`. A provider that fails
`capabilities` is taken to have `keep` alone, which is what a program written as a keeper does.

**Windows.** Each window W is `{"id", "pid", "cmdline", "foreground", "user_vars", "cwd"}`, as `kitten @
ls` has them: its id, its own process, its program, the command lines in the foreground of its terminal,
its user variables, and its directory. kspaces' own windows are left out.

**Listing.** The sidebar runs `list` at each refresh, with every window. `spaces` names the provider's
spaces; one no configured space is named after is a topic space, below. Each session P is:

| Field | |
| --- | --- |
| `id` | the provider's own reference, opaque to kspaces, given back to every other verb |
| `name` | what the sidebar shows it as |
| `space` | the space it is in; empty is `default` |
| `host` | the machine it runs on, empty for this one |
| `state` | `running`, `exited`, `shelved` or `dead`; only `running` is offered Kill and Shelve, only the others Remove |
| `busy` | whether a command is running, which a confirmation says |
| `title`, `command` | what the row shows after its name: the title, else the command; `command` is also what a layout saved from a window showing it runs |
| `cwd` | optional: where it is, which a layout saved from a window showing it starts in |
| `status` | a program's own report of itself, as OSC 7501's states: `idle`, `working`, `done`, `blocked`, `error`, or empty |
| `tags` | the provider's labels; `worktree`, when the space has that worktree, picks the kitty session `open` uses |
| `windows` | the ids of the windows that show it, which is the provider's to decide: by a window's pid, a user variable, its program, anything W carries |

A session with no `windows` is detached and listed under its space, by host, unless it is a kept
session's: a session whose `id` or `name` equals a handle `keep` returned for a kitty session kspaces closed
or suspended, as `.kept-sessions` lists them, is that kitty session's, whose row stands for it, and comes
back with it, so it is not listed as detached. Once that kitty session is opened again and its handles
leave the list, the session is an ordinary one again. A `list` that fails leaves the
sidebar showing the last one that did not, so a provider restarting under it, as a multiplexer upgrading
does, blanks nothing; after three failures in a row it is asked again a minute later, or at `r`.

**Opening.** `open` prints the command to run in a new tab, and optionally where it starts, such as `cm
attach @ID` or `tmux attach -t NAME`. kspaces opens the tab in the session's own place first: the kitty
session of the worktree its `worktree` tag names, when the space has that worktree; else that of the
worktree whose directory holds its `cwd`, the deepest, for a session from before tagging; else the focused
kitty session, when that is in the session's space; else the space's own session. A missing worktree is
never made again for it. The tab carries the [launch variables](#spaces-and-sessions) of the session it
opens in.

**Acting.** `kill` ends a running session, `shelve` ends one so it can be brought back as it was, `remove`
discards one that has ended, and `move` puts one in another space, which the provider creates if it must.
`remove-space` removes one of its spaces, refusing as it must, such as one that still holds sessions, or
`default`. kspaces confirms `kill`, `remove` and `remove-space`, and refreshes after each.

**Topic spaces.** A space in `spaces` that no configured space has the name of is listed with the sessions in
no space, as a topic space: one kitty session of its name, opened with a shell tab in the home directory, and
its detached sessions listed as any space's are.

Without a provider, or one without `list`, there are no detached sessions and no topic spaces.

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
- `default`: optional, a template of the built-ins below, such as `"Review PR #{pr}"`; a multi-line TOML
  string, `"""` to `"""`, for several lines;
- `required`: optional; an empty required input refuses to run.

A command with several inputs or long ones can list them as tables of their own instead, each
`[[commands.inputs]]` belonging to the `[[commands]]` above it.

`run` and the values of `env` are templates. `{NAME}` is replaced, as text within the argument, by an
input's value or a built-in: `{space}`, `{root}`, `{worktree}` (its path), `{branch}`, `{base}` (the
branch the worktree's was created from, kept in git config as its branch's `kspaces-base`, else the main
worktree's branch), `{session}`, `{pr}`, the PR's number when the worktree was created from one or its
branch pulls from a PR's head, else empty, and `{issue}`, the number of the issue the worktree was created
for, kept in git config as its branch's `kspaces-issue`, else empty. Values are never split or expanded,
whatever is in them: a value's newlines stay in its word, so `claude {prompt}` gets a prompt of several
lines as one argument.
`{NAME?}` is the same, except that a word of `run` with one whose value is empty or only spaces is left
out: `["claude", "{prompt?}"]` runs `claude` alone for an empty prompt, rather than with an empty argument,
and `"--pr={pr?}"` goes when there is no PR. `{{` and `}}` are a literal brace. Anything else in braces, a bad input or an unknown `open` is an error when the
config loads.

An input's default can use the built-ins, not other inputs. The forms show it filled in, and the new
worktree form fills it in again as its branch, PR or issue changes, until it is typed in: `{pr}` is the
PR's number once one is chosen, `{issue}` the issue's, and `{branch}` the branch once looked up. `--yes`
and opening a default layout afresh fill it in the same way. A default with a literal brace writes it `{{`
or `}}`.

The command also gets `KSPACES_SPACE`, `KSPACES_ROOT`, `KSPACES_WORKTREE`, `KSPACES_BRANCH`, `KSPACES_BASE`,
`KSPACES_SESSION`, `KSPACES_PR`, `KSPACES_ISSUE` and `KSPACES_INPUT_NAME` for each input, then `env`, which
can override them. kitty strips spaces from either end of an environment value.

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

`kspaces run NAME [PATH...]` opens the same form for the worktree PATH, or the current directory, is in, with
`--input NAME=VALUE` filling inputs in, a value's newlines kept, as in `--input "prompt=$(cat task.md)"`,
and `--open=WHERE` overriding `open` once. `--yes` runs it without the form, refusing a required input
that is empty. Several PATHs run it in each worktree, as the [sidebar](sidebar.md) says of Run.

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

A pane running a command can set defaults for its inputs, in place of the command's own, as templates of
the built-ins as theirs are. A layout for reviewing a PR, as the space's `pr_layout`, can ask with the
prompt filled in, to edit or keep:

```toml
[[layouts]]
name = "review"
tabs = [{ panes = [{ command = "claude", inputs = { prompt = "Review PR #{pr}" } }, { command = "gr", split = "right" }] }]
```

A name `inputs` has must be one of the command's inputs, and only a pane with `command` has them. Two panes
sharing an input set it alike, or both leave it to the command.

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
pr_layout = "review"
issue_layout = "fix"

[space.gr]                  # by the space's name
layout = "plain"
worktree_layout = "agent"
pr_layout = "review"
issue_layout = "fix"
```

The new worktree form's branch tab starts on the linked worktree's default layout, its Pull request tab on
the PR layout: the space's `pr_layout`, else the top-level `pr_layout`, else the linked worktree's default;
and its Issue tab on the issue layout, `issue_layout` in the same order. Both are only the form's:
reopening the worktree afresh later uses its default layout.

A default layout with an input that is required and has no default is not opened without asking: the
session opens with its one tab, and the sidebar's last line says why. The new worktree form, **Open layout**
and `kspaces layout NAME [PATH]`, with `--input NAME=VALUE` and `--yes` as `kspaces run`, ask for it.

**Open without layout** (`o`) on a row of the sidebar, `shift+enter` in the picker, or
`kspaces open --no-layout [PATH]` opens the one tab instead of the default layout. It replaces only the
default layout, so a session with windows is switched to, and one with a saved file that opens windows is
restored from it, as ever; the sidebar and the picker offer it only where the default layout would open.
The new worktree form and **Open layout** list `shell` before the layouts: one tab in the worktree, a
shell, added to a session with windows as a layout's first tab would be.

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
integration; else for a window that shows one of the [session provider](#session-providers)'s sessions,
the `command` its `list` says that session runs, and its `cwd` when it says one; else the program in the
window's foreground, when that is not a shell; else the shell at its prompt.

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

## More config files

The config can be split across files: config.toml, then every other `.toml` file beside it in
`~/.config/kspaces`, by name, hidden ones left out. A `work.toml` kept out of dotfiles can add a job's spaces,
groups, commands and layouts, and config.toml stays the same on every machine:

```toml
# work.toml, beside config.toml
spaces = [
  { path = "~/projects/work/api", group = "work" },
  { path = "~/projects/work/web", group = "work" },
]

[space.api]                    # a key config.toml's [space.api] does not set
worktree_layout = "review"

[[layouts]]                    # can run config.toml's commands, and config.toml can open it
name = "review"
tabs = [{ panes = [{ command = "gr" }] }]
```

They add up as one file saying all of it would:

- `spaces`, `groups`, `[[commands]]` and `[[layouts]]` are listed one file's after another's. config.toml's
  `groups` can name a group only another file has spaces in: `groups = ["work", "tools"]` puts `work` first
  where `work.toml` exists, and a group no space is in is left out elsewhere.
- `[space.NAME]` tables, and inline `spaces` entries, are merged by key.
- Every other setting, `launch`, `sessions`, `layout`, a key of `[worktrees]`, `[sidebar]` or `[kitty]`, and
  each key of a space, is one file's to set. Set in two files, or a command, layout or group defined in two,
  it is an error naming both, rather than one silently winning.

An error in one file names that file, and one only the files together have, such as a layout running a
command none defines, names the directory. Without config.toml the other files are the config.

Edits go to the file they belong to, checked with the other files: adding a space appends it to
config.toml; saving a layout replaces it in the file that has it, and adds a new one to config.toml.
Moving a space reorders its own file's `spaces`. The sidebar lists a group's spaces as the files do, one
file's after another's, so moving a space before one from a file earlier than its own is an error, and
the files are left as they were.

## Per-space config

Some layouts and commands belong to one repository, and a work repository's should not end up in
config.toml, which is often in dotfiles. A space can have its own, in `.kspaces.toml` at the top of its
main worktree; its linked worktrees use the main worktree's, as it is on disk. Commit it, or keep it to
yourself in `.git/info/exclude`.

```toml
layout = "tests"            # the space's default layouts, in place of its [space.NAME]'s
worktree_layout = "agent"
pr_layout = "review"
issue_layout = "fix"

[worktrees]                 # in place of config.toml's
branch_prefix = "pr/me"

[[commands]]
name = "test"
run = ["make", "test"]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "test" }, { command = "claude", split = "right" }] }]
```

It can have `[[layouts]]`, `[[commands]]`, `layout`, `worktree_layout`, `pr_layout`, `issue_layout` and
`[worktrees]`'s `dir`, `branch_prefix` and `delete_branch`, checked as config.toml's are; anything else is
an error naming the key, `spaces`, `launch`, `sessions`, `groups`, `[sidebar]`, `[kitty]` and `[space.NAME]`
included, which only config.toml has, and so is `group`, which is the space's `[space.NAME]`'s. For
that space alone, in its forms, its rows' menus, opening its worktrees and `kspaces layout` and `kspaces
run` in them, its layouts and commands are added to config.toml's, one called as one of config.toml's in
that one's place, and a layout of either can run a command of either. Its `layout` and `worktree_layout`
come first, then the space's `[space.NAME]`'s, then the top-level `layout`; a linked worktree's falls back
on the file's own `layout` before `[space.NAME]`'s. Its `pr_layout` and `issue_layout` come before
`[space.NAME]`'s, then the top-level ones, then the linked worktree's default. Its worktree settings are
used in place of config.toml's they name. Other spaces never see any of it.

A file with an error is left out, and only for its space: the space's row says `config error`, and
`kspaces list` what it is.

A repository can bring a `.kspaces.toml` with a clone, a pull or a branch checked out, and its commands
run, so a file is used only once trusted, and only with the content trusted: kspaces keeps the sha256 of
each file's trusted content, by its path, in `$XDG_STATE_HOME/kspaces/state.json`. Until then, and again
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

The sidebar reads every space's file as it lists their worktrees, so trusting or editing a file shows at
the next poll. A file that has not changed, by its size, time and inode, is not read again: every two
seconds is a stat.

## Installing the kitty files

kitty needs a watcher to dock the sidebar in an OS window, and a couple of kittens. They are built into
kspaces, and need kitty 0.51.0 or later, which the watcher says when kitty starts if it is older:

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
map f7 kspaces_pick --scope=windows
map cmd+alt+[ kspaces_prev
map cmd+alt+] kspaces_next
map cmd+alt+/ kspaces_recent
map cmd+alt+, kspaces_attention_prev
map cmd+alt+. kspaces_attention_next
map cmd+alt+n kspaces_new
map cmd+alt+t kspaces_run test
```

`kspaces_pick --scope=windows` opens the picker on every window by what runs in it, see
[The picker](sidebar.md#the-picker).

`kspaces_recent` switches to the session focused most recently other than the focused window's, as alt-tab
does, so pressing it twice goes back; sessions in no space count, and docks never count as focusing one.
With no other session open it fails, saying so in kitty's output, as the other actions do.

The form actions open the sidebar's forms in a popup for the focused window, on its session, worktree or
space: `kspaces_new`, `kspaces_new_pr` and `kspaces_new_issue` create a worktree, `kspaces_remove`,
`kspaces_cleanup`, `kspaces_close`, `kspaces_suspend`, `kspaces_layout NAME`, `kspaces_save_layout`,
`kspaces_run NAME` and `kspaces_trust` act on what they say, and `kspaces_add` adds the repository of the
window's directory. Each runs its command with `--from @active-kitty-window-id`, as `kspaces_pick` does in
a panel, and kitty appends a mapping's words, such as the layout's or command's name, to it.

The watcher and the actions find `kspaces` the way kitty's `launch` does: kitty's `exe_search_path`, its
own PATH, `~/.local/bin`, then the PATH of kitty.conf's `env` and your login shell. kitty started from the
Dock has a short PATH, so if kspaces is somewhere else, such as mise's shims, set
`env PATH=~/.local/share/mise/shims:${PATH}` or `exe_search_path`.

## What the files do

The sidebar is an OS window dock: one `kspaces sidebar` in all of kitty, shown in every tab of its OS
window. kitty lists it in `kitten @ ls` in the OS window's active tab, so the sidebar's session is the
active tab's. `kspaces_watcher.py`:

- docks `kspaces sidebar` at the left of an OS window with a tab that belongs to a session, the active OS
  window first, with the `kspaces` user variable and `--keep-focus`, in no session, as kitty moves it from
  tab to tab. Other OS windows get none; when the sidebar's OS window closes, the next with a session's tab
  gets it, and any sidebar beyond the first is closed;
- sizes the dock to `[sidebar] width`, all of it the sidebar's (kitty's dock border, if any, is the
  separator), and resizes it when it changes. Dragging the sidebar's border resizes it, and the sidebar
  keeps the new width in `$XDG_STATE_HOME/kspaces/state.json` once it has held for a poll, so the next
  sidebar opens at it. It stands in for `[sidebar] width` until that is changed;
- hides the tab bar of the panels kspaces opens, marked with the `kspaces_popup` user variable: kitty has
  no option for one OS window's tab bar, and `tab_bar_filter` always shows the active tab, so it sets
  kitty's own flag for it, which a config reload resets and its next check, within two seconds, sets again;
- notes when each program status record changed state, in the window's `program_status_at` user variable,
  which the status list's ages come from;
- tells the sidebar when focus moves, with a datagram on a socket beside the git states' file, which the
  sidebar answers with a `kitten @ ls` alone, so a switch to another session shows at once.

`kspaces_toggle.py` hides and shows the sidebar, and `kspaces_focus.py` moves the keyboard into the
sidebar, when the current OS window has it, and back. `kspaces_clear_status.py` drops windows' program
status records without focusing them, for the status list's clear buttons; the sidebar runs it through
the `kspaces_clear_status` action. `kspaces_screen.py` says how many cells fit a window's screen, for a
popup sized as a percentage of it; kspaces runs it with `kitten @ kitten`.

Every two seconds the sidebar runs `kitten @ ls` and `git worktree list` for each space, and in the
background `git status` and `gh` for the [rows' git state](sidebar.md#spaces). It writes what git and gh
said to a file in `$XDG_RUNTIME_DIR/kspaces/`, or `kspaces-UID/` in the temporary directory, named after
kitty's `KITTY_LISTEN_ON` socket, for the picker and the forms, and a sidebar started again carries on
from it. `r` loads at once, and has git and gh asked again.

`save_sessions.py`, with `save_sessions = true`, saves each session to its own file at quit and as its
windows close, take focus or change directory, and loads the ones that were open at startup, ending on the
session that had focus at the last save of them all, as at quit. A space then
reopens as it was left, and afresh once its last window has closed: its file is removed, or, for the
default session, which is `startup_session`'s, emptied. A session kspaces [closed or
suspended](#closing-and-suspending-sessions) keeps its file. Closing an OS window as a whole keeps its
sessions' files. Windows in no session are saved with the default session, at quit and on demand only.
The files go next to kitty's `startup_session`, so point kspaces' `sessions` at that directory.
`kspaces_save_sessions` saves them on demand, the sessions named after it or all of them. The docks, the
forms kspaces opens and the picker carry a `kspaces*` user variable, and are never saved.

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

  A form is 90 by 24 cells; New worktree, Save as layout, clean up and Trust space config, which list
  what they act on, and Reply, which shows a window's screen, 100 by 32; the picker 120 by 32, room for
  its preview beside the list. `[sidebar.popup_size]` sets each, as COLUMNSxLINES, each in cells, at
  least 40x10, or as a percentage of the screen, so a size suits a laptop's screen and an ultrawide's:

  ```toml
  [sidebar.popup_size]
  form = "90x24"
  list = "100x32"
  picker = "60%x70%"
  ```

  A percentage is of the work area of the monitor the window it is for is on, in that window's cells, as
  `kspaces_screen.py` reports it, and no less than 40x10. Where that is not known, such as before
  `kspaces kitty install` has installed it, the popup has its default size, and kspaces' log says why.

  A mapping can open its own size with `--size COLUMNSxLINES`, which `kspaces pick` and every command a
  form action runs take with `--from`: `map f7 kspaces_pick --scope=windows --size=80%x80%`. It sizes a
  panel only; an overlay is the size of its window.
  A panel is drawn in a rounded border in the accent, `+`, `-` and `|` in ASCII, a form's heading
  set into its top and its buttons and keys on the row above its bottom; the watcher hides its tab bar. An
  overlay is drawn as it always was.
  A panel closes when it loses focus, so none is left behind, unseen, in another window's way; one that is
  working, such as switching sessions, which moves the keyboard itself, finishes first, and one that then
  fails takes the keyboard back to say why. Closing one any other way, with `esc` or by finishing without
  switching anywhere, gives the keyboard back to the window it was opened from.

A panel is in no session's tab, so it is told the window it was opened from in `KSPACES_FROM`, and works
as an overlay over that window would: the current session is that window's, Save as layout's "this tab"
is its tab, and kitty actions run on it. Some of kitty still goes by the focused OS window, the panel's:
a session restored from its file is loaded into it, and a kitten standing in for `launch` that launches
through remote control, as `boss.call_remote_control(window, ('launch', ...))` does without `--self`, puts
its tabs and windows in kitty's active tab, whatever `--match` or `--next-to` say. So before its first
action a panel hands the keyboard back to the window it was opened from, and waits up to a second for
kitty to have moved its focus; if kitty does not, it refuses with an error rather than fill the panel.

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

## Replying to a window

A status entry's Reply, `t` in the sidebar, and `kspaces reply WINDOW_ID [TEXT]` type into a window
without switching to it, with `kitten @ send-text --match id:N -- TEXT`. The text goes to the program as
the bytes of what it says, a newline as a line feed, then Enter as a carriage return, `\r`, unless sent
without Enter; a backslash is doubled, as kitten reads its text with bash's `$'...'` escapes, so it goes
as one. send-text writes to the window's terminal, past kitty's keyboard handling, so a program in the
kitty keyboard protocol's first mode, disambiguate, gets what typing sends there, Enter as `\r`; one that
asks for every key as an escape code gets the bytes all the same, not the codes its keys would send. A
window running `cm attach` passes them on to the program in its session as they are.

## Keys without focusing the sidebar

The sidebar's keys can be sent from any window with `kspaces_key`, which kspaces.conf defines with
`send-key`. A keyboard mode makes that a set of mappings: `f6`, then `j`/`k` move the cursor and `enter`
switches.

```conf
map --new-mode spaces --on-unknown passthrough_and_end f6
map --mode spaces j kspaces_key j
map --mode spaces k kspaces_key k
map --mode spaces ctrl+d kspaces_key ctrl+d
map --mode spaces ctrl+u kspaces_key ctrl+u
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
