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
- Jira **Cloud vs Server/DC** confirmation, since `/rest/api/3/` + email:token Basic Auth assumes Cloud.

May warrant a quick `/prototype` pass on the Settings screen if layout is unclear.

## Answer
