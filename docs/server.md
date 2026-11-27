# The kspaces server

The design of the server, for the [user stories](stories.md): working in any host's copy of a space from any
machine, with or without a session provider.

## Why

Before the server, every kspaces process read what it needed itself: the config, each space's worktrees and
own file, git and gh, the shared layouts through the provider. That leaves three things out of reach:

- **Another host's spaces.** Opening `white:homelab` needs homelab's directory on white, its worktrees and
  their git state, which only white can say. White runs no kitty, so no sidebar there to ask.
- **One writer per layout.** Layouts shared through the provider are written by every machine showing
  them, which is what makes them diverge and needs the merge, the divergence marker and its menu.
- **Answering at once.** A worktree created by a form shows in the sidebar at its next poll, and the picker
  and the forms read git states from a file the sidebar last wrote.

## Roles

kspaces is two roles, in one binary:

- **The host server**, `kspaces server`, one per host, kitty or not. It owns what is the host's: the
  config, the spaces and their worktrees, git and gh, the worktrees being created or removed, and the
  layouts of the host's copies. It runs git and gh once for every client, and tells its clients when what
  it holds changes.
- **The kitty view**, the sidebar, one per kitty. It owns what is kitty's: its windows, the session files,
  the provider's sessions as its windows show them, moving windows whose space changed, the dock's width.
  It is a client of this host's server, and of each other host's whose copies it shows.

The picker and the forms are clients of this host's server too, for what they read of the host.

## Lifecycle

The server listens on a Unix socket in `$XDG_RUNTIME_DIR/kspaces/`, else `kspaces-UID/` in the temporary
directory, named after the config file and the state directory, so a sandbox with its own
`XDG_CONFIG_HOME` or `XDG_STATE_HOME` gets a server of its own.

A client that finds no server starts one: the same kspaces binary, `kspaces server`, in a new session so
it outlives the client, and waits for its socket. Two clients starting one at once are fine: the server
holds a lock on a file beside the socket, and the second exits. The server exits once it has had no client
for five minutes, when its binary is replaced, as by installing another kspaces, and on SIGTERM.

A client that loses the server, as when it exits or crashes, keeps what it last had and connects again,
starting it if need be. A sidebar shows the last host data meanwhile, and its last line says the server is
gone if it cannot get one back.

**Versions.** The first message each way says the protocol's version and kspaces' own. A client of a newer
protocol than its local server asks it to exit and starts its own, as after installing a new kspaces; a
client of an older one fails, saying to restart kitty, so an old sidebar never replaces a newer server.
A remote server of another protocol is an error on that host's rows.

## Protocol

JSON, one message per line, over the socket, or over ssh to another host:

```
-> {"id": 1, "method": M, "params": {...}}
<- {"id": 1, "result": {...}}            or {"id": 1, "error": "text"}
<- {"method": "update", "params": {...}}  pushed to a client that subscribed
```

| Method | Params | Result |
| --- | --- | --- |
| `hello` | `{"protocol", "version"}` | `{"protocol", "version", "host"}` |
| `load` | `{"fresh"}` | the host data, loaded now when `fresh`, else the last |
| `refresh` | `{}` | `{}`; git and gh are asked again at the next load |
| `subscribe` | `{}` | the host data; then `update`s with it whenever it changes |
| `exit` | `{}` | `{}`, then the server exits |
| `new-worktree` | `{"root", "branch", "base", "dir"}` | the host data with it |
| `remove-worktree` | `{"path", "delete_branch", "force"}` | the host data without it |
| `new-space` | `{"name", "dir"}` | `{}`; an ad hoc space the host keeps, in the host data's `ad_hoc` |
| `remove-space` | `{"name"}` | `{}`; ends the provider's sessions in it there, removes the provider's space, and forgets it |
| `load-layouts` | `{"layouts": [{"key", "revision"}...]}` | `{"layouts": [...]}`: those at another revision, 0 for none |
| `save-layout` | `{"key", "revision", "layout"}` | `{"revision"}`, or an error when not the latest |

