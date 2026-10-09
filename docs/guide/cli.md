# The CLI

Every action is a subcommand of `punchcard`. The CLI is stateless: each call locks the state file, reads it fresh, applies one change, writes, and exits. No process stays behind. Bare `punchcard` opens the [TUI](tui.md) on a terminal and prints `status` otherwise.

All output below was produced by running the binary in throwaway directories, most of it with no Jira configured. Where a fake Jira was answering, the example says so.

## Commands

| Command | Does | Talks to Jira |
| --- | --- | --- |
| `start [KEY]` | Start the timer on KEY, pausing whatever ran. No KEY: pick from recents on a terminal. | Summary fetch for a ticket new to the list, or still unverified |
| `pause` | Pause the running timer. The entry stays open. | no |
| `resume [KEY]` | Resume the most recently paused entry. `resume KEY` is `start KEY`. | as `start` |
| `stop [KEY]` | Stop an entry and send its worklog. No KEY: the running entry, else the single paused one. | yes, one POST |
| `status` | The running ticket, paused tickets, unsent worklogs, the Jira connection. | no (reads the token) |
| `status --short` | One line for a shell prompt. | no |
| `ls` | The list, most recently active first. | no |
| `add KEY...` | Put tickets on the list without starting anything. | as `start` |
| `rm KEY` | Take a ticket off the list. | no |
| `sync [KEY]` | Send every pending and failed worklog, or only KEY's. | yes |
| `setup` | The credentials wizard. Needs a terminal. | Test Connection |
| `version` | Version, contract version, platform. | no |
| `help [command]` | Help text. `--help` on any command does the same. | no |

Hidden, for the Claude Code plugin:

| Command | Does |
| --- | --- |
| `touch [KEY] [--cwd DIR]` | Record presence, and focus on KEY or on the key in DIR's git branch. Never waits for the lock. |
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
| `PUNCHCARD_TOKEN` | The Jira API token. Overrides the Keychain and the token file while set. `setup` never writes it. |
| `CLAUDECODE`, `AI_AGENT` | Either set: the origin is `agent`. Claude Code sets both in its shell. |
| `NO_COLOR` | Set: no colour, even on a terminal. |
| `XDG_CONFIG_HOME`, `XDG_DATA_HOME` | Where config and data live. See [Files](files-and-troubleshooting.md). |

Colour is used only on a terminal with `NO_COLOR` unset: green `▶`, yellow `⏸`, red `!` and `punchcard:` on errors.

## The commands, one by one

### `start [KEY]`

```
$ punchcard start DEMO-1
+ DEMO-1 added (not verified with Jira yet)
▶ DEMO-1 running  0:00:00  held
```

With Jira configured, a ticket new to the list gets its summary (a 2 second budget), and a key Jira does not know is refused with nothing changed:

```
$ punchcard start DEMO-1
+ DEMO-1 added
▶ DEMO-1 running  0:00:00  held  Add the worklog POST

$ punchcard start TYPO-999
punchcard: Jira has no ticket TYPO-999 that this account can see
  check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed.
[exit 5]
```

Starting a second ticket pauses the first and reports it as an event:

```
$ punchcard start DEMO-2
⏸ DEMO-1 paused
+ DEMO-2 added (not verified with Jira yet)
▶ DEMO-2 running  0:00:00  held
```

Starting the ticket that already runs changes nothing, except that by hand it holds an un-held timer:

```
$ punchcard start DEMO-1
DEMO-1 is now held: only you can move it
▶ DEMO-1 running  0:00:00  held
```

With no key, on a terminal, a numbered picker of up to 9 recents. A number or a key typed in full starts it; an empty line cancels with exit 0:

```
$ punchcard start
  1) DEMO-1  
  2) DEMO-2  
  3) DEMO-7  
Start which? (number or KEY, empty cancels) 2
▶ DEMO-2 running  0:00:00  held
```

Not on a terminal, or with `--json`, a missing key is a usage error (exit 64).

A key must look like `ABC-123` (capital letters and digits, a dash, digits). Anything else is exit 64 with `"nope" is not a ticket key`.

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
▶ DEMO-2 running  0:00:00  held

