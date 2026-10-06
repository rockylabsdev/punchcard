# Punchcard — v1 Spec

Status: ready-for-agent

Derived from the wayfinder map at [`docs/wayfinder/map.md`](wayfinder/map.md) and its five resolved tickets. Domain vocabulary is authoritative in root [`CONTEXT.md`](../../CONTEXT.md) — `TrackedTicket`, `TimeEntry`, `Segment`, `Worklog`, `lastSeen`.

Resolution is **deliberately uneven**, per [02 - Walking-skeleton build order](wayfinder/issues/02-walking-skeleton-build-order.md): slices 0–5 are specified in enough detail to build from; slices 6–8 are specified as intent plus explicitly flagged open questions. That is a decision, not an omission.

## Problem Statement

I work against Jira tickets all day and I am expected to log my time against them. Neither half of that works today.

Jira's own time logging is a form buried several clicks inside a ticket, so logging time means stopping what I am doing, navigating to the ticket, and typing a duration. In practice I don't do it while working — I reconstruct it later from memory, at the end of the day or the end of the week. Those reconstructions are guesses, and they are systematically wrong: work I switched away from and back gets counted once, short interruptions vanish, and anything I forgot entirely never gets logged at all.

The generic timer apps that would measure this accurately don't know what a Jira ticket is. They give me a stopwatch and a text label, which means I still have to transcribe every result into Jira by hand — the same tedious step, just moved later, with a transcription error added.

What I actually need is a timer that is *already* attached to the ticket, that lives one click away at all times, and that puts the result into Jira itself so there is no second step to forget.

## Solution

**Punchcard** is a personal, single-user macOS menu-bar app that times work against a Jira ticket and syncs the elapsed time back to Jira as a worklog.

It has no Dock icon and no main window. It is an icon in the menu bar; clicking it opens a popover listing the Jira tickets I have added. Each row shows the ticket key, its real summary fetched from Jira, and its accumulated time. One click starts the timer on a row, and at most one row is ever running.

Pausing suspends accrual without producing anything — it is for interruptions, and resuming continues the same stretch of work. Stopping is the deliberate act that ends the work and produces a `Worklog`: I get a chance to correct the duration and optionally add a comment, and then it is posted to Jira. Time is truncated to the minute before it is sent, because minute-precision is what a timesheet means.

Elapsed time is computed from timestamps, never from a ticking counter, so closing the lid, sleeping, quitting, or changing the clock cannot silently corrupt it. If the app dies while a timer is running, the next launch caps that stretch at the last moment the app was known alive and flags the row for me to check, rather than either discarding the work or claiming credit for the hours the machine spent asleep.

The local file is the source of truth. Jira is a destination that may be temporarily unreachable — a failed sync leaves the worklog pending and retryable, and never blocks a running timer.

## User Stories

### Timing work

1. As a developer, I want a timer that is always one click away in my menu bar, so that starting it costs me nothing and I actually do it.
2. As a developer, I want the app to have no Dock icon and no main window, so that it does not take up space in my workflow or my ⌘-Tab order.
3. As a developer, I want to start timing a ticket with a single click on its row, so that starting is faster than the interruption that made me want to start.
4. As a developer, I want at most one ticket running at a time, so that my logged time cannot exceed my actual working time.
5. As a developer, I want starting a second ticket to be unambiguous about what happens to the first, so that I never accidentally lose or double-count a stretch of work.
6. As a developer, I want to pause a running timer, so that I can step away without ending the piece of work I am in the middle of.
7. As a developer, I want resuming after a pause to continue the same stretch of work rather than beginning a new one, so that an interrupted hour reaches Jira as one worklog rather than five.
8. As a developer, I want pause and stop to be visibly different actions, so that I never end work when I meant to step away for coffee.
9. As a developer, I want to see the accumulated time on each ticket at a glance, so that I can see where my day went without opening anything.
10. As a developer, I want the running row to be visually obvious, so that I can tell at a glance whether I am currently on the clock.
11. As a developer, I want elapsed time derived from timestamps rather than a ticking counter, so that sleep, quit, and clock changes cannot silently corrupt it.
12. As a developer, I want to close my laptop lid and reopen it later without my timer having drifted, so that I can trust the number.

