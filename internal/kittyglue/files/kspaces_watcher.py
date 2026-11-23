# Keep one kspaces sidebar docked at the left of an OS window that has a tab in a session.
#
# The sidebar is an OS window dock (launch --type=os-window-dock), shown in every tab of its OS window and
# running one `kspaces sidebar`. There is one in all of kitty: this adds it whenever a window is created or
# the tab bar changes and there is none, which covers kitty starting, tabs loaded from a session file and
# the sidebar's OS window closing, and closes any beyond the first.
#
# Only an OS window with a tab in a session gets it, the active one first. Spaces are sessions, and the
# exclusion keeps the sidebar out of OS windows that are not part of the workspace at all, such as the tab
# switcher's or a kspaces popup's.
#
# kspaces_toggle.py hides and shows the sidebar by setting boss.kspaces_hidden.
#
# It tells the sidebar when focus moves, see notify_sidebar.
#
# It hides the tab bar of a kspaces popup's panel, see hide_popup_tab_bar.
#
# It also notes when each program status record (OSC 7501) of a window last changed state, which kitty
# does not report, so the sidebar can show how long a program has been blocked or working.
#
# The sidebar's width is [sidebar] width in kspaces' config, applied when it changes. A sidebar dragged to
# another width keeps it: the sidebar writes the new width to the config.
#
# Installed by `kspaces kitty install`, which writes this file; edits to it are overwritten.

import json
import os
import socket
import tempfile
import time
import tomllib

from kitty.constants import version
from kitty.fast_data_types import add_timer

DOCK_VAR = 'kspaces'
# The user variable of a kspaces popup in a panel.
POPUP_VAR = 'kspaces_popup'
# Where this file was installed. The sidebar checks the files there against the ones it would install.
GLUE_DIR = os.path.dirname(os.path.abspath(__file__))
# The socket the sidebar listens on for focus changes, one for each kitty, given to it in
# KSPACES_FOCUS_SOCKET.
FOCUS_SOCKET = os.path.join(
    os.path.join(os.environ['XDG_RUNTIME_DIR'], 'kspaces') if os.environ.get('XDG_RUNTIME_DIR')
    else os.path.join(tempfile.gettempdir(), f'kspaces-{os.getuid()}'),
    f'focus-{os.getpid()}.sock')
# kspaces' default for [sidebar] width, the widths it allows, and how often its config is checked for a new
# one.
DEFAULT_WIDTH = 28
MIN_WIDTH, MAX_WIDTH = 12, 120
CONFIG_CHECK_SECONDS = 2
# The first kitty with OS window docks.
MIN_VERSION = (0, 51, 0)
SUPPORTED = version >= MIN_VERSION

_scheduled = False
_notifying = False
# The config's sidebar width, and the files it was read from with their modification times.
_width = (DEFAULT_WIDTH, None)
# The width the dock was dragged to and the config's width it was dragged from, from kspaces' state, and the
# state file's modification time.
_dragged = (None, None, None)
# The width last given to the sidebar, None before the first check.
_applied = None


# config_paths are kspaces' config files: config.toml, then every other .toml file beside it by name,
# hidden ones left out, as kspaces reads them.
def config_paths():
    base = os.environ.get('XDG_CONFIG_HOME') or os.path.expanduser('~/.config')
    d = os.path.join(base, 'kspaces')
    try:
        names = sorted(n for n in os.listdir(d) if n.endswith('.toml') and not n.startswith('.') and n != 'config.toml')
    except OSError:
        names = []
    return [os.path.join(d, n) for n in ['config.toml'] + names]


# sidebar_width is the width the sidebar's dock was dragged to, kept in kspaces' state.json, while the
# config's width is still the one it was dragged from, so a width set in the config since wins; else the
# config's.
def sidebar_width():
    global _dragged
    cfg = config_width()
    base = os.environ.get('XDG_STATE_HOME') or os.path.expanduser('~/.local/state')
    path = os.path.join(base, 'kspaces', 'state.json')
    try:
        mtime = os.stat(path).st_mtime
    except OSError:
        mtime = None
    if mtime != _dragged[2]:
        width = over = None
        try:
            with open(path, 'rb') as f:
                sidebar = json.load(f).get('sidebar', {})
            width, over = sidebar.get('width'), sidebar.get('widthOver')
        except (OSError, ValueError, AttributeError):
            pass
        _dragged = (width, over, mtime)
    width, over, _ = _dragged
    if isinstance(width, int) and MIN_WIDTH <= width <= MAX_WIDTH and over == cfg:
        return width
    return cfg


