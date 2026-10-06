# 01 — Menu-bar shell and app store

**What to build:** Punchcard exists and runs. An icon appears in the menu bar, the app has no Dock icon and does not appear in ⌘-Tab, and clicking the icon opens and dismisses a popover. The popover's contents can be a placeholder — this ticket is about the shell being right.

The Xcode project does not exist yet. Creating it to the shape settled in [wayfinder ticket 01](../wayfinder/issues/01-project-bootstrap-scaffolding.md) is part of this ticket rather than a separate scaffolding commit, which would not be demoable.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

Build slice 1 of the [spec](../spec.md).

- [ ] macOS App target created: SwiftUI interface, Storage = None, Swift 6 language mode with Default Actor Isolation = MainActor, deployment target macOS 13.0
- [ ] Bundle identifier is `com.jdsmith.Punchcard`; the product is named Punchcard, with no "Jira" in the name
- [ ] App Sandbox enabled with the network-client entitlement
- [ ] Unit-test target present; no UI-test target
- [ ] Feature-folder grouping in place (app, timer, Jira, credentials, persistence, UI); single target, no SPM module split
- [ ] `LSUIElement` set, so the app has no Dock icon and no ⌘-Tab entry
- [ ] `MenuBarExtra` with the `.window` style hosts the popover
- [ ] A single `@Observable` store exists and is injected through the environment, even if it holds only one field — one store for the whole app, not one per feature
- [ ] **Manual check:** icon appears in the menu bar, no Dock icon, popover opens and closes
- [ ] Launch-at-login is *not* implemented — deferred by decision
