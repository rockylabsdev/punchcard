# The TUI

Bare `punchcard` in a terminal opens the full-screen app. It is one more writer of the same files the CLI uses: every key asks for a change and the screen shows what is now on disk. Quit with `q` and your timers keep running.

Every frame on this page is the app's own rendering at 80×24, taken from the golden test files under `internal/tui/testdata/TestFrames/`, with colour stripped. The app also lays out at 120×32 and anything in between.

## The frame

With no tracker, after a morning's work: one worklog logged, one timer running.

```
╭─[1] Tickets · today 1h 22m────────────────────╮╭─[2] invoices────────────────╮
│›▶ invoices                           0:42:34  ││                             │
│ · payroll                                40m  ││                             │
│                                               ││▶ RUNNING             0:42:34│
│                                               ││  stop logs this as one      │
│                                               ││  worklog — x                │
│                                               ││                             │
│                                               ││ 08:30–now   0:42:34 counted │
│                                               ││                             │
│                                               ││WORKLOGS TODAY               │
│                                               ││  nothing yet                │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
╰───────────────────────────────────────────────╯╰─────────────────────────────╯
╭─[3] Today · logged 40m───────────────────────────────────────────────────────╮
│ STATUS                        TICKET       TIME   STOPPED                    │
│ ✓ logged                      payroll       40m   07:45                      │
╰──────────────────────────────────────────────────────────────────────────────╯
                                                                                
 space pause  x stop…  a add  d remove  S setup  ? help  q quit                 
```

With Jira as the tracker, the same frame also tracks what is owed to Jira and what failed to reach it:

```
╭─[1] Tickets · today 1h 22m────────────────────╮╭─[2] PUN-142─────────────────╮
│›▶ PUN-142       Wire flock on the s… 0:15:34  ││Wire flock on the side       │
│ ‖ PUN-138       Stop dialog: durati…     12m  ││state.lock                   │
│ · OPS-77        Rotate staging certs     15m ○││                             │
│ · PUN-120       Spike: Bubble Tea v…     40m ✗││▶ RUNNING             0:15:34│
│ · DATA-9        ? unverified — Jira…       —  ││  stop sends this as one     │
│ · PLATFORM-4410 Investigate intermi…       —  ││  worklog — x                │
│                                               ││                             │
│                                               ││ 08:58–now   0:14:34 counted │
│                                               ││ 08:15–08:58 ‖ paused 43m    │
│                                               ││             not counted     │
│                                               ││ 08:14–08:15      1m counted │
│                                               ││                             │
│                                               ││WORKLOGS TODAY               │
│                                               ││  nothing yet                │
│                                               ││                             │
╰───────────────────────────────────────────────╯╰─────────────────────────────╯
╭─[3] Today · logged 0m · owed 55m─────────────────────────────────────────────╮
│ STATUS                        TICKET       TIME   STOPPED                    │
│ ✗ failed — 503 · r            PUN-120       40m   07:45                      │
│ ○ not sent yet · r            OPS-77        15m   08:01                      │
╰──────────────────────────────────────────────────────────────────────────────╯
 ✗ 1 worklog failed to reach Jira · r to retry · nothing is lost                
 space pause  x stop…  a add  d remove  r sync 2  S setup  ? help  q quit       
```

Top to bottom: the Tickets pane and the Ticket pane side by side, the day's log, the status line, the key bar.

### [1] Tickets

What you are tracking and where today went. About 62% of the width, never under 44 columns. Rows are in recents order, so the running ticket is always first. Each row:

| Column | Meaning |
| --- | --- |
| `›` | The cursor. The row is also highlighted. |
| symbol | The ticket's state (table below). |
| key | The ticket key. The column widens to the longest key. |
| summary | From the tracker, cut with `…` to fit; blank with no tracker. With Jira, an unverified ticket says `? unverified — Jira not reached yet`. A row changed from outside shows a tag first, such as `started by agent · Rotate staging certs`, for 6 seconds. |
| number | **Time today** for the ticket. The running row ticks as `h:mm:ss`; others show `1h 12m`, or `—` for none. |
| marker | With a tracker: `✗` if any of its worklogs failed, `○` if any is waiting to be sent. Blank otherwise, and always blank with no tracker. |

The pane title carries the day's total across every ticket. With no tickets it says `No tickets yet. Press a to add one by key.`, and with no tracker one more line, `no tracker · S to set one up`, which goes once the list has a ticket:

