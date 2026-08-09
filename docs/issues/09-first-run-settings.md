# 09 — First-run settings

**What to build:** A fresh install reaches a working state with no code edits. On the very first launch the app introduces itself rather than silently adding one more icon to a crowded menu bar. The user enters their Jira site URL, email, and API token, tests the connection, and saves. Credentials can be re-edited later, and credentials that stop working are surfaced without destroying anything.

**Blocked by:** 07 — Handle failed syncs; 08 — Store the token in the Keychain.

**Status:** ready-for-agent

Build slice 8 of the [spec](../spec.md), designed in full in [wayfinder ticket 03](../wayfinder/issues/03-first-run-auth-flow.md).

- [ ] Settings is **its own window scene**, opened programmatically with the app activated — not a view inside the popover, which would dismiss on the browser trip to mint a token, and not the standard `Settings` scene, which has no public opener on the macOS 13 floor
- [ ] Three fields in **validation order**: site URL, email, token (secure field)
- [ ] Site URL normalized hard — bare host, full URL, or a pasted board URL all reduce to `https://<host>`, discarding scheme, path, and query
- [ ] The normalized value is **echoed back** beneath the field, so a mangling is visible before testing
- [ ] **Test Connection runs two calls in order:** the anonymous server-info call first, so a bad URL reports as a bad URL rather than a login failure, asserting the deployment is Cloud; then the authenticated identity call
- [ ] Success names the account — "Connected as …" — rather than showing a bare tick, catching authentication as the wrong account
- [ ] **Save is disabled until the test passes, and any keystroke in any field resets it to idle and re-disables Save.** Without the second half the gate is decorative
- [ ] Four button states: idle, testing, passed, failed with a specific message
- [ ] **The scoped-token trap is named twice:** helper text under the token field stating a **classic (unscoped)** token is required with a link to Atlassian's token page, and 401 copy raising it as the likely cause. The copy stops at "these don't work here" and **does not explain why** — the mechanism was deliberately never verified
- [ ] Token stored in the Keychain; site URL and email in `UserDefaults`
- [ ] Token field is **blank on every visit**, captioned that it is stored and that blank keeps it — the test uses the stored token when blank and the typed one when not
- [ ] The window is never trapped: close and Cancel always work, and cancelling a later edit leaves good credentials intact
- [ ] **First launch with no stored site URL** auto-opens Settings and activates the app; every subsequent launch never auto-opens, showing a gate view in the popover with an Open Settings button
- [ ] Settings reachable thereafter from a footer item beside Quit
- [ ] A later 401 sets one flag and shows a persistent banner — **running timers keep running**, worklogs stay pending and retryable, and the **Keychain item is never auto-deleted**, so a transient outage cannot wipe stored credentials
- [ ] Settings contains credentials and nothing else — one section, no tabs, no sidebar
- [ ] **Manual check:** on a machine with no stored credentials, launch, complete Settings, and time and sync a ticket without editing any code
