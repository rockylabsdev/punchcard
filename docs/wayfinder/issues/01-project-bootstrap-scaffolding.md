# 01 - Project bootstrap & scaffolding

Kind: wayfinder
Status: closed
Assignee: j-dsmith
Type: grilling
Blocked by: None

## Question

Lock the concrete shape of the new Xcode project so a first-time Swift dev can start building without second-guessing structure. Decide and record:

- Xcode template + options (macOS App, SwiftUI interface, Swift, Storage = None).
- Single app target with feature-folder grouping vs any module split.
- New GitHub repo name + where the throwaway UI prototype HTML lands.
- Personal-use packaging path (Archive → Copy App → right-click-Open Gatekeeper bypass; signing deferred).
- Minimum project config for a menu-bar-only app: `LSUIElement`, `MenuBarExtra` window style, `SMAppService` auto-launch — which of these are set at bootstrap vs added later.

Recommendation already on the table (see map Notes / session): boring standard single-target SwiftUI app, Storage None, feature folders, dedicated repo, Archive-based personal install. This ticket confirms or adjusts it.

## Answer

Environment at time of decision: Xcode 26.6, Swift 6.3.3, macOS 26.5.2.

**Product identity.** Name **Punchcard**; bundle id `com.jdsmith.Punchcard`; repo `j-dsmith/punchcard` (private). "Jira" deliberately kept out of the name: it's Atlassian's trademark, and since the map already anticipates a teammate wanting a copy, a rename later would mean migrating a Keychain item and a sandbox container path. It appears only descriptively ("Jira account") in Settings.

**Xcode project.** macOS App template — SwiftUI interface, Swift, Storage = None. **Swift 6 language mode with Default Actor Isolation = MainActor** (the Xcode 26 default): a single-user menu-bar app is main-actor by nature, so the only isolation boundaries are the `await`s into Jira. Swift 5 mode was rejected as teaching idioms that would need unlearning. Deployment target macOS 13.0 (carried from handoff). **App Sandbox ON** plus `com.apple.security.network.client`. Unit-test target included; UI-test target not (macOS UI testing against a menu-bar popover is a rabbit hole, and the agreed test strategy is a manual throwaway ticket).

**Structure.** Single app target, feature folders — `App/`, `Timer/`, `Jira/`, `Credentials/`, `Persistence/`, `UI/`. No SPM module split: modules buy compile-time isolation this size doesn't need and cost Xcode friction a first-time Swift dev can't yet debug. Domain types stay literal and un-branded — `TrackedTicket` (a popover row), `TimeEntry` (one start→stop period), `Worklog` (the Jira POST body). The brand does not leak into the model.

**Repo layout.** One repo. Xcode project at root (`Punchcard.xcodeproj`, `Punchcard/`); wayfinder artifacts under `docs/wayfinder/`; throwaway UI mockup under `docs/prototype/`.

**Menu-bar config at bootstrap.** `LSUIElement` = YES and `MenuBarExtra` with `.menuBarExtraStyle(.window)` — these are the app's personality, and building a Dock app to convert later means re-learning the shell mid-flight. `SMAppService` launch-at-login **deferred**: pure polish, and it needs a Settings toggle that won't exist for weeks.

**Packaging.** Signing Certificate = "Sign to Run Locally", no team, no Apple Developer account. Install via Product → Archive → Distribute Content → Copy App → drag to `/Applications`. Correction to the question as originally written: there is **no** Gatekeeper right-click-Open step — a locally-built app is never quarantined, so that ritual applies only to apps that arrive over the network.

**Consequence for the map.** Sandbox ON fixes the persistence location at `~/Library/Containers/com.jdsmith.Punchcard/Data/Library/Application Support/`, closing that half of the on-disk-persistence fog.