# config_width is [sidebar] width from kspaces' config, from the file that sets it, read again only when
# the files change. kspaces checks the config itself and shows what is wrong with it in the sidebar, so
# here a value it would reject just keeps the width there was.
def config_width():
    global _width
    stamps = []
    for path in config_paths():
        try:
            stamps.append((path, os.stat(path).st_mtime))
        except OSError:
            pass
    if not stamps or stamps == _width[1]:
        return _width[0]
    width = DEFAULT_WIDTH
    try:
        for path, _ in stamps:
            with open(path, 'rb') as f:
                sidebar = tomllib.load(f).get('sidebar', {})
            if isinstance(sidebar, dict) and 'width' in sidebar:
                width = sidebar['width']
                break
        if not (isinstance(width, int) and MIN_WIDTH <= width <= MAX_WIDTH):
            width = _width[0]
    except (OSError, ValueError):
        width = _width[0]
    _width = (width, stamps)
    return width


def docks(tm):
    return [g for g in tm.os_window_docks if g.dock is not None and any(DOCK_VAR in w.user_vars for w in g)]


# resize_docks gives the sidebar width, as dragging its border does, which lays out every tab of its OS
# window again. Only when sidebar_width changes, so a dragged sidebar is not put back. kspaces docks
# are sized in columns, never in percent.
def resize_docks(boss, width):
    for tm in tuple(boss.all_tab_managers):
        host = tm.os_window_docks_host
        for g in docks(tm):
            if host is not None and not g.dock.size_is_percent and g.dock.size != width:
                host.resize_dock(g, width - g.dock.size)


def check_config(boss):
    def run(timer_id):
        global _applied
        if (width := sidebar_width()) != _applied:
            resize_docks(boss, width)
            _applied = width
        # A config reload shows the bars again, see hide_popup_tab_bar.
        for tm in tuple(boss.all_tab_managers):
            hide_popup_tab_bar(tm)

    add_timer(run, CONFIG_CHECK_SECONDS, True)


# hide_popup_tab_bar hides the tab bar of a kspaces popup's panel, an OS window of its own with one tab,
# where the bar is only in the way. kitty has no option for one OS window's tab bar, and tab_bar_filter
# always shows the active tab, so this sets the TabManager's own tab_bar_hidden, which is kitty's internal
# API: tab_bar_style hidden sets it, and a config reload (TabManager.apply_options) resets it from that.
def hide_popup_tab_bar(tm):
    if not tm.tab_bar_hidden and any(POPUP_VAR in w.user_vars for t in tm for w in t):
        tm.tab_bar_hidden = True
        tm.mark_tab_bar_dirty()
        tm.resize()


# Called when kitty loads this watcher, at startup and whenever kitty.conf's list of watchers changes. The
# flag on boss keeps a config reload from starting a second check.
def on_load(boss, data):
    if getattr(boss, 'kspaces_checking_config', False):
        return
    boss.kspaces_checking_config = True
    if not SUPPORTED:
        boss.show_error('kspaces', 'kspaces needs kitty {} or later, this is kitty {}.'.format(
            '.'.join(map(str, MIN_VERSION)), '.'.join(map(str, version))))
        return
    check_config(boss)


def wants_dock(tm):
    return any(tab.created_in_session_name and tab.windows.groups for tab in tm)


# kspaces_path is the kspaces kitty would run: kitty's own PATH under launchd is short, so this searches as
# kitty does for launch, which also takes kitty.conf's `env PATH` and the login shell's. Looked up at each
# launch, as installing a new version, with mise for one, can move it.
def kspaces_path():
    from kitty.utils import which
    return which('kspaces') or os.path.expanduser('~/.local/bin/kspaces')