### Managing tickets

13. As a developer, I want to add a Jira ticket to my list by typing its key, so that I can start timing without leaving the app.
14. As a developer, I want to see the ticket's real summary fetched from Jira, so that I can confirm I added the ticket I meant to.
15. As a developer, I want recently used tickets offered to me, so that adding the ticket I worked on yesterday takes one click instead of remembering its key.
16. As a developer, I want a ticket to stay in my list whether or not it currently has time against it, so that I can queue up today's work before starting any of it.
17. As a developer, I want my list ordered by most recent activity, so that what I am working on now is at the top without me managing the order.
18. As a developer, I want to remove a ticket from my list, so that finished work stops cluttering the popover.
19. As a developer, I want the ticket summaries to appear immediately when I open the app, so that I am not looking at a list of bare keys while the network catches up.
20. As a developer, I want a typo'd ticket key to tell me it could not be found, so that I do not spend an hour logging time against nothing.
21. As a developer, I want the not-found message to be honest that the ticket may exist but be invisible to me, so that I am not misled into thinking a real ticket does not exist.

### Sending time to Jira

22. As a developer, I want stopping a timer to be what sends time to Jira, so that there is exactly one moment where work becomes a permanent record.
23. As a developer, I want to review and correct the duration before it is sent, so that a timer I forgot to stop does not put six hours on a ticket.
24. As a developer, I want to optionally add a comment when I stop, so that the worklog says what I actually did.
25. As a developer, I want the duration truncated to the minute before sending, so that Jira shows round numbers rather than false precision.
26. As a developer, I want the worklog to leave the ticket's remaining estimate untouched, so that logging time does not silently rewrite someone else's planning.
27. As a developer, I want to see whether each stopped stretch has actually reached Jira, so that I know what is done and what is still owed.
28. As a developer, I want a failed sync to stay pending and retryable, so that a dropped connection costs me nothing.
29. As a developer, I want to retry a failed sync manually, so that I am not waiting on a background process I cannot see.
30. As a developer, I want an already-synced worklog to be impossible to send twice, so that a retry after a timeout does not double-log my time.
31. As a developer, I want a failed sync to never block or stop a running timer, so that a Jira problem does not become a lost-work problem.
32. As a developer, I want my local record to be the source of truth, so that Jira being down is an inconvenience rather than data loss.

### Surviving crashes and restarts

33. As a developer, I want my tickets and their time to survive quitting the app, so that I do not have to leave it running forever.
34. As a developer, I want a timer that was running when the app died to be capped at the last moment the app was known alive, so that I am not credited for the twelve hours my machine was asleep.
35. As a developer, I want a recovered timer left paused rather than silently resumed, so that the app does not keep accruing time for work I am not doing.
36. As a developer, I want a recovered entry visibly flagged on its row, so that I notice it needs checking even if I relaunch hours later.
37. As a developer, I want that flag to persist until I act on the row, so that a notice I dismissed reflexively at launch does not lose me the information.
38. As a developer, I want to correct a recovered duration before it goes to Jira, so that an approximate cap becomes an accurate worklog.
39. As a developer, I want a first launch with no data file to just work, so that installing the app is not an error state.
40. As a developer, I want a corrupted data file backed up rather than deleted, so that I have some chance of recovering unsent worklogs.
41. As a developer, I want to be told where that backup went, so that recovery is actually possible.
42. As a developer, I want the app to refuse to load a file written by a newer version rather than resetting it, so that downgrading cannot destroy my pending worklogs.
43. As a developer, I want a failed save to warn me rather than crash, so that a full disk does not cost me the session I am in the middle of.

### First run and credentials

