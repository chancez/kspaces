# kspaces

A sidebar for kitty that lists repositories ("spaces") and their git worktrees. Each worktree is a kitty
session. Clicking a worktree switches to its session, or creates it with one tab in the worktree the first
time.

```
SPACES

  default

  gr pr/chancez/dev
  docs-reference main
    release-caveats pr/ch...

---------------------------
STATUS
  gr claude
  default make
```

Each row starts with an icon, from the Nerd Fonts symbols kitty ships with: a chevron for the session of
the tab the sidebar is in, a dot for a session with windows open, and none for a worktree with no session
yet, which is drawn faint. A cross next to a space means its worktrees could not be listed. Linked
worktrees have a branch icon before their name. A blank row separates spaces.

Sessions that are open or saved (see `sessions` below) but are not any worktree's, such as the one kitty
starts with, are listed above the spaces.

## Folding and new worktrees

A space with linked worktrees has a triangle before its name. Clicking it, or `space` on the space's row,
hides its linked worktrees; the space's row then stands for all of them, showing the most urgent state and
the current session of any. Which spaces are folded is kept in `$XDG_STATE_HOME/kspaces/folded`, so every
tab's sidebar agrees.

Right-clicking a row, or `m` on it, opens a menu: Open, Fold or Unfold worktrees, and New worktree. New
worktree, or `n`, opens a form in an overlay over the tab's window, as the sidebar is too narrow to type
in. It has two fields, and `tab` moves between them:

- **branch**, which starts with `branch_prefix` and a `/` when that is set, so typing `fix` gives
  `PREFIX/fix`. Delete the prefix for a branch without it. An existing branch is checked out; a new one
  is created from what the space's main worktree has checked out.
- **directory**, empty for the default shown in it: `worktree_dir` under the space's root and the branch
  without the prefix, any `/` turned into `-`. A directory typed in is relative to the space's root unless
  absolute, and `~` is expanded.

`enter` creates the worktree and switches to its session; git's error stays in the form if it refuses.
`kspaces new [SPACE]` opens the same form, for the space given by root or name, or the current one.

## Status

The STATUS list starts a little over halfway down the sidebar, below a separator, and stays there with
"Nothing to report" when it is empty. It shows every window whose program reports a status with the program status
protocol (OSC 7501), most urgent first, with kitty's icons for tab.program_status: a warning sign for
blocked on you, an error sign for failed, a check for done and a spinner for working. The worktree rows
show the most urgent state of their session in place of the dot. Clicking an entry, or `enter` on it,
focuses that window.

Entries are grouped by state under a title with the count: Blocked, Failed, Done, Working and Idle, in
that order, the most recent change first in each group. Idle entries are programs that said they are
waiting for their next task. Each entry's first row has the session, or the window title for a window in
no session, `+N` when the window has N more records, and how long ago the record changed to its state.
Below it is one row with the program's name and what it said: a progress bar when it reports progress,
otherwise its title, otherwise its message. With the sidebar's cursor on an entry, the entry opens up to
show all of it: what a blocked program waits for (permission, a question or a sign-in), the title,
progress, up to six rows of the message, the record's id and the window's other records. Resting the
mouse on an entry shows the same in full, with no limit on the title and message, drawn over the entries
below it so the list does not move.

kitty does not say when a record changed, so the kitty watcher that adds the sidebars also notes it, in
the window's `program_status_at` user variable. With the watcher not loaded, entries show no age.

kitty keeps the records and applies the protocol's rules, and kspaces reads them from `kitten @ ls`:
working and blocked end at the next shell prompt, done and failed stay until the window is focused or typed
in. Anything that writes OSC 7501 to its terminal shows up, an agent or a build alike.

## How it fits into kitty

- A space is a repository root listed in the config. Its worktrees come from `git worktree list`, so new
  ones show up without configuring them.
- A worktree's session is named after the space for the main worktree (`gr`), and `SPACE@DIR` for the
  others (`gr@dev`), so two repositories with a worktree of the same name do not share a session.
- Opening a worktree whose session has windows focuses the most recently focused one. Otherwise, if
  `sessions` is set and has a `SESSION.kitty-session` file, kspaces loads it with `goto_session`. Failing
  both, it runs the configured `new_tab` action with `--cwd=WORKTREE --add-to-session=SESSION` appended.
- Set `tab_bar_filter session:. or session:^$` in kitty.conf so the tab bar only shows the current
  session's tabs. `~`, which kitty's docs suggest, also shows the session last switched to with
  `goto_session`, and kspaces switches by focusing a window.
- kitty can only dock a window to a single tab, so a sidebar in every tab is one `kspaces sidebar` dock per
  tab. A global watcher in kitty.conf adds them. Docks are marked with the `kspaces` user variable.
- kitty places docks inside the tab bar, so keep the tab bar at the top or bottom.

## Install

```sh
go build -o ~/.local/bin/kspaces .
```

## Config

`~/.config/kspaces/config`, or `$XDG_CONFIG_HOME/kspaces/config`:

```
# One repository per line, in display order. ~ is expanded.
space ~/projects/gr
space ~/projects/kitty

# The kitty action that opens a worktree's first tab, split on spaces. Defaults to `launch --type=tab`.
new_tab launch --type=tab

# Where kitty session files are saved, one per session named SESSION.kitty-session. Unset by default,
# which never restores from a file.
sessions ~/.local/share/kitty/sessions

# The new worktree form's defaults: worktrees go under worktree_dir, relative to their space's root, and
# new branch names start with branch_prefix and a /. Default to .worktrees and no prefix.
worktree_dir .worktrees
branch_prefix pr/me
```

## Commands

```
kspaces [sidebar]   the sidebar, meant to run in a kitty dock
kspaces pick        a fuzzy picker over sessions, worktrees and status entries, for a kitty overlay
kspaces list        every space's worktrees with their session names
kspaces open PATH   switch to the session of the worktree at PATH, creating it if needed
kspaces next, prev  switch to the next or previous session with windows open, in sidebar order
kspaces new [SPACE] create a worktree in a form and switch to it, meant for a kitty overlay
```

`next` and `prev` act on the window that has the keyboard, so they suit a kitty mapping. A background
process needs `--allow-remote-control` to reach kitty:

```conf
map cmd+alt+[ launch --type=background --allow-remote-control kspaces prev
map cmd+alt+] launch --type=background --allow-remote-control kspaces next
```

Clicking a row opens it. When the sidebar has keyboard focus, which needs a dock that is not started with
`--dock-skip-focus`, `j`/`k` move, `g`/`G` jump to the first and last row, `enter` or `l` opens, `esc` or
`q` returns focus to the window it came from and `r` refreshes. It refreshes every two seconds on its own.

The same keys work without focusing the sidebar, sent with kitty's `send-key`. A kitty keyboard mode
makes that a set of mappings:

```conf
action_alias spaces_key remote_control send-key --match "var:^kspaces$ and state:parent_focused"
map --new-mode spaces --on-unknown passthrough_and_end f6
map --mode spaces j spaces_key j
map --mode spaces k spaces_key k
map --mode spaces enter combine : spaces_key enter : pop_keyboard_mode
map --mode spaces esc combine : spaces_key esc : pop_keyboard_mode
```

Keys that arrive without focus start from the sidebar's own session and draw the cursor until `enter`
switches, `esc` stops, or ten seconds pass without a key.

Remote control has to be enabled in kitty (`allow_remote_control` and `listen_on`).
