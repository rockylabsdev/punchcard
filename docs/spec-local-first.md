# Punchcard — Local-first Spec (tracker as adapter, `.punchcard`, `log`)

Status: ready-for-agent

Derived from ADR [0008](adr/0008-local-first-tracker-as-adapter.md), which records every decision below, and from the v2 spec at [`docs/spec.md`](spec.md), which this spec amends and does not replace. Domain vocabulary is authoritative in root [`CONTEXT.md`](../CONTEXT.md); this spec adds **Tracker** and **local** to it. Nothing is released yet, so this lands before the first tag and changes no schema version.

## Problem Statement

Punchcard times my work well, but it only makes sense if I have Jira. Every surface assumes a remote exists: the TUI's third pane is called Jira, the band above my prompt turns yellow with "Jira not connected", `status` says I am not configured, every stopped entry is "owed", and the first run sends me to a credentials wizard. If I don't have Jira, or I have stopped using it, the app apologises at me continuously for a thing I never asked for.

The timing itself is the part I value: it follows my attention between sessions, it recovers a timer I forgot, it never double-counts the hours I lived. None of that needs a remote. What I need without one is a way to *see* the time once it's final: a week's worth of worklogs I can paste into a timesheet, an invoice, a standup message, or import into whatever tracker my team happens to use. Today the only view of that is the TUI, one ticket at a time, and the only export is reading `state.json`.

Two smaller things fall out of the same assumption. A ticket has to look like a Jira key, so without Jira I'd have to invent fake ticket numbers. And the plugin only knows which ticket a session is for when the git branch carries a Jira key, so on `main`, or in a repository whose branches don't name tickets, the timer never follows me.

## Solution

Punchcard becomes a **time tracker first**. A machine with no configuration times work with no ceremony: `punchcard start payroll` works on a blank install, the TUI opens to an empty list with one quiet hint, and nothing is ever owed to anyone. Sending time somewhere is a feature on top: a **tracker**, chosen in `setup`, of which Jira is the first and only one. With a tracker configured, everything a Jira user has today is unchanged: summaries are fetched, stops are posted, debt is tracked and retried, the band and the strip show standing with Jira.

A stopped entry's worklog is **final at stop** whether or not a tracker exists. Without one its state is **local**; with one it is pending until the tracker takes it. Removing a tracker offers, once, to mark everything still owed as local.

A **ticket key** is whatever the tracker says it is. Without one it is a slug: `payroll`, `INV-2041`, `docs.guide`. With Jira it is a Jira key, as now. A repository can name its ticket in a **`.punchcard` file**, one key on the first line, which the plugin's switching uses before it looks at the branch. The binary resolves a session's key in one place and the plugin asks for it.

**`punchcard log`** lists worklogs over a range as JSON, CSV or Markdown: one row per worklog, grouped by day with totals in Markdown, a header and import-friendly columns in CSV. It is a read, never presence, never a tracker call. CSV is how a trackerless user reaches Clockify, Toggl, Harvest, Tempo or a spreadsheet; Markdown is for the standup and the invoice; JSON is for an agent answering "what did I do this week".

## User Stories

### Timing work without a tracker