```
╭─[1] Tickets · today 0m────────────────────────╮╭─[2] Ticket──────────────────╮
│                                               ││                             │
│  No tickets yet.                              ││                             │
│  Press a to add one by key.                   ││                             │
│                                               ││                             │
│  no tracker · S to set one up                 ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
│                                               ││                             │
╰───────────────────────────────────────────────╯╰─────────────────────────────╯
╭─[3] Today · logged 0m────────────────────────────────────────────────────────╮
│ nothing logged today — x on a row stops it and logs it here                  │
╰──────────────────────────────────────────────────────────────────────────────╯
                                                                                
 a add  S setup  ? help  q quit                                                 
```

When the list is longer than the pane, it scrolls to keep the cursor in view.

### [2] Ticket

What happened on the selected ticket. The full summary, then a headline that is **the time stop would log**:

| Headline | When |
| --- | --- |
| `▶ RUNNING 0:15:34` | The open entry is counting. |
| `‖ PAUSED 22m` | The open entry is suspended. |
| `! RECOVERED 45m` | Recovery capped and paused it. |
| `· NOT RUNNING` with `space starts it` | No open entry. |

Under it, `stop logs this as one worklog — x` (with Jira, `stop sends this as one worklog — x`), then the entry's stretches newest first. A counted stretch puts its length in the number column. A gap carries its length in its label, so the column cannot be summed by eye:

```
 06:50–now   ! away 2h 22m
             not counted
 06:20–06:50     30m counted
 06:15–06:20 ‖ paused 5m
             not counted
 06:00–06:15     15m counted
```

`! away` is time cut off by recovery. A running entry that was recovered earlier says `recovered earlier — check the duration at stop` under the headline.

Last, `WORKLOGS TODAY` for this ticket: owed worklogs from any day, logged ones from today, in the same words as the day's log, or `nothing yet`.

### [3] Today

The day's log. With no tracker the title is `[3] Today · logged 40m` and every row is `✓ logged`: a worklog is final the moment it stops.

With Jira the title adds what is owed and when Jira last took a worklog, `[3] Today · logged 30m · owed 55m · last sync 08:17`, or in place of the last sync why nothing can sync: `token rejected`, `Keychain error` or `not connected`. The rows lead with the outcome, worst first, newest first within each:

| Row | Meaning |
| --- | --- |
| `✓ logged` | Logged, with no tracker. |
| `✓ logged 05:46` | With Jira: in Jira, at that time. |
| `✗ failed — 503 · r` | With Jira: Jira refused it (the HTTP status), or `unreachable`. `r` retries. |
| `○ not sent yet · r` | With Jira: pending. `r` sends it. |

Owed worklogs from any day are listed; logged ones only from today. The strip is sized to its rows, at most 5 at 80×24 (a quarter of the height), with the last row `+N more` when they do not fit. With nothing to show: `nothing logged today — x on a row stops it and logs it here`.

### The status line

One slot, filled in this order:

1. A **toast** for 5 seconds: a refusal (`nothing is running`), a notice (a corrupt file moved aside), or an outcome (`stopped PUN-142 · logged 15m`). While `r` is sending: `○ syncing…`.
2. The **banner**, with Jira only: the worst condition that holds now. With no tracker there is never a banner. It cannot be dismissed and clears only when the fact clears. Worst first:
   - `✗ Jira rejected the token at 09:12 · 2 worklogs waiting · S to fix`
   - `Keychain error: <cause> · worklogs wait here, nothing is lost`
   - `Not connected to Jira · S to set up · worklogs wait here, nothing is lost`
   - `✗ 1 worklog failed to reach Jira · r to retry · nothing is lost`
3. The **last event**, with its time and who caused it: `09:12 you started PUN-142`, `09:13 agent started OPS-77`, `09:14 prompt paused PUN-142`.

### The key bar

Only the keys that apply now. `space` reads `start`, `pause` or `resume` to match the selected row. `x stop…` appears when the row has an open entry. `p pause` appears when something runs and the cursor is on another row. `r sync 2` appears when 2 worklogs are owed. Inside a dialog the bar shows the dialog's keys.

## Keys

