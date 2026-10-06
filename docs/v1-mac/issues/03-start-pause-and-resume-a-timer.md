# 03 — Start, pause, and resume a timer

**What to build:** Time accrues against the ticket in the popover. Starting begins timing, pausing holds the total, and resuming continues the *same* stretch of work rather than beginning a new one. The displayed total is computed from timestamps, so it stays correct across a sleep. In memory only — quitting still loses everything.

**Blocked by:** 02 — Read a ticket summary from Jira.

**Status:** ready-for-agent

Build slice 3 of the [spec](../spec.md). `TimeEntry` and `Segment` are defined in [`CONTEXT.md`](../../../CONTEXT.md) and are authoritative.

- [ ] `TimeEntry` and `Segment` introduced
- [ ] **A pause closes a `Segment`; it does not end the `TimeEntry`.** One entry holds a list of segments and may span several start/pause cycles — this preserves the invariant that one `TimeEntry` produces exactly one `Worklog`
- [ ] Elapsed time is the sum over segments, derived from timestamps — never a ticking accumulator
- [ ] The running state is derived, not stored: the final segment having no end time *is* running
- [ ] At most one ticket may be running at a time
- [ ] The running row is visually distinct from the others
- [ ] **Test:** elapsed-time arithmetic as a pure function over a segment list with an **injected `now`** — no segments; one closed; several closed; closed set plus an open one; a single open segment
- [ ] **Manual check:** time accrues, pause holds it, resume continues it; sleep the machine and confirm the total is still right
