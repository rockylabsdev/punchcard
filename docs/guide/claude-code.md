# Claude Code

One optional plugin, `punchcard`, connects Punchcard to Claude Code. It has two parts:

- A **mod**: code that runs inside Claude Code. It turns your own prompts into presence and focus, draws a band above the prompt, adds the `/punch` command, and gates an agent's stop.
- A **skill**: a page of instructions the agent reads when you ask it about time or timers. It teaches the commands and the rules.

Everything the plugin does goes through the bare `punchcard` on your `PATH` with `--json`. The CLI and the TUI work fully without the plugin. There is no MCP server and no settings hook.

The plugin's source is the `plugin/` directory of the repository. Its gate is tested against Claude Code's own engine (`claude plugin test plugin`) and that test passes on this branch.

## Installing the plugin

`punchcard setup` offers to install it when `claude` is on your `PATH`, by running:

```sh
claude plugin marketplace add j-dsmith/punchcard
claude plugin install punchcard@punchcard -s user
```

The marketplace pins the plugin to the binary's release tag, so an install always gets the mod and skill that match the binary. **Until the first release is tagged, the install fails**: `fatal: Remote branch v0.1.0 not found in upstream origin`. Setup reports the failure and leaves your credentials alone. Until then, load it from your clone for one session at a time:

```sh
claude --plugin-dir /path/to/punchcard/plugin
```

The band will then warn that `plugin 0.1.0 ≠ punchcard 0.0.0-…` because a build from a clone has no release version. That warning is expected until there is a release.

After `brew upgrade`, the binary leads and the plugin follows: the band says the versions differ and `punchcard setup` offers `claude plugin update punchcard@punchcard`. Why it works this way: [ADR 0007](../adr/0007-plugin-pinned-to-binary-release-tag.md).

Remove it with `claude plugin uninstall punchcard@punchcard`.

## How the mod knows the ticket

A session's ticket is the first thing shaped like `ABC-123` (capital letters and digits, a dash, digits) in the git branch name of the session's directory. `feature/ABC-123-login` names `ABC-123`. Nothing else names a session's ticket: not the directory name, not an Orca work item, not the conversation. A session on `main` has no ticket.

## What your prompts do

When you submit a prompt that you typed yourself, the mod runs:

```
punchcard touch --cwd <session directory> --origin prompt --json
```

That is **presence** (the timer is yours, you are here) and, if the branch names a ticket, **focus**: an un-held timer running on another ticket moves to this one. A held timer stays where it is. Nothing starts a timer when nothing is running: today a prompt only moves a timer that already runs, so start one first (by hand, with `/punch start`, or by asking the agent).

A prompt that is a slash command (`/punch pause`) is presence only, so the pause is not undone by the same keypress. Typing in the prompt box is presence, at most once every 30 seconds, and never moves anything. Scheduled prompts, subagents and messages from other sessions are none of these.

Side effects of the mod's own calls show as one-shot toasts: `⏸ ABC-123 paused`, `▶ DEF-9 started`, `+ DEF-9 added`, `! ABC-123 recovered: fix the duration at stop`, `ABC-123 worklog sent`.

## The band above the prompt

The mod polls `punchcard status --json` every 10 seconds, and right after any of its own calls, and draws one line above the prompt. Nothing is drawn when you are idle with nothing owed and nothing wrong.

| Band | Meaning |
| --- | --- |
| `▶ ABC-123 1:12 · held · Fix the login form` | Running, for 1 h 12 min, started by hand, with its summary. |
| `▶ ABC-123 1:12 · Fix the login form` | Running and not held: your prompts can move it. |
| `⏸ ABC-123 0:40 · Fix the login form` | Paused. `+1 paused` follows when more are paused. |
| `! ABC-123 recovered · fix at stop` | Recovery capped it. Stop it with a duration you choose. |
| `! ABC-123 stale 1:12` | Running with no presence for over 2 hours; the next change will cap it. |
| `… · 2 unsent` | Two worklogs are owed to Jira. |
| `… · Jira auth failed` | Jira rejected the token. Run `punchcard setup`. |
| `… · Keychain error: <cause>` | The Keychain could not be read and there is no token file. |
| `… · Jira not connected` | No credentials. Timers still work; worklogs wait. |
| `… · here: DEF-9` | This session's branch names a ticket other than the running one. |
| `… · plugin 0.1.0 ≠ punchcard 0.2.0 · run punchcard setup` | The plugin and the binary come from different releases. |
| `punchcard not found on PATH` | Claude Code cannot run `punchcard`. Presence does nothing until it can. |
| `punchcard: status timed out` | A status call took over 5 seconds. |
| `punchcard: status gave no JSON` | The binary answered, but not with the envelope. |