| Key | Does |
| --- | --- |
| `space`, `enter` | Start, pause or resume the selected ticket. Starting one pauses whatever ran. With Jira, on an unverified ticket it also asks Jira for the summary. |
| `p` | Pause whatever is running, from any row. |
| `x` | Open the stop dialog for the selected ticket's open entry. On a row with none: `nothing to stop on OPS-77 — space starts it`. |
| `a` | Open the add dialog. |
| `d` | Open the remove dialog for the selected ticket. |
| `r` | With Jira: send every pending and failed worklog. With nothing owed: `nothing to send: Jira has everything`. While a sync is already out, nothing. With no tracker: `no tracker · nothing is sent · S to set one up`. |
| `S` | Suspend the TUI, run `punchcard setup` on the same terminal, then redraw and re-read the store and the tracker. Setup is where a tracker is chosen. |
| `?` | Help: every key and glyph, over the whole body. Any key closes it. |
| `j`, `↓` | Move down. |
| `k`, `↑` | Move up. |
| `g`, `home` | First row. |
| `G`, `end` | Last row. |
| `q` | Quit. Timers keep running. |
| `ctrl+c` | Quit from anywhere, dialogs included. |

Every key the TUI acts on is presence (at most one write per 30 seconds). Moving the cursor and reading never changes a timer. An open TUI nobody touches is not presence.

## Symbols and colours

The colours are the terminal's own 16-colour theme, so they follow light or dark. The one exception is Punchcard's brand colour, a chartreuse (`#C8FF00` on a dark background, a darker `#5F8700` on a light one): the running state, the focused pane's border and title, and the cursor. Every state also has a symbol, so the screen reads with no colour at all. Blue is reserved for anything about sending to a tracker.

| Symbol | Colour | Meaning |
| --- | --- | --- |
| `▶` | chartreuse, bold | running: the brand colour |
| `‖` | yellow | paused (the CLI prints `⏸` for the same thing) |
| `!` | magenta, bold | recovered: check the duration at stop |
| `·` | grey | idle: no open entry |
| `?` | grey | unverified: not confirmed by the tracker yet (Jira only) |
| `○` | blue | a worklog waiting to be sent to the tracker; also the `r` key everywhere it appears |
| `✗` | red, bold | a failed worklog, a refusal, the worst banners |
| `✓` | grey | a logged worklog (with Jira, one in Jira) |
| `›` | chartreuse, bold | the cursor |
| yellow text | — | the "not connected" and Keychain banners (Jira only) |
| italic text | — | a row's "changed elsewhere" tag |

On a 16-colour terminal the brand falls back to bright green, the greys to bright black, and the row highlight to the terminal's own background, leaving `›` to mark the row.

## Dialogs

A dialog is laid over the Ticket pane at exactly its size, so opening one never moves anything else. On a window too narrow for that pane it covers the whole body. While a dialog is open it takes every key but `ctrl+c`.

### Stop (`x`)

With no tracker:

```
╭─Stop invoices───────────────╮
│invoices                     │
│measured 0:42:34 across 1    │
│stretch                      │
│                             │
│Duration                     │
│  42m                        │
│Comment                      │
│  optional                   │
│                             │
│→ logs 42m as one worklog    │
```

Key bar: `enter stop & log  tab field  esc cancel — keep timer`.

With Jira:

```
╭─Stop PUN-142────────────────╮
│PUN-142 Wire flock on the si…│
│measured 0:15:34 across 2    │
│stretches                    │
│                             │
│Duration                     │
│  15m                        │
│Comment                      │
│  optional                   │
│                             │
│→ sends 15m to Jira as one   │
│worklog                      │
```

Key bar: `enter stop & send  tab field  esc cancel — keep timer`.

The duration is prefilled with the measured time truncated to the minute and keeps following the clock until you type. It takes `1h 20m`, `1h20m`, `1h`, `80m`, `80` (minutes) or `1:20`, from 1 minute to 24 hours; spaces are ignored. The last line previews what will be logged, or says why it cannot: `✗ under a minute: type a duration`, `✗ can't read that duration`. `tab`, `shift+tab`, `↑` and `↓` move between the two fields. `enter` stops the entry and logs the worklog; the status line says `stopped invoices · logged 42m`. With Jira it sends it too: `sending 15m for PUN-142 to Jira…`, then `stopped PUN-142 · logged 15m`, or `stopped PUN-142 · 15m not sent: Jira not reachable · nothing is lost`. `esc` closes the dialog and leaves the timer exactly as it was.

The dialog is pinned to the entry it opened on. If that entry was stopped from elsewhere meanwhile, enter is refused: `nothing is running`.

A recovered entry is never prefilled:

