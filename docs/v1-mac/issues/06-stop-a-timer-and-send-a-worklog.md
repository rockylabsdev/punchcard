# 06 — Stop a timer and send a worklog to Jira

**What to build:** Stopping a timer is the deliberate act that ends the work and sends it to Jira. Stopping opens a sheet showing the measured duration for review, allows correcting it, and takes an optional comment. On confirm the time is truncated to the minute and posted to Jira, and the row shows it as synced. This is the happy path — failure handling is ticket 07.

**Blocked by:** 05 — Survive quit and crash.

**Status:** ready-for-agent

First half of build slice 5→6 of the [spec](../spec.md); tickets 06 and 07 together are slice 6. Split for size only — the build order is unchanged.

- [ ] `Worklog` introduced, hanging off the `TimeEntry` and absent until stop — one entry produces exactly one worklog
- [ ] Stop opens a confirm sheet with the measured duration **editable**, plus an optional comment
- [ ] Duration is **truncated to the minute** before sending
- [ ] `POST /rest/api/3/issue/{key}/worklog?adjustEstimate=leave`, sending `timeSpentSeconds` — the remaining estimate is deliberately left untouched, and the human-readable `timeSpent` string is Jira's to derive
- [ ] `started` sent in `yyyy-MM-dd'T'HH:mm:ss.SSSZ` via the shared formatter — a numeric offset like `+0000` is required and `+00:00` is rejected
- [ ] **`timeSpentSeconds` and the comment are persisted on the worklog, not recomputed from segments.** The edited, truncated number is what Jira was told; the segments are what Punchcard measured. Recomputing on retry would silently discard the user's correction
- [ ] Jira's returned worklog id is stored (as a **string**) on success, and the state becomes synced
- [ ] **Idempotency guard: never POST when the worklog state is already synced.** Jira accepts no idempotency header, so the guard is entirely local — the `TimeEntry`'s own UUID is the key, and the stored worklog id is what makes a retry after an ambiguous timeout safe
- [ ] Jira echoes `started` back in the account's timezone, not what was sent — **compare dates, never strings**
- [ ] The row shows whether a stopped stretch has reached Jira
- [ ] **Decide the deferred UI question:** running-row controls — ghost pause plus a Stop capsule, pause-as-capsule, or two capsules. Decide with a real running row on screen, then record it
- [ ] **Test:** truncate-to-minute — exact minutes, sub-minute remainders, a total under one minute, zero
- [ ] **Manual check:** the throwaway ticket receives a worklog, then delete it from Jira
