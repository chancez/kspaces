# The sidebar

## Spaces

Each row starts with a fold triangle for a space with linked worktrees, then an icon: a repository for a
space, a branch for a linked worktree, a terminal for a session in no space, or a cross for a space whose
worktrees could not be listed. The most urgent status of the row's sessions replaces the icon. The icon is
green when the session has windows open; the row is faint when it has none. The sidebar's own session is
drawn as a bar.

Sessions that are open or saved but belong to no worktree, such as the one kitty starts with, are listed
above the spaces.

Clicking a row, or `enter` on it, switches to its session, creating it the first time. Clicking a space's
triangle, or `space` on its row, folds its linked worktrees into the space's row. Folded spaces are kept in
`$XDG_STATE_HOME/kspaces/folded`, so every tab's sidebar agrees.

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

## The menu

Right-click a row, or press `m` on it:

| Item | Key | Does |
|---|---|---|
| Open | `enter` | switch to the session |
| Fold / Unfold worktrees | `space` | hide or show a space's linked worktrees |
| New worktree... | `n` | create a worktree in a form |
| Close session... | `c` | close every window of the session, on a session with windows open |
| Remove worktree... | `x` | close the session and remove a linked worktree |
| Add space... | `a` | add a repository to the spaces, in a form; on every row |

Each item that changes something opens a form in an overlay, with buttons and keys to confirm or cancel.

**New worktree** has a branch field, which starts with `worktrees.branch_prefix/`, and a directory field,
empty for the default: `worktrees.dir` under the space's root, named after the branch. An existing branch is
checked out; a new one is created from what the main worktree has checked out. Create switches to the new
session.

**Close session** closes every window the way closing them by hand does, so whatever kitty.conf runs when a
window closes still runs.

**Remove worktree** shows the directory, the session's windows and the files that are not committed, which
are lost. It closes the session, waits for its windows to go, deletes the session's saved file and runs
`git worktree remove`. The branch is kept. What git would refuse, such as a locked worktree, is said before
you confirm. The removal runs as a kitty background process, so an error after that is shown by kitty.

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
| `n`, `c`, `x` | new worktree, close session, remove worktree |
| `a` | add a space, wherever the cursor is |
| `!` | focus the window that needs you most, wherever the cursor is |
| `/` | filter the spaces list; see [Spaces](#spaces) |
| `r` | refresh, which also happens every two seconds |
| `esc` / `q` | give focus back to the window it came from |

Each list scrolls on its own. A list with entries out of view says how many on its first or last row;
clicking that row scrolls a page.

## The picker

`kspaces pick`, which kitty.conf's `kspaces_pick` runs in an overlay, lists the status entries and then
every session and worktree, narrowed by a fuzzy query. `enter` switches to the entry under the cursor.

Beside the list in an overlay 90 columns or wider, and below it in a narrower one, a preview shows the
entry under the cursor: a status entry in full, as the sidebar's opened entry; a worktree's space, path,
branch, session, how many windows it has open, whether it has a saved session file, and its status
entries. An overlay too short for both shows only the list.

Under what it says of the entry, where there is room, the preview shows the bottom of a window's screen,
with its colors, under a rule: a status entry's own window, or for a session with windows open the one
switching to it focuses, the one focused most recently, as `goto_session` picks it. Without focus times
from kitty, it is the active window of the session's active tab. A session with no windows open shows no
screen, and neither do the picker's own window, the docks and the forms. The screen is read with
`kitten @ get-text` when the cursor lands on the entry and again every second while it stays, so it
follows what the window is doing. Only colors and styles are kept from it; anything else the window drew,
such as links or cursor moves, is left out. Beside the list there is room for most of a screen; below it
only under an entry with few details, such as a status entry.