```
╭─Stop PUN-142────────────────╮
│PUN-142 Wire flock on the si…│
│measured 0:45:00 across 2    │
│stretches                    │
│! recovered — capped at      │
│06:50, your last presence;   │
│how long did you work?       │
│                             │
│Duration                     │
│  e.g. 1h 20m                │
│Comment                      │
│  optional                   │
│                             │
│type the time you actually   │
│worked                       │
```

### Add (`a`)

```
╭─Add ticket──────────────────╮
│Ticket key                   │
│› PUN-123                    │
│enter adds it · ↓ picks a    │
│recent one                   │
│                             │
│recent                       │
│  PUN-142 Wire flock on the …│
│› PUN-138 Stop dialog: durat…│
│  OPS-77 Rotate staging certs│
```

Key bar: `enter add  ↑↓ recents  esc cancel`.

Type a key, or pick one of up to 8 recents with `↓` and `↑` (the list filters on what you have typed, matching keys and summaries). With no tracker a key is any slug of letters, digits, `.`, `_` and `-` (`payroll`, `ABC-123`), kept as typed, and it is added at once. With Jira it must be shaped like `ABC-123`. A key that does not fit is refused in place with `<key> is not a ticket key (like …)`, naming an example. A key already on the list and confirmed closes the dialog and moves the cursor to it. Otherwise, with Jira, the dialog asks Jira (`asking Jira about PUN-123…`, keys wait meanwhile): found adds it with its summary and selects it; a 404 keeps the dialog open with `PUN-123 not found — or not visible to this Jira account`; unreachable or unconfigured adds it unverified. `esc` cancels.

### Remove (`d`)

```
╭─Remove PUN-142?─────────────╮
│PUN-142 Wire flock on the si…│
│                             │
│✗ its unsent 15m would be    │
│lost: it never reaches Jira  │
│                             │
│y removes it from your list ·│
│any other key keeps it       │
```

Key bar: `y remove  any key keep it`. The warning line appears only when the ticket has an open entry or, with Jira, an unsent worklog; with no tracker it reads `its unlogged 15m would be lost: it is never logged`. `y` then removes it anyway. Remove takes the ticket, its history and any unlogged or unsent time off your list. Nothing in a tracker changes.

### Help (`?`)

```
╭─Help─────────────────────────────────────────────────────────────────────────╮
│ Keys                                                                         │
│  space / enter   start · pause · resume the selected ticket                  │
│  p               pause whatever is running, from any row                     │
│  x               stop the selected entry: review, then log it                │
│  a / d           add a ticket / remove the selected one                      │
│  r               sync: send pending and failed worklogs                      │
│  S               setup: a tracker, the Claude Code plugin                    │
│  j k ↑ ↓ g G     move                                                        │
│  q               quit; timers keep running without the TUI                   │
│                                                                              │
│ Glyphs                                                                       │
│  ▶ running   ‖ paused   ! recovered: check the duration at stop              │
│  · idle   ? not confirmed by the tracker yet                                 │
```

## Changes made elsewhere

Once a second the TUI checks whether `state.json` was replaced (another terminal, the Claude Code plugin, an agent). When it was, it re-reads and compares. A row whose timer moved gets a tag for 6 seconds, `started by agent`, `resumed by prompt`, `paused by hand`, `stopped by hand`, and the status line logs it:

```
│ ▶ OPS-77        started by agent · … 0:15:00 ○││
│›‖ PUN-142       Wire flock on the s…     22m  ││
…
 agent started OPS-77
```

Adds, removes, verifications and syncs are not tagged. The check is a read: an untouched TUI never writes.

## Not connected (Jira only)

With no tracker there is nothing to connect, and nothing here applies. With Jira chosen in setup but no working connection (no credentials, or the Keychain unreadable), the TUI is still fully usable. Starts, pauses and stops work; worklogs wait. The banner says so and the day's log title says `not connected`:

```
╭─[3] Today · logged 0m · owed 55m · not connected─────────────────────────────╮
…
 Not connected to Jira · S to set up · worklogs wait here, nothing is lost      
```

## Exit codes

| Code | When |
| --- | --- |
| 0 | `q` or `ctrl+c`. |
| 1 | The terminal could not be set up; the message is printed. |
| 130 | Interrupted from outside (a signal). Timers are untouched. |

Why the TUI polls instead of watching, and never saves its own copy of the state: [ADR 0002](../adr/0002-flock-on-a-side-lock-no-cached-writes.md).
