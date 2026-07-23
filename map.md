# Jira Time Tracker Wayfinder Map

Source request: Handoff doc `jira-time-tracker-HANDOFF.md` (no Jira key — greenfield personal tool). Personal single-user macOS menu-bar app that times work against a Jira ticket and syncs elapsed time as a worklog.

## Destination

A spec-ready understanding of the greenfield app so `to-spec` can write `.scratch/jira-time-tracker/spec.md`. The product decisions are settled (see handoff decision log); this map only clears the remaining **bootstrap / build-order** and **first-run auth-flow** fog — sharpened by the fact this is the user's first Swift/SwiftUI project.

## Notes

- **This is the user's first Swift/SwiftUI project.** Wayfinder's job here is less "resolve product fog" and more "sequence the unknowns (MenuBarExtra, Keychain, URLSession/Codable) into a de-risking build order." Favour recommendations with rationale over open-ended questions.
- Product decision log is **settled** in the handoff — do not re-litigate: one running timer, pause vs stop, timestamp-based elapsed + `lastSeen` heartbeat, manual key entry + recents, `POST /rest/api/3/issue/{key}/worklog` with `adjustEstimate=leave`, local-state-as-source-of-truth + idempotency guard, editable duration + optional comment on stop, truncate-to-minute, no v1 reporting, JSON-file persistence (not SwiftData), macOS 13+ floor.
- Settled this session: Jira **site URL = config field at first run**; first-run = **Settings-first onboarding**; test strategy = **manual throwaway ticket, delete worklog after** (no app support, no ticket).
- The app lives in its **own new repo**, not `korweb_companion_app-1`. This scratch dir only hosts planning artifacts.
- UI direction is a reference mood/IA only (throwaway HTML mockup), NOT a SwiftUI blueprint. Skills to consult when resolving tickets: `/grilling`, `/domain-modeling`, `/prototype`.

## Decisions so far

<!-- one line per closed ticket -->

## Not yet specified

- **On-disk persistence shape**: exact JSON schema, file location (Application Support vs container), schema-version field, and the concrete restart-recovery flow (cap running timer at `lastSeen`, move to paused, flag it). Settled in principle; the on-disk contract graduates to a ticket once build-order fixes what data exists.
- **Two open UI micro-calls** (handoff): running-row controls (ghost-pause + Stop capsule vs pause-as-capsule vs two capsules); whether blue is reserved exclusively for Sync/Retry. Non-blocking; decide during build or an early prototype pass.

## Out of scope

- Shared/multi-user backend, OAuth 3LO, automatic idle detection, live ticket autocomplete, in-app reporting/analytics, remaining-estimate management, App Store distribution, cross-platform (Windows/Linux). All ruled out for v1 in the handoff decision log.
- Code signing / notarization — deferred until a teammate actually wants a copy.
