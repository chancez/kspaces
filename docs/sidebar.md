# The sidebar

The sidebar fills its dock and draws no line against the window beside it: kitty's dock border, if any,
is the separator. The cursor's bar shows while keys move it.

## Spaces

Each row starts with a fold triangle for a space with linked worktrees, then an icon: a repository for a
space, a branch for a linked worktree, a terminal for a session in no space, or a cross for a space whose
worktrees could not be listed. The most urgent status of the row's sessions replaces the icon. The icon is
green when the session has windows open; the row is faint when it has none, its icon a pause sign (`z` in
ASCII, a double bar in Unicode) once the session is [suspended](kitty.md#closing-and-suspending-sessions).
A worktree being created has an hourglass (`+` in ASCII, a dotted circle in Unicode) and one being removed
a trash can (`~` in ASCII, a circled minus in Unicode), in the accent, over its status. A new worktree's
row shows from when the form or `kspaces new` starts making it. Until it is done the row offers no action:
opening it, from the sidebar, the picker or `kspaces open`, says it is being created or removed. Whatever
creates or removes a worktree marks it so in `pending/` in the state directory, a file naming the process,
which is ignored once that process is gone.
The sidebar's own session's
row is marked at its left edge, where a caret goes, by a thin bar in the accent (`|` in ASCII), or by its
caret in the accent on a space's row, and its name is bold: a bar across the row would read as the
cursor's.

At the right end of a worktree's row, a space's own row included, is what git and GitHub say of it: a dot
and 3 for three files not committed, an up arrow and 2 and a down arrow and 1 for two commits ahead of its
upstream and one behind, and `#123` for its branch's PR, green when open, grey as a draft, purple merged
and red closed. A space after the number is what the PR's checks come to: a green check when all passed, a
red cross when any failed, errored, was cancelled, timed out or needs action, else a yellow circle while
any is still to run (`v`, `x` and `~` in ASCII), and nothing for a PR without checks. A space after that
is its review: a green star approved, a red plus-minus with changes requested (`A` and `R` in ASCII), and
nothing while a review is only required. They are faint with the row, as the number is. The branch is cut
to make room for them, and a long worktree name for the changes and the PR, down to 6 cells; in a narrow
sidebar ahead and behind go first, then the review, the checks, the PR, and the changes last. Anything
cut ends in a one-cell ellipsis, `...` in ASCII, unless there is no room for it and a cell of text (2
cells, 4 in ASCII), when it is cut bare. Only the
leading sidebar asks: `git status` every 10 seconds, and `gh pr list --head BRANCH` every 3 minutes, its
checks and review decision in the same call, which needs gh signed in to GitHub. Without gh, a GitHub
remote or a network there is no PR, and none is looked up for the repository's default branch. `r` asks
both again at once.

A space with its [own config](kitty.md#per-space-config) that is not in use says why on its own row, where
the git state goes: a lock and `untrusted config` (`?` in Unicode and ASCII) while the file is not trusted
as it is, or the cross of a broken space and `config error` when it has an error, which `kspaces list`
shows. A narrower row says `untrusted`, then shows the mark alone.

A worktree whose [shared layout](kitty.md#shared-layouts) on a host has diverged from its panes here ends
its row with a muted `~ diverged`, after the git state, as does a folded space's row when one of its
worktrees has. On the hosts tab it is under the host the layout diverged from alone. It is not in the
status list, which is for what programs say of themselves. See [Shared layouts](#shared-layouts) for
resolving it.

Sessions that are open or saved but belong to no worktree, such as the one kitty starts with, are listed
above the spaces.

A session named `SPACE@DIR` after a space that matches none of its worktrees, open or saved, is a missing
worktree of that space: its directory is gone, as when `git worktree remove` runs while its session still
has windows. It is listed under the space after its linked worktrees, with a broken link (a cross in
Unicode and ASCII) for its icon and `gone` where the branch goes, and folds with the linked worktrees.
While its session has windows it is drawn as an open session is, but with its icon in yellow, or its
status's icon when it reports one; with only a saved file it is faint.
git is not asked about it. A missing worktree with windows open can be opened and
closed as any session; one with only a saved file cannot be opened, as that would make it afresh in a
directory that is gone, and Forget deletes the file. A space whose worktrees could not be listed has no
missing worktrees: its sessions are listed above the spaces as before.

Clicking a row, or `enter` on it, switches to its session, creating it the first time, with its
[default layout](kitty.md#layouts) if it has one, but for a missing worktree with no windows. Clicking a
space's triangle, or `space` on its row, folds its linked worktrees into the space's row. Folded spaces are
kept in `$XDG_STATE_HOME/kspaces/state.json`, so they stay folded across restarts.

Resting the mouse on a row for 2 seconds shows its worktree in detail in a popup under it, as the
picker's preview does: its space, path, branch, changes, upstream, PR and its state, checks and review in
words (`checks   failing`, `review   changes requested`) when it has them, session, windows,
saved file and status entries, each cut to the sidebar's width. Passing over rows shows nothing. The popup
closes as a status entry's does, and a click on it is one on its row.

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

`f`, or a click on the funnel on the SPACES title, left of the sort button, shows only what is going on:
the rows whose session has a status that is not idle, blocked, error, done or working, with their spaces'
rows above them, and a folded space's row when any of its sessions has one. The funnel, a white
down-pointing triangle in Unicode and `Y` in ASCII, is lit in the attention color while the filter is on,
and with nothing to show the list says `Nothing needs you`. It applies along with `/`, so a query narrows
what it shows. The cursor stays on its row while it shows, and goes to the first row shown when it does
not. `f` or the funnel again shows the whole list. It is kept in `$XDG_STATE_HOME/kspaces/state.json`, so
it stays on across restarts, and does not end with navigating as the query does.

`O` shows only the open sessions: the rows whose session has windows, with their spaces' rows above them,
and a folded space's row when any of its sessions has windows. With nothing open the list says
`Nothing is open`. It applies along with `/` and `f`, and is kept as `f` is.
`O` again shows the whole list.

`Z` hides the [suspended](kitty.md#closing-and-suspending-sessions) sessions: their rows go, a space's
row staying above the rows of it that show, and a folded space's row goes only when all of its sessions
are suspended. With everything suspended the list says `Everything is suspended`. It applies along with
`/`, `f` and `O`, and is kept as they are. `Z` again shows the whole list.

The search button, left of the funnel, opens the [picker](#the-picker), as `P` does; a sidebar too narrow
for it drops it. While any filter is on the funnel is lit, and the SPACES title names the filters on after it, muted:
`active` for `f`, `open` for `O`, `awake` for `Z`, joined by commas. A sidebar too narrow for the whole summary drops it.

A right click on the SPACES title or its buttons, or on an empty row of the list, opens the list's menu:
the sort modes, see [Order](#order), in a Filter section `Only what is not idle`, `Only open
sessions` and `Hide suspended sessions`, each ticked while its filter is on, which turn them on and off, in a Rows section **Select**, see
[Selecting](#selecting), and with [groups](#groups) configured a Groups section.

## Groups

With spaces in [groups](kitty.md#spaces-and-sessions), set in kspaces.toml, the spaces in no group come
first, under no header, after the sessions in no space; then each group, in `groups`' order, those it
does not list after by name. A group has a header row above its spaces, drawn as a divider rather than a
row: a rule in the column of the spaces' fold triangles, the group's name in capitals, its fold icon,
and the rule on to the right edge, all muted, as a quieter SPACES. The icon is an open folder while the
group's spaces show and a closed one when it is folded, a minus and a plus in Unicode and ASCII, where
the rule is `-`. A narrow sidebar drops the rule after the icon first, then cuts the name, keeping the
icon. While filtering, which ignores folds, there is no icon. Within a group the spaces are in the
[sort mode](#order) in use.

Clicking anywhere on a header, its rule, its name or its icon, or `space` on it, folds the group's spaces
into it, as a
space folds its worktrees; its menu has **Fold group** and **Unfold group**. Folded groups are kept with
the folded spaces, in `$XDG_STATE_HOME/kspaces/state.json`, as `group:NAME`, which no space's root is. A
folded group's header stands for its spaces' sessions, as a folded space does for its worktrees: it is
marked as the sidebar's own session's row while that session is in it, and the attention filter shows it
when any of them is not
idle.

The list's menu has a Groups section, each group ticked while it shows; choosing one hides it, header and
spaces, and choosing it again shows it. Hidden groups are kept in `$XDG_STATE_HOME/kspaces/state.json`, so
they stay hidden across restarts, unlike the attention filter: hiding work for the evening should not
come back with a restart.
A hidden group is out of sight, so its sessions no longer call for attention: their entries leave the
STATUS list, `!` and `kspaces next --attention` skip them, and the attention filter, which shows only
rows, has none of them. They are found by name, the space's or `SPACE@DIR`. The picker still lists them,
as it searches everything. The sessions in no space are in no group, and always show.

## Detached sessions

A [session provider](kitty.md#session-providers), a program the config names, owns what runs in windows and
the sessions that run with no window, a multiplexer's on this machine or another. With one that lists its
sessions, the leading sidebar asks it at each refresh, with kitty's windows, and it says which windows
show which of its sessions. The worked example is [cm](https://github.com/chancez/cm), with a launch
wrapper that runs each window in a cm session, filed by the [launch
variables](kitty.md#spaces-and-sessions) under the space of its kitty session: `kspaces-provider-cm`
answers for it, and a window shows the session its cm client is attached to.

A space's sessions that no kitty window shows are listed after the row of the worktree they are in, the
one their `worktree` tag names, else the space's main worktree, as one from before tagging or from outside
kspaces is. They are under a row of how many there are, `14 detached`, or by host once the provider lists
more than one machine, `local 3, server 9`. The sessions a window shows are where their windows are, and not listed again. `space` on the row, or a
click on it or its caret, shows them under it, by name, under a row for each host when there are several,
and again hides them; they start hidden, and showing them is not kept across restarts. A session's row has
its name, what it is doing in a word, `busy` while a command runs, else `running`, `shelved`, `exited` or
`dead`, then its title, else its command. Its icon is its program's status when it reports one that is not
idle; one that is not running is faint. A folded space or group shows none of them, and nor do the query,
`f` and `O`, which look for kitty sessions.

`enter` on one, or a click, opens a tab running what the provider's `open` says, such as `cm attach ID`,
which can bring back one that has ended or been shelved. The tab goes where the session belongs: in the linked
worktree's session its `worktree` tag names, when the space has that worktree; else in that of the
worktree whose directory holds the session's directory, the deepest; else in the sidebar's own session,
when that is in the space; else in the space's own session. It starts where the provider says, else in the
session's directory, and closes when its command ends, as at a detach. A session with no windows is
restored from its saved file first, or gets the tab as its first, without its default layout.

What a row offers is what the provider does, as its `capabilities` say, asked once per sidebar and again at
`r`; a provider without `shelve` shows no Shelve:

- `x` on a running session kills it, and on an ended one removes it, in a form that confirms with `y` and
  says what a busy one runs; `F` removes an ended one too.
- `z` shelves a running one, at once, as it ends it only so that it comes back as it was.
- `M` moves one to another space, in a form: typing a name lists the provider's spaces it matches, the
  session's own left out, `down` and `up` choose one, and `enter` moves to it or, with none chosen, to the
  name as typed, a new space, which the provider creates. This is how sessions that all landed in `default`
  are filed where they belong.
- `p` on a row of how many opens **Clean up sessions**, listing all of the space's detached sessions, each
  with a box, ticked at first for those that exited or died, not for a shelved one, which was ended to come back,
  nor a running one, which may be someone's work, and a busy one says what it runs. `y` kills the ticked
  running ones and removes the ticked ended ones; one the provider refuses is said in the form, which
  stays, and the rest still go.

In the [select mode](#selecting) a session's row ticks as a worktree's does, and the row of how many ticks
all of the space's; **Move** `M` on the bar moves the ticked ones in one form.

`kspaces sessions kill ID...`, `remove ID...`, `move ID... [--to SPACE]` and `cleanup SPACE` open the same
forms, by the provider's ids; `--yes`, or `--to`, without asking.

A space the provider lists that is no configured space's is a topic space: one kitty session named after
it, listed with the sessions in no space above the spaces, whether or not it has windows. Opening it makes
the session with one shell tab in the home directory, as for any session in no space, and its detached
sessions are listed under it as a space's are. `default` is one too.

Without a provider, or one that does not list, there are no detached sessions and no topic spaces, and
nothing says so. A list that fails leaves the last one that did not showing, so a provider restarting
under the sidebar blanks nothing; after three failures in a row it is asked again a minute later, or at
`r`.

### The hosts tab

With a provider that lists sessions, or [other hosts](kitty.md#other-hosts) in the config, the title is two
tabs, `SPACES HOSTS`. `tab`, or a click on the other
tab, switches between them, and the choice is kept across restarts. The hosts tab lists the spaces tab's
rows by host: a row for each host, `local` first and then the rest by name, and under it what the spaces
tab would list of that host. This machine has the whole list. Another host has the spaces and the
sessions in no space it runs a session in: a space keeps its own row and, of its linked worktrees, those
its sessions there are tagged with, and a worktree only that host has is listed after them by its
directory. Each host's detached sessions are under their own `N detached` row, which shows and hides them
there alone. `space` or a click on its caret folds a host's row, not kept across restarts. A host's row has
the rule across the sidebar that a group's header has on the spaces tab, and a group's header under a host
has none, so a group reads as inside its host rather than as the next one.

A host the config names lists its own copies of its spaces instead, as its kspaces server says: their
worktrees there and their git state. Its row says `unreachable` while its server cannot be reached, and
its copies stay as it last said. Nothing that works in a directory here applies to a copy, so its menu
offers only folding.

A row does what it does on the spaces tab, so a worktree listed under several hosts opens the same kitty
session from each. `enter` on a worktree only another host has opens a tab of one of its sessions there,
in a kitty session of its name. The filters narrow each host's rows as they narrow the spaces tab's, and
a host they leave nothing of is left out. Rows do not move on the hosts tab, by drag or by `K` and `J`.

## Order

The button left of the plus on the SPACES title shows how the list is sorted, and a click on it goes on
to the next way, as `s` does; a right click opens the list's menu, the three ticking the one in use, see
[Spaces](#spaces).

| Mode | Icon | Order |
|---|---|---|
| Manual | bars; a triple bar in Unicode, `=` in ASCII | the spaces as kspaces.toml lists them, and each space's linked worktrees as they were moved, any not moved yet after them in git's order |
| Activity | a clock; a clock face in Unicode, `@` in ASCII | the session focused most recently first, a space by its most recent session; sessions with no windows last, in manual order |
| Status | a warning sign; `!` in Unicode and ASCII | the most urgent status first, as the status list ranks them, blocked, failed, done, working, idle, then none; a space by its most urgent session, ties by activity |

In every mode a space's main worktree stays its own row, first, and its missing worktrees last; only its
linked worktrees move within it. Spaces in [groups](#groups) are sorted within their group, and the groups
keep their order. The sessions in no space stay above the spaces, ordered the same way, by
name when manual. Activity is when one of a session's windows, docks left out, was last focused, as
kitty's `ls` reports it. The mode is kept in `$XDG_STATE_HOME/kspaces/state.json`, manual until one is chosen,
so every sidebar sorts the same way.

The automatic modes sort again at each refresh, and the cursor stays on its row wherever that goes. While
the mouse is pressed on a row or a menu is open the rows stay where they are, so what is under the mouse
does not move from under it.

Dragging a space's row moves the space, and dragging a linked worktree's row moves it among its space's.
The rows show where it goes as the mouse moves, the dragged row lit, and letting go keeps it there; `esc`
or a right click while dragging puts it back. A press let go of on the row it was on is a click, which is
why a row opens when the mouse is let go rather than pressed. `K` and `J` move the cursor's row up and
down the same way. A session in no space and a missing worktree do not move, and neither does anything
while a filter is on, nor by dragging in the [select mode](#selecting), where a drag ticks rows. `K` and
`J` move a space among the spaces of its group, or of none, and do nothing past the first or the last of
them. Dragging a space onto another group's space, or onto its header, first in it, moves the space into
that group; onto a session in no space, first in no group. Groups are not reordered by moving, only by
`groups` in kspaces.toml. Moving a space rewrites the order of `spaces` in kspaces.toml, or in the
[config file](kitty.md#more-config-files) that lists it, as text, each entry with the comments on the lines
right above it and after it on its line, through a symlinked config to its target, each group's spaces
together as the list shows them. Moving it into another group rewrites its entry too, a path becoming
`{ path = "~/src/x", group = "work" }` and a table's group changing or going, and moves the entry, with its
comments, to the config file that lists the most of that group's spaces, as **Add space** picks the file,
so a space dragged into the group a `conf.d/work.toml` holds goes there. A group set in a `[space.NAME]` table
stays that table's: dragging the space out of it says to change it there. Moving a worktree keeps its
space's order, by the worktrees' directories, in `$XDG_STATE_HOME/kspaces/state.json`. Either moves the order of
every sidebar.

Moving in activity or status switches to manual, starting from the order shown: the spaces are written to
kspaces.toml as they were shown and every space's worktrees saved as they were shown, then the move is
made.

## Selecting

A click opens a row and a drag moves it, so acting on several rows at once, as removing the worktrees a
week of work left behind, has a mode of its own. `v`, **Select** in a row's menu, which ticks the row, or
in the list's menu, or alt and a click on a row, which ticks it, turns it on. alt, as kitty keeps shift and
a click or a drag for selecting text even in a program that takes the mouse.

While it is on, every row that can be acted on has a box after its caret, ticked or not, `[ ]` and `[x]` in
ASCII and a ballot box in Unicode: linked and missing worktrees, sessions in no space, and a space's own
row, its main worktree. A click on a row ticks or unticks it rather than opening it, and a click on a caret
still folds. Pressing on a row and dragging gives every row the mouse passes what the first one changed
to, ticked or not, and moves nothing; alt and a drag outside the mode turns it on and does the same. A
space's row stands for the rows under it: ticking it ticks its main worktree and its linked and missing
worktrees, shown or folded, and unticking it unticks them, and while only some are ticked its box is partly
ticked, `[-]`. A group's header does the same for its spaces' rows. Under a filter they stand for only the
rows it shows, and hiding a group unticks its rows, so nothing out of sight is acted on. `space` ticks the
cursor's row, `j` and `k` move as ever, `A` ticks every row shown, and `esc` leaves the mode, letting go of
the ticks. No row gets the cursor's bar in the mode: a click changes only its row's box and leaves the
cursor where it was, and the cursor is the box drawn lit, reversed, as that is what `space` ticks.

A bar at the bottom of the spaces list, above the STATUS rule, is titled by a rule with how many rows are
ticked set into it, `8 selected`, or the number alone in a narrow sidebar. Under it is a row for each
action on the ticked rows, as a menu's items: **Remove** `x`, **Close** `c`, with the session saver
**Suspend** `z`, **Forget** `F`, with commands configured **Run** `R`, with a provider that moves **Move**
`M`, for the ticked [detached sessions](#detached-sessions), and last **Cancel** `esc`. Each row
has the action's name on the left, then how
many of the ticked rows it applies to and its key, each in a column of its own, so the rows line up; an
action that applies to none of them is muted, with `-` for its count, and stays in its row. A click
anywhere on a row runs it, and the row under the mouse is lit, but a muted one, which does nothing. The
hint line under the rows says what the muted ones have nothing to do to, as `Nothing to close or forget`,
and with the mouse on a row what it does, or why it is muted, as `None has windows open`; the key says
the same on the sidebar's last line. A sidebar too narrow for the columns drops the counts first, then
the keys, rather than wrap a row; the keys work whether shown or not.

- **Remove** opens [Clean up worktrees](#the-menu), titled **Remove worktrees**, with every ticked row:
  linked worktrees ticked unless they have changes, with its checks and **Also delete their branches**,
  missing ones to close and forget, and a main worktree or a session in no space listed as refused, so it
  says why it leaves them.
- **Close** opens one form listing the ticked sessions with windows open, and closes them all.
- **Suspend** does the same, suspending them.
- **Forget** opens one form listing the saved files of the ticked sessions with no windows open, missing
  worktrees and closed ones, and deletes them.
- **Run** opens the Run form for the ticked worktrees, but missing ones, with the first one's first
  command chosen, and runs it in each, a tab each as Run opens it, without switching sessions; one that
  fails is said on the form, which stays, rather than on the sidebar's last line, as the form is a process
  of its own.

Each form confirms in a popup of its own, so the mode ends once the rows show the action done: a worktree
gone from the list, a session with no windows, a saved file gone, or a session with a window more. A form
cancelled changes nothing, and the ticks stay for another go.

## Shared layouts

With a provider that [shares layouts](kitty.md#shared-layouts), a worktree's tabs follow it between
machines. When this machine's panes and a host's layout both changed while apart, the worktree has
diverged from that host, and its row ends with `~ diverged` (see [Spaces](#spaces)) until it is resolved:
each machine keeps showing its own version, and nothing is shared between the two.

`enter` on a diverged row changes nothing and says on the last line what to do, as `gr@dev diverged from
desktop: pick a layout from the menu (m)`. The row's menu ends with a Layout section of three items, each
for the layout as a whole: Use this machine's, Use HOST's and Keep both, the first two with how many tabs
each side has and how old it is, as `5 tabs now` and `2 tabs 3h`. They have no keys and are last, so
neither a key pressed by habit nor `enter` on the item the menu opens on resolves anything: move to one
with `j` / `k` and press `enter`, or click it. A pane closed in resolving
only has its window closed; its session keeps running, and the detached row lists it. A row standing for
several divergences, as a folded space's or a worktree's diverged from two hosts, offers none: each is
resolved from its own row, on the hosts tab under the host it diverged from.

## The menu

Right-click a row or a status entry, or press `m` on it. The menu lists what applies to the row, in
sections, each named on the rule above it: what acts on the row's session, then on its worktree, then on
the space as a whole, the same from any of its rows. A status entry's has Open, Reply and, on one with a done or failed record, Clear. A menu of one
section names none. Each item shows its key at its right edge, when the sidebar leaves room for it beside
the label, and the key runs the item from the menu as it does on the row. `j` / `k` and the mouse go over
the items only. `esc`, a click outside it, or a click in another window closes it. On a hover popup, a right click is on the entry it shows. On the SPACES
title, its buttons or an empty row of the list, a right click opens the list's own menu, of the sort modes,
the attention filter and the groups; see [Spaces](#spaces). A group's header's has Fold group or Unfold
group, and Add space.

| Section | Item | Key | Does |
|---|---|---|---|
| Session | Open | `enter` | switch to the session |
| Session | Open without layout | `o` | switch to the session, opening one tab rather than its [default layout](kitty.md#layouts); on a worktree whose session has no windows and no saved file that would restore it, with a default layout |
| Session | Run | `R` | run a configured command in the worktree's session, in a form; with `[[commands]]` configured |
| Session | Open layout | `L` | add a configured layout's tabs, or one shell tab, to the worktree's session, in a form; with `[[layouts]]` configured |
| Session | Save as layout | | save the session's tabs as a layout in kspaces.toml, in a form; on a worktree's session with windows open |
| Session | Close session | `c` | close every window of the session, on a session with windows open; with the session saver, keeping it to open again |
| Session | Suspend session | `z` | stop what runs in the session's windows and close them, keeping them to resume, on a session with windows open; with the session saver |
| Session | Reply | `t` | write to the window of a status entry without switching to it, in a form; on a status entry |
| Session | Clear | `x` | drop a status entry's done and failed records without focusing its window; on a status entry with one |
| Worktree | Open PR #N | `b` | open the branch's PR in the browser with `gh browse N`, on a worktree whose row shows a PR |
| Worktree | Remove worktree | `x` | close the session and remove a linked worktree |
| Worktree | Forget | `F`, `x` | delete the saved session file of a session with no windows open, and what the provider kept of it; `x` on a missing worktree |
| Space | New worktree | `n` | create a worktree in the space, in a form; on a linked worktree's row, a new branch starts from its branch |
| Space | New worktree from PR | `N` | the same form on its Pull request tab, to check out one of the space's open PRs |
| Space | New worktree from issue | `I` | the same form on its Issue tab, to create a branch for one of the space's open issues, from the row's branch as New worktree |
| Space | Fold / Unfold worktrees | `space` | hide or show a space's linked worktrees |
| Space | Clean up worktrees | `p` | remove the space's linked worktrees that look done with, and its missing ones, on a space's row |
| Space | Remove space | `D` | take a configured space out of the config, its repository kept, or remove a topic space from the provider, closing its kitty sessions, in a form; on a space's row |
| Space | Trust space config | | show what the space's own config adds, and trust it, in a form; while its `.kspaces.toml` is untrusted or changed |
| Group | Fold / Unfold group | `space` | hide or show a group's spaces, on its header |
| Space | Select | | turn on the [select mode](#selecting) with the row ticked, a group's header too |
| Session | Open | `enter` | on a detached session, open a tab showing it, in a session of its space; see [Detached sessions](#detached-sessions) |
| Session | Kill | `x` | end a running detached session, in a form; with a provider that kills |
| Session | Shelve | `z` | end a running detached session so it comes back as it was; with a provider that shelves |
| Session | Remove | `F`, `x` | discard an ended detached session, in a form; with a provider that removes |
| Session | Move to space | `M` | move a detached session to another space, or a new one, in a form; with a provider that moves |
| Space | Show / Hide sessions | `space` | show or hide a space's detached sessions, on the row of how many |
| Space | Clean up sessions | `p` | kill and remove a space's detached sessions chosen in a form, on the row of how many |
| Space | Add space | `a` | add a repository to the spaces, in a form; on every row, on a group's header or its spaces' rows in that group |
| Layout | Use this machine's | | save this machine's version of a [diverged](#shared-layouts) layout over the host's; on a diverged row |
| Layout | Use HOST's | | take the host's version here, opening and closing panes to match; on a diverged row |
| Layout | Keep both | | open what the host's version has that this machine's does not, and save the two; on a diverged row |

Each item that changes something opens a form in a popup, with buttons and keys to confirm or cancel: an
overlay over the window beside the sidebar, or with `[sidebar] popup = "panel"` a centered panel of its own,
which closes when it loses focus; [docs/kitty.md](kitty.md#popups-overlays-and-panels) has the details.
A click on a text field, its label or its box, gives it focus, as `tab` and `shift+tab` do, and a click in
its text puts the cursor there; in a one-line field too long for its box, which may be scrolled, at its end.
`?` lists the form's keys in a sheet up from the popup's bottom, over up to half of it, while no text field
has focus, as on a choice or in a form that only confirms; in a text field it is typed, and `ctrl+/` lists
them. In the picker it lists the picker's keys while the query is empty, and is typed into one that is not,
where `ctrl+/` lists them; with the menu open it lists the menu's keys and its items'. The sheet scrolls and closes as the
sidebar's list of keys does, see [Keys](#keys).

An item that fails, or a key such as `space` folding a space, says why on the sidebar's last line until
the next key or click. Loading's own errors, such as a broken config, show there when no such error does,
until a load works. The line has the error on one line, cut to the sidebar's width; resting the mouse on
a cut one shows all of it, wrapped, in a popup above the line, which ends as a status entry's does. A
click on the error or its popup, or **Show error** in the menu a right click on it opens, shows it in full
in a form in a popup, as the other forms are: wrapped, `up`/`down`, `j`/`k`, `pgup`/`pgdown`, `space`
or the wheel scroll it, `y` copies it to the clipboard, and `esc`, `q` or `enter` closes it. The copy is
an OSC 52 write, which kitty's default `clipboard_control` allows. Showing the error leaves it on the
line, until the next key or click in the sidebar.

**Open PR** is the PR the row shows, which needs gh signed in. gh runs in the worktree, in the
background, so the sidebar does not wait for it; what it says when it fails shows on the sidebar's last
line.

**New worktree**, **New worktree from PR** and **New worktree from issue** open one form with three tabs,
`Branch`, `Pull request` and `Issue`, on a strip below its heading, the one shown on a chip. The strip is
the form's first field: `left` and `right` while it has focus, or a click on a tab, switch tabs. Each tab
keeps its own fields, so switching loses nothing typed. Create creates from the tab shown.

With several spaces configured, a `space` choice above the strip picks the space the worktree goes in, so a
form opened from one space creates a worktree in another. It starts on the space the form was opened for,
and changing it makes the form one for the space chosen, on the tab shown: what was typed for the space
left is kept as its draft, and the other's draft, if it has one, is restored.

The branch tab has a branch field, which starts with `worktrees.branch_prefix/`, and a directory field,
empty for the default: `worktrees.dir` under the space's root, named after the branch. An existing branch is
checked out; a new one is created from what the main worktree has checked out. When the space has other
local branches, a `from` choice below the branch, chosen as `open with` below is, picks another to start
from, to build on another branch's feature: the main worktree's first, then the rest, the last committed
to first. It starts on the branch of the worktree whose row the form was opened from, else the main
worktree's. A base other than the main worktree's is recorded in git config as the branch's `kspaces-base`,
and is [`{base}`](kitty.md#running-commands) in the worktree from then on, so a prompt diffing against
`{base}` diffs against the branch it was built on. Create switches to the new session. Typing `#` in the
branch field, after the prefix or in place of it, goes to the PR tab, typing going on in its PR field; a
PR's URL typed or pasted there goes to the PR tab with it in the PR field, and an issue's URL to the Issue
tab with it in the issue field.

The PR tab's first field is the PR: its number, `#N`, its URL, or words of its title or its author's login,
as `number, URL or title` says while it is empty. The first time the tab shows, the form lists the space's
open PRs with `gh pr list` in its root. While the field has focus the open PRs it matches show below it, up
to five, each with its author and whether it is a draft, the one of the number typed first: `up` and `down`
go through them, and the one highlighted, the first at first, is the PR Create checks out, so an empty field
is the newest. A click on one chooses it, fills the field in with its number and moves on to the directory,
closing them. A number or URL no open PR has is looked up with `gh pr view` once typing rests on it. Without
focus the field shows the PR chosen and its title.
The form shows the PR's title and the branch it checks out, which the directory follows. A PR from the
repository itself is its branch, fetched and tracking the remote's. A PR from a fork is fetched from the
remote's `pull/N/head` into a branch named after the fork's, or `pr-N-BRANCH` when the space has a branch of
that name for something else, and pulls from `pull/N/head`, as `gh pr checkout` sets up a fork it has no
remote for. The remote is the one for the repository the PR is to, else `origin`. A branch set up for the
PR before is checked out as it is. When gh cannot say, because it is not installed, there is no such PR or
the repository is not on GitHub, the form shows gh's error as it does git's.

The Issue tab's first field is the issue, chosen as the PR tab's PR is, from the open issues `gh issue list`
lists, or looked up with `gh issue view`. Below it is the branch to create for it, filled in with the
prefix, the issue's number and the words of its title, `pr/me/12-crash-on-empty-input`, and filled in again
as the issue changes until it is typed in; then `from`, as the branch tab's, and the directory, which
follows the branch. An existing branch is checked out. Create records the issue's number in git config, as
the branch's `kspaces-issue`, so [`{issue}`](kitty.md#running-commands) is the issue's number in the
worktree from then on, for a prompt such as `Fix issue #{issue}`. git deletes it with the branch.

`kspaces new [SPACE] --from-pr` opens the form on the PR tab, and `--pr N` on PR N, its number or URL,
looked up at once; `--from-issue` and `--issue N` do the same with the Issue tab. `--base BRANCH` starts
`from` on a local branch, as a worktree's row does with its branch.

With [layouts](kitty.md#layouts) or [commands](kitty.md#running-commands) configured, the form has an
`open with` field below the directory, what the new session opens with: `shell`, one tab, a shell, then
the layouts, then the commands, and below it a field for each input of the one chosen. It is set at first
to the space's layout for its linked worktrees, on the PR tab to its [`pr_layout`](kitty.md#layouts) and on
the Issue tab to its `issue_layout`, else `shell`. A layout chosen opens its tabs rather than one shell
tab; a command chosen runs as the session's first tab, which starts your shell once it exits. `tab` and
`shift+tab` go round all the fields. A choice like `open with` is drawn `< claude >`, and while it has
focus says `left/right change  space list` after it, and the keys row lists `left/right` and `space`:
`left` and `right`, or `h` and `l`, go to the one before or after; `space` or a click opens a list below the field, or
above it when it would run off the bottom, of every choice, the one chosen marked and selected, a layout
with how many tabs and panes it has and a command with its command line. In the list `up`, `down`, and `j`
and `k` before anything is typed, move; typing keeps the choices with what is typed in their names, shown
on the list's top rule, and `backspace` takes it back; `enter` or a click chooses; `esc` or a click outside
the list closes it as it was. A list longer than the screen has room for scrolls, with how many are out of
view above and below. While it is open the form's keys are the list's, so `enter` there chooses and never
submits the form, nor `esc` cancels it. Otherwise `enter` submits the form from any field, a choice
included, as the button, `Create enter`, says. Every form with a choice, below, has these. A required
input left empty refuses to create.

The last field, `background`, is a box: `space` or a click ticks it. Ticked, the form creates the
worktree and opens its session, its layout or command started, without switching to it: focus stays on
the window the form was opened from, so a prompt or a command runs while you carry on. A layout's tabs are
each left on the pane opened last rather than its `focus` pane. The box starts as it was for the last
worktree the form created.

`kspaces new [SPACE] --yes` creates the worktree without the form, as Create would, and prints its path:
on `--branch NAME`, after the configured prefix unless it starts with it, on `--pr N`, or on `--issue N`,
its branch named after the issue unless `--branch` names one. `--dir` and `--base` are the form's fields.
The session opens with the form's first `open with`, or `--layout NAME`, `--command NAME`, or the command
line after `--`, as [`kspaces run`](kitty.md#running-commands) runs one; `--input NAME=VALUE` fills in an
input, and a required one not given, or a layout or command not configured, creates nothing.
`--background` is the box.

An input's field takes several lines, for a prompt: `shift+enter` starts a new one, as do `alt+enter` and
`ctrl+j`, for a terminal that sends `shift+enter` as `enter`, and `enter` still submits the form. The field
grows to show its lines, a long one wrapped, up to six, then scrolls with the cursor, and the rows below it
move down. `up` and `down` move between its lines, and leave it only from the first or the last. While it
has focus the keys row lists `shift+enter newline`. Every form with an input, below, has these.

Left without creating, by `esc`, a click elsewhere that closes its panel, or its window closing, the form
keeps what it has: every tab's fields, choices and inputs, and the tab shown. Opened again for the space it
has them back, what it was opened with, such as a PR from `--pr`, still filled in over them. While it has
other than what it opens with, a **Clear** `ctrl+r` button puts it back so. Creating the worktree drops
what was kept, as does a week without opening it. It is kept in `$XDG_STATE_HOME/kspaces/state.json`.

**Run** chooses a command, with the same `command` field when there is more than one, and has a field for
each input of the one chosen. It says where the command opens: as the session's first tab when the session
has no windows, else where the command's `open` says, beside the window switching to the session focuses.
Run switches to the session and opens it there. `kspaces run NAME PATH...` with several worktrees opens
**Run in N worktrees**, listing their sessions and offering the commands every worktree's space has, by
name, in the first's order, NAME chosen, or else the first of them; none in common is an error. Run opens
the command in each, as its own space configures it and with the inputs it has, without switching
sessions: launch keeps the focus, so the form stays in view. An input left at its default, shown filled in
for the first worktree, is filled in for each with its own built-ins. One failing does not stop the rest,
and the form stays, a line for each that failed saying why. `--yes` runs in each without the form. Left
without running, the form keeps its command and every command's inputs, as the new worktree form does, and
has them back opened again on the same worktrees with the same NAME and `--input`s.

**Open layout** chooses a layout, the worktree's default at first, else the first configured, or `shell`,
listed first as in the new worktree form, for one tab in the worktree that is a shell, with a `layout`
field, and has a field for each of its inputs. It says what each tab holds, and whether the tabs go after
the session's own or, for a session with no windows, open it; for `shell` it says it adds one tab, a
shell, or opens the session with it. Open builds them, as [layouts](kitty.md#layouts) says, and switches
to the session; `shell` adds a tab as `launch` opens it, by the window switching to the session focuses,
or opens a session with no windows with it alone, as **Open without layout** does. Left without opening,
the form keeps its layout and inputs, as the run form does.

**Save as layout** saves the session's tabs, as kitty has them, as a [layout](kitty.md#saving-a-layout):
a `name` field, then `save`, `every tab` at first or `this tab`, which is the active tab for its
own session, else the session's most recently focused. `save to` says where: `SPACE's .kspaces.toml`, the
space's [own config](kitty.md#per-space-config), at first, or `the global config`, kspaces.toml. A space
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
were. A layout saved to the space's file under a name kspaces.toml has takes that one's place in the space.
`kspaces layout save [NAME] [PATH]` opens the same form for the worktree PATH, or the current directory, is
in, its name filled in, with `--tab` for this tab at first: the one kspaces runs in, if it is the
session's. Left without saving, the form keeps what it has for the worktree, NAME and `--tab`, as the new
worktree form does; the panes' choices come back only while the panes run what they did.

**Close session** closes every window the way closing them by hand does, so whatever kitty.conf runs when a
window closes still runs; with the session saver it saves the session first and keeps it to open again, as
[Closing and suspending sessions](kitty.md#closing-and-suspending-sessions) says. `kspaces close SESSION...`
with several sessions lists each with its windows and closes them all; one with no windows is an error,
and `--yes` closes them without asking. **Suspend session** and `kspaces suspend SESSION...` are the same
form, suspending. They, Remove worktree, Clean up worktrees, Remove space, Forget and Trust space config
confirm with `y` or `enter`, and cancel with `n` or `esc`.

With a [session provider](kitty.md#session-providers) that can end or shelve them, Close session and
Remove space have a `sessions` choice, `left` and `right`, `h` and `l`, or a click on its arrows, for what
becomes of the provider's sessions the closing windows show: **end**, at first, which kills the running
ones and removes the ended and shelved ones; **shelve**, which for Close suspends the session, exactly as
Suspend session does, so opening it again brings them back, and otherwise is the provider's shelve; or
**leave running**, which only closes the windows. It sits above what the form says the choice changes,
each name padded to the widest, so its arrows stay where they are as it changes. The line under it says
how many sessions it does that to, and what a busy one runs, in the error
color when ending or shelving stops it. Only what the provider allows is offered, and without a provider
there is no choice; one alone, as a topic space's end, is said as it is. Remove worktree and Clean up
worktrees have no choice: they end the sessions the windows show, and those of the space tagged with the
worktree, as once the directory is gone there is nothing for them to run in or come back to, and say so on
the same lines. Closing saves the session's file
and marks its windows before the sessions end, as a multiplexer's client exits with its session and its
window can close before kspaces closes it. Left to the windows closing, a multiplexer's close watcher would decide instead, and cm's
shelves an idle session and detaches a busy one, so a removed worktree's sessions came back as detached
rows of its space. `--sessions end`, `shelve` or `leave` on `kspaces close` and `kspaces remove-space
--yes` choose without the form; `kspaces remove` has no `--sessions`, and always ends them. Suspend session has no choice: it is shelving them.

**Remove space** takes a configured space out of the config, as text, its entry with its own comments and
its `[space.NAME]` tables, from the file that has them; the repository stays as it is. A topic space is
removed from the provider with its `remove-space`, which refuses `default` and a space that still holds
sessions, so a topic space's sessions are only ended, never shelved or left. Either way the space's kitty
sessions close and their saved files go, as nothing lists them under a space any more, and the provider's
sessions in it get the `sessions` choice: ended, a configured space's provider space is removed too; left
running or shelved, it stays, listed as a topic space holding them. `kspaces remove-space NAME` opens the
same form; it runs as a kitty background process, so an error is shown by kitty.

**Remove worktree** shows the directory, the session's windows and the files that are not committed, which
are lost. It ends the provider's sessions, closes the session, waits for its
windows to go, deletes the session's saved file and runs `git worktree remove`. A session the provider fails
to end does not stop it, as the removal was confirmed: kitty shows the provider's error with the rest. What git would refuse, such as a locked worktree, is said before you confirm. The
removal runs as a kitty background process, so an error after that is shown by kitty.

The branch is kept unless you tick **Delete branch**, off at first, with `d` or a click. The box is not
there for a detached HEAD, the default branch, or a branch another worktree has checked out. It says what
deleting loses: nothing when the default branch or the branch's upstream has all its commits, or when the
branch's PR, as the sidebar last saw it, is merged though its commits are not, as after a squash;
otherwise how many commits only the branch has, which are lost. The branch goes after the worktree, with
`git branch -d`, or `-D` only when the form said commits are lost or the PR is merged. A remote branch is
never deleted. If git refuses after the worktree is gone, as `-d` does a branch its upstream lacks, kitty
shows why and the worktree stays removed.

With `delete_branch = true` under `[worktrees]`, in kspaces.toml or the space's `.kspaces.toml`, the box
starts ticked when deleting loses nothing, merged or by its PR, and unticked, saying the commits are lost,
when it would lose some, so the option never drops work unasked. It only sets where the forms start, this
one opened from the sidebar or by `kspaces remove PATH`, Clean up worktrees and the select mode's Remove:
`kspaces remove --yes` follows its flags alone and deletes a branch only with `--delete-branch`.

**Clean up worktrees** lists the space's linked worktrees whose branch's PR is merged, whose upstream is
gone, or whose branch is merged into the main worktree's, each with why and the files it has not
committed. A branch merged into the main worktree's counts only once it has moved since it was made, so a
new worktree not worked on yet is not offered. Each has a box: ticked at first unless it has changes,
which ticking it says are lost, and a locked one cannot be ticked, its box greyed. The boxes are the
[select mode](#selecting)'s and take what it takes: a click ticks or unticks one, pressing and dragging
gives every row passed what the first changed to, a locked one left as it is, `space` ticks the cursor's
and `A` every one that can be. `y` removes the ticked ones one after another, each as Remove worktree
does, in one kitty background process.
git is forced only for those the form said have changes. The PR is the one the sidebar shows, or looked
up with gh as the form opens when the sidebar has none yet.

After them come the space's missing worktrees, each saying its directory is gone: ticked at first when only
its saved file is left, not when its session has windows open, which ticking it says are closed. Removing
one closes its session, waiting for its windows to go, and deletes its saved file, in the same background
process, after the linked ones (`kspaces remove --yes --forget-session SESSION`), and the form's note says
so when one is ticked.

`kspaces cleanup --session SESSION...` opens the same form, titled **Remove worktrees**, for exactly the
sessions given, in their order, each named by its session when they span spaces: a linked worktree with
its reasons, if any, and the same checks, a missing one as above, and, refused, a main worktree, which
cannot be removed, or a session in no space.

**Also delete their branches**, off at first, `d` or a click, deletes each removed worktree's branch by
Remove worktree's rules, and gives each worktree a line saying what deleting its branch loses, or why it is
kept. A branch whose commits are not merged is deleted with `-D`, its line saying the commits are lost once
its worktree is ticked; one whose PR is merged, as after a squash, says so and loses nothing. With
`worktrees.delete_branch` on, for every space listed with a branch that can go, the box starts ticked, and a
worktree whose branch would lose commits starts unticked, so only branches that lose nothing go unless you
tick it.

**Forget** shows the saved session file of a session with no windows open, a missing worktree's or a closed
or suspended one's, and Delete deletes it, so kitty does not restore the session, and has the provider forget
what it kept of a kept session. `kspaces forget SESSION...` does the same for any sessions with no windows open,
several in one form that lists each one's file and says the directory is gone only of a missing
worktree's; one with windows open is refused, and `--yes` deletes without asking.

**Trust space config** shows the space's `.kspaces.toml` and what it would add, as
[Per-space config](kitty.md#per-space-config) says: each command and the line it runs, each layout and
what each of its panes runs, and the settings it sets. Trust, `y`, trusts what it shows; a file with an
error says what it is, and cannot be trusted until fixed. `kspaces trust [PATH]` does the same for the
space PATH is in; `--yes` without asking, `--revoke` to forget it.

**Add space**, which the plus at the right end of the SPACES title also opens, has a field for a directory,
filled in with the repository of the window the sidebar is beside, and shows the root it adds: the top of
the directory's work tree, `~/` for the home directory. A directory in no repository, or a repository
already in the spaces, is refused before you confirm. With spaces in [groups](#groups), a `group` choice
below it puts the new space in one, `none` or the groups in the sidebar's order: from a group's header, or
one of its spaces' rows, that group at first, else `none`. Add appends the root to `spaces` as text, so the
file's comments and layout stay as they were, and writes through a symlinked config to its target: in a
group as `{ path = "~/src/x", group = "work" }`, in the config file that lists the most of the group's
spaces, whatever its name, so a `conf.d/work.toml` holding the work group's spaces gets the new one, else in
kspaces.toml, which the form names when it is another file. Every sidebar shows the new space at its next
refresh. `kspaces add [PATH] --group NAME` opens the form on a group. Left without adding, the form keeps
its path and group for the next time it opens on the same PATH and group.

**Reply**, on a status entry, is for answering a program that waits, a blocked one above all, without
switching to it. It shows the last lines of the entry's window's screen that fit, with their colors,
fetched again every second, and under them a `reply` field, which takes several lines as an input does.
`enter` sends what it holds and then Enter, or Enter alone when it is empty; `ctrl+s` sends it as it is,
without Enter, as for a single key a choice menu takes. `shift+enter`, `alt+enter` or `ctrl+j` start a new
line, which goes to the program as a line feed. The form then closes; it does not switch sessions. Left
unsent, the reply is kept for the session, as the new worktree form keeps what it has.
`kspaces reply WINDOW_ID [TEXT]` sends TEXT and Enter to the window, `--no-enter` TEXT alone, and with no
TEXT opens the form. [docs/kitty.md](kitty.md#replying-to-a-window) says how the text gets there.

## Status

The STATUS list shows every window whose program reports its state with OSC 7501, kitty's program status
protocol, but for the sessions of [hidden groups](#groups), grouped as Blocked, Failed, Done, Working and
Idle. Each entry has the session, how long ago the
state changed, and a summary: progress, the title or the message. Clicking an entry, or `enter` on it,
focuses its window.

With the cursor on an entry it opens up to show everything: what a blocked program waits for, the title,
progress, the message, the record's id and the window's other records. Resting the mouse on an entry shows
the same in a popup, when there is more than the summary.

kitty keeps the records and ends them by the protocol's rules: working and blocked at the next shell
prompt, done and failed when the window is focused or typed in. The age comes from the kitty watcher; without
it entries show none.

An entry with a done or failed record has a button at the right end of its first row, which clears those
records, as does `x` on it: kitty drops them without the window being focused, and an entry with no
other record leaves the list. Working, blocked and idle records cannot be cleared, as they end by
themselves at the next prompt. The button on the STATUS title, or `X` wherever the cursor is, clears every
done and failed record the list shows, and is there only while it shows one. A program that reports again brings its entry back, as does a multiplexer that replays the
records it keeps when a window attaches to it.

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
| `ctrl+d` / `ctrl+u` | half a page down / up, in the list the cursor is in |
| `ctrl+f` / `ctrl+b`, `pgdown` / `pgup` | a page down / up |
| `g` / `G` | first / last row |
| `enter` / `l` | open |
| `space` | fold or unfold the space's worktrees, on a group's header the group's spaces, or on a space's detached sessions' row show or hide them |
| `x`, `F`, `z`, `M` | on a detached session: kill, or remove an ended one; remove; shelve; move to another space; see [Detached sessions](#detached-sessions) |
| `m` | menu |
| `?` | list the keys where the cursor is |
| `n`, `N`, `c`, `z`, `x` | new worktree, new worktree from a PR, close session, suspend session, remove worktree (forget, on a missing worktree) |
| `F` | forget a session's saved file, on a session with no windows open |
| `p` | clean up the space's worktrees, on a space's row; its detached sessions, on the row of how many |
| `D` | remove the space, on a space's row or a topic space's |
| `R` | run a command in the worktree, with commands configured |
| `L` | open a layout in the worktree's session, with layouts configured |
| `o` | open without the default layout, where it would open |
| `b` | open the worktree's PR in the browser, when its row shows one |
| `a` | add a space, wherever the cursor is |
| `!` | focus the window that needs you most, wherever the cursor is |
| `t` | reply to the cursor's status entry: write to its window without switching to it |
| `x` | clear the done and failed records of the cursor's status entry, without focusing its window; see [Status](#status) |
| `X` | clear the done and failed status entries, wherever the cursor is |
| `/` | filter the spaces list; see [Spaces](#spaces) |
| `P` | open the picker; see [The picker](#the-picker) |
| `f` | show only what is not idle, or everything again; see [Spaces](#spaces) |
| `O` | show only open sessions, or everything again; see [Spaces](#spaces) |
| `Z` | hide suspended sessions, or show everything again; see [Spaces](#spaces) |
| `s` | sort the list the next way; see [Order](#order) |
| `K` / `J` | move the cursor's space within its group, or linked worktree within its space, up / down |
| `v` | select rows to act on; see [Selecting](#selecting) |
| `tab` | switch between the spaces and hosts tabs; see [The hosts tab](#the-hosts-tab) |
| `space`, `A` | in the select mode, tick the cursor's row, every row shown |
| `x`, `c`, `z`, `F`, `R`, `M` | in the select mode, remove, close, suspend, forget, run in, move what is ticked |
| `r` | refresh, which also happens every two seconds |
| `esc` / `q` | give focus back to the window it came from; `esc` in the select mode leaves it |

Each list scrolls on its own. A list with entries out of view says how many on its first or last row;
clicking that row scrolls a page. Dragging the rule above STATUS resizes the two lists; the STATUS list
keeps its height in every sidebar and across restarts, in `$XDG_STATE_HOME/kspaces/state.json`.

`?` lists over the sidebar the keys that do something where the cursor is: the row's, then the list's and
the sidebar's, each under the name of what it acts on, and in the select mode the mode's first. In the
menu it lists the menu's keys and its items'. `j` / `k`, `g` / `G` and the wheel scroll the list, and
`esc`, `q`, `?` or a click close it. While the filter is open `?` is typed into it, and `ctrl+/` lists the
filter's keys.

## The picker

`kspaces pick`, which kitty.conf's `kspaces_pick` runs in a popup, as do the sidebar's `P` and the search
button on its SPACES title, left of the funnel, an overlay over the window the key was pressed in, or
beside the sidebar, or a 120 by 32 panel for it, as `[sidebar] popup` says, lists the status entries and then
every session and worktree in the sidebar's order, narrowed by a fuzzy query. A space in a group is listed
after its group's name, `work / gr` and `work / gr / dev`, so the group's name finds its entries.

That is its **Spaces** scope. **Windows** lists every kitty window but kspaces' own, the most recently
focused first and the one the picker is for last, each named for its worktree's entry, else its session,
else its tab, with what runs in it and its title: `gr / dev  nvim main.go`. What runs is found as
[saving a layout](kitty.md#saving-a-layout) finds it: the command the shell reports running, for a window showing a
session provider's session what the provider says it runs, else the program in its foreground; nothing for
the shell at its prompt. **All** lists both. `kspaces pick --scope spaces|windows|all` says which it lists,
`spaces` by default, so a mapping picks its own: `map cmd+p kspaces_pick --scope=windows`. `shift+tab`
lists the next, and the strip of scopes at the end of the query's line, in a picker 60 columns or wider,
lists the one clicked. `enter` on a window switches to its session and focuses it; a window has no menu.
`enter` switches to the
entry under the cursor. `shift+enter`, or `alt+enter` for a terminal that sends `shift+enter` as `enter`,
switches to it without its [default layout](kitty.md#layouts), as **Open without layout** does; the last
line says `shift+enter switch without layout` after the count while that is what the entry would open
with. A missing worktree has the sidebar's icon for it and `gone` for its branch, and is
not switched to without windows open.

`tab` opens the menu of the entry under the cursor, below its row or above it near the bottom, over the
preview but not over the list: a worktree's or session's has what its row's menu in the sidebar has but
folding and selecting, such as **Open layout**, **Run** and **New worktree**; a status entry's **Open** and
**Reply**. Each item shows its key, the same as its row's in the sidebar, which runs it. `up` / `down`
move in it, `enter` runs the item, and `esc` or `tab` closes it, other keys doing nothing while it is open. An item that opens a form opens it over the window the picker is for, and closes
the picker. The last line says `tab actions` after the count.

A left click on an entry switches to it, as `enter` does, and a right click opens its menu, as `tab` does.
The wheel moves the cursor. While the menu is open the mouse lights the item under it, a left click runs
it, and a click elsewhere closes it, a right click on another entry opening that entry's.

Beside the list in a popup 90 columns or wider, and below it in a narrower one, a preview shows the
entry under the cursor: a status entry in full, as the sidebar's opened entry; a worktree's space, path,
branch, its changes, upstream and PR as its row shows them, its session, how many windows it has open,
whether it has a saved session file, and its status entries. A window's says its session, tab, title, what runs in it and its
working directory. A missing worktree's path says its directory
is gone, with no branch or git state. A popup too short for both shows only the list.

Under what it says of the entry, where there is room, the preview shows the bottom of a window's screen,
with its colors, under a rule: a status entry's own window, or for a session with windows open the one
switching to it focuses, the one focused most recently, as `goto_session` picks it; a window's own. Without focus times
from kitty, it is the active window of the session's active tab. A session with no windows open shows no
screen, and neither do the picker's own window, the docks and the forms. The screen is read with
`kitten @ get-text` when the cursor lands on the entry and again every second while it stays, so it
follows what the window is doing. Only colors and styles are kept from it; anything else the window drew,
such as links or cursor moves, is left out. Beside the list there is room for most of a screen; below it
only under an entry with few details, such as a status entry.
