# Save every kitty session to its own file, and bring them back at startup.
#
# Loaded twice: as a watcher, to save on quit and as windows change, and to restore at startup; and as a
# kitten, the kspaces_save_sessions action, to save on demand.
#
# Installed by `kspaces kitty install` when kspaces' config has [kitty] save_sessions = true. Edits to this file
# are overwritten.
#
# One file per session because kitty names a session after the file it was loaded from and labels every tab
# the file creates with that name. Saving all tabs into one file would bring every session back as that one
# file's session. When every session is saved, as at quit, the default session, named after startup_session,
# also takes the tabs that belong to no session, so nothing open is left unsaved.
#
# kspaces sidebar docks are never saved. kspaces_watcher.py adds them back to restored tabs.

import os
import re

from kitty.config import atomic_save
from kitty.fast_data_types import add_timer, current_focused_os_window_id, get_options, last_focused_os_window_id
from kitty.session import parse_save_as_options_spec_args
from kittens.tui.handler import result_handler

# Names of the sessions open at the last save, other than the default one, which startup_session already
# loads, but for the one focused at the save, which goes last, the default one too. Read and removed at
# startup.
OPEN_LIST = '.open-sessions'

# Sessions kspaces closed to open again later, a line each of tab-separated fields: how it keeps them, the
# session, and what the keeper kept of it. kspaces writes it; this keeps the file of a listed session whose last window
# closed, and drops a session from it once it has windows again.
KEPT_LIST = '.kept-sessions'


def startup_path():
    path = get_options().startup_session
    return os.path.expanduser(path) if path and path != 'none' else ''


def session_path(name):
    return os.path.join(os.path.dirname(startup_path()), name + '.kitty-session')


def default_name():
    return os.path.basename(startup_path()).rpartition('.')[0]


def kept_path():
    return os.path.join(os.path.dirname(startup_path()), KEPT_LIST)


def kept_lines():
    try:
        with open(kept_path()) as f:
            return [line for line in f if len(line.split('\t')) >= 2]
    except FileNotFoundError:
        return []


def kept_names():
    return {line.split('\t')[1].rstrip('\n') for line in kept_lines()}


def unkeep(name):
    lines = [line for line in kept_lines() if line.split('\t')[1].rstrip('\n') != name]
    if lines:
        atomic_save(''.join(lines).encode(), kept_path())
    else:
        try:
            os.remove(kept_path())
        except FileNotFoundError:
            pass


# What save_as_session --match writes, Boss.serialize_state_as_session, with the OS windows in
# os_window_order rather than kitty's order by focus. The first OS window with matching windows is loaded
# into the current one, the others into new ones.
def save_matching(boss, match, path, main):
    opts, _ = parse_save_as_options_spec_args(['--use-foreground-process', f'--match={match}'])
    matched = frozenset(boss.match_windows(match))
    lines, first = [], True
    for os_window_id in os_window_order(boss, main):
        out = boss.os_window_map[os_window_id].serialize_state_as_session(path, matched, opts, is_first=first)
        first = first and not out
        lines.extend(out)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    atomic_save('\n'.join(lines).encode(), path)


# The OS windows in the order they are saved: kitty's, by focus, but with main last. kitty creates the OS
# windows of startup_session in order and focuses the last, and restore() loads the spaces' files into the
# focused one, so this puts the spaces back in the OS window they were in rather than in whichever had
# focus at quit.
def os_window_order(boss, main):
    order = {current_focused_os_window_id(): 2, last_focused_os_window_id(): 1, main: 3}
    return sorted(boss.os_window_map, key=lambda wid: order.get(wid, 0))


# main_os_window is the OS window holding the most windows of the spaces' sessions, or the last focused.
def main_os_window(boss, sessions):
    counts = {}
    for w in boss.all_windows:
        if w.created_in_session_name in sessions:
            counts[w.os_window_id] = counts.get(w.os_window_id, 0) + 1
    return max(counts, key=counts.get) if counts else last_focused_os_window_id()


# LAUNCH_VARS are the user variables every window kspaces opens in a session carries for a launch wrapper,
# which, unlike every other kspaces* variable, mark no window of kspaces' own.
LAUNCH_VARS = ('kspaces_space', 'kspaces_worktree')

# OWN_VARS are the variables of kspaces' own windows, and of windows being kept, as a kitty match leaves them
# out by name: a match's var: is a regex searched for, so var:kspaces would leave out every window with a
# launch variable too.
OWN_VARS = ('kspaces', 'kspaces_form', 'kspaces_picker', 'kspaces_popup', 'kspaces_keep')
NOT_OWN = ' and '.join(f'not var:^{v}$' for v in OWN_VARS)


# own reports whether window is one of kspaces' own, or being kept, by its variables.
def own(window):
    return any(k.startswith('kspaces') and k not in LAUNCH_VARS for k in window.user_vars)


# session_match matches the windows saved as session name. With unsessioned, the default session also takes
# the windows in no session.
def session_match(name, unsessioned=False):
    if unsessioned and name == default_name():
        return f'(session:^{re.escape(name)}$ or session:^$) and {NOT_OWN}'
    return f'session:^{re.escape(name)}$ and {NOT_OWN}'


