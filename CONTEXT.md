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

### The list

The `TrackedTicket`s not removed. Removing a ticket (`rm`, the TUI's `d`) hides it from the list and sets its `removedAt`; it never deletes a worklog, so a removed ticket's logged worklogs stay in the log. Only time that was never logged (an open entry, or with a tracker an unsent worklog) is lost, and only when the user forces it. Starting, resuming, adding or focusing a removed ticket's key **restores** it: back on the list with its `addedAt` and history. Everything that shows "the list" (`ls`, the Tickets pane, recents, `status`) leaves removed tickets out; everything that answers for a range of time (`log`, the day's log) does not.

### Unverified

A `TrackedTicket` whose key the tracker has not yet confirmed. It was added while the tracker could not be reached, so it has no summary and may be a typo. Time against it accrues normally. It stops being unverified the first time the tracker confirms the key; a key the tracker reports not found is never added in the first place. Without a tracker there is nothing to confirm, so no ticket is ever unverified.

### TimeEntry

One start→stop period against a `TrackedTicket`.

Elapsed time is derived from timestamps rather than accumulated by a ticking counter, so that sleep, quit, and clock changes cannot silently corrupt it.

A pause does **not** end a `TimeEntry` — it closes the current **segment**. An entry is a list of segments and its elapsed time is their sum, so one entry may span several start/pause cycles. This preserves the invariant that **one `TimeEntry` produces exactly one `Worklog`**.

### Segment

One uninterrupted stretch of accrual inside a `TimeEntry`. A segment with no end time is the running one; there is at most one such segment across every writer: the TUI, the CLI, and any agent session. Every change to the state is one indivisible transition that closes any open segment before it opens another, so starting one ticket while another runs pauses the first.

A segment may be closed or opened at a time the user names (`pause --at`, `resume --at`, `start --at`) instead of now. That corrects measured time within the day; it never touches a stopped entry or `lastSeen`, it never overlaps time already measured, and it never moves a recovered entry's pause past `lastSeen`, the upper bound recovery capped it at. A gap the user made this way is a pause like any other.

On screen, segments are listed without a label: a segment is just its range and length. Only the gaps are marked, by their reason: every gap the user made between two segments is a pause (`‖ paused 5m`), however they made it, and time cut off by recovery after `lastSeen` is time away (`! away 2h 22m`). The header states once that gaps are left out of the worklog.

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

Evidence of which `TrackedTicket` the user is working on: a ticket they start or switch to by hand, or their own prompt in a Claude Code session whose ticket can be told from where it runs: a `.punchcard` file naming a key, found at the nearest ancestor of the session's directory within its repository, or failing that a ticket key in the git branch name (in lower case only for a project already on the list, spelled as that project is). Nothing else names a session's ticket. A session whose ticket cannot be told gives presence but never focus.

The timer runs on the ticket of the session the user last prompted in. If they move it elsewhere themselves, by hand or by asking an agent, it stays there until they prompt in a session on a different ticket. Nothing starts a timer except the user.

That is: focus from a prompt moves a running timer only when it **changes**, when the prompt's session ticket differs from the **last focus**, and then it moves the timer whoever set it, by hand, by prompt or by agent. The user's newest signal outranks their older one. It never starts a timer when nothing runs, and a paused timer counts as nothing running. A `punchcard touch KEY` typed by hand is the same call and follows the same rule; it never sets the timer by hand. Repeating the same focus prompt after prompt moves nothing, so a timer the user moved away from the session's ticket stays moved until focus changes.

### Last focus

The session ticket of the user's most recent prompt that had one, or the key of a more recent `punchcard touch KEY` typed by hand. Every such prompt or touch records it, whether or not it moved the timer. Nothing else writes it: not a start or switch by hand or by an agent, not an agent's touch, not a pause or stop, and not a prompt from a session whose ticket cannot be told. It never expires.

### Origin

Who caused a change: **hand** (the user, at a terminal or in the TUI), **prompt** (the user's own prompt in a Claude Code session, relayed as focus), or **agent** (an agent acting on its own).

Origin decides two things. Only **hand** sets a timer **by hand**: an agent cannot move a timer set by hand (its start, pause and resume refuse it), while the user's own focus can, when it changes. A timer set by focus or by an agent is not by hand, and an agent may move it. **hand** and **prompt** are presence; **agent** is not. Origin never decides recovery: any change that finds a stale timer recovers it first, whoever caused the change.

A command the user types into an agent's shell counts as **agent**. Origin is about who issued the change, as far as Punchcard can tell. It is not about whose fingers were on the keys.

### lastSeen

The last moment of **presence**. **Reading state is never presence**: asking what is running neither vouches for it nor changes it.

`lastSeen` is not a sign of life. No Punchcard process needs to be alive for a timer to run, and a timer accrues whether or not the user is typing. A running timer is an open segment and nothing more.

`lastSeen` is the upper bound on how long the work could truthfully have lasted. Recovery caps the running entry there rather than trusting the wall clock.

### Stale

A running timer is **stale** when there has been no presence for longer than the stale threshold. Staleness is derived, never stored, and can be observed only once: the first act of presence to find a stale timer caps its open segment at `lastSeen` and flags the entry **recovered** *before* recording itself, which would otherwise erase the gap. That capping is **recovery**; it replaces the old trigger of an app relaunching after a crash.

A **recovered** entry stays flagged until it is stopped, where its duration is corrected, not merely until it is resumed.
