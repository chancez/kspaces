# Say how many cells fit the screen of a window, for a popup sized as a percentage of it. Run by kspaces as
#
#   kitten @ kitten --match id:WINDOW kspaces_screen.py
#
# which prints {"columns": C, "lines": L}: the work area of the monitor the window's OS window is on, the
# one holding its center, in the window's cells. Empty when there is no such window or monitor.
# Installed by `kspaces kitty install`; edits to this file are overwritten.

import json

from kittens.tui.handler import result_handler
from kitty.fast_data_types import get_os_window_pos, get_os_window_size, glfw_get_monitor_workarea


def main(args):
    return ''


@result_handler(no_ui=True)
def handle_result(args, answer, target_window_id, boss):
    w = boss.window_id_map.get(target_window_id)
    size = w and get_os_window_size(w.os_window_id)
    if not size:
        return ''
    # The position, size and work areas are in screen coordinates, the cells in the framebuffer's pixels.
    x, y = get_os_window_pos(w.os_window_id)
    cx, cy = x + size['width'] // 2, y + size['height'] // 2
    cw, ch = size['cell_width'] / size['xscale'], size['cell_height'] / size['yscale']
    for ax, ay, aw, ah in glfw_get_monitor_workarea():
        if ax <= cx < ax + aw and ay <= cy < ay + ah:
            return json.dumps({'columns': int(aw / cw), 'lines': int(ah / ch)})
    return ''
