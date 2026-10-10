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

A build from a clone has no release version (`go install` stamps a pseudo-version such as `v0.0.0-20261009220046-aaacff0d8d07+dirty`), so the band and `punchcard setup` never compare it with the plugin's version: there is no `plugin ≠ punchcard` warning for it, as for `dev`.

After `brew upgrade`, the binary leads and the plugin follows: the band says the versions differ and `punchcard setup` offers `claude plugin update punchcard@punchcard`. Why it works this way: [ADR 0007](../adr/0007-plugin-pinned-to-binary-release-tag.md).

Remove it with `claude plugin uninstall punchcard@punchcard`.

## How the mod knows the ticket

The mod has no rule of its own: it asks `punchcard status --json --cwd <session directory>` and reads `data.here`, the directory's key. The binary answers in this order:

1. **A `.punchcard` file**, in the directory or the nearest ancestor that has one, up to and including the git repository's root (the filesystem root outside a repository). See [`.punchcard` files](#punchcard-files).
2. **The git branch.** The first thing shaped like `ABC-123` (capital letters and digits, a dash, digits) in the branch name. `feature/ABC-123-login` names `ABC-123`.

`here.source` says which one answered (`"file"` or `"branch"`); `here` is `null` when neither names a key. Nothing else names a session's ticket: not the directory name, not an Orca work item, not the conversation. A session on `main` with no `.punchcard` has no ticket.

### `.punchcard` files

A `.punchcard` file names the ticket for a directory tree, whatever branch is checked out. It is how a repository on `main`, or a key that is not shaped like `ABC-123` (`payroll`), gets a ticket. The first line that is neither blank nor a `#` comment is the key, trimmed; the rest of the file is ignored.

```
$ cat .punchcard
# this repository is billed to
payroll
$ punchcard status --json --cwd src
{"v":1,"ok":true,"outcome":"ok","command":"status","data":{"running":null,"paused":[],"unsent":{"count":0,"seconds":0},"authFailedAt":null,"tracker":"none","here":{"key":"payroll","source":"file"}},"events":[],"error":null}
```

That run was from `src/`, a subdirectory: the file at the repository's root answered. The key must be valid under the tracker in effect: any slug (letters, digits, `.`, `_`, `-`, starting with a letter or digit) with no tracker, the `ABC-123` shape with Jira. A line that is not a key is skipped with a notice, on stderr and in the envelope's `notices`, and the branch answers instead:

```
$ punchcard status --json --cwd .
punchcard: /…/payroll-app/.punchcard: "pay roll" is not a key (like payroll); using the branch
{"v":1,"ok":true,"outcome":"ok","command":"status","data":{…,"tracker":"none","here":{"key":"ABC-123","source":"branch"}},"events":[],"error":null,"notices":["/…/payroll-app/.punchcard: \"pay roll\" is not a key (like payroll); using the branch"]}
```

Commit the file if everyone working in the repository bills the same ticket; add it to `.gitignore` if it is yours alone.

## What your prompts do

When you submit a prompt that you typed yourself, the mod runs:

```
punchcard touch --cwd <session directory> --origin prompt --json
```

That is **presence** (the timer is yours, you are here) and, if the directory names a ticket (`here`), **focus**: an un-held timer running on another ticket moves to this one. A held timer stays where it is. Nothing starts a timer when nothing is running: today a prompt only moves a timer that already runs, so start one first (by hand, with `/punch start`, or by asking the agent).

A prompt that is a slash command (`/punch pause`) is presence only, so the pause is not undone by the same keypress. Typing in the prompt box is presence, at most once every 30 seconds, and never moves anything. Scheduled prompts, subagents and messages from other sessions are none of these.

Side effects of the mod's own calls show as one-shot toasts: `⏸ ABC-123 paused`, `▶ DEF-9 started`, `+ DEF-9 added`, `! ABC-123 recovered: fix the duration at stop`, and with Jira `ABC-123 worklog sent`.

## The band above the prompt

