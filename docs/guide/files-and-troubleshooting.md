# Files and troubleshooting

## Where everything lives

| Path | Holds | Written by |
| --- | --- | --- |
| `~/.local/share/punchcard/state.json` | Your tickets, entries, worklogs, and (with Jira) whether Jira last rejected the token. | every change |
| `~/.local/share/punchcard/heartbeat.json` | `lastSeen`: the last moment of presence. | presence, at most every 30 s |
| `~/.local/share/punchcard/state.lock` | The lock every change takes. Empty. | every change |
| `~/.local/share/punchcard/sync.lock` | The lock held while sending worklogs. Empty. | `stop`, `sync`, the TUI's `r` |
| `~/.local/share/punchcard/state.json.corrupt-<timestamp>` | A backup of a state file that could not be read. | the change that found it |
| `~/.config/punchcard/config.json` | The tracker you chose, and for Jira your site and email. Nothing secret. Absent until you run `setup`, which means no tracker. | `setup` |
| `~/.config/punchcard/token` | Jira only: the API token, mode 0600. Only on Linux, or on macOS when the Keychain write failed. | `setup` |
| macOS Keychain item, service `punchcard`, account = your email | Jira only: the API token on macOS. | `setup`, through `/usr/bin/security` |
| `.punchcard` in a project directory | The key for that directory and everything below it, up to the repository root. See [`.punchcard`](cli.md#punchcard-a-directorys-key). | you |
| `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR/settings.json`) | The ask rule, under `permissions.ask`. | `setup`, if you said yes |

`XDG_CONFIG_HOME` replaces `~/.config` and `XDG_DATA_HOME` replaces `~/.local/share` when set, on macOS as well as Linux. With neither set and no `HOME`, there is no data directory and every command but `version` exits 1 saying so. Nothing is ever written relative to the working directory; a `.punchcard` is only read.

With Jira as the tracker, the token is looked up in this order: `PUNCHCARD_TOKEN`, the Keychain (macOS only), the token file. `punchcard status` says which one it found. Why the Keychain is reached through the `security` tool and not directly: [ADR 0006](../adr/0006-token-via-security-cli-xdg-paths.md).

## `config.json`

No file means no tracker: worklogs are logged on this machine and nothing is sent. `setup` always writes `tracker`. With none:

```json
{
  "schemaVersion": 1,
  "tracker": "none"
}
```

With Jira:

```json
{
  "schemaVersion": 1,
  "tracker": "jira",
  "siteUrl": "https://acme.atlassian.net",
  "email": "you@example.com"
}
```

A file with a site and an email but no `tracker` (one written before there was a choice) reads as Jira. Switching from Jira to none in `setup` keeps `siteUrl` and `email` in the file, and the token where it was, so choosing Jira again finds them.

You can edit it by hand. A file with a higher `schemaVersion` than the binary knows, or a `tracker` it does not know, is refused, never rewritten.

## `state.json`

This is a real file after starting a ticket, pausing it, starting it again and stopping it with a comment, with Jira chosen but not connected (so the worklog waits as `pending`; with no tracker it would be `local`):

```json
{
  "schemaVersion": 1,
  "trackedTickets": [
    {
      "key": "DEMO-1",
      "unverified": true,
      "addedAt": "2026-10-09T10:20:03.129-0500",
      "entries": [
        {
          "id": "2DE72121-0788-46DB-9D06-98CCCC766F06",
          "segments": [
            {
              "startedAt": "2026-10-09T10:20:03.129-0500",
              "endedAt": "2026-10-09T10:20:03.346-0500"
            },
            {
              "startedAt": "2026-10-09T09:55:03.533-0500",
              "endedAt": "2026-10-09T10:20:03.903-0500"
            }
          ],
          "held": true,
          "origin": "hand",
          "recovered": false,
          "worklog": {
            "state": "pending",
            "startedAt": "2026-10-09T10:20:03.129-0500",
            "timeSpentSeconds": 1500,
            "comment": "Wired the POST"
          }
        }
      ]
    }
  ]
}
```

How to read it:

