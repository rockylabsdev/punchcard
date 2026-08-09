# 03 - First-run Settings-first auth flow

Kind: wayfinder
Status: resolved
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

Settings is **slice 8** — the last slice — per [02 - Walking-skeleton build order](02-walking-skeleton-build-order.md). Nothing depends on this design until then; it is settled now only because ticket 05 unblocked it and the spec covers slices 6–8 as intent plus flagged questions.

### Settings is its own window, not a popover route

`Window("Punchcard Settings", id: "settings")`, opened via `@Environment(\.openWindow)` and fronted with `NSApp.activate`. Not a view inside the `MenuBarExtra` popover, and not the standard `Settings` scene.

The deciding constraint is the **browser round trip**. Getting an API token means leaving the app, minting one at Atlassian, and pasting it back. A `MenuBarExtra(.window)` popover dismisses when it loses key-window status, so the most-travelled path in the entire flow is the one that would destroy half-entered input. That disqualifies the in-popover option regardless of how much simpler it is to build.

The `Settings` scene is the "correct" macOS answer and was still rejected: on the macOS 13 floor from ticket 01 there is no `SettingsLink` (macOS 14+) and no public `openSettings`, leaving only the fragile `showSettingsWindow:` selector — a known trap for `LSUIElement` apps and a poor thing to hit on a first Swift project. A plain `Window` scene uses only public macOS 13 API and can be given a ⌘, shortcut later if it is ever wanted.

### Storage splits by sensitivity, not by convenience

Three values, two homes:

| Value | Home |
|---|---|
| API token | Keychain — `kSecClassGenericPassword`, service `com.jdsmith.Punchcard.jira`, account = email |
| Site URL | `UserDefaults` |
| Email | `UserDefaults` |

Only the token is a secret. Putting the other two in the Keychain buys nothing and costs a Keychain read before the app can render anything at launch. `kSecClassGenericPassword` over `kSecClassInternetPassword` deliberately: fewer attributes to get wrong on a first Keychain outing.

This also keeps **config out of ticket 04's JSON file**, which stays purely tracked tickets and time entries. Config and state are separate concerns with separate lifetimes.

### The site URL is normalized hard, and the result is shown back

Accept anything: `company.atlassian.net`, `https://company.atlassian.net/`, or a full board URL pasted from the address bar. Take the host, discard scheme, path, and query, store exactly `https://<host>` as a `String`.

Then **echo the normalized value back under the field** — "Will connect to `https://company.atlassian.net`" — so a silent mangling is visible before Test Connection runs. Every call site is then `stored + "/rest/api/3/…"` with no slash-juggling.

### Validation is two calls, and it gates Save

One **Test Connection** button running ticket 05's two proven probes in order:

1. `GET /rest/api/3/serverInfo` — **anonymous**. A failure here means the **URL** is wrong or unreachable, so the error can say exactly that instead of collapsing into "login failed". Also assert `deploymentType == "Cloud"` and warn loudly if not, since the entire API surface assumes Cloud.
2. `GET /rest/api/3/myself` — Basic auth. Success renders **"Connected as Jamey Smith"**, naming the account rather than showing a bare tick, which catches authenticating as the wrong account entirely.

Splitting these is the whole point: bad URL and bad token are different problems with different fixes, and ticket 05 proved they can be told apart.

**Save is disabled until Test Connection passes** — and, critically, **any keystroke in any of the three fields resets the state to idle and re-disables Save**. Without that rule you can pass a test, edit the site URL, and save a combination that was never verified; the gate would be decorative. Cheap to implement, and it is what makes the gate real.

The window is never trapped — close and Cancel always work. On a later edit, cancelling leaves the previously-good credentials intact.

### Scoped tokens are named twice — before and after

Ticket 05's most dangerous finding: a **scoped** token 401s on every endpoint including `/myself`. It is minted in the same Atlassian UI, it looks entirely valid, and it never authenticates. Untreated, this is comfortably the most likely first-run failure — and it presents exactly like a typo.