$ punchcard resume DEMO-1
⏸ DEMO-2 paused
▶ DEMO-1 running  0:00:00  held
```

Nothing paused is exit 4: `no timer: nothing is paused`.

### `stop [KEY]`

| Flag | Does |
| --- | --- |
| `--duration D` | Log D instead of the measured time: `1h30m`, `1h`, `90m` or bare minutes, 1m to 24h. `1:30` and `30s` are refused with exit 64. |
| `-m`, `--message TEXT` | The worklog comment. |
| `--no-sync` | Record the worklog as pending without sending it. |
| `--yes` | Send without prompting. An agent's stop needs it. |
| `--dry-run` | Show the exact worklog a stop would send, and change nothing. Always exit 0. |

With no KEY, stop targets the running entry, else the single paused one. Several paused and nothing running is exit 4 naming them.

**On a terminal** (no `--json`, no `--yes`, no `--dry-run`, not an agent), stop prompts. The duration defaults to the measured time truncated to the minute; Enter accepts it. A bad answer is explained and asked again. The comment is optional:

```
$ punchcard stop
DEMO-7 Duration [5m]: 1:30
  "1:30": use 1h30m, 1h, 90m or minutes
DEMO-7 Duration [5m]: 25m
Comment (optional): 
■ DEMO-7 stopped: 25m, worklog pending
```

Under a minute there is no default and an empty Enter asks again; type a duration. Ctrl-d at the duration cancels: `punchcard: not stopped; the timer is untouched`, exit 0. `--duration` and `-m` prefill the prompts.

**Not on a terminal, or with `--json` or `--yes`**, stop uses the measured time with no prompt. Under one minute it refuses:

```
$ punchcard stop
punchcard: a worklog under one minute is never sent
  pass --duration
[exit 64]
```

The worklog goes to Jira at once. What you see depends on how that went:

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
$ punchcard stop -m "Wired the POST"
■ DEMO-1 stopped: 25m, worklog pending
punchcard: the worklog is saved but not sent: not connected to Jira: no /…/punchcard/config.json (run punchcard setup)
  run `punchcard sync` later; never stop again
[exit 7]
```

Exit 7 is a success with debt: the stop happened and is on disk, only the send is owed. Run `sync`. Never run `stop` again for it, which would stop a different timer. A stop that did not happen at all (nothing open, too short, busy) exits with `ok: false` and changes nothing.

`--no-sync` records the worklog as pending and exits 0:

```
$ punchcard stop DEMO-2 --duration 2h --no-sync
■ DEMO-2 stopped: 2h, worklog pending
```

`--dry-run`:

```
$ punchcard stop --dry-run
DEMO-1: would send 47m, started 2026-10-09 09:33

$ punchcard stop --dry-run --json
{"v":1,"ok":true,"outcome":"ok","command":"stop","data":{"preview":{"key":"DEMO-1","entryId":"2DE72121-0788-46DB-9D06-98CCCC766F06","worklog":{"state":"pending","startedAt":"2026-10-09T10:20:03.129-0500","timeSpentSeconds":1500},"recovered":false}},"events":[],"error":null}
```

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
$ punchcard stop --json
{"v":1,"ok":false,"outcome":"needs_confirmation","command":"stop","data":{"preview":{"key":"DEMO-1","entryId":"614BAD78-45E0-4798-BDFA-67C436787510","worklog":{"state":"pending","startedAt":"2026-10-09T09:50:27.165-0500","timeSpentSeconds":1800},"recovered":false}},"events":[],"error":{"code":"needs_confirmation","message":"an agent's stop is a proposal: DEMO-1: would send 30m, started 2026-10-09 09:50","hint":"show the user this worklog; only `punchcard stop --yes` sends it"}}
[exit 6]
```

The stop is pinned to the entry it previewed. If that entry was stopped by someone else in between, the stop is refused with exit 4 rather than stopping another.

### `status`

```
$ punchcard status
▶ DEMO-7 running  0:00:00  held  Rotate staging certs
⏸ DEMO-3 paused  0:00:00
Jira token: PUNCHCARD_TOKEN

$ punchcard status
Nothing running.
⏸ DEMO-2 paused  0:00:00
1 unsent worklog(s), 0:25:00
Not connected to Jira: no /…/punchcard/config.json (run punchcard setup)
```

The lines, in order: the running timer or `Nothing running.`; each paused timer; the unsent count and total when any; `! Jira rejected the token at 10:20; run punchcard setup` after a 401 until a call succeeds; then the Jira line: `Jira token: Keychain`, `file` or `PUNCHCARD_TOKEN`, or `Not connected to Jira: <why>`, or `! Not connected to Jira: macOS Keychain: <error>`. A stray token file next to a working Keychain item gets its own warning line.

A stale timer (running, no presence for over 2 hours) is shown capped, as the next change will record it:

```
$ punchcard status
! DEMO-1 stale  0:10:00 (capped; 3:10:00 uncapped)
  last seen 07:31 (3h00m ago); the next action will recover it
