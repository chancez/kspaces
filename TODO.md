# TODO

## Known issues

- The default session's `startup_session` file is emptied, not removed, once it has no windows; kitty then
  starts with one shell, and kspaces opens `default` with `launch`.
- Windows in no session are saved into `default` at quit and come back labelled `default`, including one
  open only then, such as a tab switcher in an OS window of its own. Left as is for now.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.
- Detached sessions are only in the sidebar: the picker does not list them, the query does not find them,
  and their row does not show the most urgent status among them while hidden.
- The select mode's bar only moves detached sessions; killing or removing several is the head's Clean up.

## kitty work

One thing at a time upstream, each kept short, describing the use case only as switching between sets of
tabs with sessions and `tab_bar_filter`, not kspaces. Drafts are in `~/projects/kitty/.research/drafts/`.

1. `save_as_session --match`: kovidgoyal/kitty#10844, in 0.51.0.
2. `detach_tab` and window moves losing their session: fixed by the maintainer in b261b08b7, in 0.51.0.
3. Watcher events: asked on discussion #10828; `on_close` reports `reason` and `cause`, and
   `on_cwd_change` was added (65da47b61), in 0.51.0. `save_sessions.py` uses both, and the dotfiles'
   `cm_reap.py` skips closes on quit by `reason` (dotfiles d8f6a6f).
4. Watcher calls each taking a timer: kovidgoyal/kitty#10853, fixed in cb66ed55c, in 0.51.0.
5. A dock for the whole OS window: kovidgoyal/kitty#10854, `launch --type=os-window-dock` in 0.51.0. The
   sidebar is one.
6. Toggling a dock: show and hide a dock running a given program, such as `gr` for reviews beside the
   current window, from one mapping, keeping the program running while hidden rather than restarting it.
   kitty has no such action; kspaces_toggle.py only closes and re-adds kspaces' own docks. In the interim,
   a kitten (in kspaces' kitty files) toggling a dock by a user var, `kitten kspaces_dock.py --var=NAME
   --edge=right --size=80 -- PROG ARGS`; upstream, a `toggle_dock` action or `launch --type=*-dock
   --toggle`, as a proposal after the dock one.
7. One launcher for every new window: filed as kovidgoyal/kitty#10858; the maintainer implemented it in
   f8556bbf8 as `launch_wrapper.py` in the kitty config dir, in 0.51.0: `prefix_for_launch(ctx)` returns
   a prefix put before the command of every window opened at the user's request (launch, remote control,
   sessions, new_* actions, the tab bar's button, the macOS menus, the first window), not kittens or
   overlays kitty opens itself. The dotfiles use it to run every window in a cm session (d8f6a6f), in
   place of cm_launch.py and its mappings, and kspaces' `launch` is plain `launch` again. The window's cm
   session is kept in its `cm_session` user var, which kitty saves in sessions, and the wrapper skips
   kspaces' own windows by their `kspaces*` vars and docks and overlays by `ctx.launch_type`.
8. Resizing docks by dragging their inner border and with `resize_window`: kovidgoyal/kitty#10857, in
   0.51.0. A dragged sidebar's width is written to `[sidebar] width`.
9. `launch --type=tab --next-to` over remote control goes by the target tab, the active one by default,
   so the tab lands in the focused OS window, while the docs say the matched window's; the `launch` action
   honors it. It put a kspaces panel's new sessions in the panel until cm_launch.py passed `--self`
   (dotfiles d164b2f) and kspaces left the panel before acting (22bddf5). Not filed yet; the draft, a docs
   mismatch offering a docs line or letting `--next-to` choose the OS window, is
   `drafts/issue-launch-next-to-rc.md`.
10. `launch --hold` with a launch wrapper runs the held shell outside the wrapper, and `LaunchContext`
    does not say the window is held: kovidgoyal/kitty#10901. Until then a cm wrapper cannot tell kspaces'
    held commands, which it types into the session's shell (`cm-attach --in-shell`), from others.

