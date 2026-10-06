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

One uninterrupted stretch of accrual inside a `TimeEntry`. A segment with no end time is the running one; there is at most one such segment across every writer: the TUI, the CLI, and any agent session. Every change to the state is one indivisible transition that closes any open segment before it opens another, so starting one ticket while another runs pauses the first.

### Worklog

The record submitted to Jira for a completed stretch of work — the body of the `POST` to Jira, and the thing whose sync state (pending / synced) the app tracks.

A `Worklog` is what Jira stores; a `TimeEntry` is what Punchcard measured. They are not the same thing: the duration is editable at stop time, and is truncated to the minute before it becomes a `Worklog`.

### Running / Paused / Stopped

- **Running** — the timer is accruing against exactly one `TrackedTicket`. At most one ticket may be running at a time.

A running timer measures **the user's attention, not an agent's work**. Several agents working in parallel do not make several timers: the user's time is one stream, and switching between agents' tickets is switching the one timer. A `Worklog` therefore records the user's own hours, and a day's worklogs never sum past the hours the user actually lived.
- **Paused** — accrual suspended, no `Worklog` produced; resuming continues the same work.
- **Stopped** — the work is finished and a `Worklog` is produced for Jira.

Pause and stop are distinct actions with distinct consequences; stopping is the only path that reaches Jira.

### Presence

Evidence that the user is at the machine: a keystroke in the TUI, a command run, or typing or submitting a prompt in any Claude Code session. Only the user's own input counts. An agent's activity, a scheduled task, a message from another session, and an open but untouched TUI are not presence.

Presence says nothing about *which* ticket the user is on. That is **focus**.

### Focus

Evidence of which `TrackedTicket` the user is working on: a ticket they start or switch to by hand, or their own prompt in a Claude Code session whose ticket can be told from where it runs. A session whose ticket cannot be told gives presence but never focus.

### Held

A running timer the user set by hand. Focus from a prompt never moves a held timer. Only another act by hand does. A timer started automatically, from a prompt's focus, is not held and follows the user's focus from session to session.

### lastSeen

The last moment of **presence**. **Reading state is never presence**: asking what is running neither vouches for it nor changes it.

`lastSeen` is not a sign of life. No Punchcard process needs to be alive for a timer to run, and a timer accrues whether or not the user is typing. A running timer is an open segment and nothing more.

`lastSeen` is the upper bound on how long the work could truthfully have lasted. Recovery caps the running entry there rather than trusting the wall clock.

### Stale

A running timer is **stale** when there has been no presence for longer than the stale threshold. Staleness is derived, never stored, and can be observed only once: the first act of presence to find a stale timer caps its open segment at `lastSeen` and flags the entry **recovered** *before* recording itself, which would otherwise erase the gap. That capping is **recovery**; it replaces the old trigger of an app relaunching after a crash.

A **recovered** entry stays flagged until it is stopped, where its duration is corrected, not merely until it is resumed.