```

A recovered entry carries `(recovered: check the duration at stop)` until it is stopped.

`status` reads the token to say where it came from (about 30 ms on a Mac, through the Keychain). It never calls Jira and never writes. `status --short` skips the token read.

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

Columns: marker (`▶`, `⏸`, `!` for stale, blank), key, state (`running`, `paused`, `stale`, `-`), the open entry's elapsed, the unsent count, the summary or `(unverified)`. Most recently active first. Empty list: `No tickets yet. Add one with `punchcard add KEY` or `punchcard start KEY`.`

### `add KEY...`

```
$ punchcard add DEMO-3 DEMO-4
+ DEMO-3 added
+ DEMO-4 added

$ punchcard add DEMO-3
DEMO-3 is already on the list
```

Each new key is looked up in Jira as `start` does; a 404 refuses the whole command with exit 5.

### `rm KEY`

Refused while the ticket has a running or paused entry or an unsent worklog:

```
$ punchcard rm DEMO-2
punchcard: ticket has unlogged time: DEMO-2 has a running or paused entry or an unsent worklog
  stop and sync it first, or pass --force to drop the unsent time
[exit 4]

$ punchcard rm --force DEMO-2
- DEMO-2 removed
```

A key not on the list is exit 4: `ticket not tracked: NOPE-1`. Removing never touches Jira.

### `sync [KEY]`

Sends every pending and failed worklog, one at a time, under a lock so two syncs never send the same one:

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

See [Getting started](getting-started.md#2-connect-it-to-jira-with-punchcard-setup). Not on a terminal, or with `--json`:

```
$ punchcard setup
punchcard: setup is interactive; run it in a terminal
  see punchcard --help
[exit 64]
```

### `version`

```
$ punchcard version
punchcard v0.0.0-20261009151055-725c73585087 (contract v1, go1.26.9 darwin/arm64)

$ punchcard version --json
{"v":1,"ok":true,"outcome":"ok","command":"version","data":{"version":"v0.0.0-20261009151055-725c73585087","contract":1,"schemaVersion":1,"go":"go1.26.9","platform":"darwin/arm64"},"events":[],"error":null}
```

`contract` is the `--json` envelope's version; `schemaVersion` is the state file's.

### `touch [KEY] [--cwd DIR]` (hidden)

The plugin's presence call. With no key, presence only. With a key, or with `--cwd` pointing at a git checkout whose branch names a key (`feature/DEMO-2-fix` names `DEMO-2`), it is also focus: an un-held running timer on another ticket moves to that key. It never starts a timer when nothing runs, never moves a held timer, and tries the lock once rather than waiting (busy is a silent exit 0). From an agent it does nothing.

```
$ punchcard touch --cwd /path/to/checkout --origin prompt --json
{"v":1,"ok":true,"outcome":"ok","command":"touch","data":{"running":{"key":"DEMO-2","unverified":true,"entryId":"C8B037EC-7C7B-4507-9830-271E88466168","state":"running","held":false,"origin":"prompt","recovered":false,"elapsedSeconds":0},"paused":[{"key":"DEMO-1","unverified":true,"entryId":"0936890C-2F84-4B16-A20B-DE31A42D5802","state":"paused","held":false,"origin":"prompt","recovered":false,"elapsedSeconds":0}],"unsent":{"count":1,"seconds":1800},"authFailedAt":null},"events":[{"kind":"paused","key":"DEMO-1"},{"kind":"added","key":"DEMO-2"},{"kind":"started","key":"DEMO-2"}],"error":null}
```

Today a prompt never starts a timer from idle; it only moves one that is already running. That is how the code behaves and is documented as such, not as a decision.

## Origin

Every change records who caused it. The CLI infers `agent` when `CLAUDECODE` or `AI_AGENT` is set and `hand` otherwise. The hidden `--origin` flag overrides it; the plugin always passes one.

| Origin | Holds the timer | Counts as presence | Stop |
| --- | --- | --- | --- |
| `hand` | yes | yes | prompts on a terminal, else sends |
| `prompt` | no | yes | as `hand` |
| `agent` | no | no | exit 6 without `--yes`; `--yes` sends |

Inside Claude Code:

```
$ CLAUDECODE=1 punchcard start DEMO-2
punchcard: timer is held: DEMO-1 was set by hand
  the user started it by hand; only they can move or pause it