44. As a new user, I want the app to introduce itself on first launch, so that installing a Dock-less menu-bar app does not appear to do nothing at all.
45. As a new user, I want to enter my Jira site URL, email, and API token in one place, so that setup is a single sitting.
46. As a new user, I want to paste any form of my Jira URL — bare host, full URL, or a board URL from my address bar — and have it normalized, so that setup does not fail on a trailing slash.
47. As a new user, I want to see the normalized URL echoed back, so that I can catch a mangled value before anything depends on it.
48. As a new user, I want a Test Connection button, so that I find out setup is wrong now rather than the first time I try to log time.
49. As a new user, I want a bad site URL and a bad token to produce different errors, so that I know which field to fix.
50. As a new user, I want a successful test to name the account I connected as, so that I catch authenticating as the wrong account.
51. As a new user, I want to be told up front that I need a classic unscoped token, so that I do not mint a scoped one that looks valid and never works.
52. As a new user, I want a rejected-credentials error to raise the scoped-token possibility, so that the most likely first-run failure is not indistinguishable from a typo.
53. As a new user, I want Save disabled until the connection test passes, so that I cannot store a combination that was never verified.
54. As a new user, I want editing any field after a passing test to re-disable Save, so that the gate is real rather than decorative.
55. As a new user, I want the settings window to survive my trip to the browser to mint a token, so that I do not lose half-entered input on the most-travelled path in setup.
56. As a new user, I want to be able to close settings without saving, so that I am never trapped in a form.
57. As a returning user, I want to reach settings from the popover whenever I want, so that I can fix credentials without reinstalling.
58. As a returning user, I want the token field blank on every visit with a note that it is stored, so that changing only my email does not require minting a new token.
59. As a returning user, I want my token in the Keychain rather than a file, so that my work credentials are stored the way secrets should be.
60. As a returning user, I want credentials that stop working to show a persistent banner, so that I notice before a day's worklogs pile up unsent.
61. As a returning user, I want my timers to keep running when credentials fail, so that an auth problem does not become lost work.
62. As a returning user, I want my stored token never auto-deleted on a failure, so that a transient Jira outage does not wipe my setup.

### Building it (first Swift project)

63. As a first-time Swift developer, I want each unfamiliar API introduced in its own slice, so that a failure has exactly one plausible cause.
64. As a first-time Swift developer, I want every slice to end at a commit that builds and runs, so that I always have a working app to fall back to.
65. As a first-time Swift developer, I want each slice to have one manual check I can perform by hand, so that "done" is observable rather than a feeling.
66. As a first-time Swift developer, I want the Jira API proven by curl before any Swift is written, so that a slice 2 failure is unambiguously a Swift failure.
67. As a first-time Swift developer, I want the captured real API responses as my fixtures, so that writing the `Codable` types is transcription rather than guesswork.
68. As a first-time Swift developer, I want a single observable store from the first slice, so that I do not scatter state across views and hit a wall when the timer needs a home no view owns.
69. As a first-time Swift developer, I want Keychain deferred until late, so that my first network call cannot fail ambiguously between a bad token and a bad Keychain read.
70. As a first-time Swift developer, I want tests only where the logic is pure and fiddly, so that I am not maintaining a test suite for a personal tool.

## Implementation Decisions

### Project shape — [ticket 01](wayfinder/issues/01-project-bootstrap-scaffolding.md)

