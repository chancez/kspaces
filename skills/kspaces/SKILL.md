---
name: kspaces
description: Hand work off to a new agent in a new kspaces worktree, or run a command in a kspaces tab the user can watch. Use when the user asks for either, by name or as "hand it off", "in a new kspace", "in a tab I can watch", or says yes to your offer to.
argument-hint: "[what to hand off, or the command to show]"
---

# kspaces

kspaces gives every git worktree its own kitty session, listed in a sidebar. Two recipes start something
in one: **hand off** and **show**. Both open in the background, so the user's focus stays where it is and
the new tab appears in the sidebar for them to switch to.

Being invoked is the user's go-ahead: carry the recipe through without asking first. The arguments, or
the request that invoked this, say what: a piece of work to hand off, or a command to show. Pick the
recipe from that, and fill in what they leave open (space, branch name, the prompt) from the
conversation. Ask only when the request names neither a task nor a command.

Check first that kspaces can drive kitty here: `kspaces new --help` lists `--yes`, and `$KITTY_WINDOW_ID`
is set. Without either, say so and fall back to doing the work in place.

## Hand off

Use it when the next piece of work is its own concern: a different branch, a fix that would muddy the
current PR, or a change in another repository. A fresh agent in a fresh worktree keeps each branch one
concern, and leaves you free to finish the one you are in.

1. **Pick the space.** `kspaces list` prints one line per worktree: session, path, branch. The space is the
   session's name before any `@`, such as `gr` in `gr@fix`. Use the current space unless the work belongs
   in another repository.
2. **Pick the branch name**: a short topic, such as `retry-timeouts`. kspaces adds the configured branch
   prefix itself, and derives the directory from the branch.
3. **Write the prompt.** The new agent starts with none of your context, so the prompt is the whole
   brief: the goal, why it is needed, what you found (files, functions, the failing command and its
   output), what is out of scope, and how to tell it is done. Name the worktree and branch it came from
   when the work depends on it. Write it to a file, as it is usually several paragraphs.
4. **Create and start it**, with the agent command the config has, such as `claude`:

   ```sh
   kspaces new SPACE --yes --background --branch TOPIC \
     --command claude --input "prompt=$(cat /tmp/handoff-TOPIC.md)"
   ```

   It prints the new worktree's path. `--base BRANCH` starts the branch from a local branch rather than
   the main worktree's; `--pr N` or `--issue N` start from a PR or an issue instead of `--branch`. If the
   config has no command for the agent, give its command line after `--` instead of `--command`:
   `-- claude "$(cat /tmp/handoff-TOPIC.md)"`.

5. **Tell the user** in one or two lines: the session (`SPACE@TOPIC`), the branch, and what the new agent
   was asked to do. The hand off is done when the new session exists; the work itself is now that
   agent's, so leave it to them.

An error from `kspaces new` creates nothing when it is about the command or its inputs; when it is from
git, such as a branch that exists, pick another name or check that branch out with `--branch` as is.

## Show

Use it when the user should see output live in their terminal: tests they want to watch, a dev server, a
build, a program that takes input. Run it in the worktree's session:

```sh
kspaces run PATH --background -- go test ./...
```

PATH is any directory in the worktree, `.` for the one you are in, or another worktree's path from
`kspaces list`. It opens a new tab; `--open split` puts it beside the session's window instead. The words
after `--` reach the program as typed, through no shell, so wrap a pipeline in `sh -c '...'`. The window
keeps a shell once the command exits, so the output stays.

You do not see this output. When you need the result too, tee it to a file and read that:
`-- sh -c 'go test ./... 2>&1 | tee /tmp/test-out.log'`.

A command the config names, with its inputs, runs the same way: `kspaces run NAME PATH --yes --background
--input K=V`.

Tell the user which session and tab it opened in.

`kspaces new --help` and `kspaces run --help` list every flag.
