# 04 - On-disk persistence contract

Kind: wayfinder
Status: open
Type: grilling
Blocked by: 02

## Question

Graduated from the map's fog once [02 - Walking-skeleton build order](02-walking-skeleton-build-order.md) fixed what data exists. Slice 5 of the build order (JSON persistence + `lastSeen` heartbeat + restart recovery) sits inside the range the spec covers in detail, so the contract can no longer be left vague.

Location is already fixed by the App Sandbox decision in ticket 01: `~/Library/Containers/com.jdsmith.Punchcard/Data/Library/Application Support/`. What remains:

- **Exact JSON schema.** The shape of the persisted `TrackedTicket` list and its `TimeEntry` history — field names, date encoding (ISO-8601 vs epoch), what the `Worklog` sync state looks like at rest (pending/synced/failed + the idempotency key).
- **Schema-version field.** Present from v1 or added later; how an unrecognised version is handled (refuse to launch, back up and reset, best-effort decode).
- **Write strategy.** Write-on-every-change vs debounced; atomic replace vs in-place; what happens if the write fails.
- **`lastSeen` heartbeat.** Cadence, where it lives in the file, and its cost given the file is rewritten to update it.
- **Concrete restart-recovery flow.** The settled rule is "cap the running timer at `lastSeen`, move it to paused, flag it" — pin down what "flag it" means to the user (a badge? a one-time notice? an editable duration on the row?), and the behaviour when the file is missing, empty, or corrupt.

Consult `/grilling` and `/domain-modeling`; the glossary in root `CONTEXT.md` is the authority on `TrackedTicket` / `TimeEntry` / `Worklog`.
