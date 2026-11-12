# TODO

## Known issues

- One `kspaces sidebar` per tab, because kitty can only dock to one tab. They share one poller, so only one
  of them runs `kitten @ ls` and `git worktree list`, but each is a process, with its own screen to draw.
  A dock for the whole OS window would fix that.
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
  starts with one shell, and kspaces opens `default` with `launch`.
- Windows in no session are saved into `default` at quit and come back labelled `default`, including one
  open only then, such as a tab switcher in an OS window of its own. Left as is for now.
- kitty's own dock border can sit next to the sidebar's, which may look like a double line.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.

## kitty work

One thing at a time upstream, each kept short, describing the use case only as switching between sets of
tabs with sessions and `tab_bar_filter`, not kspaces. Drafts are in `~/projects/kitty/.research/drafts/`.

1. `save_as_session --match`, the first two bugs above: kovidgoyal/kitty#10844, merged; the workaround in
   `save_sessions.py` comes out once a release has it.
2. `detach_tab` and window moves losing their session: draft PR kovidgoyal/kitty#10851.
3. Watcher events (a close reason on `on_close`, an event for a window's directory changing): commented on
   discussion #10828, offering PRs.
4. Watcher calls each taking a timer, and the macOS and Linux tables of 128: issue kovidgoyal/kitty#10853.
5. A dock for the whole OS window, the fix for the first known issue: issue kovidgoyal/kitty#10854.
6. Toggling a dock: show and hide a dock running a given program, such as `gr` for reviews beside the
   current window, from one mapping, keeping the program running while hidden rather than restarting it.
   kitty has no such action; kspaces_toggle.py only closes and re-adds kspaces' own docks. In the interim,
   a kitten (in kspaces' kitty files) toggling a dock by a user var, `kitten kspaces_dock.py --var=NAME
   --edge=right --size=80 -- PROG ARGS`; upstream, a `toggle_dock` action or `launch --type=*-dock
   --toggle`, as a proposal after the dock one.
7. One launcher for every new window: today running each window in a cm session takes a mapping per way
   of opening one (new tab, split, OS window, each through `kitten cm_launch.py`), and kitty's own
   new_tab, new_window, new_os_window and splits still start the plain shell. An option that wraps the
   program of every new window, such as `launch_wrapper cm-attach {session_name}`, or that names a kitten
   to run in place of `launch`, would make kitty's defaults and every mapping go through it. Check how
   `shell`, `--hold` and `startup_session` interact first; a proposal, after the others.

Dropped: resizable docks, which the maintainer turned down in #2391; `[sidebar] width` covers it.

When a kitty release has a fix, drop its workaround behind a check of kitty's version, as older kittys
still need it.
