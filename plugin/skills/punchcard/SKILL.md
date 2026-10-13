---
name: punchcard
description: Time tracking with the punchcard CLI. Use when the user asks to start, pause, resume or stop a timer, log time or a worklog, check what is running, report what they worked on over a day, week or month, or sync worklogs to a tracker such as Jira.
---

# Punchcard

`punchcard` times work against tickets and logs each stopped timer as a worklog. A ticket key is any slug (`payroll`, `ABC-123`). A tracker is optional: with none, a worklog is final the moment it stops (`state: "local"`); with Jira, it is also sent there. `status --json` says which in `data.tracker` (`"none"` or `"jira"`).

Run the bare `punchcard` through Bash and always pass `--json`; read the envelope `{v, ok, outcome, command, data, events, error}`. Report `events` to the user: they are side effects (a ticket paused, a timer recovered).

## Commands

| Command | Does |
| --- | --- |
| `punchcard status --json --cwd "$PWD"` | What runs, what is paused, `tracker`, and `here`: this directory's key. Never changes anything. |
| `punchcard ls --json` | Tracked tickets, most recent first. |
| `punchcard start KEY --json` | Start KEY (pauses whatever ran). |
| `punchcard pause --json` | Pause the running timer. |
| `punchcard resume [KEY] --json` | Resume the last paused timer, or KEY. |
| `punchcard stop --json` | **Preview** a stop: exits 6 with `data.preview`, the exact worklog. Changes nothing. |
| `punchcard stop --yes [--duration D] [-m "comment"] --json` | Stop and log the worklog (and send it, with a tracker). **The user must approve this call.** |
| `punchcard log --json [--week \| --month \| --from DATE --to DATE]` | Worklogs over a range. Never changes anything. |
| `punchcard sync --json` | With a tracker only: send worklogs that are still owed. |

`--duration` takes `1h30m`, `1h 30m`, `1h`, `90m`, `1:30` or bare minutes (1 m to 24 h).

`data.todaySeconds` in `punchcard status --json` answers "how long have I worked today": the day's total over every ticket.

## Rules

1. **The ticket key.** Use the key the user names. Otherwise run `punchcard status --json --cwd "$PWD"` and use `data.here.key` (from the directory's `.punchcard` file, else its git branch; `data.here.source` says which). When `data.here` is null, ask the user for a key.
2. **Stopping is two steps.** First run `punchcard stop --json` (add `--duration` and `-m` if the user gave them). It exits 6 with `data.preview`. Show the user the preview: ticket, duration (`worklog.timeSpentSeconds`) and comment. Only after showing it run the same command with `--yes`. That call always asks the user to approve; if they decline, the timer keeps running and you stop there.
3. **A recovered entry** (`data.preview.recovered: true`, exit 6 even with `--yes`) needs a duration from the user. Ask how long they actually worked.
4. **Exit 7 means the stop happened** but the worklog is not sent to the tracker yet (`ok: true`). Run `punchcard sync --json` later. **Never run `stop` again** after exit 7: that would stop a different timer.
5. **Unattended** (no one to approve, the `--yes` call was denied): leave the gate as it is and leave the timer running. Tell the user: "run `/punch stop` when you're back".
6. **Start a timer only when the user asks.** Start, pause and resume need no approval, but they move only a timer the user has not set by hand. If `start` reports a timer set by hand (exit 4, `error.code` `by_hand`), say so and leave it.
7. **Reporting a period** ("what did I work on this week", a timesheet): run `punchcard log --json` with the range, and answer from `data.rows` (each: `date`, `started`, `key`, `summary`, `minutes`, `comment`, `state`), `data.open` (a running or paused timer, not yet logged, or null) and `data.range` (`from`, `to`). The default range is Monday through today; `--month` is the 1st through today; `--from`/`--to` are inclusive `YYYY-MM-DD`. For a table to paste, `punchcard log --md` (no `--json`) prints Markdown.

## Exit codes

0 ok · 4 nothing to act on, a timer set by hand (`by_hand`), or several candidates (`data.candidates`: ask which) · 6 needs confirmation (`data.preview`) · 64 usage (read `error.hint`) · 75 busy (retry once after a moment).

These come only from a tracker, so read `data.tracker` before acting on them:

- **3** not configured: Jira is chosen but its credentials are missing (tell the user to run `punchcard setup`), or `sync` with no tracker (nothing to send; every worklog is already logged, so tell the user so and stop there).
- **5** the tracker looked the key up and says it does not exist (check the key).
- **7** stopped, worklog owed to the tracker (run `sync` later, never `stop`). Never happens with no tracker.
- **8** Jira rejected the token (tell the user to run `punchcard setup`). Jira only.

With `tracker: "none"` the only one of these is `sync`'s 3, so on a trackerless machine run `sync` only if the user asks, and send the user to `setup` only if they want a tracker.