- **Punchcard**, bundle id `com.jdsmith.Punchcard`, private repo `j-dsmith/punchcard`. "Jira" is deliberately absent from the name (Atlassian trademark; and renaming later would mean migrating a Keychain item and a sandbox container path). It appears descriptively only, as "Jira account" in Settings.
- macOS App template, SwiftUI interface, **Storage = None**. **Swift 6 language mode, Default Actor Isolation = MainActor** — a single-user menu-bar app is main-actor by nature, so the only isolation boundaries are the `await`s into Jira.
- Deployment target **macOS 13.0**. This floor has one hard consequence, discharged under Settings below.
- **App Sandbox ON** plus the network-client entitlement. This fixes the persistence location inside the app's sandbox container.
- **Single app target, feature-folder grouping** — app, timer, Jira wire types, credentials, persistence, UI. No SPM module split: modules buy compile-time isolation at a size that does not need it, at the cost of Xcode friction a first-time Swift dev cannot yet debug.
- Unit-test target **yes**; UI-test target **no**.
- Personal install via Archive → Distribute Content → Copy App, signed to run locally, no Apple Developer account. A locally built app is never quarantined, so there is no Gatekeeper right-click-Open step.
- `LSUIElement` and `MenuBarExtra` with `.window` style are set **at bootstrap** — they are the app's personality, and converting a Dock app later means re-learning the shell mid-flight. Launch-at-login is **deferred**; it is polish requiring a Settings toggle that will not exist for weeks.

### Domain model

`CONTEXT.md` is authoritative. The two points that most shape the code:

- A **`TrackedTicket`** is a Jira ticket the user has added to their list, **whether or not it has time against it**. "Tracked" means *on the list*, not *running*. It is not defined as a popover row.
- A **pause closes a `Segment`, it does not end the `TimeEntry`.** An entry holds a list of segments and its elapsed time is their sum, so one entry may span several start/pause cycles. This preserves the invariant that **one `TimeEntry` produces exactly one `Worklog`** — which is what gives the idempotency guard a single object to attach to.

Jira's API says *issue*; this domain says *ticket*. That word is confined strictly to the wire types, making the mismatch one deliberate mapping at the network boundary rather than a leak running through the codebase.

**Types arrive at their use site** — there is no "define the model" commit, since that is precisely the non-demonstrable commit the slice rule forbids, and types written before their use site get invented rather than discovered. `TrackedTicket` at slice 2, `TimeEntry` at slice 3, `Worklog` at slice 6.

### Build order — [ticket 02](wayfinder/issues/02-walking-skeleton-build-order.md)

A slice is **a commit that builds, runs, and is demonstrable by hand in the running app** — never a refactor-only or scaffolding-only commit.

| # | Slice | Manual check |
|---|---|---|
| 0 | curl spike — no Xcode, no Swift | *(complete — see ticket 05)* |
| 1 | Menu-bar shell + one `@Observable` store | Icon appears, no Dock icon, popover opens and closes |
| 2 | Jira read: credential-store protocol (hardcoded impl) + fetch summary | Popover shows the real summary of the throwaway ticket |
| 3 | Timer start/pause/resume, timestamp elapsed, one ticket, in-memory | Time accrues, pause holds it, resume continues it |
| 4 | Manual key entry + tracked list + recents, in-memory | Add two tickets, switch between them |
| 5 | JSON persistence + `lastSeen` heartbeat + restart recovery | Quit mid-run, relaunch, see the entry capped and flagged as paused |
| 6 | Stop → confirm/edit → truncate → POST worklog → pending/synced + idempotency | Throwaway ticket receives a worklog, then delete it |
| 7 | Keychain: swap the credential-store implementation | Credentials survive a relaunch with no file on disk |
| 8 | Settings-first onboarding — replaces hardcoded config | Fresh install reaches a working state with no code edits |

Three orderings here are decisions, not accidents:

- **Credentials come last, not second.** A credential-store *protocol* is defined at slice 2 with a hardcoded implementation; the real Keychain implementation lands at slice 7, adjacent to Settings — the only thing that ever *writes* to the Keychain. Putting Keychain first would make the first Jira call fail ambiguously between a bad token, a bad Keychain read, and a bad request.
- **Multi-ticket precedes persistence.** If slice 5 persisted a single-ticket shape, the file format would be rewritten one slice later. Persist the real shape once.
- **One `@Observable` store from slice 1**, injected through the environment, even holding a single field. The characteristic first-SwiftUI-project failure is scattering `@State` across views and hitting a wall at slice 3 when the timer must tick from somewhere no view owns. **One store, not one per feature.**