The host data is the config, or why it could not be read; each space's name, root, worktrees and own
config; the worktrees being created or removed; and each worktree's git state. The server loads it every
two seconds while a client is subscribed, and at once for `load` with `fresh`, which is what a command
asks for after changing something; an `update` goes out only when it changed.

## Other hosts

```toml
[hosts.white]
ssh = "white"            # the ssh destination, the host's name by default
kspaces = "kspaces"      # the kspaces to run there
```

A client reaches another host by `ssh DEST KSPACES server proxy`, which connects its stdin and stdout to
that host's server, starting it as a local client would. ssh does the authentication; the remote host needs
kspaces installed, not kitty. A host that does not answer keeps its last host data, marked stale on the hosts
tab, and is asked again with a backoff.

## Copies

A copy is a space or a worktree as one host has it, `HOST:SPACE@WORKTREE`, see [the stories](stories.md).
The sidebar lists them by host: this machine's copies under it, as before, and another host's under that
host, from that host's data:
its spaces, where they are there, their worktrees and git state.

**Opening** `white:gr@dev` opens a kitty session of that name. Its windows carry the `kspaces_host` user
variable, `white`, beside `kspaces_space` and `kspaces_worktree`, so a launch wrapper knows where they
belong. Each pane runs, in the worktree's directory on white:

- with a provider that has `launch`, the command it prints for `launch {"host", "cwd", "space",
  "worktree"}`: a new session there, attached;
- a pane of the copy's layout that shows a provider session, the provider's `open` of it;
- without a provider, `kitten ssh --kitten cwd=DIR DEST`, which kitty follows for new windows: a split of it
  opens on white, in its directory.

**Changing** `white:gr` itself, a new worktree or removing one, is a request to white's server, which runs
git there.

## Layouts

Each host's server keeps the layouts of its copies, by the copy's kitty session there, with a revision each,
in its state directory. It is the only writer: a sidebar showing a copy captures its kitty session's tabs as
before, and sends `save-layout` to the copy's host with the revision it last had. When another machine
saved since, the save is refused; the sidebar takes the host's layout, merges it into its kitty session
against the layout it last had, and saves again. Every few seconds it asks each host with `load-layouts`
for those saved since, and merges them the same way. There is nothing to reconcile, as there is one
version, the host's: the per-host layouts, the divergence, its marker and its menu go, as do the provider's
`save-layout` and `load-layouts`. See [Shared layouts](kitty.md#shared-layouts).

## Steps

Each is a release of its own.

1. **Local server.** `kspaces server`, the protocol's `hello`, `load`, `refresh`, `subscribe` and `exit`.
   The sidebar, the picker and the forms read the host data from it; the git states' file goes.
   Nothing else changes.
2. **Other hosts, read only.** `[hosts]`, `kspaces server proxy`, and the hosts tab listing each host's
   copies from its server.
3. **Remote copies.** `kspaces open HOST:...`, the hosts tab opening them, `kspaces_host`, the provider's
   `launch`, the ssh panes, and `new-worktree` and `remove-worktree`.
4. **Layouts kept by their host.** `load-layouts` and `save-layout`, and the per-host layouts and their
   divergence removed.

## Not yet

- Writes to the config and the state file still go to the files from each process, under their locks; the
  server could take them, and batch them.
- The kitty view stays in the sidebar: with one sidebar per kitty, it has nothing to share.
- Without a provider, a copy's layout is not kept: a layout names the provider's sessions, and a pane of
  `kitten ssh` has none to name, so a copy opened again elsewhere is one shell tab.
- A copy's new worktree form takes a branch and what it starts from; the host's branches, PRs and issues
  are not listed, nor its layouts or commands offered.
- Most commands, `list`, `open PATH`, `new` with `--yes` and the like, still read the config and the
  spaces themselves rather than ask the server.
- A pane split from a copy's `kitten ssh` window opens on the host by kitty's own doing, which needs the
  host's shell integration reporting the remote directory.
