# 08 — Store the token in the Keychain

**What to build:** The API token moves out of the gitignored source file and into the macOS Keychain. Delete that file, rebuild, and the app still authenticates. Nothing user-facing changes — this swaps the implementation behind the `CredentialStore` protocol.

**Blocked by:** 02 — Read a ticket summary from Jira.

**Note on ordering:** this only needs the `CredentialStore` protocol to exist, so it is **takeable any time after ticket 02** and does not depend on tickets 03–07. It is numbered late by convention rather than by dependency: it sits adjacent to ticket 09, which is the only thing that ever *writes* to the Keychain, so building the pair together is one context switch instead of two.

**Status:** ready-for-agent

Build slice 7 of the [spec](../spec.md).

- [ ] Keychain-backed `CredentialStore` implementation, stored as a **generic password** — service `com.jdsmith.Punchcard.jira`, account = the user's email
- [ ] Generic password rather than internet password, deliberately: fewer attributes to get wrong
- [ ] The hardcoded implementation and its gitignored source file are removed
- [ ] Only the **token** goes to the Keychain; the site URL and email are not secrets and belong in `UserDefaults`
- [ ] **Manual check:** delete the local credentials file, rebuild, and the app still fetches a ticket summary