**Accepted cost:** a live API token sits in a gitignored source file from slice 2 to slice 7. The mitigation is procedural and non-negotiable — that file is gitignored **in the same commit that creates it**, never a commit later.

### Jira integration — [ticket 05](wayfinder/issues/05-jira-api-curl-spike.md), proven against real Jira

- **Jira Cloud**, `/rest/api/3/`, Basic auth with email plus API token.
- **A classic (unscoped) API token is required.** A scoped token returns **401 on every endpoint including `/myself`** — it is minted in the same Atlassian UI, looks entirely valid, and never authenticates. This is comfortably the most likely first-run failure and it presents exactly like a typo. (A merely missing scope should give 403; the likeliest reading is that scoped tokens do not authenticate over Basic auth at all. **This was not investigated** — error copy must not assert a mechanism nobody verified.)
- Endpoints: `GET /myself` (validate), `GET /issue/{key}?fields=summary` (read), `POST /issue/{key}/worklog?adjustEstimate=leave` (201), `DELETE /issue/{key}/worklog/{id}?adjustEstimate=leave` (204), `GET /serverInfo` (anonymous reachability check).
- **Always send `?fields=summary`.** The unfiltered issue is 19 KB against 297 bytes, and none of the rest is used.
- **`started` must be formatted `yyyy-MM-dd'T'HH:mm:ss.SSSZ` with a POSIX locale.** `ISO8601DateFormatter` emits no milliseconds and a `Z` instead of a numeric offset, and **will fail** — Jira 400s and names the format in the error. `+0000` is accepted, `+00:00` is rejected. **One formatter serves parsing, sending, and the on-disk file.**
- **A single error envelope decodes every failure** — a list of error messages plus a field-keyed error map, one populated and the other empty depending on the failure.
- **Ids are strings, not integers** — issue and worklog alike. The account id contains a colon and is opaque; do not parse it.
- **Jira echoes `started` in the account's timezone**, not what was sent — the same instant with a different wall clock. **Compare dates, never strings.**
- **A 404 is ambiguous** between "no such ticket" and "no permission". Error copy must not claim the ticket does not exist.
- `timeSpent` is a human-readable string derived by Jira; send `timeSpentSeconds`.
- Real responses are committed as fixtures, and the spike script re-runs the whole probe whenever a Jira assumption is in doubt.

### Persistence — [ticket 04](wayfinder/issues/04-on-disk-persistence-contract.md)

**Two files**, both inside the sandbox container's Application Support directory. Splitting them is deliberate: folding the heartbeat into the state file would rewrite the entire history every 30 seconds, putting the valuable file on the hot write path for the sake of one timestamp — and a crash mid-write would risk real data to save something worthless if lost.

Config lives in neither. These files carry **only domain data** — no secrets, no settings.

The state file, which encodes the schema decisions more precisely than prose:

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

The heartbeat file holds a schema version and `lastSeen`, nothing else.

**Every state is derived; none is stored.**

- **running** — the final segment has no `endedAt`
- **paused** — the final segment is closed and `worklog` is `null`
- **stopped** — `worklog` is non-null
- **needs sync** — the worklog state is `pending` or `failed`
- **list order** — by most recent activity, computed from the segments

A stored status field would be a second source of truth able to disagree with the segments; an explicit order field would imply drag-to-reorder, which is not in v1.

Further decisions:

