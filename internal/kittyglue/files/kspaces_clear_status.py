# Remove program status records (OSC 7501) without focusing their windows, as focusing a window does for
# its done and error records. Run by the sidebar through the kspaces_clear_status action:
#
#   kspaces_clear_status [--state STATE,...] [WINDOW_ID ...]
#
# removes the records of each window given, or of every window with none, in the states given, or in any
# state with none.
# Installed by `kspaces kitty install`; edits to this file are overwritten.

from kittens.tui.handler import result_handler


def main(args):
    return ''


@result_handler(no_ui=True)
def handle_result(args, answer, target_window_id, boss):
    states, ids = None, []
    rest = iter(args[1:])
    for a in rest:
        if a == '--state':
            states = frozenset(next(rest, '').split(','))
        else:
            ids.append(int(a))
    windows = [boss.window_id_map.get(i) for i in ids] if ids else boss.all_windows
    for w in windows:
        if w is None:
            continue
        # program_status_changed tells the watchers, as kitty's own removals do, so the ages and the tab
        # bar follow.
        if w.program_status.remove(lambda r: states is None or r.state.value in states):
            w.program_status_changed()