1. As a user with no Jira, I want `punchcard start payroll` to work on a blank machine, so that I can time work the moment I install the app.
2. As a user with no Jira, I want to pause, resume, switch and stop exactly as a Jira user does, so that the timer's rules (one timer, held, focus, recovery) are the same for everyone.
3. As a user with no Jira, I want a stopped entry's worklog to be final and never "owed", so that nothing nags me to send it anywhere.
4. As a user with no Jira, I want `status` and `ls` to show no warnings about being unconfigured, so that a clean state reads as clean.
5. As a user with no Jira, I want the TUI to open to my list with no banner about Jira, so that the app is quiet.
6. As a user with no Jira, I want the TUI's empty list to tell me once, in one line, that there is no tracker and how to set one up, so that I know the option exists without being nagged.
7. As a user with no Jira, I want to remove a ticket whose worklogs are all local without a warning about unlogged time, so that local worklogs count as logged.
8. As a user with no Jira, I want to type a summary when I add a ticket, so that my list reads as well as a Jira user's does.
9. As a user with no Jira, I want `start NEWKEY` to add the ticket with no summary and no "unverified" mark, so that a ticket is never shown as unconfirmed by a tracker I don't have.
10. As a user with no Jira, I want keys like `payroll`, `INV-2041` and `docs.guide` to be accepted, so that I can name work the way my team does.
11. As a user with no Jira, I want keys with spaces, slashes or quotes refused with a usage error, so that a key is always safe in a CSV cell, a branch name and a shell.
12. As a user with no Jira, I want keys to be case-sensitive and kept as I typed them, so that what I see is what I wrote.
13. As a user with no Jira who later adds Jira, I want my existing keys and worklogs left alone, so that switching on a tracker loses nothing.

### A tracker as an adapter

14. As a Jira user, I want everything I have today to work unchanged with `tracker: jira`, so that this change costs me nothing.
15. As a Jira user, I want my existing config file (site and email, no tracker field) to be read as Jira, so that I don't have to run setup again.
16. As a user, I want `status --json` to say which tracker is configured (`none` or `jira`), so that the plugin and scripts can tell the two modes apart.
17. As a user, I want exit 3 (not configured) only when a command actually needs a tracker that isn't there, so that `start` and `stop` on a trackerless machine exit 0.
18. As a user, I want exit 5 (ticket not found) only when a tracker looked and said no, so that a trackerless `start` never refuses a key.
19. As a user, I want exit 7 (stopped, worklog owed) never to occur without a tracker, so that an agent reading the exit code never runs `sync` for nothing.
20. As a user, I want exit 8 (auth failed) only when Jira is configured and rejected the token, so that the code keeps its meaning.
21. As a user removing Jira in `setup`, I want to be offered, once, to mark every pending and failed worklog as local, so that I can leave Jira with no debt showing.
22. As a user removing Jira who declines that offer, I want the worklogs to stay owed, so that I can reconfigure Jira later and send them.
23. As a user, I want no CLI flag that marks a pending worklog local, so that an agent can never forgive debt on my behalf.
24. As a user, I want `sync` with no tracker to exit 3 and say there is no tracker, so that the command is honest rather than silently a no-op.
25. As a Jira user, I want a Jira key to be upper-cased on entry as it is today, so that `abc-123` and `ABC-123` are the same ticket.

### Knowing which ticket a session is for

26. As a plugin user, I want a `.punchcard` file in my repository to name the ticket, so that sessions there give focus to it whatever the branch is called.
27. As a plugin user, I want the file to be one key on the first line, with `#` comment lines ignored, so that it is trivial to write by hand.
28. As a plugin user in a monorepo, I want the nearest `.punchcard` above my session's directory to win, stopping at the repository root, so that each package can name its own ticket.
29. As a plugin user, I want the file to take precedence over the branch name, so that an explicit choice beats a convention.
30. As a plugin user on a branch named `feature/ABC-123-login` with no file, I want `ABC-123` to be the session's key exactly as today, so that current users notice nothing.
31. As a plugin user, I want a `.punchcard` whose first line is not a valid key to count as no file, with a notice naming the path, so that a typo is visible and not silently ignored.
32. As a plugin user, I want `/punch start` with no key to use the file's key before the branch's, so that one keystroke starts the right ticket.
33. As a plugin user, I want the band to name this session's key from the same source, so that the band and the switching never disagree.
34. As a plugin author, I want `status --json --cwd DIR` to report `here` with the key and its source (`file` or `branch`), so that the plugin never carries its own regex.
35. As a plugin user, I want focus to behave as it does today (switches an un-held running timer, adds the ticket if needed, never starts from idle, never moves a held timer), so that `.punchcard` changes where the key comes from and nothing else.
36. As a Jira user whose branches don't carry keys, I want `.punchcard` to work for me too, so that the file is not a trackerless-only feature.

