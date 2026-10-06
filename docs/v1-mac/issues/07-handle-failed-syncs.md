# 07 — Handle failed syncs

**What to build:** Jira being unreachable costs nothing. A worklog that fails to send stays pending and retryable, visibly so, and can be retried by hand when the connection is back. Nothing about a sync failure interferes with timing work — the local record is the source of truth and Jira is only a destination.

**Blocked by:** 06 — Stop a timer and send a worklog to Jira.

**Status:** ready-for-agent

Second half of build slice 6 of the [spec](../spec.md). Split from ticket 06 for size only.

- [ ] The worklog state machine persists across relaunch in all three shapes — pending, synced, failed — plus absent
- [ ] A failed send leaves the worklog **pending or failed, never synced**, carrying the error message
- [ ] Manual retry from the row; a successful retry transitions to synced and stores the worklog id
- [ ] **A 401 is a retryable outcome, not a terminal one** — this is a constraint ticket 09's credential handling depends on
- [ ] **A sync failure never blocks or stops a running timer.** Timing is local and needs no Jira; stopping the clock because a sync failed would turn a network problem into lost work
- [ ] Unsent work is visible at a glance, so a day of failed syncs cannot pile up unnoticed
- [ ] The idempotency guard from ticket 06 holds across retries — an already-synced worklog cannot be sent twice
- [ ] **Decide the deferred UI question:** whether blue is reserved exclusively for Sync and Retry actions
- [ ] **Manual check:** disconnect the network, stop a timer, see it stay pending; reconnect, retry, see it sync; confirm the worklog appears in Jira exactly once, then delete it