The colour follows the worst thing on the line: green while running, yellow when paused or when something waits, red for recovered, stale, an auth failure or a missing binary.

## `/punch`

Act by hand without leaving the session. Everything `/punch` does has origin `hand`, so a start holds the timer.

| Command | Does |
| --- | --- |
| `/punch start [KEY]` | Start KEY, or this branch's ticket when no key is given. With neither: `This branch names no ticket: use /punch start KEY.` |
| `/punch pause` | Pause the running timer. |
| `/punch resume [KEY]` | Resume the last paused entry, or start KEY. |
| `/punch stop [duration] [-m comment]` | Preview the worklog, then ask. |

`/punch stop` runs a dry run first and shows a dialog with the exact worklog, for example `Log 1h30m to ABC-123 with "fixed the formatter"?`, and three answers:

- **Submit** sends it.
- **Keep pending** stops the timer and keeps the worklog to send later (`punchcard sync`).
- **Cancel** leaves the timer running.

The stop that follows is pinned to the ticket and duration you were shown, in whole minutes. A recovered entry is not stopped without a duration: `/punch stop 1h30m`. The reply in the transcript says what happened: `Logged 1h30m to ABC-123.`, `Stopped ABC-123: 1h30m kept pending.`, or `Stopped ABC-123: 1h30m saved but not sent (…). Run `punchcard sync` later; never stop again.` Anything else answers with the CLI's error and hint.

## What an agent may do

An agent runs the same CLI through its shell, where the origin is `agent`. It may start, pause and resume without asking you, and those moves only ever touch a timer you did not set by hand. A held timer refuses it (exit 4, `held`), and the skill tells the agent to say so rather than force it.

**An agent's stop is a proposal.** `punchcard stop --json` from an agent always exits 6 with the exact worklog in `data.preview` and changes nothing. The skill tells the agent to show you the ticket, duration and comment, and only then run the same command with `--yes`. That call is **gated**: Claude Code puts a yes/no to you before it runs, with the reason `Punchcard: this stops the timer and sends the worklog to Jira. Approve only the worklog you were shown.` Decline, and the timer keeps running.

Two gates produce that one dialog:

1. The mod's hook on Bash commands answers `ask` for anything that names `punchcard`, `stop` and `--yes` in one command line, in any order, inside compound commands too. It never answers `allow` (a plugin's allow would override your own rules), and if the hook itself fails it still answers `ask`.
2. The ask rule `Bash(punchcard stop *--yes*)` in `permissions.ask` of your Claude Code user settings, which `setup` offers to add. It holds even with the plugin disabled or uninstalled.

Both ask under `--dangerously-skip-permissions`. Under `claude -p` or any other run with nobody to answer, the gated call is denied, nothing is sent, and the timer stays running until you stop it or it goes stale. The skill tells the agent to say "run `/punch stop` when you're back" and not to try again. (The denial under `-p` is the behaviour the design records as verified on Claude Code 2.1.291; it was not re-run for this guide.)

A command you type yourself with `!` inside Claude Code runs in the agent's shell and counts as an agent. Use `/punch`, the TUI or a plain shell to act by hand.

Why an agent proposes and you dispose: [ADR 0005](../adr/0005-agent-stop-gated-by-mod-and-ask-rule.md).

## What the skill teaches the agent

The skill is one page the agent loads when you talk about timers, logging time or worklogs. It says: run the bare `punchcard` with `--json` and read the envelope; report the `events` to the user because they are side effects; take the ticket key from what the user says, else from the branch name, and ask rather than guess; a stop is two steps, preview then `--yes`, and the `--yes` call always asks the user; a recovered entry needs a duration the user gives; exit 7 means the stop happened and only the send is owed, so run `sync` and never `stop` again; when unattended, do not retry or work around the gate, and tell the user to run `/punch stop` when they are back; start, pause and resume need no approval but a held timer is the user's. It ends with the exit code table and what to do on each.

## Checking it is working

- `punchcard status` in a plain shell shows what the band shows.
- The TUI, open in another pane, tags a row `started by prompt` or `paused by agent` within a second of the plugin or the agent changing anything.
