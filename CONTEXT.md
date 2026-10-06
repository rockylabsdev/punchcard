# Context

Punchcard is a personal, single-user terminal app — a TUI plus a CLI sharing one core — that times work against a Jira ticket and syncs the elapsed time back to Jira as a worklog. It is built to be driven by a human at the keyboard or by an agent (Claude Code) through the CLI.

## Glossary

### Ticket

A unit of work in Jira. **"Ticket" is the domain word** — it is what you say out loud — and it is used throughout the app's own types and UI.

Jira's own API says **issue** (`/rest/api/3/issue/{key}`). That word is confined strictly to the wire types under `Jira/`, so the mismatch is one deliberate mapping at the network boundary rather than a leak running through the codebase.

Note the third sense in this repo: the planning artifacts under `docs/wayfinder/issues/` are also called tickets. Context distinguishes them; nothing in the app model refers to them.

### TrackedTicket

A Jira ticket the user has added to their list, **whether or not it currently has time against it**.

"Tracked" means *on the list*, not *running*. A ticket added a moment ago with zero elapsed time is a `TrackedTicket`; so is one last worked a week ago. This is deliberately not defined as "a row in the TUI" — the TUI renders `TrackedTicket`s, but the concept does not depend on being displayed.

### TimeEntry

One start→stop period against a `TrackedTicket`.

Elapsed time is derived from timestamps rather than accumulated by a ticking counter, so that sleep, quit, and clock changes cannot silently corrupt it.

A pause does **not** end a `TimeEntry` — it closes the current **segment**. An entry is a list of segments and its elapsed time is their sum, so one entry may span several start/pause cycles. This preserves the invariant that **one `TimeEntry` produces exactly one `Worklog`**.

### Segment

One uninterrupted stretch of accrual inside a `TimeEntry`. A segment with no end time is the running one; there is at most one such segment in the whole app.

### Worklog

The record submitted to Jira for a completed stretch of work — the body of the `POST` to Jira, and the thing whose sync state (pending / synced) the app tracks.

A `Worklog` is what Jira stores; a `TimeEntry` is what Punchcard measured. They are not the same thing: the duration is editable at stop time, and is truncated to the minute before it becomes a `Worklog`.

### Running / Paused / Stopped

- **Running** — the timer is accruing against exactly one `TrackedTicket`. At most one ticket may be running at a time.
- **Paused** — accrual suspended, no `Worklog` produced; resuming continues the same work.
- **Stopped** — the work is finished and a `Worklog` is produced for Jira.

Pause and stop are distinct actions with distinct consequences; stopping is the only path that reaches Jira.

### lastSeen

A heartbeat timestamp persisted while the app runs, recording when it was last known alive.

If the app dies while a timer is running, `lastSeen` is the upper bound on how long that work could truthfully have lasted — recovery caps the running entry there rather than trusting the wall clock.