- **Entries nest under their ticket.** An entry cannot exist without its ticket, and nesting makes an orphan unrepresentable rather than merely unlikely.
- **`timeSpentSeconds` is stored, not derived.** Duration is editable at stop and truncated to the minute, so the number sent to Jira is deliberately *not* the segment sum. Both persist: the segments are what Punchcard measured, `timeSpentSeconds` is what Jira was told. Recomputing on retry would resurrect the pre-edit value and silently discard the user's correction. `comment` likewise belongs to the worklog — captured at stop, existing only to be sent.
- **Identity.** A `TrackedTicket` is identified by its Jira key; a parallel UUID would be a second identity to keep in sync. A `TimeEntry` carries its own UUID, and **that is the idempotency key**.
- **The idempotency guard is entirely local.** Jira accepts no idempotency header, so the rule is: **never POST when the worklog state is `synced`**. Storing Jira's returned worklog id is what makes retry after an ambiguous timeout safe rather than duplicating.
- **ISO-8601 strings throughout, using the same formatter built for Jira.** One date format defined once, used at both the disk and wire boundaries. Epoch doubles were rejected because the file must stay hand-readable — verifying restart recovery means quitting the app and reading it.
- **The worklog enum gets a hand-written `Codable`** producing the flat tagged shape above; the synthesized encoding would emit a nested `{"synced":{"_0":"…"}}` and undo that readability on the single most important field in the file.
- **A nil optional is omitted, not written as `null`** — so an open segment simply has no `endedAt` key, and that absence is the one bit restart recovery keys off.
- **`schemaVersion` is present from day one.** Adding it later would mean writing migrations for files that predate versioning; two bytes now, unpayable later.
- **Write on every state change, atomic replace.** State changes are human-paced — a handful per hour — so debouncing buys nothing and opens a window where a crash loses exactly the event slice 5 exists to survive. Writing on quit is never relied upon: the app dying unexpectedly is the case being designed for.
- **A failed write must not stop a running timer.** In-memory state remains the truth, a persistent banner makes the risk visible, and the next state change retries naturally. Crashing would convert a recoverable disk problem into lost work.
- **Heartbeat every 30 seconds, only while a timer is running.** With nothing running there is no entry to cap, so a ticking write is pure noise. 30s bounds the worst-case over-count at 30 seconds, which truncate-to-minute usually erases entirely.
- **Nothing is pruned.** There is no v1 reporting, so no feature depends on history, and a year of heavy use is tens of kilobytes. Pruning is code that silently eats data when its condition is subtly wrong, bought for no v1 benefit.

**Restart recovery.** On launch, find the entry whose final segment is open, cap that segment at `lastSeen`, and mark it recovered. The entry is then paused *by derivation* — no separate state change. If `lastSeen` predates the segment's start (clock change), **clamp to the start** rather than persisting a negative duration.

"Flag it" is **a persistent marker on the row**, cleared when the user resumes or stops it — **not** an alert at launch. Relaunch may happen hours later; a launch alert gets dismissed reflexively and then the information is gone. The duration is already editable at stop, which is the natural correction point; the marker's only job is ensuring the user notices before reaching it.

File-state handling on load:

- **missing or zero-byte** → first run, empty state, no error
- **undecodable** → rename to a timestamped corrupt-backup, start fresh, show a non-blocking notice **naming the backup path**
- **unknown (newer) version** → refuse to load, blocking notice, write nothing

Corruption and version mismatch are treated differently because relaunching cannot fix corruption: refusing would brick the app with no user-side remedy, while silently resetting would hide destroyed worklogs. Backing up preserves hand-recovery, which on a single-user tool is a real option. Conversely an unknown version is only reachable by downgrading, the fix is relaunching the newer build, and resetting would destroy pending worklogs that never reached Jira.

### Settings and first run — [ticket 03](wayfinder/issues/03-first-run-auth-flow.md) *(slice 8)*

**Settings is its own window scene**, opened programmatically and fronted by activating the app. The deciding constraint is the **browser round trip**: minting an API token means leaving the app and pasting the result back, and a `MenuBarExtra` popover dismisses when it loses key-window status — so the most-travelled path in setup is the one that would destroy half-entered input. The standard `Settings` scene is the "correct" macOS answer and was **still rejected**: on the macOS 13 floor there is no `SettingsLink` and no public settings-opener, leaving only a fragile private selector that is a known trap for `LSUIElement` apps. A plain window scene uses only public macOS 13 API.

**Storage splits by sensitivity, not convenience:** the **token** goes to the Keychain as a generic password, service `com.jdsmith.Punchcard.jira`, account = email. The **site URL** and **email** go to `UserDefaults`. Only the token is a secret; putting the rest in the Keychain buys nothing and costs a Keychain read before the app can render anything at launch. This is also what keeps config out of the state file.

