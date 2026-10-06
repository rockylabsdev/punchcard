# 04 — Track multiple tickets

**What to build:** The popover becomes a real list. Add a Jira ticket by typing its key and see its summary resolve, add a second, switch the running timer between them, and remove one you are done with. Recently used tickets are offered when adding, so returning to yesterday's work is a click. Still in memory only.

**Blocked by:** 03 — Start, pause, and resume a timer.

**Status:** ready-for-agent

Build slice 4 of the [spec](../spec.md). This deliberately precedes persistence: persisting a single-ticket shape would mean rewriting the file format one slice later.

- [ ] Add a `TrackedTicket` by typing its key; the summary is fetched and shown
- [ ] A ticket with no time against it is still tracked — "tracked" means *on the list*, not *running*
- [ ] Recently used tickets are offered when adding
- [ ] Switching the running timer between tickets is unambiguous about what happens to the one already running
- [ ] Still at most one running ticket across the whole list
- [ ] Remove a ticket from the list
- [ ] List order is **derived** from most recent activity — no stored order field, no drag-to-reorder
- [ ] A key that cannot be fetched surfaces an honest error that does not assert the ticket does not exist
- [ ] **Manual check:** add two tickets, switch between them, remove one
