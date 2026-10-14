# The CLI

Every action is a subcommand of `punchcard`. The CLI is stateless: each call locks the state file, reads it fresh, applies one change, writes, and exits. No process stays behind. Bare `punchcard` opens the [TUI](tui.md) on a terminal and prints `status` otherwise.

All output below was produced by running the binary in throwaway directories, most of it with no tracker. Without one, every worklog is logged on your machine the moment you stop (`local`) and nothing leaves it. Jira is the optional tracker, chosen in `setup`; the examples that need it say so, and where a fake Jira was answering, the example says that too.

## Commands

| Command | Does | With Jira as the tracker |
| --- | --- | --- |
| `start [KEY]` | Start the timer on KEY, pausing whatever ran. No KEY: pick from recents on a terminal. | Fetches the summary for a ticket new to the list, or still unverified |
| `pause` | Pause the running timer. The entry stays open. `--at TIME`: pause then. | no call |
| `resume [KEY]` | Resume the most recently paused entry. `resume KEY` is `start KEY`. `--at TIME`: resume then. | as `start` |
| `stop [KEY]` | Stop an entry and log its worklog. No KEY: the running entry, else the single paused one. | Also sends the worklog, one POST |
| `status` | The running ticket, paused tickets, the tracker; with Jira, unsent worklogs and the connection. | no call (reads the token) |
| `status --short` | One line for a shell prompt. | no call |
| `ls [--all]` | The list, most recently active first. `--all` adds removed tickets. | no call |
| `add KEY... [--summary TEXT]` | Put tickets on the list without starting anything. | as `start` |
| `rm KEY` | Take a ticket off the list; its logged worklogs stay in the log. | no call |
| `log` | Worklogs over a range (this week by default) as Markdown, CSV or JSON. | no call |
| `sync [KEY]` | With a tracker: send every pending and failed worklog, or only KEY's. Without one: exit 3, nothing to send. | yes |
| `setup` | The wizard: choose a tracker (none or Jira), then the Claude Code plugin. Needs a terminal. | Test Connection |
| `version` | Version, contract version, platform. | no call |
| `doctor [--no-redact]` | What a bug report needs, in one paste: files, tracker, timer, Claude Code, plugin, ask rule, terminal, OS. Redacted; always exit 0. | no call (names the token's source) |
| `help [command]` | Help text. `--help` on any command does the same. | no call |

Without a tracker no command calls anything; the third column is what changes once Jira is set up.

Hidden, for the Claude Code plugin:

| Command | Does |
| --- | --- |
| `touch [KEY] [--cwd DIR]` | Record presence, and focus on KEY or on DIR's key (its [`.punchcard`](#punchcard-a-directorys-key), else its git branch). Never waits for the lock. |
| `retry [KEY]` | An alias of `sync`. |
| `--origin hand\|prompt\|agent` | Override who caused the change. Any command takes it. |

### Global flags

| Flag | Does |
| --- | --- |
| `--json` | Print one JSON envelope on stdout, success or failure. Never prompts. `PUNCHCARD_JSON=1` does the same. Stdout not being a terminal never switches it on. |
| `--origin` | Hidden. See [Origin](#origin). |

### Environment

| Variable | Effect |
| --- | --- |
| `PUNCHCARD_JSON=1` | As `--json`. |
| `PUNCHCARD_TOKEN` | With Jira as the tracker, the API token. Overrides the Keychain and the token file while set. `setup` never writes it. Ignored without a tracker. |
| `CLAUDECODE`, `AI_AGENT` | Either set: the origin is `agent`. Claude Code sets both in its shell. |
| `NO_COLOR` | Set: no colour, even on a terminal. |
| `XDG_CONFIG_HOME`, `XDG_DATA_HOME` | Where config and data live. See [Files](files-and-troubleshooting.md). |

Colour is used only on a terminal with `NO_COLOR` unset: green `▶`, yellow `⏸`, red `!` and `punchcard:` on errors.

## The commands, one by one

### `start [KEY]`

```
$ punchcard start payroll
+ payroll added
▶ payroll running  0:00:00  by hand
```

Without a tracker a key is any slug: a letter or digit, then letters, digits, `.`, `_` or `-`. It is case-sensitive and kept as typed. Anything else is exit 64:

```
$ punchcard start "pay roll"
punchcard: usage: "pay roll" is not a ticket key
[exit 64]
```

With Jira as the tracker a key must look like `ABC-123` (letters and digits, a dash, digits) and is upper-cased. A ticket new to the list gets its summary (a 2 second budget), and a key Jira does not know is refused with nothing changed:

```
$ punchcard start DEMO-1
+ DEMO-1 added
▶ DEMO-1 running  0:00:00  by hand  Add the worklog POST

$ punchcard start TYPO-999
punchcard: Jira has no ticket TYPO-999 that this account can see
  check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed.
[exit 5]
```

When Jira cannot be reached, or has no token, the ticket is added anyway and marked unverified, to be confirmed the next time it can be:

```
$ punchcard start DEMO-1
+ DEMO-1 added (not verified with Jira yet)
▶ DEMO-1 running  0:00:00  by hand
```

Starting a second ticket pauses the first and reports it as an event:

```
$ punchcard start ABC-123
⏸ payroll paused
+ ABC-123 added
▶ ABC-123 running  0:00:00  by hand
```

Starting the ticket that already runs changes nothing, except that a start by hand marks an agent's or a prompt's timer as set by hand, so an agent can no longer move it:

```
$ punchcard start DEMO-1
DEMO-1 set by hand: agents can't move it
▶ DEMO-1 running  0:00:00  by hand
```

With no key, on a terminal, a numbered picker of up to 9 recents. A number or a key typed in full starts it; an empty line cancels with exit 0:

```
$ punchcard start
  1) DEMO-1  
  2) DEMO-2  
  3) DEMO-7  
Start which? (number or KEY, empty cancels) 2
▶ DEMO-2 running  0:00:00  by hand
```

Not on a terminal, or with `--json`, a missing key is a usage error (exit 64).

### `pause`

```
$ punchcard pause
⏸ DEMO-2 paused  0:00:00
```

Nothing running is exit 4: `no timer: nothing is running`.

### `resume [KEY]`

Resumes the most recently paused entry. With a timer already running, it is a no-op with exit 0.

```
$ punchcard resume
▶ DEMO-2 running  0:00:00  by hand

$ punchcard resume DEMO-1
⏸ DEMO-2 paused
▶ DEMO-1 running  0:00:00  by hand
```

Nothing paused is exit 4: `no timer: nothing is paused`.

### `--at TIME`: pause, resume or start at an earlier time

`--at` on `pause`, `resume` or `start` makes the change at TIME instead of now. The two common fixes:

**A meeting you forgot to pause for.** Pause at when it started, resume at when it ended (or `-` in the [TUI](tui.md#away--)):

```
$ punchcard pause --at 10:00
⏸ DEMO-2 paused at 10:00  1:02:00
$ punchcard resume --at 10:45
▶ DEMO-2 running since 10:45  1:07:00  by hand
```

**A lunch you paused for late.** You left at 12:00 but paused at 12:40. With nothing running, `pause --at` moves the last pause, so move it back, then resume when you got back:

```
$ punchcard pause --at 12:00
$ punchcard resume --at 13:00
```

Either way the entry keeps both stretches, and the gap between them is a pause like any other, left out of the worklog (the TUI's Ticket pane shows it as `10:00–10:45 ‖ paused 45m`). `start KEY --at 09:00` starts KEY at 09:00, and a ticket it pauses is paused at 09:00 too.

TIME is one of:

- `HH:MM`: the most recent such local time at or before now, so `23:50` typed at 00:10 is last night.
- `-D`: that long ago, D in the [duration](#stop-key) forms: `-45m`, `-1h30m`, `-1:30`.

Nothing else, and no dates: a correction is within the day, and `stop --duration` covers the rest.

It refuses, with exit 64 and nothing changed, when:

- TIME is not one of those forms, or is in the future;
- `pause --at` is before the open segment started, or moves a recovered entry's pause past `lastSeen`, the last time you were seen (recovery capped it there);
- `resume --at` is before the pause (the gap cannot be negative), or has no KEY while any timer runs (`resume KEY --at` is `start KEY --at`);
- `start --at` is before the running ticket's open segment started, or names the running ticket itself (use `pause --at` then `resume --at`);
- the time would overlap time already measured on any ticket: the ticket's last stopped entry, or another ticket's stretch. The message points at the way out: `that overlaps PUN-7's time from 09:00 to 10:00; fix the length at stop instead`.

A cut that would leave a stretch under a minute drops it instead, and says so: `the 30s from 09:59 was under a minute, so it was dropped`.

`--at` changes measured time and never logs anything, so it needs no approval: an agent may use it under the same rules as the plain commands, so a timer set by hand refuses it (`by_hand`), and so does moving the pause of an entry paused while set by hand. It is presence like any command by hand. It never touches a stopped entry or `lastSeen`. In `--json`, `data` is the timer after the change as usual, and the `paused` and `started` events carry `at`:

```
$ punchcard pause --at 10:00 --json
{"v":1,"ok":true,"outcome":"ok","command":"pause","data":{…},"events":[{"kind":"paused","key":"DEMO-2","at":"2026-10-13T10:00:00.000-0500"}],"error":null}
```

### `stop [KEY]`

| Flag | Does |
| --- | --- |
| `--duration D` | Log D instead of the measured time: `1h30m`, `1h 30m`, `1h`, `90m`, bare minutes or `1:30` (hours and minutes), 1m to 24h. Case and spaces are ignored. Seconds and fractions (`30s`, `1.5h`) are refused with exit 64. |
| `-m`, `--message TEXT` | The worklog comment. |
| `--no-sync` | With a tracker: record the worklog as pending without sending it. Without one it changes nothing; the worklog is logged. |
| `--yes` | Log without prompting. An agent's stop needs it. |
| `--dry-run` | Show the exact worklog a stop would log, and change nothing. Always exit 0. |

With no KEY, stop targets the running entry, else the single paused one. Several paused and nothing running is exit 4 naming them.

**On a terminal** (no `--json`, no `--yes`, no `--dry-run`, not an agent), stop prompts. The duration defaults to the measured time truncated to the minute; Enter accepts it. A bad answer is explained and asked again. The comment is optional:

```
$ punchcard stop
DEMO-7 Duration [5m]: 30s
  "30s": use 1h30m, 1h 30m, 1:30, 90m or minutes
DEMO-7 Duration [5m]: 25m
Comment (optional): 
■ DEMO-7 stopped: 25m, worklog pending
```

(That run had Jira chosen but not connected, hence `worklog pending`; without a tracker the last line ends `logged`.)

Under a minute there is no default and an empty Enter asks again; type a duration. Ctrl-d at the duration cancels: `punchcard: not stopped; the timer is untouched`, exit 0. `--duration` and `-m` prefill the prompts.

**Not on a terminal, or with `--json` or `--yes`**, stop uses the measured time with no prompt. Under one minute it refuses:

```
$ punchcard stop
punchcard: a worklog under one minute is never logged
  pass --duration
[exit 64]
```

Without a tracker the worklog is logged and that is the end of it, exit 0:

```
$ punchcard stop payroll --yes --duration 25m -m "pay run"
■ payroll stopped: 25m, logged

$ punchcard stop --yes --duration 30 --json
{"v":1,"ok":true,"outcome":"ok","command":"stop","data":{"key":"ABC-123","entryId":"068D7DC8-8E58-4AA2-A3AF-EE78F1A94ED7","worklog":{"state":"local","startedAt":"2026-10-09T16:55:15.356-0500","timeSpentSeconds":1800}},"events":[],"error":null}
```

With Jira as the tracker the worklog goes to Jira at once. What you see depends on how that went (these runs used a fake Jira, then Jira with no token):

```
$ punchcard stop -m "Wired the POST" --json
{"v":1,"ok":true,"outcome":"ok","command":"stop","data":{"key":"DEMO-1","entryId":"787EF344-3A5E-4799-A900-E711323E9F9D","worklog":{"state":"synced","startedAt":"2026-10-09T09:33:10.989-0500","timeSpentSeconds":2820,"comment":"Wired the POST","worklogId":"110001","syncedAt":"2026-10-09T10:20:11.483-0500"}},"events":[{"kind":"synced","key":"DEMO-1"}],"error":null}
[exit 0]
```

```
$ punchcard stop
■ DEMO-2 stopped: 12m, worklog failed
punchcard: the worklog is saved but not sent: Jira answered 503: Service unavailable
  run `punchcard sync` later; never stop again
[exit 7]
```

```
$ punchcard stop --yes --duration 25m
■ DEMO-1 stopped: 25m, worklog pending
punchcard: the worklog is saved but not sent: not connected to Jira: no API token (run punchcard setup, or set PUNCHCARD_TOKEN)
  run `punchcard sync` later; never stop again
[exit 7]
```

Exit 7 is a success with debt: the stop happened and is on disk, only the send to the tracker is owed. Run `sync`. Never run `stop` again for it, which would stop a different timer. Without a tracker nothing is owed, so `stop` never exits 7. A stop that did not happen at all (nothing open, too short, busy) exits with `ok: false` and changes nothing.

With Jira, `--no-sync` records the worklog as pending and exits 0:

```
$ punchcard stop DEMO-2 --duration 2h --no-sync
■ DEMO-2 stopped: 2h, worklog pending
```

`--dry-run`:

```
$ punchcard stop --dry-run --duration 25m
payroll: would log 25m, started 2026-10-09 16:55

$ punchcard stop --dry-run --duration 25m --json
{"v":1,"ok":true,"outcome":"ok","command":"stop","data":{"preview":{"key":"payroll","entryId":"0CBDA564-2012-4E5D-94CA-EBE4BA0B4AB5","worklog":{"state":"local","startedAt":"2026-10-09T16:55:36.588-0500","timeSpentSeconds":1500},"recovered":false}},"events":[],"error":null}
```

With Jira the line reads `would send` and the preview's `state` is `pending`.

**A recovered entry** needs a duration you choose. On a terminal the prompt says why; the capped time is offered as the default only when there is at least a minute of it:

```
$ punchcard stop
DEMO-7 was recovered: its time stops at the last presence. Correct the duration if needed.
DEMO-7 Duration: 50m
Comment (optional): 
■ DEMO-7 stopped: 50m, worklog pending
```

Not on a terminal and without `--duration`, it is exit 6 with the preview and the hint `rerun with --duration, e.g. --duration 1h30m`.

**An agent's stop** (origin `agent`) without `--yes` is always a proposal, exit 6:

```
$ punchcard stop --duration 25m --json
{"v":1,"ok":false,"outcome":"needs_confirmation","command":"stop","data":{"preview":{"key":"payroll","entryId":"0CBDA564-2012-4E5D-94CA-EBE4BA0B4AB5","worklog":{"state":"local","startedAt":"2026-10-09T16:55:36.588-0500","timeSpentSeconds":1500},"recovered":false}},"events":[],"error":{"code":"needs_confirmation","message":"an agent's stop is a proposal: payroll: would log 25m, started 2026-10-09 16:55","hint":"show the user this worklog; only `punchcard stop --yes` sends it"}}
[exit 6]
```

The stop is pinned to the entry it previewed. If that entry was stopped by someone else in between, the stop is refused with exit 4 rather than stopping another.

### `status`

Without a tracker:

```
$ punchcard status
▶ ABC-123 running  0:00:00  by hand
⏸ payroll paused  0:00:00
```

With Jira as the tracker (the second run had no token):

```
$ punchcard status
▶ DEMO-7 running  0:00:00  by hand  Rotate staging certs
⏸ DEMO-3 paused  0:00:00
Jira token: PUNCHCARD_TOKEN

$ punchcard status
Nothing running.
⏸ DEMO-2 paused  0:00:00
1 unsent worklog(s), 0:25:00
Not connected to Jira: no API token (run punchcard setup, or set PUNCHCARD_TOKEN)
```

The lines, in order: the running timer or `Nothing running.`; each paused timer; the unsent count and total when any. The rest is Jira only: `! Jira rejected the token at 10:20; run punchcard setup` after a 401 until a call succeeds; then the Jira line: `Jira token: Keychain`, `file` or `PUNCHCARD_TOKEN`, or `Not connected to Jira: <why>`, or `! Not connected to Jira: macOS Keychain: <error>`. A stray token file next to a working Keychain item gets its own warning line. Last, when anything is counted today, the day's total over every ticket (removed ones included), as the TUI's Tickets title reads it: `Today: 3h 10m` (a stale timer counts capped). `status --short` never shows it.

A stale timer (running, no presence for over 2 hours) is shown capped, as the next change will record it:

```
$ punchcard status
! DEMO-1 stale  0:10:00 (capped; 3:10:00 uncapped)
  last seen 07:31 (3h00m ago); the next action will recover it
```

A recovered entry carries `(recovered: check the duration at stop)` until it is stopped.

With Jira, `status` reads the token to say where it came from (about 30 ms on a Mac, through the Keychain). It never calls the tracker and never writes. `status --short` skips the token read; without a tracker there is no token to read.

#### `status --cwd DIR`

Also reports DIR's key, the one the Claude Code plugin uses for the session. The human output only gains the notice below; `--json` adds `here`, `{key, source}` with `source` `file` or `branch`, or `null` when neither names a key:

```
$ punchcard status --cwd ~/src/app --json
{"v":1,"ok":true,"outcome":"ok","command":"status","data":{"running":null,"paused":[],"unsent":{"count":0,"seconds":0},"todaySeconds":0,"authFailedAt":null,"tracker":"none","here":{"key":"payroll","source":"file"}},"events":[],"error":null}
```

### `.punchcard`: a directory's key

A directory's key comes from, in order:

1. **A `.punchcard` file**, in the directory or the nearest ancestor that has one, up to and including the git repository's root (the filesystem root outside a repository). Its first line that is not blank and does not start with `#`, trimmed, is the key:

   ```
   # what this checkout bills
   payroll
   ```

2. **The git branch**: the first `ABC-123`-shaped token in its name (`feature/DEMO-2-fix` names `DEMO-2`). A lowercase or mixed-case token (`feature/demo-2-fix`) counts only once its project is on the list, spelled as the tracked ticket's project is (`DEMO-2` with `DEMO-1` tracked); until then it names nothing, so `release-2026` is never a ticket by accident.

A `.punchcard` whose line is not a valid key under the current tracker (`pay roll` anywhere, or `payroll` with Jira) is skipped with a notice on stderr, repeated in the envelope's `notices`, and the branch decides:

```
$ punchcard status --cwd ~/src/app --json
punchcard: /…/app/.punchcard: "pay roll" is not a key (like payroll); using the branch
{"v":1,"ok":true,"outcome":"ok","command":"status","data":{"running":null,"paused":[],"unsent":{"count":0,"seconds":0},"todaySeconds":0,"authFailedAt":null,"tracker":"none","here":{"key":"DEMO-2","source":"branch"}},"events":[],"error":null,"notices":["/…/app/.punchcard: \"pay roll\" is not a key (like payroll); using the branch"]}
```

`status --cwd`, `touch --cwd` and the plugin's `/punch start` all use this rule. Commit the file to share it with everyone on the repository, or list it in `.gitignore` to keep it yours. It holds one key; a repository whose branches name tickets needs no file at all.

### `status --short`

For a shell prompt: one line, nothing when idle with nothing owed, always exit 0 (even when the state file is unreadable).

```
$ punchcard status --short
▶ DEMO-1 0:00
```

Forms: `▶ KEY h:mm`, `⏸ KEY h:mm` (the first paused timer), `! KEY stale`, each followed by ` · N unsent` when worklogs are owed. Idle with worklogs owed prints only `1 unsent`.

For zsh:

```sh
RPROMPT='$(punchcard status --short)'
```

### `ls`

```
$ punchcard ls
▶ DEMO-7  running  0:00:00    Rotate staging certs
⏸ DEMO-3  paused   0:00:00    (unverified)
  DEMO-2  -                   Fix flaky CI timeouts
  DEMO-1  -        1 unsent   Add the worklog POST
```

Columns: marker (`▶`, `⏸`, `!` for stale, blank), key, state (`running`, `paused`, `stale`, `-`, or `removed` with `--all`), the open entry's elapsed, the unsent count, the summary or `(unverified)`. Most recently active first. Empty list: `No tickets yet. Add one with `punchcard add KEY` or `punchcard start KEY`.`

`ls --all` lists removed tickets too, marked `removed`, so you can find one to restore:

```
$ punchcard ls --all
  DEMO-2   -                 Fix flaky CI timeouts
  payroll  removed           Monthly payroll run
```

### `add KEY...`

```
$ punchcard add DEMO-3 DEMO-4
+ DEMO-3 added
+ DEMO-4 added

$ punchcard add DEMO-3
DEMO-3 is already on the list

$ punchcard add payroll
+ payroll restored
```

A removed ticket's key **restores** it: back on the list with its `addedAt` and its history. `start KEY` and `resume KEY` restore it too (`+ payroll restored`, event `restored`), and so does focus when it moves the timer there. With Jira, a restored ticket that is still unverified is looked up again, as `add` looks up a new one.

With Jira, each new key is looked up as `start` does, and a 404 refuses the whole command with exit 5. Without a tracker nothing is looked up.

Without a tracker, `--summary TEXT` gives the ticket its summary, the way it should read in `ls` and the TUI (trimmed; a ticket already on the list keeps its own). It takes one KEY. With a tracker it is usage (exit 64): the summary is the tracker's.

```
$ punchcard add payroll --summary "Monthly payroll run"
+ payroll added
```

### `rm KEY`

```
$ punchcard rm payroll
- payroll removed (its 3 worklogs stay in the log)

$ punchcard rm standup
- standup removed
```

`rm` hides the ticket from the list. It deletes no worklog: every logged worklog (synced, or `local` with no tracker) stays in `punchcard log`, and the message counts them when there are any. `ls --all` still shows the ticket, and `add`, `start` or `resume` with its key restores it.

Refused while the ticket has a running or paused entry, or (with a tracker) a worklog not sent yet. A `local` worklog is logged, so it never blocks:

```
$ punchcard rm DEMO-2
punchcard: ticket has unlogged time: DEMO-2 has a running or paused entry or an unsent worklog
  stop and sync it first, or pass --force to drop the unsent time
[exit 4]

$ punchcard rm --force DEMO-2
- DEMO-2 removed
```

`--force` discards only that open or unsent time, which was never logged; the ticket's logged worklogs are kept as always.

A key not on the list (never tracked, or already removed) is exit 4: `ticket not tracked: NOPE-1`. Removing never touches the tracker.

### `sync [KEY]`

Without a tracker there is nothing to send, and `sync` says so with exit 3 (`data.remaining` lists any worklogs left pending from before the tracker was removed):

```
$ punchcard sync
punchcard: there is no tracker to send to
  worklogs are logged here; `punchcard setup` connects one
[exit 3]
```

With Jira, it sends every pending and failed worklog, one at a time, under a lock so two syncs never send the same one:

```
$ punchcard sync
✓ DEMO-2 12m sent (2026-10-09)
```

```
$ punchcard sync
Nothing to send.
```

One remaining means exit 7 with the reason; a 401 stops the run with exit 8 and leaves everything pending:

```
$ punchcard sync
✗ DEMO-2 12m failed: Jira answered 503: Service unavailable
punchcard: Jira answered 503: Service unavailable
  run `punchcard sync` again later
[exit 7]

$ punchcard sync
punchcard: Jira rejected the credentials; the worklogs stay pending
  check the email and API token with `punchcard setup`; a scoped token is refused too
[exit 8]

$ punchcard sync
punchcard: not connected to Jira: no API token (run punchcard setup, or set PUNCHCARD_TOKEN)
[exit 3]
```

A successful send also clears a recorded 401.

### `setup`

Its first step is the tracker, none or Jira; none goes straight to the Claude Code plugin steps. See [Getting started](getting-started.md). Not on a terminal, or with `--json`:

```
$ punchcard setup
punchcard: setup is interactive; run it in a terminal
  see punchcard --help
[exit 64]
```

### `log`

One row per worklog whose day (the local date its entry started, so a stretch past midnight stays on the day it began) is in the range, by day then start. The range is this week, Monday through today, by default and with `--week`; `--month` is the first of the month through today; `--from DATE` and `--to DATE` (`YYYY-MM-DD`, inclusive) may stand alone: `--from` runs through today (or is just that day, when it is still to come), and `--to` starts on the Monday of its own week, so `--to` with last Friday is last week through Friday. Two ranges, two formats, a bad date or `--from` after `--to` is usage (exit 64); anything else exits 0, an empty range included. `log` only reads: no tracker, no presence, no write.

```
$ punchcard log
## 2026-08-03 Mon · 2h 45m

| key | summary | time | comment |
|---|---|---|---|
| AA-1 | Fix the thing | 1:30 | Fixed the formatter |
| AA-2 | Other thing | 1:15 | Split \| the parser |

## 2026-08-04 Tue · 0h 30m · 1 pending

| key | summary | time | comment |
|---|---|---|---|
| AA-3 | Release notes | 0:30 | pending |

**Total: 3h 15m**

Paused: AA-1 0:45, not logged
```

The Markdown is the default, on a terminal or not; `--md` asks for it by name. `--csv` is RFC 4180 (CRLF, quoted where needed) with the header `date,started,key,summary,minutes,comment,state,remote_id,entry_id,recovered`, `started` as local `HH:MM`, for a spreadsheet or another tracker's import. Without a tracker every row's `state` is `local`:

```
$ punchcard log --csv
date,started,key,summary,minutes,comment,state,remote_id,entry_id,recovered
2026-10-09,16:58,payroll,,30,"a, ""b""",local,,B70EDF2B-A3B0-4E55-9205-DAC177D81D64,false
```

`--json` is the envelope (see [`data` by command](#data-by-command)). `--csv` or `--md` wins over `PUNCHCARD_JSON=1`.

### `version`

```
$ punchcard version
punchcard v0.0.0-20261009151055-725c73585087 (contract v1, go1.26.9 darwin/arm64)

$ punchcard version --json
{"v":1,"ok":true,"outcome":"ok","command":"version","data":{"version":"v0.0.0-20261009151055-725c73585087","contract":1,"schemaVersion":1,"go":"go1.26.9","platform":"darwin/arm64"},"events":[],"error":null}
```

`contract` is the `--json` envelope's version; `schemaVersion` is the state file's.

### `doctor`

Everything a bug report needs, in one paste. It only reads: it never changes a file (an unreadable `state.json` is reported in `status`'s words and left where it is), never calls the tracker, never reads the token itself (only says where it would come from, as `status` does), and always exits 0.

```
$ punchcard doctor
punchcard v0.2.0 (contract v1, go1.26.9 darwin/arm64)
binary      /opt/homebrew/bin/punchcard
data        ~/.local/share/punchcard  state.json ok (schema 1, 6 tickets)  heartbeat 08:12
config      ~/.config/punchcard/config.json  tracker jira  site <redacted>  email <redacted>
token       Keychain
timer       ▶ PUN-142 1:12 by hand
unsent      0
claude      2.1.291 at /opt/homebrew/bin/claude
plugin      punchcard@punchcard 0.2.0  ✓ matches binary
ask rule    present in ~/.claude/settings.json
terminal    TERM=xterm-ghostty  TERM_PROGRAM=ghostty  COLORTERM=truecolor  NO_COLOR unset  120x32
os          darwin 15.1 arm64
```

The Jira site, the email and the home directory (as `~`) are redacted by default; `--no-redact` shows them, for your own eyes. `token` is `Keychain`, `file`, `PUNCHCARD_TOKEN`, `none`, or `none (no tracker)`. `plugin` says `≠ X.Y.Z → run punchcard setup` when the plugin is another release, `not installed → run punchcard setup`, or `claude not found`; a build that is not a release is never compared. `ask rule` is `present`, `absent → punchcard setup adds it`, or the settings file `unparseable`. `data` adds `state is locked by another punchcard` while another punchcard holds the lock, and a heartbeat from another day carries its date.

### `touch [KEY] [--cwd DIR]` (hidden)

The plugin's presence call. With no key, presence only. With a key, or with `--cwd` pointing at a directory that names one (its [`.punchcard`](#punchcard-a-directorys-key), else its git branch), it is also focus: a running timer on another ticket moves to that key, whoever set it, but only if the key differs from the last focus, the key of the previous touch from a prompt or by hand that had one. Every such touch records it, even when it moves nothing. It never starts a timer when nothing runs (a paused timer counts as nothing running), never sets the moved timer by hand, and tries the lock once rather than waiting (busy is a silent exit 0). From an agent it does nothing.

```
$ punchcard touch --cwd ~/src/app --origin prompt
⏸ ABC-123 paused
+ payroll added
▶ payroll started

$ punchcard touch --cwd /path/to/checkout --origin prompt --json
{"v":1,"ok":true,"outcome":"ok","command":"touch","data":{"running":{"key":"DEMO-2","unverified":true,"entryId":"C8B037EC-7C7B-4507-9830-271E88466168","state":"running","byHand":false,"origin":"prompt","recovered":false,"elapsedSeconds":0},"paused":[{"key":"DEMO-1","unverified":true,"entryId":"0936890C-2F84-4B16-A20B-DE31A42D5802","state":"paused","byHand":false,"origin":"prompt","recovered":false,"elapsedSeconds":0}],"unsent":{"count":1,"seconds":1800},"todaySeconds":1800,"authFailedAt":null},"events":[{"kind":"paused","key":"DEMO-1"},{"kind":"added","key":"DEMO-2"},{"kind":"started","key":"DEMO-2"}],"error":null}
```

A prompt never starts a timer from idle, or resumes a paused one; it only moves one that is already running, whoever set it.

## Origin

Every change records who caused it. The CLI infers `agent` when `CLAUDECODE` or `AI_AGENT` is set and `hand` otherwise. The hidden `--origin` flag overrides it; the plugin always passes one.

| Origin | Sets the timer by hand | Counts as presence | Stop |
| --- | --- | --- | --- |
| `hand` | yes | yes | prompts on a terminal, else logs |
| `prompt` | no | yes | as `hand` |
| `agent` | no | no | exit 6 without `--yes`; `--yes` logs |

Inside Claude Code:

```
$ CLAUDECODE=1 punchcard start DEMO-2
punchcard: DEMO-1 was set by hand
  the user set it by hand; only they can move or pause it
[exit 4]
```

The reasons: [ADR 0004](../adr/0004-cli-contract-exit-codes-json-envelope-origin.md).

## Exit codes

| Code | `outcome` | Meaning | What a script should do |
| --- | --- | --- | --- |
| 0 | `ok` | Done, or a harmless no-op. Reads (`status`, `ls`, `log`) always exit 0. | Carry on. |
| 1 | `internal` | Unexpected error, failed write, no data directory. | Show `error.message`; nothing changed. |
| 3 | `not_configured` | Jira is the tracker, a command needed it, and there are no credentials (or the Keychain failed). Also `sync` with no tracker, which reports that there is none. | With Jira: run `punchcard setup`. Without a tracker: nothing to do, every worklog is logged. Timers are fine either way. |
| 4 | `no_timer` | Nothing running or paused to act on, an ambiguous target (`data.candidates` lists them), a ticket not on the list, an agent acting on a timer set by hand (`error.code: by_hand`), or `rm` on unlogged time. | Read `error.code`; name a key, or ask the user. |
| 5 | `ticket_not_found` | The tracker looked the key up and has no such ticket (Jira answered 404). Nothing changed. Never without a tracker. | Check the key. |
| 6 | `needs_confirmation` | The stop is a proposal. `data.preview` is the exact worklog. | Show the preview. An agent reruns with `--yes`; a recovered entry needs `--duration`. |
| 7 | `sync_pending` | **Partial success**, `ok: true`. The stop happened; its worklog is pending or failed. Also `sync` with worklogs remaining. Never without a tracker. | Run `sync` later. **Never `stop` again.** |
| 8 | `auth_failed` | Jira answered 401. Worklogs stay pending; the token is untouched. Jira only. | Run `punchcard setup`. |
| 64 | `usage` | Bad arguments, a bad key, a bad duration, under one minute, `setup` with no terminal, two ranges or two formats to `log`. | Read `error.hint`. |
| 65 | `state_unreadable` | A file written by a newer Punchcard. Nothing written. | Upgrade. |
| 75 | `busy` | Another Punchcard held the state lock for about 2 seconds. Nothing changed. | Retry once after a moment. |

Exit 2 is never used: a Claude Code hook that exits 2 blocks the user's action. With no tracker the codes you can see are 0, 1, 4, 6, 64, 65 and 75, plus 3 from `sync`.

## The `--json` envelope

One shape on stdout, success or failure. Notices and warnings go to stderr, so stdout always parses.

```json
{"v":1,"ok":true,"outcome":"ok","command":"status","data":{…},"events":[],"error":null}
```

| Field | Meaning |
| --- | --- |
| `v` | The contract version. Additive changes keep `1`. |
| `ok` | `true` on exit 0 and on exit 7. |
| `outcome` | The exit code's name, from the table above. |
| `command` | The command that ran (`""` for an unknown one). |
| `data` | Per command, below. `null` on most failures. |
| `events` | Side effects nobody asked for. |
| `error` | `null`, or `{code, message, hint?}`. Present on exit 7 too, beside `ok: true`. |
| `notices` | Only when there were any: the notices also printed on stderr, such as an invalid `.punchcard`. |

### `data` by command

| Command | `data` |
| --- | --- |
| `status` | `{running, paused, unsent, todaySeconds, authFailedAt, tracker, configured?, tokenSource?, strayTokenFile?, keychainError?, notConfigured?, here?}`. `running` is a timer or `null`; `paused` is a list of timers; `unsent` is `{count, seconds}`; `todaySeconds` is the day's total over every ticket, removed ones included, open entries counted up to now and a stale timer counted capped; `authFailedAt` is a timestamp or `null`; `tracker` is `none` or `jira`. `configured`, `tokenSource` (`env`, `keychain` or `file`), `strayTokenFile`, `keychainError` and `notConfigured` appear only with Jira. `here` appears only with `--cwd`: `{key, source}` with `source` `file` or `branch`, or `null`. |
| `status --short` | The same without the Jira fields. |
| `start`, `pause`, `resume`, `touch` | `{running, paused, unsent, todaySeconds, authFailedAt}` after the change. |
| `ls` | `{tickets: [{key, summary?, unverified, addedAt, removedAt?, state?, elapsedSeconds, todaySeconds, unsynced}]}`. `state` is `running`, `paused` or `stale`, absent when idle. `removedAt` appears only on a removed ticket, which only `ls --all` lists. |
| `add` | As `ls`, for the keys given. |
| `rm` | `{key, keptWorklogs}`: `keptWorklogs` counts the logged worklogs that stay in the log. |
| `stop` | `{key, entryId, worklog}`. |
| `stop --dry-run`, exit 6 | `{preview: {key, entryId, worklog, recovered}}`. |
| `sync` | `{sent: [...], remaining: [...]}`, each `{key, entryId, worklog}`. With no tracker `sent` is empty and `remaining` lists any old pending worklogs. |
| `log` | `{rows: [{date, started, key, summary, minutes, comment, state, remoteId, entryId, recovered}], open, range: {from, to}}`. `date` is `YYYY-MM-DD`, `started` a timestamp; absent values are `""`. `open` is the running entry, else the most recently paused one, as a timer, or `null`. |
| `version` | `{version, contract, schemaVersion, go, platform}`. |
| `doctor` | `{punchcard, binary, data, config, token, timer, unsent, claude, plugin, askRule, terminal, os}`, the human output's fields, redacted the same way. `punchcard` is `version`'s data. `data` is `{dir, state, problem?, schemaVersion?, tickets, heartbeat, locked}` with `state` `ok`, `none`, `unreadable`, `newer` or `error`. `config` is `{path, exists, tracker, site?, email?, problem?}`. `token` is `env`, `keychain`, `file` or `none`. `timer` is a timer or `null`. `claude` is `{found, version?, path?}`; `plugin` is `{installed, version?, matches, skewed, problem?}`; `askRule` is `{path?, state}` with `state` `present`, `missing`, `unparseable`, `unreadable` or `unknown`. `terminal` is `{TERM, TERM_PROGRAM, COLORTERM, NO_COLOR, width?, height?}`; `os` is `{goos, version?, arch}`. |
| exit 4, ambiguous | `{candidates: ["KEY", ...]}`. |

A timer: `{key, summary?, unverified, entryId, state, byHand, origin, recovered, elapsedSeconds, uncappedSeconds?, lastSeen?}`. `state` is `running`, `paused` or `stale`. For a stale timer `elapsedSeconds` is the capped figure, `uncappedSeconds` the wall-clock one, and `lastSeen` the cap.

A worklog: `{state, startedAt, timeSpentSeconds, comment?, worklogId?, syncedAt?, message?, refusedStatus?, failedAt?, recovered?}`. `recovered` is `true` when the entry was recovered when it stopped. `state` is `pending`, `synced` or `failed`, or `local` when there is no tracker (final, never owed or sent); `refusedStatus` is the HTTP status when the tracker answered with a refusal.

Timestamps are `2026-10-09T10:20:03.129-0500`: milliseconds, numeric offset, no colon.

### Events

| `kind` | When |
| --- | --- |
| `added` | A start or a focus put an untracked ticket on the list. |
| `restored` | A start, resume, add or focus put a removed ticket back on the list, its history intact. |
| `paused` | Starting this ticket's rival paused it, or a `pause --at` closed it. `at` is the time, when it was not now; `message` says when a stub under a minute was dropped. |
| `started` | Focus moved the timer to this ticket, or a `start --at` or `resume --at` opened it; `at` is the time. |
| `byHand` | A start by hand made the running timer one an agent can't move. |
| `recovered` | A stale timer was capped; `at` is where. |
| `synced` | The tracker took the worklog. |
| `syncFailed` | It did not; `message` says why. |

### Error codes

`error.code` is more specific than `outcome`: `usage`, `internal`, `not_configured`, `no_timer`, `by_hand`, `ticket_not_found`, `needs_confirmation`, `busy`, `state_unreadable`, `auth_failed`, and on exit 7 the reason the worklog is owed: `not_configured`, `auth_failed`, `unreachable`, `sync_failed` (the tracker answered with an error), `sync_busy` (another Punchcard is sending), `sync_pending` (anything else).

### Examples

```
$ punchcard start DEMO-5 --json
{"v":1,"ok":false,"outcome":"busy","command":"start","data":null,"events":[],"error":{"code":"busy","message":"state is locked by another punchcard","hint":"try again"}}
[exit 75]
```

```
$ punchcard frobnicate --json
{"v":1,"ok":false,"outcome":"usage","command":"","data":null,"events":[],"error":{"code":"usage","message":"unknown command \"frobnicate\" for \"punchcard\"","hint":"see punchcard --help"}}
[exit 64]
```

In a shell script:

```sh
if out=$(punchcard stop --yes --json); then
  echo "$out" | jq -r '.data.worklog.state'   # local, or synced with a tracker
else
  case $? in
    7) punchcard sync --json ;;               # stopped; send later
    6) echo "$out" | jq .data.preview ;;      # needs a duration or --yes
    *) echo "$out" | jq -r .error.message ;;
  esac
fi
```

## Human output rules

Results go to stdout. Errors, notices and warnings go to stderr, prefixed `punchcard:`, with a hint on the next line when the message does not already give it. With a tracker, while any worklog is unsent, every change in human mode warns on stderr:

```
punchcard: 1 unsent worklog(s), 0:25:00; run `punchcard sync`
```

`--json` never prints that warning; `data.unsent` carries it.