**The site URL is normalized hard and echoed back.** Accept a bare host, a full URL, or a pasted board URL; keep the host, discard scheme, path, and query, store exactly `https://<host>`. Then show "Will connect to …" beneath the field so a silent mangling is visible before Test Connection runs. Every call site is then a simple concatenation with no slash-juggling.

**Validation is two calls and it gates Save.** One Test Connection button runs the proven probes in order: first the **anonymous** server-info call — a failure here means the **URL** is wrong, stated as such rather than collapsed into "login failed", and it also asserts the deployment is Cloud — then the authenticated identity call, whose success renders **"Connected as <name>"**, naming the account rather than showing a bare tick, which catches authenticating as the wrong account entirely.

**Save is disabled until the test passes, and any keystroke in any field resets it to idle and re-disables Save.** Without that second rule you could pass a test, edit the URL, and save a combination that was never verified — the gate would be decorative. The window is never trapped: close and Cancel always work, and cancelling a later edit leaves previously-good credentials intact.

**The scoped-token trap is named twice** — as helper text beneath the token field before the attempt ("Punchcard needs a classic (unscoped) token", with a link to Atlassian's token page), and in the 401 copy after it: *"Jira rejected these credentials. If you created a scoped token, Punchcard needs a classic (unscoped) one."* The copy stops at "these don't work here" and **does not explain why**.

**Form layout is three fields in validation order** — site URL, email, token — so the form reads top-to-bottom in the same sequence the test checks it, and a failure always points at a field at or below the last one that passed. Token is a secure field. Test Connection has four states: idle, testing, passed, failed-with-a-specific-message. Settings contains credentials and nothing else in v1 — one section, no tabs, no sidebar.

**First run announces itself exactly once.** With no Dock icon, first launch after install visibly does nothing — one more icon in a menu bar that already has twelve. So: **no stored site URL at all** → auto-open Settings and activate. **Every subsequent launch** → never auto-open; the popover shows a gate view with an Open Settings button. Settings is reachable thereafter from a footer item beside Quit.

**A later 401 is non-destructive and retryable.** Any 401 from any call sets one flag and shows a persistent banner. Three rules matter more than the banner: **running timers keep running** (timing is local and needs no Jira); **a worklog that 401s stays pending, never synced**, and retries later against repaired credentials — 401 is a **retryable outcome, not a terminal one**, which is a constraint slice 6 must honour; and the **Keychain item is never auto-deleted on a 401**, so a transient outage cannot silently wipe stored credentials.

**Re-opening Settings does not re-read the token.** The field is empty on every visit, captioned that the token is stored and blank keeps it. Prefilling would read a secret into a live UI field on every visit for no benefit — dots cannot be proofread. The test uses the stored token when the field is blank and the typed one when it is not, making "change only the email" a two-second job rather than a trip to Atlassian.

### Open questions flagged for slices 6–8

Per the uneven-resolution decision, these are **deliberately unresolved** and are to be decided at build time with a real running row on screen:

1. **Running-row controls** — a ghost pause button plus a Stop capsule, pause-as-capsule, or two capsules.
2. **Whether blue is reserved exclusively for Sync/Retry** actions.

The throwaway HTML mockup is a **reference mood and IA only, consulted at slices 2 and 6** — it is not a SwiftUI blueprint and never a slice of its own.

## Testing Decisions

**What makes a good test here:** it exercises external behaviour through a pure function over values, with no I/O, no `AppStore`, and no view. Given inputs, assert outputs. A test that reaches for a file, a socket, the Keychain, or a SwiftUI view hierarchy is out of scope by construction — those are covered by the per-slice manual check, which is the primary verification strategy for this app and is not a compromise.

There is **no prior art** — this is a greenfield first Swift project, so these tests set the pattern rather than following one. Ticket 01 provisioned a unit-test target and deliberately no UI-test target: macOS UI testing against a menu-bar popover is a rabbit hole.

**One seam: pure logic over values.** Four areas are tested, and nothing else:

1. **Elapsed-time arithmetic** — the sum over a segment list, including an open segment evaluated against an **injected `now`** rather than the wall clock. Cases: no segments; one closed segment; several closed segments; a closed set plus an open one; a single open segment.
2. **Truncate-to-minute** — the rule converting measured elapsed into `timeSpentSeconds`. Cases: exact minutes; sub-minute remainders; a total under one minute; zero.
3. **Recovery capping** — given segments and a `lastSeen`, produce the capped result. Cases: `lastSeen` inside the open segment (the normal case); `lastSeen` before the segment's `startedAt` (**the clamp**, from a clock change); no open segment at all (a clean quit — nothing to cap).
4. **Codable round-trip** — decode the committed real-Jira fixtures into the wire types, and round-trip the state file including the hand-written worklog enum in all three of its shapes plus `null`.

Areas 1 and 2 are the two ticket 02 named. **Areas 3 and 4 are deliberate additions**, agreed when writing this spec:

- **Recovery** is exactly the "pure and fiddly" criterion ticket 02 set, and it is the one piece of arithmetic that runs only when nobody is watching — after a crash. Verifying it by hand means engineering a crash to see it, so the manual check is expensive in a way no other slice's is.
- **The fixture round-trip is near-free** — the fixtures are already committed, and transcription accuracy was the entire reason for capturing them. It catches the string-vs-integer id trap and the `started` format trap at test time instead of against live Jira.

Explicitly **not** unit-tested, by decision rather than omission: the store, any SwiftUI view, `URLSession` calls, Keychain access, and file I/O. Their verification is the manual check listed against each slice in the build-order table.

## Out of Scope

Ruled out for v1 in the handoff decision log and reaffirmed by the map:

- Shared or multi-user backend of any kind; this is a single-user local tool.
- OAuth 3LO — Basic auth with a classic API token is proven and sufficient.
- Automatic idle detection.
- Live ticket autocomplete against Jira search; keys are entered manually, with recents.
- In-app reporting or analytics.
- Remaining-estimate management — worklogs are posted with the estimate explicitly left alone.
- App Store distribution.
- Cross-platform support.
- Code signing and notarization — deferred until a teammate actually wants a copy.
- Launch at login — deferred; additive to a form later, not a restructure.
- Pruning or archiving of history.
- Editing or deleting worklogs already synced to Jira.
- Least-privilege scoped tokens. The 401-not-403 behaviour was observed and deliberately **not** investigated; if least-privilege ever becomes a requirement it is a fresh question, not a settled one.
- Tabs, sidebar, or any Settings content beyond credentials.
- A SwiftData or Core Data store — JSON files are the settled mechanism.

## Further Notes

**This is the author's first Swift and SwiftUI project.** That fact shaped the plan more than any product requirement: the build order exists to isolate unfamiliar APIs, the test strategy is deliberately minimal, the module structure is deliberately flat, and several decisions were made for legibility over elegance. Preserve that bias when implementing — favour the boring, inspectable option.

**The map is complete.** All five wayfinder tickets are resolved and the frontier is empty; nothing remains to decide before building, except the two UI micro-calls explicitly deferred to slice 6.

**Verification against real Jira uses a throwaway ticket, and the worklog is deleted afterwards.** There is no app-side test support for this and no dedicated ticket — it is a manual discipline. The spike script already demonstrates the full create-then-delete cycle.

**The captured fixtures are redacted, and formats are preserved exactly** — ids remain strings, the account id keeps its colon, timestamps keep their offset-without-colon. They remain faithful fixtures. The spike script regenerates raw responses, so the redaction script must be re-run before committing. One unfiltered-issue capture was **deleted rather than redacted** because it held 34 populated fields of real project data where a single missed custom field would be a leak; its only finding — the 60× size difference — is recorded in ticket 05 and needed no artifact.