### Seeing the time

37. As a user, I want `punchcard log` with no arguments to list this week's worklogs, Monday through today, so that the common case needs no flags.
38. As a user, I want `--month` for the calendar month through today, so that the month-end timesheet is one command.
39. As a user, I want `--from DATE --to DATE` as inclusive local dates, so that any range is reachable and the dates are the ones I'd type.
40. As a user, I want `log --md` to show a heading per day with that day's total, a table of `key | summary | time | comment` under it, and a grand total at the end, so that it reads as a timesheet in GitHub, Obsidian or a PR body.
41. As a user, I want `log --md` to mark a pending or failed worklog on its row and count them in the day heading, rather than carry a state column, so that the exceptions stand out and the normal case is clean.
42. As a user, I want `log --md` to end with a line naming a running or paused entry and its time so far, so that the total is never silently short of the hours I'm still on.
43. As a user, I want `log --csv` with a header row and the columns `date, started, key, summary, minutes, comment, state, remote_id, entry_id, recovered`, so that every tracker and spreadsheet imports it.
44. As a user, I want `minutes` as an integer and `date`/`started` as local date and time, so that imports need no parsing of `h:mm`.
45. As a user, I want `entry_id` in the CSV, so that a re-export can be de-duplicated against an earlier one.
46. As a user, I want `recovered` in the CSV, so that a reviewer can see which durations were corrected by hand after a recovery.
47. As a user, I want `log --json` to carry the same rows plus the open entry as a separate field, so that an agent can report the week and what is still running.
48. As a user, I want `log` to exit 0 with empty rows when nothing is in range, so that scripts don't treat an empty week as an error.
49. As a user, I want `log` never to call a tracker and never to count as presence, so that a report can't move a timer or recover one.
50. As a user, I want one row per worklog and no row for an open entry, so that a row is always a final figure.
51. As a user, I want a worklog's day to be the local day it started, as CONTEXT.md defines it, so that the grouping matches what the TUI shows as today.
52. As a user, I want `log --md` to be quoted and escaped so that a comment containing `|` or a newline cannot break the table, so that the output is always valid Markdown.
53. As a user, I want `log --csv` to quote fields with commas, quotes or newlines per RFC 4180, so that the file always imports cleanly.
54. As an agent, I want `log --json` under the usual envelope with `command: "log"`, so that the output follows the contract every other command does.

### The TUI

55. As a user, I want the third pane to be the day's log (logged today, and with a tracker, what is owed), so that the pane is useful without Jira.
56. As a Jira user, I want the pane to keep showing owed and failed worklogs with `r` to retry, so that the Jira workflow is intact.
57. As a user, I want the stop dialog to say "log" rather than "send to Jira" when there is no tracker, so that the words match what will happen.
58. As a user, I want the status line's worst-condition banner to show Jira conditions (auth failed, Keychain, not connected) only when Jira is the tracker, so that a trackerless user never sees them.
59. As a user, I want the help screen to describe `S` as "setup: a tracker, the Claude Code plugin", so that help matches the new shape.

### Setup

60. As a user running `setup`, I want the first question to be which tracker, none or Jira, so that a trackerless user is done in one step.
61. As a user choosing Jira, I want the site, email, token and Test Connection steps exactly as today, so that the Jira path is unchanged.
62. As a user choosing none, I want the plugin and ask-rule steps still offered, so that the plugin is set up for everyone.
63. As a user, I want a blank machine never to be forced into the wizard, so that the first run lands in the TUI or at the command I typed.
64. As a user re-running `setup` with Jira configured, I want the option to remove it, with the mark-local offer, so that leaving Jira is a wizard step and not a file edit.
65. As a user, I want `setup` to write `tracker` into the config file, so that the choice is explicit on disk.

