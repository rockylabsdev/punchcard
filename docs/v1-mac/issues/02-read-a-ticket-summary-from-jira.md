# 02 — Read a ticket summary from Jira

**What to build:** The popover shows the real summary of a Jira ticket, fetched live from the API. The ticket key is hardcoded for now and credentials come from a gitignored source file — both are replaced later. Seeing your own ticket's actual summary in the menu bar is the demo.

**Blocked by:** 01 — Menu-bar shell and app store.

**Status:** ready-for-agent

Build slice 2 of the [spec](../spec.md). Every wire fact below was proven against real Jira in [wayfinder ticket 05](../wayfinder/issues/05-jira-api-curl-spike.md); the captured responses under `docs/jira-api/responses/` are the fixtures these types are transcribed from, not guessed against.

- [ ] `TrackedTicket` introduced — a Jira ticket the user has added to their list, whether or not it has time against it
- [ ] Jira wire types transcribed from the committed fixtures, kept in the Jira feature folder; the word *issue* appears only there, while the domain says *ticket*
- [ ] `GET /rest/api/3/issue/{key}?fields=summary` over Basic auth with email and API token — always send `?fields=summary` (the unfiltered issue is 19 KB against 297 bytes)
- [ ] Ids decoded as **strings**, not integers
- [ ] One shared date formatter using `yyyy-MM-dd'T'HH:mm:ss.SSSZ` with a POSIX locale — `ISO8601DateFormatter` will fail against Jira and must not be used
- [ ] A single error-envelope type decodes every failure (message list plus field-keyed error map)
- [ ] A 404 is reported without claiming the ticket does not exist — it is ambiguous between absent and forbidden
- [ ] `CredentialStore` defined as a **protocol**, with a hardcoded implementation reading a local source file
- [ ] That credentials file is added to `.gitignore` **in the same commit that creates it** — never a commit later
- [ ] **Test:** the committed fixtures decode into the wire types, covering the string-id and date-format traps
- [ ] **Manual check:** the popover shows the real summary of the throwaway ticket