The mod polls `punchcard status --json --cwd <session directory>` every 10 seconds, and right after any of its own calls, and draws one line above the prompt. Nothing is drawn when you are idle with nothing wrong, in a session whose directory names no ticket.

With no tracker, the band is about the timer alone: the rows marked *Jira only* below never appear, and having no Jira credentials is not a warning. With Jira, every row can appear.

| Band | Meaning |
| --- | --- |
| `▶ ABC-123 1:12 · held · Fix the login form` | Running, for 1 h 12 min, started by hand, with its summary. |
| `▶ ABC-123 1:12 · Fix the login form` | Running and not held: your prompts can move it. |
| `⏸ ABC-123 0:40 · Fix the login form` | Paused. `+1 paused` follows when more are paused. |
| `! ABC-123 recovered · fix at stop` | Recovery capped it. Stop it with a duration you choose. |
| `! ABC-123 stale 1:12` | Running with no presence for over 2 hours; the next change will cap it. |
| `… · 2 unsent` | *Jira only.* Two worklogs are owed to Jira. |
| `… · Jira auth failed` | *Jira only.* Jira rejected the token. Run `punchcard setup`. |
| `… · Keychain error: <cause>` | *Jira only.* The Keychain could not be read and there is no token file. |
| `… · Jira not connected` | *Jira only.* Jira is the tracker but has no credentials. Timers still work; worklogs wait. |
| `timer on ABC-123 1:12 · … · here: DEF-9` | Grey. The timer is running, but on another ticket than this session's (DEF-9). Only the session whose ticket is running gets the green `▶` band. |
| `⏸ ABC-123 0:40 · … · here: DEF-9` | Paused on another ticket than this session's. |
| `DEF-9 not running · /punch start` | Nothing is running or paused, and this session's directory names DEF-9. |
| `… · plugin 0.1.0 ≠ punchcard 0.2.0 · run punchcard setup` | The plugin and the binary come from different releases. |
| `punchcard not found on PATH` | Claude Code cannot run `punchcard`. Presence does nothing until it can. |
| `punchcard: status timed out` | A status call took over 5 seconds. |
| `punchcard: status gave no JSON` | The binary answered, but not with the envelope. |

The colour follows the worst thing on the line: green while running on this session's ticket (or in a session whose directory names none), grey while running on another session's ticket, yellow when paused or when something waits, red for recovered, stale, an auth failure or a missing binary. So with several sessions open, exactly one band is green: the one whose ticket is being timed.

## `/punch`

Act by hand without leaving the session. Everything `/punch` does has origin `hand`, so a start holds the timer.

| Command | Does |
| --- | --- |
| `/punch start [KEY]` | Start KEY, or this session's ticket (`here`, asked fresh) when no key is given. With neither: `No ticket here (no .punchcard, and the branch names none): use /punch start KEY.` |
| `/punch pause` | Pause the running timer. |
| `/punch resume [KEY]` | Resume the last paused entry, or start KEY. |
| `/punch stop [duration] [-m comment]` | Preview the worklog, then ask. |

`/punch stop` runs a dry run first and shows a dialog with the exact worklog, for example `Log 1h30m to ABC-123 with "fixed the formatter"?`, and these answers:

- **Submit** stops the timer and logs the worklog (with Jira, sends it).
- **Keep pending** (Jira only) stops the timer and keeps the worklog to send later (`punchcard sync`).
- **Cancel** leaves the timer running.

The stop that follows is pinned to the ticket and duration you were shown, in whole minutes. A recovered entry is not stopped without a duration: `/punch stop 1h30m`. The reply in the transcript says what happened: `Logged 1h30m to ABC-123.` (logged locally, or sent to Jira), and with Jira `Stopped ABC-123: 1h30m kept pending.` or `Stopped ABC-123: 1h30m saved but not sent (…). Run `punchcard sync` later; never stop again.` Anything else answers with the CLI's error and hint.

## What an agent may do

An agent runs the same CLI through its shell, where the origin is `agent`. It may start, pause and resume without asking you, and those moves only ever touch a timer you did not set by hand. A held timer refuses it (exit 4, `held`), and the skill tells the agent to say so rather than force it.

