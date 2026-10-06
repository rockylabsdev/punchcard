# 04 - On-disk persistence contract

Kind: wayfinder
Status: resolved
Assignee: j-dsmith
Type: grilling
Blocked by: 02

## Question

Graduated from the map's fog once [02 - Walking-skeleton build order](02-walking-skeleton-build-order.md) fixed what data exists. Slice 5 of the build order (JSON persistence + `lastSeen` heartbeat + restart recovery) sits inside the range the spec covers in detail, so the contract can no longer be left vague.

Location is already fixed by the App Sandbox decision in ticket 01: `~/Library/Containers/com.jdsmith.Punchcard/Data/Library/Application Support/`. What remains:

- **Exact JSON schema.** The shape of the persisted `TrackedTicket` list and its `TimeEntry` history — field names, date encoding (ISO-8601 vs epoch), what the `Worklog` sync state looks like at rest (pending/synced/failed + the idempotency key).
- **Schema-version field.** Present from v1 or added later; how an unrecognised version is handled (refuse to launch, back up and reset, best-effort decode).
- **Write strategy.** Write-on-every-change vs debounced; atomic replace vs in-place; what happens if the write fails.
- **`lastSeen` heartbeat.** Cadence, where it lives in the file, and its cost given the file is rewritten to update it.
- **Concrete restart-recovery flow.** The settled rule is "cap the running timer at `lastSeen`, move it to paused, flag it" — pin down what "flag it" means to the user (a badge? a one-time notice? an editable duration on the row?), and the behaviour when the file is missing, empty, or corrupt.

Consult `/grilling` and `/domain-modeling`; the glossary in root `CONTEXT.md` is the authority on `TrackedTicket` / `TimeEntry` / `Worklog`.

## Answer

Two files, not one. Every persisted state derives from data rather than a stored status field. No status enum, no order field, no pruning.

### Two files, because the heartbeat has a different write profile

`state.json` holds the domain data; `heartbeat.json` holds nothing but `lastSeen`. Folding the heartbeat into `state.json` would rewrite the entire tracked-ticket history every 30 seconds, putting the valuable file on the hot write path for the sake of one timestamp — and a crash during one of those writes would risk real data to save something that is worthless if lost. Split, the frequent write touches ~40 bytes and `state.json` is only rewritten when something actually happened.

Config lives in neither: ticket 03 put site URL and email in `UserDefaults` and the token in the Keychain, so these files carry **only** domain data — no secrets, no settings.

Both live at the sandbox path fixed in ticket 01: `~/Library/Containers/com.jdsmith.Punchcard/Data/Library/Application Support/`.

### `state.json`

```json
{
  "schemaVersion": 1,
  "trackedTickets": [
    {
      "key": "PUNCH-12",
      "summary": "Wire up the worklog POST",
      "addedAt": "2026-08-09T09:02:41.000-0500",
      "entries": [
        {
          "id": "1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88",
          "segments": [
            { "startedAt": "2026-08-09T09:02:41.000-0500",
              "endedAt":   "2026-08-09T09:48:10.000-0500" },
            { "startedAt": "2026-08-09T10:15:00.000-0500" }
          ],
          "recovered": false,
          "worklog": null
        }
      ]
    }
  ]
}
```

`worklog` is `null` until the entry is stopped, then one of three flat tagged shapes:

```json
{ "state": "pending", "startedAt": "…", "timeSpentSeconds": 1800, "comment": "Fixed the formatter" }
{ "state": "synced",  "startedAt": "…", "timeSpentSeconds": 1800, "comment": null,
  "worklogId": "110001", "syncedAt": "…" }
{ "state": "failed",  "startedAt": "…", "timeSpentSeconds": 1800, "comment": null,
  "message": "401 Unauthorized", "failedAt": "…" }
```

`heartbeat.json` is `{ "schemaVersion": 1, "lastSeen": "2026-08-09T12:31:14.000-0500" }`.

### Every state is derived, none is stored

- **running** — the final segment has no `endedAt`
- **paused** — the final segment is closed and `worklog` is `null`
- **stopped** — `worklog` is non-null
- **needs sync** — `worklog.state` is `pending` or `failed`
- **list order** — sorted by most recent activity, computed from the segments

A stored status field would be a second source of truth able to disagree with the segments. An explicit order field would imply drag-to-reorder, which is not in v1.

### Pause does not end a `TimeEntry`

The glossary says a `TimeEntry` is one start→stop period and that resuming continues the same work, which rules out one-entry-per-run-segment. So segments live **inside** the entry and elapsed is their sum. The invariant this protects is **one `TimeEntry` produces exactly one `Worklog`**: under a segment-per-entry model that becomes one-to-many, and Q4's idempotency guard would have to attach to a group of entries that no object represents. `CONTEXT.md` has been sharpened accordingly.

