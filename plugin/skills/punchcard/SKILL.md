---
name: punchcard
description: Track Jira time with the punchcard CLI. Use when the user asks to start, pause, resume or stop a timer, log or submit time or a worklog to Jira, check what is running or what is unsent, or sync worklogs.
---

# Punchcard

`punchcard` tracks time against Jira tickets and sends worklogs. Run the bare `punchcard` through Bash and always pass `--json`; read the envelope `{v, ok, outcome, command, data, events, error}`. Report `events` to the user: they are side effects (a ticket paused, a timer recovered).

## Commands

| Command | Does |
| --- | --- |
| `punchcard status --json` | What runs, what is paused, `unsent` count, `authFailedAt`. Never changes anything. |
| `punchcard ls --json` | Tracked tickets, most recent first. |
| `punchcard start KEY --json` | Start KEY (pauses whatever ran). |
| `punchcard pause --json` | Pause the running timer. |
| `punchcard resume [KEY] --json` | Resume the last paused timer, or KEY. |
| `punchcard stop --json` | **Preview** a stop: exits 6 with `data.preview`, the exact worklog. Changes nothing. |
| `punchcard stop --yes [--duration D] [-m "comment"] --json` | Stop and send the worklog. **The user must approve this call.** |
| `punchcard sync --json` | Send worklogs that are still owed. |

`--duration` takes `1h30m`, `1h`, `90m` or bare minutes (1 m to 24 h).

## Rules

1. **The ticket key.** Use the key the user names. Otherwise use a key in the git branch name (`[A-Z][A-Z0-9]+-\d+`, e.g. `feature/ABC-123-login` is `ABC-123`). If the branch names no key, ask the user for one; never guess.
2. **Stopping is two steps.** First run `punchcard stop --json` (add `--duration` and `-m` if the user gave them). It exits 6 with `data.preview`. Show the user the preview: ticket, duration (`worklog.timeSpentSeconds`) and comment. Only after showing it run the same command with `--yes`. That call always asks the user to approve; if they decline, the timer keeps running and you stop there.
3. **A recovered entry** (`data.preview.recovered: true`, exit 6 even with `--yes`) needs a duration from the user. Ask how long they actually worked; never invent one.
4. **Exit 7 means the stop happened** but the worklog is not sent yet (`ok: true`). Run `punchcard sync --json` later. **Never run `stop` again** after exit 7: that would stop a different timer.
5. **Unattended** (no one to approve, the `--yes` call was denied): do not retry and do not work around the gate. The timer is still running. Tell the user: "run `/punch stop` when you're back".
6. Start, pause and resume need no approval. They move only a timer the user has not set by hand; if `start` reports the timer is held, say so rather than forcing it.

## Exit codes

0 ok · 3 Jira not configured (tell the user to run `punchcard setup`) · 4 nothing to act on, or several candidates (`data.candidates`: ask which) · 5 ticket not found in Jira (check the key) · 6 needs confirmation (`data.preview`) · 7 stopped, worklog owed (run `sync`, never `stop`) · 8 Jira rejected the token (tell the user to run `punchcard setup`) · 64 usage (read `error.hint`) · 75 busy (retry once after a moment).
