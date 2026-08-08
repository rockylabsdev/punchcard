# Jira Time Tracker Wayfinder Map

Source request: Handoff doc `jira-time-tracker-HANDOFF.md` (no Jira key — greenfield personal tool). Personal single-user macOS menu-bar app that times work against a Jira ticket and syncs elapsed time as a worklog.

## Destination

A spec-ready understanding of the greenfield app so `to-spec` can write `.scratch/jira-time-tracker/spec.md`. The product decisions are settled (see handoff decision log); this map only clears the remaining **bootstrap / build-order** and **first-run auth-flow** fog — sharpened by the fact this is the user's first Swift/SwiftUI project.

## Notes

- **This is the user's first Swift/SwiftUI project.** Wayfinder's job here is less "resolve product fog" and more "sequence the unknowns (MenuBarExtra, Keychain, URLSession/Codable) into a de-risking build order." Favour recommendations with rationale over open-ended questions.
- Product decision log is **settled** in the handoff — do not re-litigate: one running timer, pause vs stop, timestamp-based elapsed + `lastSeen` heartbeat, manual key entry + recents, `POST /rest/api/3/issue/{key}/worklog` with `adjustEstimate=leave`, local-state-as-source-of-truth + idempotency guard, editable duration + optional comment on stop, truncate-to-minute, no v1 reporting, JSON-file persistence (not SwiftData), macOS 13+ floor.
- Settled this session: Jira **site URL = config field at first run**; first-run = **Settings-first onboarding**; test strategy = **manual throwaway ticket, delete worklog after** (no app support, no ticket).
- The app lives in its **own repo** — `j-dsmith/punchcard` (private), not `korweb_companion_app-1`. That repo now holds both the app (at root) and these planning artifacts (under `docs/wayfinder/`).
- UI direction is a reference mood/IA only (throwaway HTML mockup), NOT a SwiftUI blueprint. Skills to consult when resolving tickets: `/grilling`, `/domain-modeling`, `/prototype`.
- Domain vocabulary lives in root [`CONTEXT.md`](../../CONTEXT.md) — glossary only, no implementation detail. It is the authority on `TrackedTicket` / `TimeEntry` / `Worklog` / `lastSeen`; sharpen it in place rather than redefining terms inside ticket answers.

## Decisions so far

<!-- one line per closed ticket -->

- [01 - Project bootstrap & scaffolding](issues/01-project-bootstrap-scaffolding.md) — App is **Punchcard** (`com.jdsmith.Punchcard`, repo `j-dsmith/punchcard`); single-target SwiftUI macOS app, Swift 6 mode with MainActor default isolation, App Sandbox ON + network-client entitlement, feature folders, menu-bar personality (`LSUIElement` + `MenuBarExtra(.window)`) set at bootstrap with `SMAppService` deferred, personal install via Archive → Copy App signed to run locally.
- [02 - Walking-skeleton build order](issues/02-walking-skeleton-build-order.md) — Nine steps: a **curl spike before any Swift** (slice 0), then menu-bar shell + one `@Observable AppStore`, Jira read behind a `CredentialStore` protocol with a **hardcoded impl**, in-memory timer, multi-ticket list + recents, JSON persistence, worklog POST, **Keychain at slice 7**, Settings last. A slice = a commit that builds, runs, and is hand-demonstrable. Spec covers all nine at uneven resolution (0–5 detailed, 6–8 intent + flagged questions). Vocabulary from ticket 01 corrected and moved to root [`CONTEXT.md`](../../CONTEXT.md).

- [05 - Jira API curl spike](issues/05-jira-api-curl-spike.md) — Slice 0 proven against a real ticket: **Jira Cloud**; read, worklog POST (201) and DELETE (204) all work over Basic auth with a **classic (unscoped) API token** — a *scoped* token 401s and must not be used. `started` must be `yyyy-MM-dd'T'HH:mm:ss.SSSZ` (`ISO8601DateFormatter` will fail); one error envelope decodes every failure; ids are strings. Real responses captured in `docs/jira-api/` as the slice-2 `Codable` fixtures, and `spike.sh` reruns the whole probe.

## Not yet specified

- **Two open UI micro-calls** (handoff): running-row controls (ghost-pause + Stop capsule vs pause-as-capsule vs two capsules); whether blue is reserved exclusively for Sync/Retry. Non-blocking; ticket 02 placed these at **build slice 6**, with a real running row on screen to react to — they stay fog until then.

## Out of scope

- Shared/multi-user backend, OAuth 3LO, automatic idle detection, live ticket autocomplete, in-app reporting/analytics, remaining-estimate management, App Store distribution, cross-platform (Windows/Linux). All ruled out for v1 in the handoff decision log.
- Code signing / notarization — deferred until a teammate actually wants a copy.
