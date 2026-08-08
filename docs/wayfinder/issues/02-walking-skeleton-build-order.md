# 02 - Walking-skeleton build order

Kind: wayfinder
Status: resolved
Assignee: j-dsmith
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

The candidate six-slice order **mostly holds, but is wrong at both ends** and has a hole in the middle. Corrected sequence — nine steps, of which slice 0 is not Swift and not a slice:

```
0.  curl spike — no Xcode, no Swift                        (own ticket: 05)
1.  Menu-bar shell + @Observable AppStore
2.  Jira read: CredentialStore(hardcoded) + GET issue → summary in popover
3.  Timer: start/pause/resume, timestamp elapsed, one ticket, in-memory
4.  Manual key entry + tracked list + recents, in-memory
5.  JSON persistence + lastSeen heartbeat + restart recovery   (contract: ticket 04)
6.  Stop → confirm/edit → truncate-to-minute → POST worklog
      → pending/synced state machine + idempotency guard
7.  Keychain: swap the CredentialStore impl
8.  Settings-first onboarding — replaces hardcoded config      (design: ticket 03)
```

### The tracer bullet is a curl command, not a commit

The riskiest unknown is not a SwiftUI API — it is whether this Jira instance behaves as the handoff assumes (Cloud, `/rest/api/3/`, email:token Basic Auth, worklog POST accepted with `adjustEstimate=leave`). Proving that in Swift means debugging two unknowns simultaneously: the Swift and the assumption. **Slice 0 is three curl calls** against a throwaway ticket — `GET /myself`, `GET /issue/{key}?fields=summary`, `POST /issue/{key}/worklog` — with the real JSON responses captured into `docs/`. Those captures become the fixtures the `Codable` types are written against, which turns slice 2 from guesswork into transcription. Filed as ticket 05, `task`, takeable immediately; it now **blocks ticket 03**, because designing an auth screen against an unverified auth mechanism is the one way this map could waste real work.

### Credentials come last, not second

The candidate put Keychain at slice 2, ahead of the first network call. Rejected: Keychain is a self-contained API with no design risk and can slot in at any point, whereas putting it first makes the first Jira call fail ambiguously — bad token, bad Keychain read, or bad `URLRequest`? Instead, **define `CredentialStore` as a protocol at slice 2 with a hardcoded implementation**, and swap in the real Keychain implementation at slice 7, immediately before Settings. Settings is the only thing that ever *writes* to the Keychain, so building the pair adjacently is one context switch instead of two.

Cost accepted knowingly: a live API token sits in a gitignored file from slice 2 to slice 7. Mitigation is procedural and non-negotiable — `Punchcard/Credentials/LocalSecrets.swift`, gitignored in the *same commit that creates it*, never a commit later.

This also settles how this ticket meshes with 03: the real Settings screen is the **last** slice, not the first. Ticket 03 designs it; nothing depends on that design until slice 8.

### The missing slice: multi-ticket before persistence

The candidate list jumps from one hardcoded ticket key (slice 3) straight to worklog POST, so the **manual key entry + recents** behaviour settled in the handoff never gets built. Inserted as slice 4, and deliberately placed *before* persistence: if slice 5 persists a single-ticket shape, the file format gets rewritten one slice later. Persist the real shape once.

### An `@Observable` store from slice 1

The characteristic first-SwiftUI-project failure is scattering `@State` across views, then hitting a wall at slice 3 when the timer must tick from somewhere no view owns. Under Swift 6 with MainActor default isolation (ticket 01), a single `@Observable final class AppStore`, injected via `.environment()`, is the natural home. Introduce it at slice 1 even holding one field — ten minutes now against a mid-flight rewrite later. **One store, not one per feature**; this app is far too small for more.

### What a slice is, and what proves it done

A slice is **a commit that builds, runs, and is demonstrable by hand in the running app**. Never a refactor-only or scaffolding-only commit. Each slice carries one manual check in the spec — slice 1: "icon appears, no Dock icon, popover opens and closes"; slice 2: "popover shows the real summary of my throwaway ticket"; slice 5: "quit mid-run, relaunch, see the entry capped and flagged as paused"; slice 6: "throwaway ticket receives a worklog, then delete it".

Unit tests only where logic is genuinely pure and fiddly: **elapsed-time arithmetic and the truncate-to-minute rule**. Nothing else in v1. This follows the map's settled test strategy rather than expanding it.

### Domain types arrive at their use site

No "define the model" commit — that is precisely the non-demonstrable commit the slice rule forbids, and types written before their use site get invented rather than discovered. `TrackedTicket` at slice 2, `TimeEntry` at slice 3, `Worklog` at slice 6.

**Vocabulary correction to ticket 01.** Two problems were fixed. `TrackedTicket` was defined there as "a popover row" — a UI definition wearing a domain name, silent on whether a row with no time yet counts, or whether one stopped last week still does. Re-founded as: *a Jira issue the user has added to their list, whether or not it currently has time against it* — "tracked" means **on the list**, not **running**. Separately, Jira's API says *issue* everywhere while the model says *ticket*, and this repo already uses "ticket" for wayfinder tickets. Resolution: keep **ticket** for the domain (it is what you say out loud), confine **issue** strictly to the wire types under `Jira/`, making the seam one deliberate mapping rather than a leak. These terms are now recorded in root `CONTEXT.md` rather than buried in a resolved ticket.

### Where to stop for a spec

**Spec all nine steps, at deliberately uneven resolution**: slices 0–5 detailed enough to build from, slices 6–8 as intent plus their explicitly-flagged open questions. Truncating the order would hide the target, and knowing where you are headed is the entire value of a build order.

This forces one consequence: slice 5 sits inside the detailed range, so the on-disk persistence contract can no longer remain fog. It graduates to **ticket 04** (`grilling`, blocked by this ticket) — build order had to fix what data exists before a schema could be written, and it now has.

The throwaway HTML mockup is a **reference consulted at slices 2 and 6**, never a slice of its own. The two open UI micro-calls in the map's fog — running-row controls, and whether blue is reserved for Sync/Retry — get decided at slice 6, with a real running row on screen to react to.
