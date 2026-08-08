# 05 - Jira API curl spike

Kind: wayfinder
Status: open
Type: task
Blocked by: None
Blocks: 03

## Question

Slice 0 of the build order from [02 - Walking-skeleton build order](02-walking-skeleton-build-order.md). No Xcode, no Swift — the point is to prove the external assumptions before any Swift is written against them, so that a failure at slice 2 is unambiguously a Swift failure.

Against a **throwaway ticket** on the real Jira instance, using an API token and email over Basic Auth:

1. `GET /rest/api/3/myself` — proves the credentials and confirms **Jira Cloud vs Server/DC**.
2. `GET /rest/api/3/issue/{key}?fields=summary` — proves the read path and the summary field shape.
3. `POST /rest/api/3/issue/{key}/worklog` with `adjustEstimate=leave` — proves the write path, the accepted body shape, and the response.
4. Delete the created worklog afterwards (the map's settled test strategy).

Capture the **real JSON responses** into `docs/jira-api/` — these become the fixtures the `Codable` types are transcribed from at slice 2, rather than being guessed.

Resolution should record: Cloud or Server/DC, the exact endpoints that worked, anything the handoff assumed that turned out wrong, where the token lives, and the paths of the captured responses.