- A **ticket** has a `key`, a `summary` once a tracker confirmed it (absent while `unverified`, and always absent without a tracker), when it was added, and its entries.
- An **entry** is one start-to-stop. Its `id` is the key a tracker's worklog is matched to, so a retry can never post twice. `segments` are its counted stretches; a segment with no `endedAt` is open, and the entry is **running**. A closed last segment with no `worklog` is **paused**. `held` and `origin` describe the entry's last change. `recovered` is set by recovery and cleared only by stop.
- A **worklog** appears when the entry is stopped. `state` is `pending`, `synced` (with `worklogId` and `syncedAt`), `failed` (with `message`, `failedAt`, and `refusedStatus` when the tracker answered with a refusal), or `local` when it was stopped with no tracker: final, and never sent. `timeSpentSeconds` is the time logged (and what the tracker is told), which may differ from the segments if you corrected it.
- `authFailedAt` at the top appears, with Jira, after a 401 and disappears when a call succeeds or you run `setup`.

Timestamps are local time with a numeric offset and milliseconds: `2026-10-09T10:20:03.129-0500`. Absent fields are omitted, not written as `null`.

`heartbeat.json`:

```json
{
  "schemaVersion": 1,
  "lastSeen": "2026-10-09T10:20:03.129-0500"
}
```

### Inspecting it

```sh
jq '.trackedTickets[] | {key, entries: (.entries | length)}' ~/.local/share/punchcard/state.json
jq '.. | .worklog? // empty | select(.state == "pending" or .state == "failed")' ~/.local/share/punchcard/state.json
```

### Editing it by hand

It is safe to edit while nothing is mid-change: every command locks, reads the file fresh, and writes a new file. An open TUI notices the replaced file within a second. Keep the JSON valid and the timestamps in the format above. Two edits that are useful for testing:

- **Make a stop possible on a timer you just started.** Move the open segment's `startedAt` back 25 minutes. The measured time then reads 25 minutes.
- **Make a timer stale.** Move `lastSeen` in `heartbeat.json` back 3 hours. `punchcard status` then shows `! KEY stale`, and the next change recovers it.

A mistake is not fatal: an unreadable file is moved aside, see below.

## Problems

### "is unreadable … moved it to … and started fresh"

The state file was not valid JSON. A read says so and changes nothing; the next change moves the file aside and starts with an empty list:

```
$ punchcard status
punchcard: /…/punchcard/state.json is unreadable (invalid character 't' looking for beginning of object key string); the next change will move it aside and start fresh
Nothing running.

$ punchcard start DEMO-1
punchcard: /…/punchcard/state.json was unreadable (…); moved it to /…/punchcard/state.json.corrupt-20261009T102037.495 and started fresh
+ DEMO-1 added (not verified with Jira yet)
▶ DEMO-1 running  0:00:00  held
```

The backup keeps your worklogs. To recover them, fix the backup's JSON and copy it back over `state.json` while nothing is running.

### "state is locked by another punchcard" (exit 75)

Another Punchcard held the lock for the whole 2 second wait. Nothing was changed. Try again:

```
$ punchcard start DEMO-5
punchcard: state is locked by another punchcard
  try again
[exit 75]
```

The lock is released by the kernel when its holder exits or dies, so a crash never leaves it stuck. If it stays busy, something is still running: `lsof ~/.local/share/punchcard/state.lock` names it. Reads (`status`, `ls`) take no lock and keep working.

### "written by a newer punchcard" (exit 65)

A file carries a `schemaVersion` this binary does not know. Nothing is written, so a downgrade cannot destroy your worklogs. Upgrade:

```
$ punchcard status
punchcard: /…/punchcard/state.json has schemaVersion 2, this build reads 1: written by a newer punchcard
  upgrade punchcard
[exit 65]
```

`status --short` prints nothing and exits 0 even then. `setup` refuses a newer `config.json` the same way.

### Keychain errors

Jira only: without a tracker no token is ever read. A Keychain that errors or times out (about 1 second) is reported as itself, never as an auth failure, and only when there is no token file to fall back on:

- `status`: `! Not connected to Jira: macOS Keychain: <what security said>`
- The TUI: the banner `Keychain error: <cause> · worklogs wait here, nothing is lost` and `Keychain error` in the title of the `[3] Today` pane.
- The band: `Keychain error: <cause>`.
- `stop`: exit 7 with `error.code: not_configured`; `sync`: exit 3.

Timers are unaffected. Run `punchcard setup` again to rewrite the item. If the Keychain itself is the problem, `setup` falls back to the token file and says so. To look at the item by hand:

