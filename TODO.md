# TODO

## MVP candidates

To consider once the core pieces work:

- More UX in general, still to investigate.
- The picker (f5) has no preview of an entry's details, and cannot open a new worktree that has no session
  any differently from the sidebar.
- Close session in the menu: close every window and tab of a session. Closing has to end the programs,
  not leave multiplexer sessions running: today kitty's `close_session` closes the windows and the
  dotfiles' `cm_reap.py` kills each window's cm session on close, so going through kitty's close path
  (not detaching) is what keeps that working.

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
- Two kitty bugs, worked around in the dotfiles until fixed in kitty:
  - `save_as_session --match` writes `new_os_window` before a space's file depending on which OS window had
    focus, so with several OS windows each space was restored in a window of its own. `save_sessions.py`
    serializes with the bug fixed instead.
  - Moving a tab to another OS window (`detach_tab`) drops the tab's session name, so the tab bar filter
    shows the moved tab in every session. `kspaces_watcher.py` puts it back from the tab's windows.
- Windows outside spaces are saved into `default` and come back labelled `default`. Left as is for now.
- kitty's own dock border can sit next to the sidebar's, which may look like a double line.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.

## kitty changes, deferred

To make in kitty later, together, rather than while kspaces is still settling:

- Fix the two session bugs above, and drop the workarounds.
- Resizable docks: drag a dock's border, as borders between layout windows already can be
  (`Boss.drag_resize_start` only targets layout windows), and a command to resize one. The watcher then
  keeps a dragged width for every sidebar instead of setting `DockSpec.size` itself.
- A dock for the whole OS window, the fix for the first known issue.

## After the MVP

Not until the idea has proven worth it:

- Add new spaces from the sidebar.
