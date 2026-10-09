# Context

Punchcard is a personal, single-user terminal app — a TUI plus a CLI sharing one core — that times work against a ticket and records the elapsed time as a worklog. Optionally, a **tracker** (Jira is the first) receives each worklog. It is built to be driven by a human at the keyboard or by an agent (Claude Code) through the CLI.

## Glossary

### Tracker

The optional remote a `Worklog` is sent to. Jira is the first tracker. There is at most one, chosen in setup, and a Punchcard with none is complete: it times work and keeps worklogs; nothing is ever owed. What a ticket key looks like is the tracker's rule. Without one, a key is a **slug** (letters, digits, `.`, `_`, `-`), case-sensitive, kept as typed.

### Ticket

A unit of work, named by a key. **"Ticket" is the domain word** — it is what you say out loud — and it is used throughout the app's own types and UI. With a tracker, a ticket is a unit of work *there* (a Jira ticket); without one, it is whatever the user chose to name.

Jira's own API says **issue** (`/rest/api/3/issue/{key}`). That word is confined strictly to the wire types under the Jira package, so the mismatch is one deliberate mapping at the network boundary rather than a leak running through the codebase.

Note the third sense in this repo: the planning artifacts under `docs/wayfinder/issues/` are also called tickets. Context distinguishes them; nothing in the app model refers to them.

### TrackedTicket

A ticket the user has added to their list, **whether or not it currently has time against it**.

"Tracked" means *on the list*, not *running*. A ticket added a moment ago with zero elapsed time is a `TrackedTicket`; so is one last worked a week ago. This is deliberately not defined as "a row in the TUI" — the TUI renders `TrackedTicket`s, but the concept does not depend on being displayed.

### Unverified

A `TrackedTicket` whose key the tracker has not yet confirmed. It was added while the tracker could not be reached, so it has no summary and may be a typo. Time against it accrues normally. It stops being unverified the first time the tracker confirms the key; a key the tracker reports not found is never added in the first place. Without a tracker there is nothing to confirm, so no ticket is ever unverified.

### TimeEntry

One start→stop period against a `TrackedTicket`.

Elapsed time is derived from timestamps rather than accumulated by a ticking counter, so that sleep, quit, and clock changes cannot silently corrupt it.

A pause does **not** end a `TimeEntry` — it closes the current **segment**. An entry is a list of segments and its elapsed time is their sum, so one entry may span several start/pause cycles. This preserves the invariant that **one `TimeEntry` produces exactly one `Worklog`**.

### Segment

One uninterrupted stretch of accrual inside a `TimeEntry`. A segment with no end time is the running one; there is at most one such segment across every writer: the TUI, the CLI, and any agent session. Every change to the state is one indivisible transition that closes any open segment before it opens another, so starting one ticket while another runs pauses the first.

On screen, a segment is a **counted** stretch, and the gap between two segments is a pause, shown as **not counted**. Time cut off by recovery after `lastSeen` is shown the same way, as time away, not counted. "Counted" and "not counted" are display labels for `Segment` and its gaps. They are not separate domain terms.

### Worklog

The final record of a completed stretch of work: its day, its duration and a comment. With a tracker it is what the tracker is told — the body of the `POST` — and the thing whose sync state (pending / synced / failed) the app tracks. Without one its state is **local**: final, nowhere to send, never owed.

A `Worklog` is what is recorded; a `TimeEntry` is what Punchcard measured. They are not the same thing: the duration is editable at stop time, and is truncated to the minute before it becomes a `Worklog`.

### Today

The local calendar day that contains now. **Time today** for a ticket is the part of its segments that falls inside that day: a segment that spans midnight is split there, and only the part after midnight counts toward today. A worklog's day is the local date it started, which is its entry's first segment start. Yesterday's time is never recounted and today's never carries over.

### Running / Paused / Stopped

- **Running** — the timer is accruing against exactly one `TrackedTicket`. At most one ticket may be running at a time.

A running timer measures **the user's attention, not an agent's work**. Several agents working in parallel do not make several timers: the user's time is one stream, and switching between agents' tickets is switching the one timer. A `Worklog` therefore records the user's own hours, and a day's worklogs never sum past the hours the user actually lived.
- **Paused** — accrual suspended, no `Worklog` produced; resuming continues the same work.
- **Stopped** — the work is finished and a `Worklog` is produced.

Pause and stop are distinct actions with distinct consequences; stopping is the only path that produces a worklog, and so the only path that reaches a tracker.

### Presence

Evidence that the user is at the machine: a keystroke in the TUI, a command run, or typing or submitting a prompt in any Claude Code session. Only the user's own input counts. An agent's activity, a scheduled task, a message from another session, and an open but untouched TUI are not presence.

Presence says nothing about *which* ticket the user is on. That is **focus**.

### Focus

Evidence of which `TrackedTicket` the user is working on: a ticket they start or switch to by hand, or their own prompt in a Claude Code session whose ticket can be told from where it runs: a `.punchcard` file naming a key, found at the nearest ancestor of the session's directory within its repository, or failing that a ticket key in the git branch name. Nothing else names a session's ticket. A session whose ticket cannot be told gives presence but never focus.

### Held

A running timer the user set by hand. Focus from a prompt never moves a held timer. Only another act by hand does. A timer started automatically, from a prompt's focus, is not held and follows the user's focus from session to session.

### Origin

Who caused a change: **hand** (the user, at a terminal or in the TUI), **prompt** (the user's own prompt in a Claude Code session, relayed as focus), or **agent** (an agent acting on its own).

Origin decides two things. Only **hand** makes a timer **held**. **hand** and **prompt** are presence; **agent** is not. Origin never decides recovery: any change that finds a stale timer recovers it first, whoever caused the change.

A command the user types into an agent's shell counts as **agent**. Origin is about who issued the change, as far as Punchcard can tell. It is not about whose fingers were on the keys.

### lastSeen

The last moment of **presence**. **Reading state is never presence**: asking what is running neither vouches for it nor changes it.

`lastSeen` is not a sign of life. No Punchcard process needs to be alive for a timer to run, and a timer accrues whether or not the user is typing. A running timer is an open segment and nothing more.

`lastSeen` is the upper bound on how long the work could truthfully have lasted. Recovery caps the running entry there rather than trusting the wall clock.

### Stale

A running timer is **stale** when there has been no presence for longer than the stale threshold. Staleness is derived, never stored, and can be observed only once: the first act of presence to find a stale timer caps its open segment at `lastSeen` and flags the entry **recovered** *before* recording itself, which would otherwise erase the gap. That capping is **recovery**; it replaces the old trigger of an app relaunching after a crash.

A **recovered** entry stays flagged until it is stopped, where its duration is corrected, not merely until it is resumed.
