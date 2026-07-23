# 02 - Walking-skeleton build order

Kind: wayfinder
Status: open
Type: grilling
Blocked by: 01

## Question

For a first Swift/SwiftUI project, decide the de-risking sequence of vertical slices — what to build first so each unfamiliar API (MenuBarExtra, Keychain, URLSession/Codable, JSON persistence, timer/heartbeat) is learned in isolation before it's combined.

Produce an ordered walking-skeleton: the smallest end-to-end slice that proves the riskiest unknown first, then each subsequent slice that adds one capability. Candidate ordering to react to:

1. MenuBarExtra window shell that opens/closes (prove the menu-bar host).
2. Keychain read/write of a dummy secret (prove Security framework).
3. One authenticated `GET /issue/{key}?fields=summary` rendered in the popover (prove URLSession + Codable + Basic Auth end-to-end).
4. Local timer with start/pause/resume + timestamp-based elapsed, in-memory.
5. JSON-file persistence + `lastSeen` heartbeat + restart recovery.
6. Stop → confirm/edit dialog → `POST worklog` → pending/synced state machine + idempotency.

Decide whether this order holds, what the true "tracer bullet" first commit is, and where to stop for a spec.

## Answer
