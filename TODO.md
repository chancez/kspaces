# TODO

## MVP candidates

To consider once the core pieces work:

- More UX in general, still to investigate.
- The picker (f5) cannot open a new worktree that has no session any differently from the sidebar.
- Closing a session's last tab goes back to the last used tab, in any session: the dotfiles' kitty.conf
  `tab_switch_strategy left` focuses the tab to the left within the session (kitty picks among the tabs
  `tab_bar_filter` shows, kitty/tabs.py `remove`), but the last tab has none. Landing on a tab of the same
  space's sessions instead needs a watcher hook on tab close.

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
- Three kitty bugs, worked around in the dotfiles until fixed in kitty:
  - `save_as_session --match` writes `new_os_window` before a space's file depending on which OS window had
    focus, so with several OS windows each space was restored in a window of its own. `save_sessions.py`
    serializes with the bug fixed instead.
  - `save_as_session --match` writes `focus_tab` as the OS window's active tab's index among all its tabs,
    while loading counts only the tabs the file creates, so a space came back on the wrong tab.
    `save_sessions.py` writes the index among the saved tabs, of the one focused last.
  - Moving a tab to another OS window (`detach_tab`) drops the tab's session name, so the tab bar filter
    shows the moved tab in every session. `kspaces_watcher.py` puts it back from the tab's windows.
- `save_sessions.py` saves a session when one of its windows closes or takes focus, and removes its file
  with its last window, but kitty's watchers miss some changes:
  - Nothing says an OS window is closing as a whole, so its windows' closes are told from a user closing
    them only by the OS window being gone, and skipped. A space alone in its own OS window keeps a stale
    file when its last tab closes, as that closes the OS window too.
  - Nothing says a window's directory changed, so a `cd` is saved only at the next focus change, close or
    quit.
- On macOS kitty keeps timers in a table of 128 (glfw/cocoa_init.m), and `add_timer` returns 0 when it is
  full, never calling back. kitty fills it itself: `call_watchers` takes a 0s timer for every window resize
  and focus change, listened to or not, and none fire until the code that caused them returns. Restoring 9
  spaces in one go asked for 271 and was refused 143, each a dropped watcher call; it broke saving for good
  once. `save_sessions.py` now restores one space per timer, which was refused none, and both watchers try
  again later when refused.
- The default session's `startup_session` file is emptied, not removed, once it has no windows; kitty then
  starts with one shell, and kspaces opens `default` with `new_tab`.
- Windows in no session are saved into `default` at quit and come back labelled `default`, including one
  open only then, such as a tab switcher in an OS window of its own. Left as is for now.
- kitty's own dock border can sit next to the sidebar's, which may look like a double line.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.

## kitty changes, deferred

To make in kitty later, together, rather than while kspaces is still settling:

- Fix the three session bugs above, and drop the workarounds.
- Watcher events for an OS window closing, or a flag on `on_close` for windows closed with their OS
  window or by quitting, and for a window's directory changing, so `save_sessions.py` misses nothing.
- Watcher calls queued behind one timer rather than one per event, and only for events a watcher listens
  to. And a timer table that grows, or an error from `add_timer`, rather than a 0 that never calls back.
- Resizable docks: drag a dock's border, as borders between layout windows already can be
  (`Boss.drag_resize_start` only targets layout windows), and a command to resize one. The watcher then
  keeps a dragged width for every sidebar instead of setting `DockSpec.size` itself.
- A dock for the whole OS window, the fix for the first known issue.
