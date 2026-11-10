# TODO

## MVP candidates

To consider once the core pieces work:

- More UX in general, still to investigate.
- The picker (f5) has no preview of an entry's details, and cannot open a new worktree that has no session
  any differently from the sidebar.

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
- mise tasks for building and installing, like cm.
- A proper package structure.
- cobra/pflag for commands.