```sh
security find-generic-password -s punchcard
```

This could not be produced on demand for this guide: it needs a failing Keychain.

### A token Jira rejected (401)

Jira only. Any 401 is recorded and shown until an authenticated call succeeds or you run `setup`. The token is never deleted or rewritten on a failure. Worklogs stay pending, not failed:

```
$ punchcard stop --json
{"v":1,"ok":true,"outcome":"sync_pending","command":"stop","data":{…},"events":[],"error":{"code":"auth_failed","message":"the worklog is saved but not sent: Jira rejected the credentials (a scoped API token is refused too)","hint":"run `punchcard sync` later; never stop again"}}
[exit 7]

$ punchcard sync
punchcard: Jira rejected the credentials; the worklogs stay pending
  check the email and API token with `punchcard setup`; a scoped token is refused too
[exit 8]
```

`status` then shows `! Jira rejected the token at 10:20; run punchcard setup`, the TUI banner says `✗ Jira rejected the token at 10:20 · 1 worklog waiting · S to fix`, and the band says `Jira auth failed`. Run `punchcard setup`; its fork defaults to Yes with the time of the rejection. The most common cause is a scoped API token: make a classic one. The next successful call clears the marker:

```
$ punchcard sync
✓ DEMO-3 9m sent (2026-10-09)
```

### "punchcard not found on PATH"

The band says this when Claude Code cannot run `punchcard`. Claude Code inherits the `PATH` of the shell it was launched from. Check from a terminal with `which punchcard`, and make sure that directory (`/opt/homebrew/bin` for Homebrew, `$(go env GOPATH)/bin` for `go install`) is on the `PATH` of your login shell, then restart Claude Code. The same applies to the ask rule and the skill: all three call the bare name.

### "Not connected to Jira"

Jira only: Jira is the chosen tracker but there is no token (or no site or email). Timers work; `stop` keeps the worklog pending with exit 7; `sync` exits 3. Run `punchcard setup`, which can also switch the tracker to none. If you set `PUNCHCARD_TOKEN` in some shells only, the token is found only there. With no tracker this message never appears.

### "there is no tracker to send to" (exit 3)

`sync` with no tracker. Nothing is wrong: every worklog stopped without a tracker is already logged (`local`). If you want worklogs sent somewhere, `punchcard setup` sets up Jira.

### A `.punchcard` that is ignored

`punchcard: /…/.punchcard: "pay roll" is not a key (like payroll); using the branch`. The file's first line that is not blank or a `#` comment is not a valid key for the current tracker: a slug without a tracker, an `ABC-123` key with Jira. The branch decides instead. Fix the line; nothing else is needed.

### A stray token file

If both a Keychain item and a token file exist, the Keychain wins and `status` says: `! a stray token file at /…/punchcard/token is ignored (the Keychain wins); delete it`. `setup` removes the file when it writes the Keychain. This could not be produced for this guide without the real Keychain.

### "a worklog under one minute is never logged"

Exit 64. Pass `--duration`, or on a terminal type one at the prompt. The TUI's stop dialog says `✗ under a minute: type a duration`.

### The TUI looks wrong

Colours come from the terminal's 16-colour theme; on a terminal that reports only 16 colours the greys fall back to bright black. Every state has a symbol, so `NO_COLOR=1 punchcard` still reads. The frame is designed for 80×24 and up; narrower than 44 columns the Tickets pane takes the whole width and dialogs cover the body.

## Resetting completely

Export what you want to keep first (`punchcard log --month --csv`, or a wider `--from`), because the data directory is the only copy of your worklogs; with Jira, also `punchcard sync` anything unsent. Then:

```sh
rm -rf ~/.config/punchcard                        # config and token file
rm -rf ~/.local/share/punchcard                   # timers and worklogs
security delete-generic-password -s punchcard     # macOS Keychain item, if you used Jira
claude plugin uninstall punchcard@punchcard       # the plugin, if installed
```

And remove `"Bash(punchcard stop *--yes*)"` from `permissions.ask` in `~/.claude/settings.json` if you added the ask rule (a `settings.json.bak` from when it was added may also be there). Afterwards Punchcard starts with no tracker and an empty list; run `punchcard setup` to choose a tracker or install the plugin again.