**An agent's stop is a proposal.** `punchcard stop --json` from an agent always exits 6 with the exact worklog in `data.preview` and changes nothing. The skill tells the agent to show you the ticket, duration and comment, and only then run the same command with `--yes`. That call is **gated**: Claude Code puts a yes/no to you before it runs, with the reason `Punchcard: this stops the timer and logs the worklog (sending it to the tracker, if one is set up). Approve only the worklog you were shown.` Decline, and the timer keeps running.

Three things produce that dialog, and you see one of them, not all three:

1. The plugin's permission hook answers `ask` for any Bash command that names `punchcard`, `stop` and `--yes` in one command line, in any order, inside compound commands too. It never answers `allow` (a plugin's allow would override your own rules), and if the hook itself fails it still answers `ask`. In the default mode and under `--dangerously-skip-permissions` that `ask` is Claude Code's own permission dialog.
2. In **auto mode**, Claude Code hands an `ask` to a classifier rather than to you, so the plugin raises its own dialog on the call itself. Its words follow the tracker, read from a fresh `status` just before it asks:

   | Tracker | Question | Answers |
   | --- | --- | --- |
   | none | `Punchcard: this command stops the timer and logs the worklog. Log it?` | **Log it**, **Keep the timer running** |
   | Jira | `Punchcard: this command stops the timer and sends the worklog to Jira. Send it?` | **Send to Jira**, **Keep the timer running** |

   When `status` does not answer, the plugin uses the tracker from its last poll, and Jira's words if it has none. Anything but the first answer refuses the command with the timer untouched. Only the words change with the tracker: when the dialog appears, and that it refuses when it cannot ask, are the same for both. The plugin does this only where Claude Code would not ask you itself: auto mode, or before it knows the mode.
3. The ask rule `Bash(punchcard stop *--yes*)` in `permissions.ask` of your Claude Code user settings, which `setup` offers to add. It makes Claude Code ask in every mode, auto mode included, and it holds even with the plugin disabled or uninstalled. When it is present the plugin skips its own dialog, so you are never asked twice.

Under `claude -p` or any other run with nobody to answer, the gated call is refused, nothing is logged, and the timer stays running until you stop it or it goes stale. The skill tells the agent to say "run `/punch stop` when you're back" and not to try again. (The `-p` denial of the permission dialog is what the design records as verified on Claude Code 2.1.291; the plugin's own dialog rejecting under `-p` is what the mod API documents, and both are covered by the plugin's tests against the engine.)

A command you type yourself with `!` inside Claude Code runs in the agent's shell and counts as an agent. Use `/punch`, the TUI or a plain shell to act by hand.

Why an agent proposes and you dispose: [ADR 0005](../adr/0005-agent-stop-gated-by-mod-and-ask-rule.md).

## What the skill teaches the agent

The skill is one page the agent loads when you talk about timers, logging time, worklogs or what you worked on. It says: run the bare `punchcard` with `--json` and read the envelope; report the `events` to the user because they are side effects; take the ticket key from what the user says, else from `here` in `punchcard status --json --cwd "$PWD"` (the same answer the band uses), and ask when `here` is null; a stop is two steps, preview then `--yes`, and the `--yes` call always asks the user; a recovered entry needs a duration the user gives; exit 7 means the stop happened and only the send to the tracker is owed, so run `sync` and never `stop` again; when unattended, leave the gate alone and tell the user to run `/punch stop` when they are back; start, pause and resume need no approval but a held timer is the user's; to report a day, week or month, run `punchcard log --json` and answer from its rows. It ends with the exit codes, and says that 3, 5, 7 and 8 come only from a tracker: on a machine with no tracker the only one is `sync`'s 3, which means there is nothing to send.

## Checking it is working

- `punchcard status` in a plain shell shows what the band shows.
- The TUI, open in another pane, tags a row `started by prompt` or `paused by agent` within a second of the plugin or the agent changing anything.