### The plugin and the skill

66. As a plugin user with no tracker, I want the band to show only the timer, so that nothing warns me about a thing I don't have.
67. As a plugin user with Jira, I want the band's auth, Keychain and not-connected words exactly as today, so that the Jira experience is unchanged.
68. As a plugin user, I want the band's reminder of this session's key not to say "this branch is", so that it is true when the key came from a file.
69. As a plugin user, I want an agent's gated stop to ask "Log it" without a tracker and "Send to Jira" with one, so that the dialog says what the stop does.
70. As an agent, I want the skill to tell me to read `here` from `status --json --cwd` rather than regex a branch, so that I find the session's key the way the plugin does.
71. As an agent, I want the skill's exit-code glossary to say when 3, 5, 7 and 8 can occur, so that I never run `sync` or send the user to `setup` on a trackerless machine.
72. As an agent, I want the skill to teach `log --json` for "what did I work on", so that I answer from the data rather than the TUI.

### Documentation

73. As a reader, I want CONTEXT.md to define Tracker and local, and to describe Ticket, Unverified, Worklog, Stopped and Focus without assuming Jira, so that the glossary matches the app.
74. As a reader, I want the README and the user guide to lead with the trackerless path and present Jira as optional, so that the first impression is the new identity.
75. As a reader, I want `.punchcard` and `log` documented in the guide, so that both are discoverable.

## Implementation Decisions

### The tracker seam — ADR [0008](adr/0008-local-first-tracker-as-adapter.md)

The remote surface becomes a **tracker** package owning the client interface, the worklog and account value types, and a tracker-neutral error set: not found, unauthorized, unreachable, bad site, and a refusal carrying an HTTP-like status and message. The Jira package becomes one implementation of that interface and keeps its wire types, its ADF comment and the word "issue" exactly as now. Every caller (the worklog sender, the CLI, the TUI, the setup wizard, the status problem classifier) matches the neutral errors and never imports Jira except where a Jira client is constructed. The TUI stops parsing the text of a Jira error to find a status and reads the refusal's status field instead.

The interface gains one answer beyond the current three: the **key grammar**. The core's key validation delegates to it. The trackerless grammar is a slug (`[A-Za-z0-9][A-Za-z0-9._-]*`), case-sensitive, stored as typed. Jira's grammar is the current `ABC-123` shape with upper-casing on entry, which is a subset of the slug. The grammar is a property of the configured tracker, resolved once per invocation and passed into the core; the core does not read config.