[exit 4]
```

The reasons: [ADR 0004](../adr/0004-cli-contract-exit-codes-json-envelope-origin.md).

## Exit codes

| Code | `outcome` | Meaning | What a script should do |
| --- | --- | --- | --- |
| 0 | `ok` | Done, or a harmless no-op. Reads always exit 0. | Carry on. |
| 1 | `internal` | Unexpected error, failed write, no data directory. | Show `error.message`; nothing changed. |
| 3 | `not_configured` | A Jira call was needed and there are no credentials (or the Keychain failed). | Run `punchcard setup`. Timers are fine. |
| 4 | `no_timer` | Nothing running or paused to act on, an ambiguous target (`data.candidates` lists them), a ticket not on the list, a held timer (`error.code: held`), or `rm` on unlogged time. | Read `error.code`; name a key, or ask the user. |
| 5 | `ticket_not_found` | Jira answered 404. Nothing changed. | Check the key. |
| 6 | `needs_confirmation` | The stop is a proposal. `data.preview` is the exact worklog. | Show the preview. An agent reruns with `--yes`; a recovered entry needs `--duration`. |
| 7 | `sync_pending` | **Partial success**, `ok: true`. The stop happened; its worklog is pending or failed. Also `sync` with worklogs remaining. | Run `sync` later. **Never `stop` again.** |
| 8 | `auth_failed` | Jira answered 401. Worklogs stay pending; the token is untouched. | Run `punchcard setup`. |
| 64 | `usage` | Bad arguments, a bad key, a bad duration, under one minute, `setup` with no terminal. | Read `error.hint`. |
| 65 | `state_unreadable` | A file written by a newer Punchcard. Nothing written. | Upgrade. |
| 75 | `busy` | Another Punchcard held the state lock for about 2 seconds. Nothing changed. | Retry once after a moment. |

Exit 2 is never used: a Claude Code hook that exits 2 blocks the user's action.

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

### `data` by command

| Command | `data` |
| --- | --- |
| `status` | `{running, paused, unsent, authFailedAt, configured, tokenSource?, strayTokenFile?, keychainError?, notConfigured?}`. `running` is a timer or `null`; `paused` is a list of timers; `unsent` is `{count, seconds}`; `authFailedAt` is a timestamp or `null`; `tokenSource` is `env`, `keychain` or `file`. |
| `status --short` | The same without the Jira fields. |
| `start`, `pause`, `resume`, `touch` | `{running, paused, unsent, authFailedAt}` after the change. |
| `ls` | `{tickets: [{key, summary?, unverified, addedAt, state?, elapsedSeconds, todaySeconds, unsynced}]}`. `state` is `running`, `paused` or `stale`, absent when idle. |
| `add` | As `ls`, for the keys given. |
| `rm` | `{key}`. |
| `stop` | `{key, entryId, worklog}`. |
| `stop --dry-run`, exit 6 | `{preview: {key, entryId, worklog, recovered}}`. |
| `sync` | `{sent: [...], remaining: [...]}`, each `{key, entryId, worklog}`. |
| `version` | `{version, contract, schemaVersion, go, platform}`. |
| exit 4, ambiguous | `{candidates: ["KEY", ...]}`. |

A timer: `{key, summary?, unverified, entryId, state, held, origin, recovered, elapsedSeconds, uncappedSeconds?, lastSeen?}`. `state` is `running`, `paused` or `stale`. For a stale timer `elapsedSeconds` is the capped figure, `uncappedSeconds` the wall-clock one, and `lastSeen` the cap.

A worklog: `{state, startedAt, timeSpentSeconds, comment?, worklogId?, syncedAt?, message?, failedAt?}`. `state` is `pending`, `synced` or `failed`.

Timestamps are `2026-10-09T10:20:03.129-0500`: milliseconds, numeric offset, no colon.

### Events

| `kind` | When |
| --- | --- |
| `added` | A start or a focus put an untracked ticket on the list. |
| `paused` | Starting this ticket's rival paused it. |
| `started` | Focus moved the timer to this ticket. |
| `held` | A start by hand held the running timer. |
| `recovered` | A stale timer was capped; `at` is where. |
| `synced` | Jira took the worklog. |
| `syncFailed` | It did not; `message` says why. |

### Error codes

`error.code` is more specific than `outcome`: `usage`, `internal`, `not_configured`, `no_timer`, `held`, `ticket_not_found`, `needs_confirmation`, `busy`, `state_unreadable`, `auth_failed`, and on exit 7 the reason the worklog is owed: `not_configured`, `auth_failed`, `unreachable`, `sync_failed` (Jira answered with an error), `sync_busy` (another Punchcard is sending), `sync_pending` (anything else).

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
  echo "$out" | jq -r '.data.worklog.state'   # synced
else
  case $? in
    7) punchcard sync --json ;;               # stopped; send later
    6) echo "$out" | jq .data.preview ;;      # needs a duration or --yes
    *) echo "$out" | jq -r .error.message ;;
  esac
fi
```

## Human output rules

Results go to stdout. Errors, notices and warnings go to stderr, prefixed `punchcard:`, with a hint on the next line when the message does not already give it. While any worklog is unsent, every change in human mode warns on stderr:

```
punchcard: 1 unsent worklog(s), 0:25:00; run `punchcard sync`
```

`--json` never prints that warning; `data.unsent` carries it.
