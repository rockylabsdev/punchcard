# 01 - Project bootstrap & scaffolding

Kind: wayfinder
Status: open
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