The "open a client" function every caller already takes stays the seam. With `tracker: none` it returns a sentinel meaning "no tracker", distinct from "not configured": the sender, lookups and `sync` treat it as nothing to do, not as an error to apologise for. The lookup path (`start NEWKEY`, the TUI's add) never runs without a tracker; the ticket is added with an empty summary and `Unverified` false.

### Config

The config file gains `tracker`, one of `none` or `jira`. An absent file means `none`. A file with site and email and no `tracker` reads as `jira`, so the one existing dev machine needs no edit. `setup` always writes the field. The token lookup chain is untouched and runs only when the tracker is Jira. Being unconfigured in the old sense (Jira chosen but no credentials) keeps exit 3 and its messages.

### The local worklog state — ADR [0008](adr/0008-local-first-tracker-as-adapter.md)

`Worklog.State` gains `local`. Stop decides the state from the tracker in effect: `local` with none, `pending` with one. The derived views (`Unsent`, `Owed`, the remove guard, the TUI strip, the band's `unsent`) treat `local` exactly like `synced`: logged, never owed. `MarkSynced`, `MarkFailed` and the retry paths refuse a local worklog as they refuse a synced one. A new command marks a pending or failed worklog local, used only by the setup wizard's removal offer: it clears the failure message and timestamp and keeps everything else. No schema version change: nothing is released.

### The CLI contract — ADR [0004](adr/0004-cli-contract-exit-codes-json-envelope-origin.md), amended

Contract version stays 1; every change is additive.

- `status --json` gains `tracker` (`"none"` or `"jira"`). The existing `configured`, `tokenSource`, `keychainError` and `notConfigured` fields are present only when the tracker is Jira.
- `status --json --cwd DIR` gains `here`: `{ key, source }` with `source` one of `"file"` or `"branch"`, or `null` when neither names a key. A `.punchcard` with an invalid first line produces a notice on stderr (and in the envelope's notices) naming the path, and `here` falls through to the branch.
- `touch --cwd DIR` resolves its key the same way, through the same function.
- `worklog.state` gains `"local"`.
- Exit codes narrow: 3 only when a tracker is configured and a command needs it (or `sync` with no tracker, which reports that there is none); 5 only when a tracker verified the key and said no; 7 never without a tracker; 8 only with Jira. 0, 1, 4, 6, 64, 65 and 75 are unchanged.
- `log` is a new read command, exit 0 always (64 for bad arguments), never presence, never a tracker call.

### `log`

One row per stopped entry whose worklog day falls in the range. The day is the worklog's `startedAt` in local time. Ranges: default and `--week` is Monday of the current week through today; `--month` is the first of the current month through today; `--from`/`--to` are inclusive local dates in `YYYY-MM-DD`, either may stand alone (the other defaulting to the week's edge), and `--from` after `--to` is usage. Rows are ordered by day, then by `started`.

- `--json`: the envelope with `data.rows` (each row: `date`, `started` as the full timestamp, `key`, `summary`, `minutes`, `comment`, `state`, `remoteId`, `entryId`, `recovered`), `data.open` (the running or paused entry as the `Timer` shape `status` already uses, or `null`), and `data.range` (`from`, `to`).
- `--csv`: RFC 4180, header `date,started,key,summary,minutes,comment,state,remote_id,entry_id,recovered`; `started` as local `HH:MM`; `recovered` as `true`/`false`; empty strings for absent values.
- `--md`: for each day, a level-2 heading `YYYY-MM-DD Day · Nh NNm` (with ` · N pending` or ` · N failed` appended when any); a table `key | summary | time | comment` with `time` as `h:mm`; a trailing `**Total: Nh NNm**` line; and, when an entry is open, a final line `Running: KEY h:mm, not logged` or `Paused: …`. Pipes and newlines in cells are escaped or replaced so the table is always valid. A pending or failed row ends its comment cell with ` · pending` or ` · failed`.
- Plain output (no format flag, on a TTY or not) is the Markdown.

`log` reads the store once and derives everything from the views; it writes nothing and never touches the heartbeat.

### The session key and `.punchcard`

One function in the CLI answers "what is this directory's key": look for `.punchcard` at the directory and each ancestor up to and including the git repository root (or the filesystem root with no repository); the first found wins. Read its first non-blank, non-`#` line, trimmed. If it is a valid key under the current grammar, the answer is that key with source `file`. Otherwise (no file, or an invalid line, which also produces the notice) fall through to the current branch rule: the first `ABC-123`-shaped token in the branch name, with source `branch`. `touch --cwd`, `status --cwd` and `/punch start` all go through this function; the plugin's own key regex is deleted and the plugin reads `here` from the status poll it already makes.

### The TUI

The third pane's title becomes the day's log. Without a tracker it shows logged today; with one it adds owed and failed with the existing retry. The status line's Jira conditions are gated on the tracker being Jira. The stop dialog's "sends … to Jira as one worklog" line becomes "logs …" without a tracker. The empty-list screen gains one line, "no tracker · S to set one up", shown only while the list is empty and the tracker is none. Help text follows.

### Setup

The wizard's first step is the tracker: none or Jira, prefilled from config. None skips to the plugin steps. Jira runs the current steps. On a re-run with Jira configured, choosing none offers to mark every pending and failed worklog local, showing the count and total time; declined, they stay. The wizard writes `tracker` on every completion. The first-run auto-launch of the wizard (no config and no state) is removed; a blank machine runs the command it was given.

### The plugin and the skill

The band reads `tracker` and `here`. With `tracker: none` the Jira conditions are never shown and `configured: false` is not a warning. The session-key reminder stops saying "this branch is". The gate dialog's affirmative answer is "Log it" when the tracker is none and "Send to Jira" when it is Jira; the gate itself, its `ask` verdict and the fail-closed behaviour are unchanged (ADR [0005](adr/0005-agent-stop-gated-by-mod-and-ask-rule.md)). The skill is rewritten: the key comes from `here`, never from a regex; the exit-code glossary states when 3, 5, 7 and 8 can occur; `log --json` is the way to report a period.

### Build order

A slice is **a commit that builds, runs, is green in CI on both platforms, and has one hand check**. Seven slices, one agent session each.

| # | Slice | Lands | Hand check |
|---|---|---|---|
| 1 | **Tracker seam + config** | The tracker package with the neutral errors and the key-grammar answer; Jira as its implementation; every caller matching neutral errors; `tracker` in config with the absent-file and absent-field rules; `status --json` reporting `tracker`; the slug grammar without a tracker. Jira behaviour unchanged. | `status --json` shows `tracker: jira` on the dev machine; with `HOME` pointed at an empty dir, `start payroll` exits 0 and `status --json` shows `tracker: none` |
| 2 | **Local worklogs + quiet CLI** | `local` at stop without a tracker; the views, remove guard and sync paths treating it as logged; exit codes narrowed; the mark-local command (core and store only, no CLI surface); CLI copy without "Jira" where no tracker is involved. | Blank `HOME`: `start`, `stop --yes --duration 30`, `status` shows nothing owed, exit 0; `rm` removes without a warning |
| 3 | **`log`** | The read in all three formats, the three ranges, the open-entry footer and field, CSV quoting, Markdown escaping; golden files for `--md` and `--csv`. | A real week of local state: `log --md` pasted into a PR body renders as a timesheet; `log --csv` opens in Numbers with the right columns |
| 4 | **`.punchcard` + `here`** | The session-key function; `status --cwd` reporting `here` and its source; `touch --cwd` using it; the invalid-file notice. The plugin's regex deleted and the band and `/punch start` reading `here`. | In a repository on `main` with a `.punchcard` naming a ticket, an un-held timer on another ticket moves to it on the next prompt and the band names it |
| 5 | **TUI** | The day's-log pane, the gated Jira conditions, the stop dialog wording, the empty-list note, help. Goldens: empty list without a tracker, the pane with and without a tracker, the stop dialog without a tracker. | Blank `HOME`: bare `punchcard` opens to the empty list with the one-line note and no banner; add a ticket, the note goes |
| 6 | **Setup** | The tracker step first; none skipping to the plugin steps; the removal offer with the count; `tracker` written; the first-run auto-wizard removed. | `setup` on the dev machine, choose none, accept the offer: `status` shows nothing owed; choose Jira again, the credentials are still in the Keychain |
| 7 | **Plugin dialog, skill, docs** | "Log it" / "Send to Jira"; the skill rewrite; README and the guide updated for the trackerless path, `.punchcard` and `log` (CONTEXT.md is already updated). | An agent's `stop --yes` in a trackerless repo asks "Log it"; the skill's instructions make an agent read `here` |

Orderings that are decisions:

- **The seam first, with a hand check.** The rename alone is refactor-only; carrying `tracker` in config and in `status --json` with it gives slice 1 something to run and proves the no-tracker path before any behaviour depends on it.
- **`local` before `log`.** `log` has a `state` column and a trackerless user's rows must say `local`, not `pending`.
- **`.punchcard` lands with the plugin's half.** The regex deletion and `here` are one change; splitting them would leave the band and the switching disagreeing for a slice.
- **The TUI and setup after the contract** so their goldens and transcripts are written against final words.
- **Docs last**, in the slice that also rewrites the skill, so the three documents that teach the app change together.

**What `to-tickets` cuts:** one implementation ticket per slice, seven, each blocked by the previous one except slice 3, which needs only slice 2, and slice 4, which needs only slice 1. Blocking uses GitHub's native issue dependencies.

## Testing Decisions

**The rule is unchanged from v2:** test what a hand check can't catch or can't afford, through a seam that already exists, asserting on external behaviour a reader can see whole. Nothing here needs a real Keychain, real Jira or a real terminal.

**Primary seam: the CLI harness.** The existing `machine` harness runs the whole CLI in-process against a temp data dir, a temp `HOME`, a fixed clock and an injected tracker. Nearly everything in this spec is a CLI fact and is tested there: `local` at stop and `rm` without a warning; the four narrowed exit codes with and without a fake tracker; `tracker` and `here` in `status --json`; `.punchcard` found at the nearest ancestor of a `git init`-ed temp dir, falling through on an invalid line with the notice; the slug grammar accepting `payroll` and refusing `pay roll`; the config rules (no file, no field with site and email, explicit `none`). `log` is tested through the same harness after a scripted week of starts and stops across days and a midnight, with **golden files** for `--md` and `--csv` output (the first golden comparison in the CLI tests; the TUI's golden helper is the prior art) and envelope decoding for `--json`, including the open entry and the empty range.

**Existing harnesses for what the CLI can't render.** The TUI headless and golden harness for the day's-log pane with and without a tracker, the empty-list note, the stop dialog wording and the absent banner. The setup `scriptUI` harness for the tracker step first, none skipping to the plugin steps, the removal offer accepted and declined, and `tracker` written. The plugin's `node --test` suite for the band with `tracker: none` (quiet), with Jira (unchanged), the `here`-based reminder, `/punch start` taking `here`, and the dialog label by tracker.

**Core tests only where the CLI cannot reach:** the mark-local transition (refusing a synced worklog, clearing the failure fields) and the views' treatment of `local`, as table tests beside the existing ones.

**Not tested in Go:** the tracker-neutral error set, which is a package boundary; every CLI test against the fake tracker covers it. The Jira package's own client, wire and lookup tests are untouched.

**Prior art:** the `machine` harness and refusal tables in the CLI tests; the TUI golden helper and headless key runs; the setup transcript harness; the plugin's pure-function tests.

## Out of Scope

- An archive command or any retention of old entries outside `state.json` (deferred by ADR 0008, with the trigger named there).
- A second tracker implementation. The seam is proven by Jira plus the fake; the next tracker is its own spec.
- An HTML or rendered timesheet; Markdown and CSV cover it until shown otherwise.
- Projects, clients, tags, rates, a second grouping, or any reporting beyond one row per worklog grouped by day.
- Editing a worklog after stop, or marking one local from the CLI.
- A `.punchcard` with more than a key, or one per branch.
- Case-insensitive keys.
- Everything the v2 spec lists as out of scope.

## Further Notes

- **No release has happened**, so no schema version moves and no migration code is written; ADR 0008 records why this lands before v0.1.0.
- **CONTEXT.md is already updated** with this spec: **Tracker** (the optional remote a worklog is sent to; Jira is one), **local** (a worklog's state when there is no tracker: final, never owed), and the Jira-free wordings of Ticket, Unverified, Worklog, Stopped and Focus. Slice 7 touches the README and the guide only.
- **Jira users should notice nothing** except new words in setup and the pane title. Every slice's hand check includes running the Jira path on the dev machine once.
- The v2 spec at `docs/spec.md` remains the record of the build it describes; where the two disagree, this spec and ADR 0008 win.