Entries are **nested under their ticket** rather than a flat top-level array keyed by ticket. An entry cannot exist without its ticket, and nesting makes an orphan unrepresentable rather than merely unlikely; the normalized form is a database habit that buys nothing for a file decoded whole into memory.

### `timeSpentSeconds` is stored, not derived

Duration is editable at stop and truncated to the minute, so the number sent to Jira is deliberately **not** the sum of the segments. Both persist: the segments are what Punchcard measured, `timeSpentSeconds` is what Jira was told. Recomputing from segments on a retry would resurrect the pre-edit value and silently discard the user's correction. `comment` likewise belongs to the `worklog`, not the entry — it is captured at stop and exists only to be sent.

### Identity and the idempotency guard

`TrackedTicket` is identified by its Jira **key**; a parallel UUID would be a second identity to keep in sync. (Keys change if a ticket moves project — a non-issue for a personal tool; you re-add it.) `TimeEntry` carries its own **UUID**, and that is the idempotency key.

Jira accepts no idempotency header, so the guard is entirely local and entirely on-disk: **refuse to POST when `worklog.state` is `synced`**. Storing Jira's returned `worklogId` (a string, per ticket 05) is what makes retry after an ambiguous timeout safe rather than duplicating.

### Encoding

ISO-8601 strings throughout, using **the same formatter built for Jira** in ticket 05 (`yyyy-MM-dd'T'HH:mm:ss.SSSZ`; `ISO8601DateFormatter` fails there). One date format defined once, used at both the disk and wire boundaries. `JSONEncoder`'s default epoch doubles were rejected: the file must stay hand-readable, because verifying restart recovery means quitting the app and reading it.

For the same reason the `worklog` enum gets a **hand-written `Codable`** producing the flat tagged shape above. Swift's synthesized encoding would emit `{"synced":{"_0":"110001"}}` and undo that readability on the single most important field in the file. Roughly fifteen lines, and a good first custom-`Codable` exercise.

A nil optional is **omitted**, not written as `null` — `JSONEncoder` skips nil optionals, so an open segment simply has no `endedAt` key. That absence is the one bit restart recovery keys off.

### Versioning

`schemaVersion: 1` is present from day one. Adding it later would mean writing migrations for files that predate versioning; it costs two bytes now and is unpayable later.

An **unrecognised (newer) version → refuse to load**, with a blocking notice and nothing written. It is only reachable by running an older build against a newer file, the fix is relaunching the newer build, and resetting would destroy pending worklogs that never reached Jira.

### Write strategy

**Write on every state change, atomic replace** (`Data.write(to:options:.atomic)` — temp file plus rename, so a crash mid-write can never leave a truncated `state.json`). State changes are human-paced — start, pause, stop, add a ticket — a handful per hour, so debouncing buys nothing and opens a window where a crash loses exactly the event slice 5 exists to survive. Writing on quit is never relied on: the app dying unexpectedly is the case being designed for.

**A failed write must not stop a running timer.** In-memory state remains the truth, a persistent banner makes the risk visible, and the next state change retries naturally. Crashing would convert a recoverable disk problem into lost work.

### Heartbeat

Every **30 seconds, only while a timer is running** — when nothing is running there is no running entry to cap, so a ticking write is pure noise. 30s bounds the worst-case over-count at 30 seconds, which truncate-to-minute usually erases entirely. Written atomically like the other file.

### Restart recovery

On launch, find the entry whose final segment is open, cap that segment's `endedAt` at `lastSeen`, and set `recovered: true`. The entry is now paused by derivation — no separate state change. If `lastSeen` predates the segment's `startedAt` (clock change), **clamp to `startedAt`** rather than persisting a negative duration.

"Flag it" is **a persistent marker on that row**, cleared when the user resumes or stops it — not an alert at launch. Relaunch may happen hours later, and a launch alert gets dismissed reflexively, after which the flag is gone forever. A marker that survives until the row is acted on cannot be missed, and needs no new UI surface. The duration is already editable at stop, which is the natural correction point; the marker's only job is ensuring the user notices before reaching it.

File-state handling on load:

- **missing or zero-byte** → first run, empty state, no error
- **undecodable** → rename to `state-corrupt-<timestamp>.json`, start fresh, show a non-blocking notice naming the backup path
- **unknown version** → refuse (above)

Corruption is treated differently from a version mismatch because relaunching cannot fix it: refusing would brick the app with no user-side remedy, while silently resetting would hide destroyed worklogs. Backing up preserves hand-recovery, which on a single-user tool is a real option.

### Retention

**Nothing is pruned.** There is no v1 reporting, so no feature depends on history, and a year of heavy use is a few hundred entries — tens of kilobytes. Pruning is code that silently eats data when its condition is subtly wrong, bought for no v1 benefit. Revisit if the file ever grows, which it will not.

### Prototype not needed

The contract is fully determined by the data ticket 02 fixed and the wire shapes ticket 05 captured; there is nothing to react to that reading the JSON above does not already answer.
