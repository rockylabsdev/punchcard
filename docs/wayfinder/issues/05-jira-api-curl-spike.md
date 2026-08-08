# 05 - Jira API curl spike

Kind: wayfinder
Status: resolved
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

## Comments

**2026-08-08 — spike built, blocked on authentication.**

`docs/jira-api/spike.sh` runs the whole probe from a gitignored `.spike-env`. First run against `example.atlassian.net` / `EX-1234`:

- `GET /rest/api/3/serverInfo` → **200**, `"deploymentType": "Cloud"`. This endpoint answers anonymously, so it confirms **Jira Cloud** and a valid site URL but says nothing about credentials.
- `GET /rest/api/3/myself` → **401**, `x-seraph-loginreason: AUTHENTICATED_FAILED`.
- Everything else → **404** with `"Issue does not exist or you do not have permission to see it."` — Jira's anonymous-user response, not a missing ticket. All 404s here are downstream of the 401.

Ruled out: site URL, ticket key, token formatting (192 chars, `ATAT` prefix, no whitespace/quotes/non-ascii), and email as the variable — the same token 401s against three plausible account addresses. Stopped guessing emails to avoid a lockout.

Remaining candidates, all needing account-level inspection: token revoked or expired; token belonging to a different Atlassian org; or org policy blocking API-token Basic auth.

**Why this matters beyond the spike:** the handoff's settled auth decision is Basic auth + API token. If the org blocks that, the decision is invalidated rather than merely inconvenient, and both this ticket and [03 - First-run Settings-first auth flow](03-first-run-auth-flow.md) need rework. Establish *which* of the three causes it is before writing an answer.

Captured responses under `docs/jira-api/responses/` are failure bodies, **not** the slice-2 fixtures. Rerun after auth works.

**2026-08-08 — green run.** A newly created **classic (unscoped)** token authenticated immediately; the earlier 401 was the scoped token. All four steps passed. Answer below.

## Answer

Slice 0 is proven. `docs/jira-api/spike.sh` is repeatable — it reads a gitignored `.spike-env` and rewrites every fixture — so it can be rerun whenever a Jira assumption is in doubt.

### Deployment

**Jira Cloud** — `serverInfo` reports `"deploymentType": "Cloud"`, so the v3 REST API and ADF comment bodies both apply. `serverInfo` answers anonymously, so it is a valid pre-auth reachability check for a site URL the user has just typed.

### Token type — a decision, not just a fact

**Punchcard must use a classic (unscoped) API token.** A scoped token failed with **401 `AUTHENTICATED_FAILED`** on every endpoint, including `/myself`.

Note the 401: a merely *missing* scope should produce 403, so the likeliest reading is that scoped tokens do not authenticate over Basic auth at all and need a different scheme. **This was not investigated** — the classic token worked and unblocked the spike. If least-privilege ever becomes a requirement, that is a fresh question, not a settled one.

This confirms the handoff's Basic auth + API token decision. Nothing the handoff assumed turned out wrong.

### Endpoints proven

| Call | Result |
|---|---|
| `GET /rest/api/3/myself` | 200 — `accountId`, `displayName`, `emailAddress`, `timeZone` |
| `GET /rest/api/3/issue/{key}?fields=summary` | 200 — `{ id, self, key, fields: { summary } }` |
| `GET /rest/api/3/issue/{key}` (unfiltered) | 200 — **19 KB**; `?fields=summary` returns 297 bytes |
| `GET /rest/api/3/issue/ZZZZ-99999` | 404 — bad-key error shape |
| `POST /rest/api/3/issue/{key}/worklog?adjustEstimate=leave` | **201** — full worklog echoed back |
| `GET /rest/api/3/issue/{key}/worklog` | 200 — `{ startAt, maxResults, total, worklogs[] }` |
| `DELETE /rest/api/3/issue/{key}/worklog/{id}?adjustEstimate=leave` | **204**, no body |

Always send `?fields=summary`. The unfiltered issue is 60× larger and none of it is used.

### The `started` format trap — confirmed

The deliberate wrong-format probe **400s**, and Jira names the format in the error:

```json
{ "errorMessages": [], "errors": { "started": "Invalid date format. Please enter the date in the format \"yyyy-MM-dd'T'HH:mm:ss.SSSZ\"." } }
```

`+00:00` is rejected; `+0000` is accepted. This matters for Swift: `ISO8601DateFormatter` emits `2026-08-08T19:28:01Z` — no milliseconds, `Z` instead of a numeric offset — and **will fail**. Slice 6 needs a `DateFormatter` with `dateFormat = "yyyy-MM-dd'T'HH:mm:ss.SSSZ"` and `locale = Locale(identifier: "en_US_POSIX")`.

The same format parses responses, so one formatter serves both directions.

### Error envelope — one type covers both

Both failures share an envelope, differing only in which half is populated:

- 404: `{ "errorMessages": ["Issue does not exist or you do not have permission to see it."], "errors": {} }`
- 400: `{ "errorMessages": [], "errors": { "started": "…" } }`

So a single `struct JiraError: Decodable { let errorMessages: [String]; let errors: [String: String] }` decodes every failure. Note the 404 text is deliberately ambiguous between *absent* and *forbidden* — Punchcard must not claim "no such ticket" on a 404.

### Response quirks worth knowing at slice 2

- **Ids are strings, not ints** — issue `"100001"`, worklog `"110001"`. Decode as `String`.
- **`accountId` contains a colon** — `"712020:11111111-…"` in the redacted fixture. It is an opaque `String`; do not parse it.
- **Jira echoes `started` in the account's timezone**, not what was sent: `…T19:28:01.000+0000` came back as `2026-08-08T14:28:01.000-0500`. Same instant, different wall clock. Compare `Date`s, never strings.
- **`timeSpent` is a human string** (`"5m"`) alongside `timeSpentSeconds` (`300`). Send seconds; the string is display-only and Jira derives it.
- The POST response embeds full `author` and `updateAuthor` objects — decode only what is needed.

### Where the token lives

- **Spike:** `docs/jira-api/.spike-env`, gitignored.
- **Slices 2–7:** `Punchcard/Credentials/LocalSecrets.swift`, already gitignored, behind the `CredentialStore` protocol from [02 - Walking-skeleton build order](02-walking-skeleton-build-order.md).
- **Slice 7 onward:** Keychain.

### Captured fixtures

`docs/jira-api/responses/` — `myself`, `serverinfo`, `issue-summary`, `issue-404`, `worklog-post`, `worklog-post-badstarted`, `worklog-list`, `worklog-list-after`, plus the two request bodies sent. These are the real shapes the slice-2 `Codable` types get transcribed from.

**Redacted.** This is a work Jira, so `docs/jira-api/redact.py` rewrites the site, account id, email, display name, avatar initials, ticket key, issue/worklog ids, and the ticket summary to placeholders in place. **Formats are preserved exactly** — ids stay strings, `accountId` keeps its colon, timestamps keep the offset-without-colon — so the files remain faithful fixtures. `spike.sh` regenerates raw responses, so rerun `redact.py` before committing; it is idempotent.

`issue-full.json` was **deleted rather than redacted**: 34 populated fields of real project data (project key, components, fix versions, a full ADF bug description) where one missed custom field is a leak. Its only finding — 19 KB unfiltered vs 297 bytes for `?fields=summary` — is recorded above and needs no artifact.

Values that deliberately survive: `timeZone: "America/Chicago"`, because the timezone-echo behaviour above is unreadable without it.

Test hygiene held: the spike's worklog was deleted (204) and `worklog-list-after` reports `total: 0`. The bad-format POST created nothing. `EX-1234` is as it was found.
