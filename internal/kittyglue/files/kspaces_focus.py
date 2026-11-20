# Move keyboard focus into the kspaces sidebar, when the current OS window has it, or back out of it when
# it has focus. See kspaces_watcher.py, which adds the sidebar.
# Installed by `kspaces kitty install`; edits to this file are overwritten.

from kittens.tui.handler import result_handler

DOCK_VAR = 'kspaces'


def main(args):
    return ''


@result_handler(no_ui=True)
def handle_result(args, answer, target_window_id, boss):
    tab = boss.active_tab
    if tab is None:
        return
    active = boss.active_window
    if active is not None and DOCK_VAR in active.user_vars:
        # The normal window that was active before the sidebar took focus.
        target = tab.windows.active_main_window
    else:
        target = next((w for g in tab.windows.os_window_docks for w in g if DOCK_VAR in w.user_vars), None)
    if target is not None:
        boss.set_active_window(target)
