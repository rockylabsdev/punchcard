# 03 - First-run Settings-first auth flow

Kind: wayfinder
Status: open
Type: grilling
Blocked by: 05

## Question

Pin the interaction detail of the settled decisions (site URL = config field at first run; Settings-first onboarding). Decide:

- The credentials the Settings screen collects: **Jira site base URL** (e.g. `company.atlassian.net`), **email**, **API token** — field layout, validation, and how the base URL is normalized/stored.
- How the popover gates on missing credentials: does it show the Settings screen until a valid token is stored, then flip to the timer list? What's the "no credentials yet" empty state?
- How token validity is checked on save — a test call (e.g. `GET /myself` or a `GET /issue/{key}`), success/failure UX, and whether an invalid token blocks leaving Settings.
- Where credentials live (Keychain, per handoff) vs where the site URL lives (Keychain item vs JSON config) — and re-editing later.
- ~~Jira **Cloud vs Server/DC** confirmation~~ — **settled** by [05 - Jira API curl spike](05-jira-api-curl-spike.md): Cloud. `/rest/api/3/` + email:token Basic auth is correct.

May warrant a quick `/prototype` pass on the Settings screen if layout is unclear.

### Inputs from [05 - Jira API curl spike](05-jira-api-curl-spike.md)

- **Only a classic (unscoped) API token works** — a scoped token 401s on every endpoint. Settings has to steer the user to the right one, or the most likely first-run failure is a token that looks valid and simply never authenticates. Open question this ticket owns: how much the UI says about token type, and whether the 401 message can name the probable cause.
- **The save-time validation call is proven**: `GET /rest/api/3/myself` returns 200 with `displayName`, so Settings can confirm success by naming the account rather than showing a bare tick.
- **`GET /rest/api/3/serverInfo` needs no auth** — a site URL can be checked for reachability *before* asking for credentials, separating "bad URL" from "bad token" instead of collapsing both into one failure.
- **A 404 is ambiguous** between "no such ticket" and "no permission", so error copy must not assert the ticket doesn't exist.

## Answer
