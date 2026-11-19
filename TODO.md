# TODO

## Known issues

- The default session's `startup_session` file is emptied, not removed, once it has no windows; kitty then
  starts with one shell, and kspaces opens `default` with `launch`.
- Windows in no session are saved into `default` at quit and come back labelled `default`, including one
  open only then, such as a tab switcher in an OS window of its own. Left as is for now.
- The dotfiles' `cm_reap.py` kills a multiplexer session when one of two windows showing it closes.

## kitty work

One thing at a time upstream, each kept short, describing the use case only as switching between sets of
tabs with sessions and `tab_bar_filter`, not kspaces. Drafts are in `~/projects/kitty/.research/drafts/`.

1. `save_as_session --match`: kovidgoyal/kitty#10844, in 0.51.0.
2. `detach_tab` and window moves losing their session: fixed by the maintainer in b261b08b7, in 0.51.0.
3. Watcher events: asked on discussion #10828; `on_close` reports `reason` and `cause`, and
   `on_cwd_change` was added (65da47b61), in 0.51.0. `save_sessions.py` uses both; the dotfiles'
   `cm_reap.py` can drop its `on_quit` flag for `reason`.
4. Watcher calls each taking a timer: kovidgoyal/kitty#10853, fixed in cb66ed55c, in 0.51.0.
5. A dock for the whole OS window: kovidgoyal/kitty#10854, `launch --type=os-window-dock` in 0.51.0. The
   sidebar is one.
6. Toggling a dock: show and hide a dock running a given program, such as `gr` for reviews beside the
   current window, from one mapping, keeping the program running while hidden rather than restarting it.
   kitty has no such action; kspaces_toggle.py only closes and re-adds kspaces' own docks. In the interim,
   a kitten (in kspaces' kitty files) toggling a dock by a user var, `kitten kspaces_dock.py --var=NAME
   --edge=right --size=80 -- PROG ARGS`; upstream, a `toggle_dock` action or `launch --type=*-dock
   --toggle`, as a proposal after the dock one.
7. One launcher for every new window: today running each window in a cm session takes a mapping per way
   of opening one (new tab, split, OS window, each through `kitten cm_launch.py`), and kitty's own
   new_tab, new_window, new_os_window and splits still start the plain shell. Filed as
   kovidgoyal/kitty#10858; the maintainer implemented it in f8556bbf8 as `launch_wrapper.py` in the kitty
   config dir: `prefix_for_launch(ctx)` returns a prefix put before the command of every window opened at
   the user's request (launch, remote control, sessions, new_* actions, the tab bar's button, the macOS
   menus, the first window), not kittens or overlays kitty opens itself. kitty keeps knowing the real
   command, so it saves that in sessions, not the prefix, and calls the function again on restore;
   `ctx.user_vars` are saved, so a var naming the window's cm session can bring it back to that session.
   In 0.51.0, so cm_launch.py's mappings can become plain `launch` and the wrapper pick the cm
   session; it must skip kspaces' own windows (docks, forms, panels, by their vars or `ctx.launch_type`),
   and cm_reap.py has to find a window's session from its var, as the wrapper is not in `child.argv`.
8. Resizing docks by dragging their inner border and with `resize_window`: kovidgoyal/kitty#10857, in
   0.51.0. A dragged sidebar's width is written to `[sidebar] width`.
9. `launch --type=tab --next-to` over remote control goes by the target tab, the active one by default,
   so the tab lands in the focused OS window, while the docs say the matched window's; the `launch` action
   honors it. It put a kspaces panel's new sessions in the panel until cm_launch.py passed `--self`
   (dotfiles d164b2f) and kspaces left the panel before acting (22bddf5). Not filed yet; the draft, a docs
   mismatch offering a docs line or letting `--next-to` choose the OS window, is
   `drafts/issue-launch-next-to-rc.md`.

