# User stories

What kspaces should do for one person working across several machines, such as a desktop, two laptops and a
server, `white`. [The server design](server.md) is how.

## Copies

A **copy** is a space, or one of its worktrees, as one host has it: written `HOST:SPACE@WORKTREE`, and
without `HOST:` for this machine. `gr@dev` is this machine's copy of gr's dev worktree; `white:gr@dev` is
white's. All of a copy's panes run on its host, and its layout is kept by its host. Any machine can open any
host's copy: opening it shows that copy, not a copy of it, so every machine showing it shows the same tabs.

Two copies of one worktree on two hosts are two things, with nothing tying them together.

## `kspaces open white:gr@dev`

- With a kitty session for `white:gr@dev` open here, it switches to it.
- Otherwise it opens one: white's saved layout of it, the panes attached to white's sessions with a session
  provider, or started afresh in their directories without one.
- When white has no layout of it, the worktree's default layout, or one shell tab, in the worktree's
  directory on white.
- From then on new tabs and splits in it run on white, and changes to it are saved to white's layout of it.

## Stories

1. **"I want to connect to my server white."** No project, a shell, as `ssh white` would give.
   `kspaces open white:` opens white's `default` copy: one tab, a shell on white. It is kept, so opening it
   again later, from either laptop, brings back the same tabs.

2. **"I want to do some homelab work on white."** The repository is on white.
   `kspaces open white:homelab` opens white's copy of the space: its tabs on white, in homelab's directory
   there, which white's kspaces knows from white's config. The sidebar lists it under white on the hosts
   tab.

3. **"I'm working on my desktop and want to resume that work on my laptop."**
   On the laptop the hosts tab lists the desktop's copies. Opening `desktop:gr@dev` builds the desktop's
   layout of it on the laptop, attached to the desktop's sessions. Changes are saved to the desktop's
   copy, so back at the desktop it is as the laptop left it.

4. **"Same, but the work is on white, from either laptop."** As 3. White is the usual case, as it is always
   on.

5. **"I'm working on my laptop; nothing should be shared."** The default. `kspaces open gr@dev` opens the
   laptop's own copy, its panes local. No other machine sees it unless it opens `laptop:gr@dev`.

6. **"I have the repository on the laptop and on white, and work in both."** Two copies, `gr@dev` and
   `white:gr@dev`: two rows, two kitty sessions, nothing tying them together.

7. **"I want a new worktree, on white."** New worktree on `white:gr` creates it on white, with white's git;
   removing one removes it there. The rows' git state, changes, ahead and behind, the PR, is white's.

8. **"I closed my laptop while in `white:gr@dev`."** Its panes freeze, or end without a provider. On
   reconnecting the laptop takes white's layout as it is: nothing to reconcile, as only white keeps it.

9. **"I'm done with `white:gr@dev` on this laptop for now."** Closing its kitty session only stops showing
   it here: white's copy and its sessions are left as they are. Closing a tab in it removes the tab from
   white's copy, on every machine showing it.

10. **"A quick local shell while in `white:gr`."** Possible, but it is not part of white's copy: it is not
    saved to white's layout and shows nowhere else. Opening the local copy beside it is the way.

11. **"What's going on on white?"** The hosts tab lists white's copies, which are open here, and white's
    detached sessions, without opening anything.

12. **"My laptop rebooted."** Local copies come back as they do now. Remote copies that were open come back
    from their hosts' layouts.

## Without a session provider

A copy's layout is what is kept; a provider such as cm keeps its processes too. Without one, a remote
copy's panes are shells over ssh in the right directories: resuming it from another machine brings back
the same tabs and splits with fresh shells. With one, the panes attach to the host's sessions as they were
left.
