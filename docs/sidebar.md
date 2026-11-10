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

## The menu

Right-click a row, or press `m` on it:

| Item | Key | Does |
|---|---|---|
| Open | `enter` | switch to the session |
| Fold / Unfold worktrees | `space` | hide or show a space's linked worktrees |
| New worktree... | `n` | create a worktree in a form |
| Close session... | `c` | close every window of the session, on a session with windows open |
| Remove worktree... | `x` | close the session and remove a linked worktree |

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
| `r` | refresh, which also happens every two seconds |
| `esc` / `q` | give focus back to the window it came from |

Each list scrolls on its own. A list with entries out of view says how many on its first or last row;
clicking that row scrolls a page.