# add_dock adds the sidebar when there is none, and closes all but one when there are more, as from an
# older kspaces that docked one in every OS window.
def add_dock(boss):
    if getattr(boss, 'kspaces_hidden', False):
        return
    tms = tuple(boss.all_tab_managers)
    existing = [w for tm in tms for g in docks(tm) for w in g if DOCK_VAR in w.user_vars]
    for w in existing[1:]:
        boss.mark_window_for_close(w)
    if existing:
        return
    active = boss.active_tab_manager
    for tm in sorted(tms, key=lambda tm: tm is not active):
        tab = tm.active_tab
        if tab is None or tab.active_window is None or not wants_dock(tm):
            continue
        # Focusable, so a click or kspaces_focus.py can give the sidebar the keyboard, but --keep-focus so
        # adding one does not take focus from the window being worked in. In no session: kitty moves it
        # to whichever tab is active, and save_sessions.py leaves it out of saved sessions by its var.
        prev = tab.active_window
        boss.call_remote_control(prev, (
            'launch', '--type=os-window-dock', '--match', f'id:{tab.id}',
            '--dock-edge=left', f'--dock-size={sidebar_width()}', '--keep-focus',
            f'--var={DOCK_VAR}=1', f'--env=KSPACES_KITTY_DIR={GLUE_DIR}', f'--env=KSPACES_FOCUS_SOCKET={FOCUS_SOCKET}',
            kspaces_path(), 'sidebar',
        ))
        # --keep-focus only keeps the window that has focus. In an OS window without it the new dock
        # becomes the tab's active window, and the tab takes its title.
        if tab.active_window is not prev:
            tab.set_active_window(prev)
        return


# Called from the tab bar and window hooks below. They fire in the middle of creating tabs and loading
# sessions, so launching from inside them could act on a half-built tab, and the launch itself fires them
# again. Doing it from a timer runs it once the current change is finished, and coalesces a burst of changes
# into one pass. A timer kitty refuses, returning 0, leaves the pass to the next change.
def schedule(boss):
    global _scheduled
    if _scheduled or not SUPPORTED:
        return
    _scheduled = True

    def run(timer_id):
        global _scheduled
        _scheduled = False
        add_dock(boss)

    if not add_timer(run, 0, False):
        _scheduled = False


# notify_sidebar tells the sidebar focus has moved, as to another session, so it shows that at once rather
# than at its next poll. A datagram, sent without waiting: with no sidebar listening, or one too busy to
# read, it is dropped. From a timer, so the burst of focus changes one switch makes is one datagram.
def notify_sidebar():
    global _notifying
    if _notifying:
        return
    _notifying = True

    def run(timer_id):
        global _notifying
        _notifying = False
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM) as s:
                s.setblocking(False)
                s.sendto(b'focus', FOCUS_SOCKET)
        except OSError:
            pass

    if not add_timer(run, 0, False):
        _notifying = False


def on_focus_change(boss, window, data):
    if data.get('focused'):
        notify_sidebar()


# The sidebar's OS window closing changes nothing in the others, so its own close is a trigger too, for
# another OS window to get it.
def on_close(boss, window, data):
    if DOCK_VAR in window.user_vars:
        schedule(boss)


def on_tab_bar_dirty(boss, window, data):
    if (tm := data.get('tab_manager')) is not None:
        hide_popup_tab_bar(tm)
    schedule(boss)


# The tab bar does not change when the first window of a startup session appears, so new windows are a
# trigger too. kitty reports a window's creation as a resize from an all-zero geometry.
def on_resize(boss, window, data):
    if data['old_geometry'].xnum == 0:
        schedule(boss)


# The user variable that holds when each of a window's program status records last changed state, as JSON:
# {"pid": KITTY_PID, "records": {"RECORD_ID": ["STATE", UNIX_TIME]}}. Kept on the window and noted here, at
# the change, so the sidebar, which refreshes every two seconds, reads the time it changed, and a sidebar
# started later still has it. Not named kspaces*, as save_sessions.py leaves windows with most kspaces*
# variables out of saved sessions.
STATUS_AT_VAR = 'program_status_at'


def on_program_status_change(boss, window, data):
    try:
        prev = json.loads(window.user_vars.get(STATUS_AT_VAR, ''))
    except ValueError:
        prev = {}
    # User vars are saved into session files, so a restored window can bring times from a kitty that is
    # gone, for records that kitty no longer has. Only trust times noted by this kitty.
    known = prev.get('records', {}) if isinstance(prev, dict) and prev.get('pid') == os.getpid() else {}
    now = time.time()
    records = {}
    for r in data['program_status']:
        was = known.get(r['id'])
        records[r['id']] = was if was and was[0] == r['state'] else [r['state'], now]
    window.set_user_var(STATUS_AT_VAR, json.dumps({'pid': os.getpid(), 'records': records}) if records else None)