# focused_session is the session of the window focused most recently, kspaces' own left out, empty for none.
def focused_session(boss):
    windows = [w for w in boss.all_windows if w.created_in_session_name and not own(w)]
    return max(windows, key=lambda w: w.last_focused_at).created_in_session_name if windows else ''


def save_all(boss):
    if not startup_path():
        return
    default = default_name()
    others = sorted(n for n in boss.all_loaded_session_names if n != default)
    main = main_os_window(boss, set(others))
    for name in [default] + others:
        save_matching(boss, session_match(name, unsessioned=True), session_path(name), main)
    names = list(others)
    if focused := focused_session(boss):
        names = [n for n in names if n != focused] + [focused]
    with open(os.path.join(os.path.dirname(startup_path()), OPEN_LIST), 'w') as f:
        f.write(''.join(n + '\n' for n in names))


# Saving at quit alone left a session's file as it was at the last quit: closing a session's tabs and panes
# and opening it again brought them all back. So a session is also saved a moment after one of its windows
# closes, takes focus or changes directory, which keeps which window is focused too. A session whose last
# window closed has its file removed, so opening it again starts afresh, unless kspaces kept it to open again,
# which leaves the file as kspaces had it saved. The default session's file is startup_session's, so it is
# emptied instead.
#
# Windows in no session are left to save_all. Saved into the default session as they changed, a window that
# opens an OS window of its own, such as a tab switcher mapped to a key, stayed in its file after closing
# with that OS window, and opening the default session ran it again.
#
# Not while kitty quits, which closes every window after on_quit has saved them, nor for the windows of an
# OS window closed as a whole, which kitty also closes one by one: either would remove every file. The
# sessions keep what was saved before, as when the last of them took focus. on_close says which it is.
SAVE_DELAY = 0.5
_quitting = False
# Session names waiting to be saved, each True if one of its windows closed with its OS window or kitty.
_pending = {}
_timer = None


def note_change(boss, window, torn_down=False):
    global _timer
    name = window.created_in_session_name
    if _quitting or not name or not startup_path() or own(window):
        return
    _pending[name] = _pending.get(name, False) or torn_down
    if _timer is None:
        _timer = add_timer(lambda timer_id: save_pending(boss), SAVE_DELAY, False)


def save_pending(boss):
    global _timer, _pending
    _timer, pending, _pending = None, _pending, {}
    if _quitting or not boss.os_window_map:
        return
    main = main_os_window(boss, set(boss.all_loaded_session_names) - {default_name()})
    kept = kept_names()
    for name, torn_down in pending.items():
        if torn_down:
            continue
        match = session_match(name)
        has_windows = any(True for _ in boss.match_windows(match))
        if name in kept:
            # kspaces marks the windows it closes, which the match leaves out.
            if not has_windows:
                continue
            unkeep(name)
        if has_windows:
            save_matching(boss, match, session_path(name), main)
        elif name == default_name():
            atomic_save(b'', session_path(name))
        else:
            try:
                os.remove(session_path(name))
            except FileNotFoundError:
                pass


def on_close(boss, window, data):
    note_change(boss, window, torn_down=data.get('reason') != 'window')


def on_focus_change(boss, window, data):
    if data.get('focused'):
        note_change(boss, window)


def on_cwd_change(boss, window, data):
    note_change(boss, window)


def restore(boss):
    list_path = os.path.join(os.path.dirname(startup_path()), OPEN_LIST)
    try:
        with open(list_path) as f:
            names = [line.strip() for line in f if line.strip()]
        os.remove(list_path)
    except FileNotFoundError:
        return
    # Loading a session switches to it, so the one focused at the save, which the save put last, is loaded
    # last, after startup_session's, the default session, which kitty loaded already. A focused default
    # session is startup_session's itself.
    default, focused = default_name(), names[-1] if names else ''
    paths = [session_path(n) for n in names[:-1] if n != default]
    if startup_path():
        paths.append(startup_path())
    if focused and focused != default:
        paths.append(session_path(focused))
    for path in paths:
        if os.path.exists(path):
            boss.goto_session(path)


# Called when kitty loads this watcher, which is during startup, while startup_session is being created, and
# again whenever kitty.conf's list of watchers changes. The timer runs the restore once the startup session
# is in place. The flag on boss keeps a config reload from restoring sessions a second time.
def on_load(boss, data):
    if getattr(boss, 'sessions_restored', False):
        return
    boss.sessions_restored = True
    add_timer(lambda timer_id: restore(boss), 0, False)


# kitty has no exit event, only this quit hook, so every session is saved when the app is asked to
# terminate: the macOS menu/Dock Quit item, cmd+q, or logout.
def on_quit(boss, window, data):
    global _quitting
    # Called once before the confirmation dialog and again after it is accepted.
    if data.get('confirmed'):
        _quitting = True
        save_all(boss)


def main(args):
    return ''


# The kspaces_save_sessions action saves the sessions named after it, or every session.
@result_handler(no_ui=True)
def handle_result(args, answer, target_window_id, boss):
    if len(args) > 1:
        main = main_os_window(boss, set(boss.all_loaded_session_names) - {default_name()})
        for name in args[1:]:
            save_matching(boss, session_match(name), session_path(name), main)
        return
    save_all(boss)
