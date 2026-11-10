# TODO

## MVP candidates

To consider once the core pieces work:

- A picker: a fuzzy finder over spaces and worktrees, bound to a key, for switching without the sidebar.
- More UX in general, still to investigate.

## Status area

A first version reads OSC 7501 records from `kitten @ ls`. Open questions:

- Windows running under cm: OSC 7501 has to reach kitty for kitty to keep the record. The installed cm
  (37fe9b6) does not pass it through. cm's ideas.md has "Re-send OSC 7501 to each client", which would
  also translate its own 25453 reports, so hooks using `cm report` would show up too.
- kitty loses its records when it restarts or a window reattaches, while cm keeps the session's state.
- No time in the entries: kitty does not report when a record was set.
- Entries are one line, so a long title is cut at the dock's width.

## Known issues

- One `kspaces sidebar` per tab, because kitty can only dock to one tab. Each polls `kitten @ ls` and runs
  `git worktree list` per space every two seconds. A dock for the whole OS window, or one process feeding
  all of them, would fix both.
- `save_as_session --match` writes `new_os_window` before a space's file depending on which OS window had
  focus, so with several OS windows a space can be restored in a window of its own. A kitty bug; see the
  TODO in the dotfiles' `save_sessions.py`.
- Windows outside spaces are saved into `default` and come back labelled `default`. Left as is for now.
- kitty's own dock border can sit next to the sidebar's, which may look like a double line.
- `cm_reap.py` kills a cm session when one of two windows showing it closes.

## After the MVP

Not until the idea has proven worth it:

- Config in TOML.
- Add new spaces from the sidebar.
- mise tasks for building and installing, like cm.
- A proper package structure.
- cobra/pflag for commands.
