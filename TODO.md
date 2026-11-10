# TODO

## MVP candidates

To consider once the core pieces work:

- More UX in general, still to investigate.
- The picker (f5) has no preview of an entry's details, and cannot open a new worktree that has no session
  any differently from the sidebar.
- Remove worktree in the sidebar's right-click menu: confirm, then `git worktree remove`, closing the
  worktree's session first.
- Close session in the menu: close every window and tab of a session. Closing has to end the programs,
  not leave multiplexer sessions running: today kitty's `close_session` closes the windows and the
  dotfiles' `cm_reap.py` kills each window's cm session on close, so going through kitty's close path
  (not detaching) is what keeps that working.
- Spaces list wastes width: every row starts with a 3-column marker column (state, current, open), then
  the 2-column fold caret, then worktrees indent another branch icon. Instead, put the marker in the icon
  slot itself: a worktree's status replaces its branch icon, and a space's own row gets an icon of its
  own (a repo icon) that its status replaces the same way. Rows then start at the left edge; the caret
  and indent stay for worktrees. Saves 2-3 columns on every row of a 28-column dock.

## Status area

kspaces reads OSC 7501 records from `kitten @ ls`, and the kitty watcher notes when each changed state.
It knows nothing about what produced a record. Open questions:

- A program inside a multiplexer reaches kitty only if the multiplexer passes OSC 7501 through.
- kitty loses its records when it restarts, so the list starts empty after a restart.
- A record's time is when the watcher saw the state change. A record kitty already had before the watcher
  was loaded has no time until its state changes again.
- A collapsed entry shows one row of detail; the rest only shows with the sidebar's cursor on it.

## Known issues

- One `kspaces sidebar` per tab, because kitty can only dock to one tab. Each polls `kitten @ ls` and runs
  `git worktree list` per space every two seconds. A dock for the whole OS window, or one process feeding
  all of them, would fix both.
- `save_as_session --match` writes `new_os_window` before a space's file depending on which OS window had
  focus, so with several OS windows a space can be restored in a window of its own. A kitty bug; see the
  TODO in the dotfiles' `save_sessions.py`.
- Windows outside spaces are saved into `default` and come back labelled `default`. Left as is for now.
- kitty's own dock border can sit next to the sidebar's, which may look like a double line.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.

## After the MVP

Not until the idea has proven worth it:

- Add new spaces from the sidebar.