So the UI names it in both places:

- **Before**, under the token field: a line stating Punchcard needs a **classic (unscoped)** token, plus a link to `id.atlassian.com/manage-profile/security/api-tokens`.
- **After**, on a 401 from step 2: not "invalid credentials" but *"Jira rejected these credentials. If you created a scoped token, Punchcard needs a classic (unscoped) one."*

The copy stops at "these don't work here" and **does not explain why**. Ticket 05 observed 401 where a missing scope should give 403 and explicitly did not investigate it; error copy must not assert a mechanism nobody has verified.

Token field is a `SecureField` — this is a work Jira on a work machine.

### First run announces itself once; after that, "no credentials" is an ordinary state

The app has **no Dock icon**. On first launch after install nothing visibly happens at all — one more icon in a menu bar that already has twelve. That is a bad first thirty seconds, so first run gets special treatment, and only first run:

- **No stored site URL at all** (very first launch): open the Settings window automatically and activate the app.
- **Every subsequent launch**: never auto-open. The popover shows a gate view — "Punchcard needs your Jira details" plus an **Open Settings…** button.

So the un-credentialed state is recoverable and sit-in-able rather than a modal trap, but the app still introduces itself the one time it must.

### Credentials failing later is non-destructive and retryable

Tokens get revoked. A 401 will eventually arrive mid-session, possibly on a worklog POST. Any 401 from any call sets one global `credentialsInvalid` flag on the `AppStore`, and the popover shows a persistent banner with an **Open Settings** button.

Three rules matter more than the banner:

1. **Running timers keep running.** Timing is local and needs no Jira. Stopping the clock because auth broke would destroy real work for no reason.
2. **A worklog that 401s stays `pending`, never `synced`.** It retries later against repaired credentials. Building that state machine is slice 6's job; this ticket fixes only that **401 is a retryable outcome, not a terminal one** — which is a constraint slice 6 must honour.
3. **Never auto-delete the Keychain item on a 401.** A transient Jira outage must not silently wipe stored credentials.

### Re-opening Settings does not re-read the token

The token field is **empty on every visit**, captioned "Token stored in Keychain — leave blank to keep it."

Prefilling would mean reading a secret out of the Keychain into a live UI field on every visit for no benefit — dots cannot be proofread. Test Connection reads the Keychain when the field is blank and uses the typed value when it is not. That makes "change only the email" a two-second job rather than a trip to Atlassian to mint a replacement token.

### Layout

`Form`, three fields, ordered **in validation order** — Site URL → Email → API token — so the form reads top-to-bottom in the same sequence Test Connection checks it, and a failure always points at a field at or below the last one that passed.

```
Jira site URL   [company.atlassian.net              ]
                Will connect to https://company.atlassian.net
Email           [you@company.com                    ]
API token       [••••••••••••••••••••••••           ]
                Punchcard needs a classic (unscoped) token.
                Create one ↗

                [ Test Connection ]  ✓ Connected as Jamey Smith

                                      [ Cancel ]  [ Save ]
```

Test Connection has four states: idle, testing (spinner, disabled), passed, failed-with-a-specific-message.

**Settings contains credentials and nothing else in v1** — one section, so no tabs, no sidebar, no navigation to design. `SMAppService` / launch-at-login stays deferred per ticket 01; adding it later is an additive change to a form, not a restructure. Settings is reached from a **Settings…** item in the popover footer, next to Quit.

### No prototype

The ticket flagged a possible `/prototype` pass on the layout. **Declined.** Once the fields, their order, the four button states, and the re-edit behaviour are fixed, the screen is fully determined — there is no open "how should this look" question left for a prototype to answer, and building one would be mocking up a decision already made.

The genuinely undecided UI is the **running row at slice 6**, which the map's fog already records. That is where the prototype effort belongs.
