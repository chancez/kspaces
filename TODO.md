# TODO

## MVP candidates

To consider once the core pieces work:

- A picker: a fuzzy finder over spaces and worktrees, bound to a key, for switching without the sidebar.
- A status area: aimed at agents first (state, what each one is waiting on, jump to it), but possibly for
  more than agents, such as a long build or a failing test run.
- More UX in general, still to investigate.

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
