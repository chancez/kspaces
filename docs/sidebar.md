# The sidebar

## Spaces

Each row starts with a fold triangle for a space with linked worktrees, then an icon: a repository for a
space, a branch for a linked worktree, a terminal for a session in no space, or a cross for a space whose
worktrees could not be listed. The most urgent status of the row's sessions replaces the icon. The icon is
green when the session has windows open; the row is faint when it has none. The sidebar's own session is
drawn as a bar.

At the right end of a worktree's row, a space's own row included, is what git and GitHub say of it: a dot
and 3 for three files not committed, an up arrow and 2 and a down arrow and 1 for two commits ahead of its
upstream and one behind, and `#123` for its branch's PR, green when open, grey as a draft, purple merged
and red closed. The branch is cut to make room for them; in a narrow sidebar ahead and behind go first,
then the PR, and the changes last. Only the leading sidebar asks: `git status` every 10 seconds, and
`gh pr list --head BRANCH` every 3 minutes, which needs gh signed in to GitHub. Without gh, a GitHub remote
or a network there is no PR, and none is looked up for the repository's default branch. `r` asks both
again at once.

A space with its [own config](kitty.md#per-space-config) that is not in use says why on its own row, where
the git state goes: a lock and `untrusted config` (`?` in Unicode and ASCII) while the file is not trusted
as it is, or the cross of a broken space and `config error` when it has an error, which `kspaces list`
shows. A narrower row says `untrusted`, then shows the mark alone.

Sessions that are open or saved but belong to no worktree, such as the one kitty starts with, are listed
above the spaces.

A session named `SPACE@DIR` after a space that matches none of its worktrees, open or saved, is a missing
worktree of that space: its directory is gone, as when `git worktree remove` runs while its session still
has windows. It is listed under the space after its linked worktrees, faint whether open or not, with a
broken link (a cross in Unicode and ASCII) for its icon and `gone` where the branch goes, and folds with
the linked worktrees. git is not asked about it. A missing worktree with windows open can be opened and
closed as any session; one with only a saved file cannot be opened, as that would make it afresh in a
directory that is gone, and Forget deletes the file. A space whose worktrees could not be listed has no
missing worktrees: its sessions are listed above the spaces as before.

Clicking a row, or `enter` on it, switches to its session, creating it the first time, with its
[default layout](kitty.md#layouts) if it has one, but for a missing worktree with no windows. Clicking a
space's triangle, or `space` on its row, folds its linked worktrees into the space's row. Folded spaces are
kept in `$XDG_STATE_HOME/kspaces/folded`, so every tab's sidebar agrees.

`/` filters the list. The query goes on the row under the SPACES title, so the list keeps all its rows and
the query stays put as the list narrows. A space shows when its name, its branch, or a linked worktree's
directory or branch has the query in it, ignoring case, with only the linked worktrees that match under it;
a session in no space shows by its name. Folds are ignored while filtering, so a match in a folded space
shows, and the carets are hidden. A substring rather than the picker's fuzzy match, as the list keeps its
order and does not rank, so loose matches would sit among the real ones. Every key types into the query,
`j` and `k` too, as worktrees are named with them; `up` / `down`, or `ctrl+p` / `ctrl+n` as in the picker,
move over the rows shown. `enter` opens the cursor's row and clears the filter, and `esc` clears it. The
cursor stays on its row while it shows, and goes to the first row shown when the query hides it. Clearing
puts it back on its row, or on the folded space that holds it. The filter also goes when navigating ends:
when the sidebar loses focus, or keys sent without focus stop.

## Order

The button left of the plus on the SPACES title shows how the list is sorted, and a click on it goes on
to the next way, as `s` does; a right click opens a menu of the three, the one in use ticked.

| Mode | Icon | Order |
|---|---|---|
| Manual | bars; a triple bar in Unicode, `=` in ASCII | the spaces as config.toml lists them, and each space's linked worktrees as they were moved, any not moved yet after them in git's order |
| Activity | a clock; a clock face in Unicode, `@` in ASCII | the session focused most recently first, a space by its most recent session; sessions with no windows last, in manual order |
| Status | a warning sign; `!` in Unicode and ASCII | the most urgent status first, as the status list ranks them, blocked, failed, done, working, idle, then none; a space by its most urgent session, ties by activity |

In every mode a space's main worktree stays its own row, first, and its missing worktrees last; only its
linked worktrees move within it. The sessions in no space stay above the spaces, ordered the same way, by
name when manual. Activity is when one of a session's windows, docks left out, was last focused, as
kitty's `ls` reports it. The mode is kept in `$XDG_STATE_HOME/kspaces/sort`, manual until one is chosen,
so every tab's sidebar sorts the same way.

The automatic modes sort again at each refresh, and the cursor stays on its row wherever that goes. While
the mouse is pressed on a row or a menu is open the rows stay where they are, so what is under the mouse
does not move from under it.

Dragging a space's row moves the space, and dragging a linked worktree's row moves it among its space's.
The rows show where it goes as the mouse moves, the dragged row lit, and letting go keeps it there; `esc`
or a right click while dragging puts it back. A press let go of on the row it was on is a click, which is
why a row opens when the mouse is let go rather than pressed. `K` and `J` move the cursor's row up and
down the same way. A session in no space and a missing worktree do not move, and neither does anything
while filtering. Moving a space rewrites the order of `spaces` in config.toml, as text, each entry with
the comments on the lines right above it and after it on its line, through a symlinked config to its
target. Moving a worktree keeps its space's order, by the worktrees' directories, in
`$XDG_STATE_HOME/kspaces/order`. Either moves the order of every sidebar.

Moving in activity or status switches to manual, starting from the order shown: the spaces are written to
config.toml as they were shown and every space's worktrees saved as they were shown, then the move is
made.

## The menu

Right-click a row, or press `m` on it. The menu lists what applies to the row, in sections, each named on
the rule above it: what acts on the row's session, then on its worktree, then on the space as a whole, the
same from any of its rows. A menu of one section names none. `j` / `k` and the mouse go over the items
only.

| Section | Item | Key | Does |
|---|---|---|---|
| Session | Open | `enter` | switch to the session |
| Session | Run | `R` | run a configured command in the worktree's session, in a form; with `[[commands]]` configured |
| Session | Open layout | `L` | add a configured layout's tabs to the worktree's session, in a form; with `[[layouts]]` configured |
| Session | Save as layout | | save the session's tabs as a layout in config.toml, in a form; on a worktree's session with windows open |
| Session | Close session | `c` | close every window of the session, on a session with windows open |
| Worktree | Open PR #N | `b` | open the branch's PR in the browser with `gh browse N`, on a worktree whose row shows a PR |
| Worktree | Remove worktree | `x` | close the session and remove a linked worktree |
| Worktree | Forget | `x` | delete the saved session file of a missing worktree with no windows open |
| Space | New worktree | `n` | create a worktree in the space, in a form |
| Space | Fold / Unfold worktrees | `space` | hide or show a space's linked worktrees |
| Space | Clean up worktrees | `p` | remove the space's linked worktrees that look done with, on a space's row |
| Space | Trust space config | | show what the space's own config adds, and trust it, in a form; while its `.kspaces.toml` is untrusted or changed |
| Space | Add space | `a` | add a repository to the spaces, in a form; on every row |

Each item that changes something opens a form in an overlay, with buttons and keys to confirm or cancel.
A click on a text field, its label or its box, gives it focus, as `tab` and `shift+tab` do, and a click in
its text puts the cursor there; in a one-line field too long for its box, which may be scrolled, at its end.

**Open PR** is the PR the row shows, which needs gh signed in. gh runs in the worktree, in the
background, so the sidebar does not wait for it; what it says when it fails shows on the sidebar's last
line.

**New worktree** has a branch field, which starts with `worktrees.branch_prefix/`, and a directory field,
empty for the default: `worktrees.dir` under the space's root, named after the branch. An existing branch is
checked out; a new one is created from what the main worktree has checked out. Create switches to the new
session.

The branch field also takes a GitHub PR: `#123`, the PR's URL, or a number that is no branch, after the
prefix or in place of it. Once typing rests on it, the form looks it up with `gh pr view` in the space's root
and shows the PR's title and the branch it checks out, which the directory follows. A PR from the repository
itself is its branch, fetched and tracking the remote's. A PR from a fork is fetched from the remote's
`pull/N/head` into a branch named after the fork's, or `pr-N-BRANCH` when the space has a branch of that name
for something else, and pulls from `pull/N/head`, as `gh pr checkout` sets up a fork it has no remote for.
The remote is the one for the repository the PR is to, else `origin`. A branch set up for the PR before is
checked out as it is. When gh cannot say, because it is not installed, there is no such PR or the repository
is not on GitHub, the form shows gh's error as it does git's.

With [commands](kitty.md#running-commands) configured, the form has a `command` field below the directory,
`none` at first, and below it a field for each input of the command chosen. `tab` and `shift+tab` go round
all the fields. A choice like `command` is drawn `< claude >`, and while it has focus says
`left/right change  enter list` after it, and the keys row lists `left/right` and `enter/space`:
`left` and `right` go to the one before or after; `enter`, `space` or a click opens a list below the
field, or above it when it would run off the bottom, of every choice, the one chosen marked and
selected, a layout with how many tabs and panes it has and a command with its command line. In the list
`up`, `down`, and `j` and `k` before anything is typed, move; typing keeps the choices with what is typed
in their names, shown on the list's top rule, and `backspace` takes it back; `enter` or a click chooses;
`esc` or a click outside the list closes it as it was. A list longer than the screen has room for
scrolls, with how many are out of view above and below. While it is open the form's keys are the list's,
so `enter` there never submits the form, nor `esc` cancels it; and as `enter` on a choice opens its list,
the form's button says no key there, and `enter` submits from the other fields. Every form with a choice,
below, has these. With a command chosen, the new session's first tab runs it rather than a shell, and
starts your shell once it exits. A required input left empty refuses to create.

An input's field takes several lines, for a prompt: `shift+enter` starts a new one, as do `alt+enter` and
`ctrl+j`, for a terminal that sends `shift+enter` as `enter`, and `enter` still submits the form. The field
grows to show its lines, a long one wrapped, up to six, then scrolls with the cursor, and the rows below it
move down. `up` and `down` move between its lines, and leave it only from the first or the last. While it
has focus the keys row lists `shift+enter newline`. Every form with an input, below, has these.

With [layouts](kitty.md#layouts) configured, a `layout` field comes before `command`, set at first to the
space's layout for its linked worktrees, else `none`, and below it a field for each of its inputs. With a
layout chosen the session opens with its tabs rather than one shell tab; with a command too, the command
then runs where its `open` says, by the pane the layout left focused.

**Run** chooses a command, with the same `command` field when there is more than one, and has a field for
each input of the one chosen. It says where the command opens: as the session's first tab when the session
has no windows, else where the command's `open` says, beside the window switching to the session focuses.
Run switches to the session and opens it there.

**Open layout** chooses a layout, the worktree's default at first, else the first configured, with a
`layout` field when there is more than one, and has a field for each of its inputs. It says what each tab
holds, and whether the tabs go after the session's own or, for a session with no windows, open it. Open
builds them, as [layouts](kitty.md#layouts) says, and switches to the session.

**Save as layout** saves the session's tabs, as kitty has them, as a [layout](kitty.md#saving-a-layout):
a `name` field, then `save`, `every tab` at first or `this tab`, which is the tab the sidebar is in for its
own session, else the session's most recently focused. `save to` says where: `SPACE's .kspaces.toml`, the
space's [own config](kitty.md#per-space-config), at first, or `the global config`, config.toml. A space
file that is not there yet is made, and a box below, `keep out of git (adds it to .git/info/exclude)`,
ticked at first, `space` or a click to change, adds it to the repository's `info/exclude`; one that is
there but not trusted is refused, and the form says so under the field. Below them a line for each pane of
the tabs saved, `tab 1, pane 2: claude --resume abc`, saying what runs there now, then `runs`, chosen as
`command` is in the forms above: a configured command, the space's own included, at first the one whose
program is what runs there, as `claude --resume abc` is the `claude` command; `run`, at first for anything
else, with a `run` field holding its command line, a template, to edit; or `shell`, at first for a pane at
its prompt. Save adds the layout at the end of the file as text, so the rest of it stays as it was, and
writes through a symlink to its target; a space file kspaces writes is trusted as written. A name the file
has already asks whether to replace it, `y` or `n`; Replace puts the new layout where the old one's lines
were. A layout saved to the space's file under a name config.toml has takes that one's place in the space.
`kspaces layout save [NAME] [PATH]` opens the same form for the worktree PATH, or the current directory, is
in, its name filled in, with `--tab` for this tab at first: the one kspaces runs in, if it is the
session's.

**Close session** closes every window the way closing them by hand does, so whatever kitty.conf runs when a
window closes still runs.

**Remove worktree** shows the directory, the session's windows and the files that are not committed, which
are lost. It closes the session, waits for its windows to go, deletes the session's saved file and runs
`git worktree remove`. What git would refuse, such as a locked worktree, is said before you confirm. The
removal runs as a kitty background process, so an error after that is shown by kitty.

The branch is kept unless you tick **Delete branch**, off at first, with `d` or a click. The box is not
there for a detached HEAD, the default branch, or a branch another worktree has checked out. It says what
deleting loses: nothing when the default branch or the branch's upstream has all its commits, or when the
branch's PR, as the sidebar last saw it, is merged though its commits are not, as after a squash;
otherwise how many commits only the branch has, which are lost. The branch goes after the worktree, with
`git branch -d`, or `-D` only when the form said commits are lost or the PR is merged. A remote branch is
never deleted. If git refuses after the worktree is gone, as `-d` does a branch its upstream lacks, kitty
shows why and the worktree stays removed.

**Clean up worktrees** lists the space's linked worktrees whose branch's PR is merged, whose upstream is
gone, or whose branch is merged into the main worktree's, each with why and the files it has not
committed. A branch merged into the main worktree's counts only once it has moved since it was made, so a
new worktree not worked on yet is not offered. Each has a box: ticked at first unless it has changes,
which ticking it says are lost, and a locked one cannot be ticked. `space` ticks the cursor's, and `y`
removes the ticked ones one after another, each as Remove worktree does, in one kitty background process.
git is forced only for those the form said have changes. The PR is the one the sidebar shows, or looked
up with gh as the form opens when the sidebar has none yet.

**Also delete their branches**, off at first, `d` or a click, deletes each removed worktree's branch by
Remove worktree's rules, and gives each worktree a line saying what deleting its branch loses, or why it is
kept. A branch whose commits are not merged is deleted with `-D`, its line saying the commits are lost once
its worktree is ticked; one whose PR is merged, as after a squash, says so and loses nothing.

**Forget** shows the saved session file of a missing worktree with no windows open, and Delete deletes it,
so kitty does not restore the session. `kspaces forget SESSION` does the same for any session with no
windows open; `--yes` without asking.

**Trust space config** shows the space's `.kspaces.toml` and what it would add, as
[Per-space config](kitty.md#per-space-config) says: each command and the line it runs, each layout and
what each of its panes runs, and the settings it sets. Trust, `y`, trusts what it shows; a file with an
error says what it is, and cannot be trusted until fixed. `kspaces trust [PATH]` does the same for the
space PATH is in; `--yes` without asking, `--revoke` to forget it.

**Add space**, which the plus at the right end of the SPACES title also opens, has a field for a directory,
filled in with the repository of the window the sidebar is beside, and shows the root it adds: the top of
the directory's work tree, `~/` for the home directory. A directory in no repository, or a repository
already in the spaces, is refused before you confirm. Add appends the root to `spaces` in config.toml as
text, so the file's comments and layout stay as they were, and writes through a symlinked config to its
target. Every sidebar shows the new space at its next refresh.

## Status

The STATUS list shows every window whose program reports its state with OSC 7501, kitty's program status
protocol, grouped as Blocked, Failed, Done, Working and Idle. Each entry has the session, how long ago the
state changed, and a summary: progress, the title or the message. Clicking an entry, or `enter` on it,
focuses its window.

With the cursor on an entry it opens up to show everything: what a blocked program waits for, the title,
progress, the message, the record's id and the window's other records. Resting the mouse on an entry shows
the same in a popup, when there is more than the summary.

kitty keeps the records and ends them by the protocol's rules: working and blocked at the next shell
prompt, done and failed when the window is focused or typed in. The age comes from the kitty watcher; without
it entries show none.

`!` focuses the window that needs you most: the first blocked, failed or done entry, in the list's order.
`kspaces next --attention` and `prev --attention`, which kspaces.conf's `kspaces_attention_next` and
`kspaces_attention_prev` run, step through all of them from the focused window, wrapping around, and do
nothing when none needs you. Focusing a done or failed window ends its record, so it drops out of the list.

## Keys

The sidebar takes keys when it has keyboard focus. kitty's `send-key` can send them without focusing it,
as the [kitty setup](kitty.md#keys-without-focusing-the-sidebar) shows; the cursor then starts on the
sidebar's own session and goes after `enter`, `esc` or ten seconds without a key.

| Key | Does |
|---|---|
| `j` / `k` | down / up |
| `g` / `G` | first / last row |
| `enter` / `l` | open |
| `space` | fold or unfold |
| `m` | menu |
| `n`, `c`, `x` | new worktree, close session, remove worktree (forget, on a missing worktree) |
| `p` | clean up the space's worktrees, on a space's row |
| `R` | run a command in the worktree, with commands configured |
| `L` | open a layout in the worktree's session, with layouts configured |
| `b` | open the worktree's PR in the browser, when its row shows one |
| `a` | add a space, wherever the cursor is |
| `!` | focus the window that needs you most, wherever the cursor is |
| `/` | filter the spaces list; see [Spaces](#spaces) |
| `s` | sort the list the next way; see [Order](#order) |
| `K` / `J` | move the cursor's space, or linked worktree within its space, up / down |
| `r` | refresh, which also happens every two seconds |
| `esc` / `q` | give focus back to the window it came from |

Each list scrolls on its own. A list with entries out of view says how many on its first or last row;
clicking that row scrolls a page.

## The picker

`kspaces pick`, which kitty.conf's `kspaces_pick` runs in an overlay, lists the status entries and then
every session and worktree in the sidebar's order, narrowed by a fuzzy query. `enter` switches to the
entry under the cursor. A missing worktree has the sidebar's icon for it and `gone` for its branch, and is
not switched to without windows open.

Beside the list in an overlay 90 columns or wider, and below it in a narrower one, a preview shows the
entry under the cursor: a status entry in full, as the sidebar's opened entry; a worktree's space, path,
branch, its changes, upstream and PR as its row shows them, its session, how many windows it has open,
whether it has a saved session file, and its status entries. A missing worktree's path says its directory
is gone, with no branch or git state. An overlay too short for both shows only the list.

Under what it says of the entry, where there is room, the preview shows the bottom of a window's screen,
with its colors, under a rule: a status entry's own window, or for a session with windows open the one
switching to it focuses, the one focused most recently, as `goto_session` picks it. Without focus times
from kitty, it is the active window of the session's active tab. A session with no windows open shows no
screen, and neither do the picker's own window, the docks and the forms. The screen is read with
`kitten @ get-text` when the cursor lands on the entry and again every second while it stays, so it
follows what the window is doing. Only colors and styles are kept from it; anything else the window drew,
such as links or cursor moves, is left out. Beside the list there is room for most of a screen; below it
only under an entry with few details, such as a status entry.
