# TODO

## Known issues

- One `kspaces sidebar` per tab, because kitty can only dock to one tab. Each polls `kitten @ ls` and runs
  `git worktree list` per space every two seconds. A dock for the whole OS window, or one process feeding
  all of them, would fix both.
- Three kitty bugs, worked around in kspaces' bundled kitty scripts until a kitty release fixes them:
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
    file when its last tab closes, as that closes the OS window too. Fixable without kitty: inside
    `on_close` the OS window is already gone from `boss.os_window_map` when it closes as a whole or kitty
    quits, and `current_application_quit_request()` tells the two apart; the saver checks only later, from
    a timer.
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

## kitty work

One at a time: a small bug fix PR, its feedback, then the next. Proposals wait until the PRs have landed.
Drafts are in `~/projects/kitty/.research/drafts/`. Each describes the use case only as switching between
sets of tabs with sessions and `tab_bar_filter`; the proposals still name kspaces and need trimming first.

1. `save_as_session --match`, the first two bugs above: draft PR kovidgoyal/kitty#10844, branch
   `pr/chancez/save-as-session-match`.
2. `detach_tab` and window moves losing their session: branch `pr/chancez/detach-tab-session`, local, to
   open once #10844 has feedback.
3. Watcher events (a close reason on `on_close`, an event for a window's directory changing): the
   maintainer asked which are missing in discussion #10828, so as a comment there.
4. Watcher calls each taking a timer, and the macOS and Linux tables of 128: an issue, as the fix is a
   design choice in kitty's event loop.
5. A dock for the whole OS window, the fix for the first known issue: a proposal, last.

Dropped: resizable docks, which the maintainer turned down in #2391; `[sidebar] width` covers it.

When a kitty release has a fix, drop its workaround behind a check of kitty's version, as older kittys
still need it.
