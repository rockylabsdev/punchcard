# Punchcard — v2 Spec (terminal-native)

Status: ready-for-agent

Derived from the wayfinder map at [GitHub issue #1](https://github.com/j-dsmith/punchcard/issues/1) and its fifteen resolved tickets, and from the seven ADRs under [`docs/adr/`](adr/). Domain vocabulary is authoritative in root [`CONTEXT.md`](../CONTEXT.md): `TrackedTicket`, `TimeEntry`, `Segment`, `Worklog`, `Today`, Presence, Focus, Held, Origin, `lastSeen`, Stale. The v1 macOS plan is archived under [`docs/v1-mac/`](v1-mac/README.md); its Jira curl spike and persistence contract carry forward and are restated here.

Every slice of the build order is specified at build-from resolution. There is no "intent plus flagged questions" tier this time: every decision below rests on a closed wayfinder ticket or an ADR, and the map's fog is empty.

## Problem Statement

I work against Jira tickets all day and I am expected to log my time against them. Neither half of that works today.

Jira's own time logging is a form buried several clicks inside a ticket, so logging time means stopping what I am doing, navigating to the ticket, and typing a duration. In practice I don't do it while working; I reconstruct it later from memory, at the end of the day or the end of the week. Those reconstructions are guesses, and they are systematically wrong: work I switched away from and back gets counted once, short interruptions vanish, and anything I forgot entirely never gets logged at all.

The generic timer apps that would measure this accurately don't know what a Jira ticket is. They give me a stopwatch and a text label, which means I still have to transcribe every result into Jira by hand: the same tedious step, just moved later, with a transcription error added.

And now most of my working day happens inside a terminal: in Claude Code sessions and in Orca IDE panes, often with several agents working several tickets in parallel. A menu-bar app sits outside that loop. The timer has to live where the work is, and the agents doing the work have to be able to drive it: start the clock when they pick up a ticket, propose stopping and submitting when they finish, without me leaving the session.

What I actually need is a timer that is *already* attached to the ticket, that lives one keystroke away in the terminal, that follows my attention between agent sessions, and that puts the result into Jira itself so there is no second step to forget.

## Solution

**Punchcard** is a personal, single-user terminal app that times work against a Jira ticket and syncs the elapsed time back to Jira as a worklog. It is one `punchcard` binary with two faces sharing one core: a full-screen **TUI** in the spirit of lazygit and btop, and a scriptable **CLI** with a stable `--json` contract. A human drives it at the keyboard; Claude Code drives it through the CLI, by way of an optional plugin.

There is no GUI, no menu-bar icon, and **no daemon**. A running timer is an open segment in a JSON file; nothing needs to be alive for it to run. The TUI shows the tickets I have added, each with its real summary from Jira and its time today; one keystroke starts the timer on a row, and at most one row is ever running. The CLI does the same from any shell: `punchcard start KEY`, `pause`, `resume`, `stop`, `status`, `ls`, `add`, `rm`, `sync`, `setup`.

The timer measures **my attention**, not an agent's work. However many agents run in parallel, there is one timer, and it follows my own prompts between sessions whose git branch names a ticket. A timer I set by hand is **held** and does not follow. Presence is my own input: a keystroke in the TUI, a command I run, a prompt I type. If a timer sees no presence for two hours, the next thing I do caps it at the last moment I was present and flags it recovered until I stop it and correct the duration.

Pausing suspends accrual without producing anything, and resuming continues the same stretch of work. Stopping is the deliberate act that ends the work and produces a `Worklog`: I correct the duration, optionally add a comment, and it is posted to Jira, truncated to the minute. An agent may start, pause and resume freely, but an agent's stop is only ever a proposal: it previews the worklog, and the call that sends it is gated behind a yes/no that only a present human can answer.

The local file is the source of truth. Jira is a destination that may be unreachable or unconfigured; a failed sync leaves the worklog pending and retryable and never blocks a running timer. Failures are visible banners, never blockers.

## User Stories

### Timing work

1. As a developer, I want a timer that is always one keystroke away in my terminal, so that starting it costs me nothing and I actually do it.
2. As a developer, I want no GUI at all, no window, no menu-bar icon and no Dock presence, so that the tool lives inside the terminal loop I already work in.
3. As a developer, I want to start timing a ticket with a single keystroke on its row, so that starting is faster than the interruption that made me want to start.
4. As a developer, I want at most one ticket running at a time, so that my logged time cannot exceed my actual working time.
5. As a developer, I want starting a second ticket to pause the first and say so, so that I never accidentally lose or double-count a stretch of work.
6. As a developer, I want to pause a running timer, so that I can step away without ending the piece of work I am in the middle of.
7. As a developer, I want resuming after a pause to continue the same stretch of work rather than beginning a new one, so that an interrupted hour reaches Jira as one worklog rather than five.
8. As a developer, I want pause and stop to be visibly different actions with different keys, so that I never end work when I meant to step away for coffee.
9. As a developer, I want to see each ticket's time today at a glance, so that I can see where my day went without opening anything.
10. As a developer, I want the running row to be unmistakable, so that one glance tells me whether I am on the clock.
11. As a developer, I want elapsed time derived from timestamps rather than a ticking counter, so that sleep, quit, and clock changes cannot silently corrupt it.
12. As a developer, I want to close my laptop lid and reopen it later without my timer having drifted, so that I can trust the number.
13. As a developer, I want a timer to keep running with no Punchcard process alive, so that an agent can start it from a hook and I can quit the TUI without stopping work.
14. As a developer, I want "time today" to count only the part of a stretch that falls inside the local day, so that a timer left running over midnight does not inflate tomorrow.

### Managing tickets

15. As a developer, I want to add a Jira ticket to my list by typing its key, so that I can start timing without leaving the terminal.
16. As a developer, I want starting a ticket that is not yet on my list to add it automatically, so that starting work is never a two-step act.
17. As a developer, I want to see the ticket's real summary fetched from Jira, so that I can confirm I added the ticket I meant to.
18. As a developer, I want recently used tickets offered to me when I add or start, so that adding the ticket I worked on yesterday takes one keystroke instead of remembering its key.
19. As a developer, I want a ticket to stay in my list whether or not it currently has time against it, so that I can queue up today's work before starting any of it.
20. As a developer, I want my list ordered by most recent activity, so that what I am working on now is at the top without me managing the order.
21. As a developer, I want to remove a ticket from my list, so that finished work stops cluttering the pane.
22. As a developer, I want removal to refuse while the ticket has a running or paused entry or an unsent worklog, so that I cannot throw away time I have not logged.
23. As a developer, I want a typo'd ticket key to tell me it could not be found and to change nothing, so that I do not spend an hour logging time against nothing.
24. As a developer, I want the not-found message to be honest that the ticket may exist but be invisible to me, so that I am not misled into thinking a real ticket does not exist.
25. As a developer, I want starting a ticket while Jira is unreachable to start the timer anyway and mark the ticket unverified, so that an outage never stops me working.
26. As a developer, I want an unverified ticket to be confirmed the next time Jira can be reached, so that the marker clears without me doing anything.

### Sending time to Jira

27. As a developer, I want stopping a timer to be what sends time to Jira, so that there is exactly one moment where work becomes a permanent record.
28. As a developer, I want to review and correct the duration before it is sent, so that a timer I forgot to stop does not put six hours on a ticket.
29. As a developer, I want to optionally add a comment when I stop, so that the worklog says what I actually did.
30. As a developer, I want the duration truncated to the minute before sending, so that Jira shows round numbers rather than false precision.
31. As a developer, I want a worklog under one minute refused with a hint, so that I never send Jira a zero.
32. As a developer, I want the worklog to leave the ticket's remaining estimate untouched, so that logging time does not silently rewrite someone else's planning.
33. As a developer, I want to see whether each stopped stretch has actually reached Jira, so that I know what is done and what is still owed.
34. As a developer, I want a failed sync to stay pending and retryable, so that a dropped connection costs me nothing.
35. As a developer, I want to retry failed syncs with one key or one command, so that I am not waiting on a background process I cannot see.
36. As a developer, I want nothing to sync in the background, so that every POST to Jira is one I asked for and can see the result of.
37. As a developer, I want an already-synced worklog to be impossible to send twice, so that a retry after a timeout does not double-log my time.
38. As a developer, I want two writers to be unable to POST the same worklog at once, so that retrying from the TUI while an agent syncs cannot duplicate it.
39. As a developer, I want a failed sync to never block or stop a running timer, so that a Jira problem does not become a lost-work problem.
40. As a developer, I want my local record to be the source of truth, so that Jira being down is an inconvenience rather than data loss.
41. As a developer, I want a stop whose POST failed to still count as a stop, so that I (or an agent) run `sync` afterwards rather than stopping again.

### Surviving crashes, restarts and forgotten timers

42. As a developer, I want my tickets and their time to survive quitting the TUI, so that I do not have to leave it running forever.
43. As a developer, I want a timer that saw no presence for two hours to be capped at the last moment I was present, so that I am not credited for the weekend my machine spent awake.
44. As a developer, I want a recovered timer left paused rather than silently resumed, so that the app does not keep accruing time for work I am not doing.
45. As a developer, I want a recovered entry visibly flagged on its row, so that I notice it needs checking even hours later.
46. As a developer, I want that flag to persist until I stop the entry, so that resuming it once does not lose me the information.
47. As a developer, I want to correct a recovered duration before it goes to Jira, so that an approximate cap becomes an accurate worklog.
48. As a developer, I want reading my status to never count as presence, so that an agent polling `status` cannot keep a forgotten timer alive.
49. As a developer, I want a first run with no data file to just work, so that installing the tool is not an error state.
50. As a developer, I want a corrupted data file backed up rather than deleted, so that I have some chance of recovering unsent worklogs.
51. As a developer, I want to be told where that backup went, so that recovery is actually possible.
52. As a developer, I want the app to refuse to load a file written by a newer version rather than resetting it, so that downgrading cannot destroy my pending worklogs.
53. As a developer, I want a failed save to say so rather than crash, and to leave my running timer alone, so that a full disk does not cost me the session I am in the middle of.
54. As a developer, I want the TUI, the CLI and an agent to be able to change state at the same time without corrupting it, so that I never have to think about which one is open.
55. As a developer, I want a change made elsewhere to show up in the TUI within a second, tagged with what happened and who did it, so that an agent pausing my timer never surprises me.
56. As a developer, I want a command that finds Punchcard busy to wait briefly and then fail with a distinct exit code, so that nothing ever deadlocks or silently drops a change.

### First run and credentials

57. As a new user, I want a bare `punchcard` on a true first run to walk me through setup, so that installing a tool with no GUI does not appear to do nothing at all.
58. As a new user, I want to enter my Jira site URL, email, and API token in one guided flow, so that setup is a single sitting.
59. As a new user, I want to paste any form of my Jira URL, bare host, full URL, or a board URL from my address bar, and have it normalized, so that setup does not fail on a trailing slash.
60. As a new user, I want to see the normalized URL echoed back, so that I can catch a mangled value before anything depends on it.
61. As a new user, I want the connection tested automatically after I enter my token, so that I find out setup is wrong now rather than the first time I try to log time.
62. As a new user, I want a bad site URL and a bad token to produce different errors and return me to the right step, so that I know which field to fix.
63. As a new user, I want a successful test to name the account I connected as, so that I catch authenticating as the wrong account.
64. As a new user, I want to be told up front that I need a classic unscoped token, with a link to where to mint one, so that I do not mint a scoped one that looks valid and never works.
65. As a new user, I want a rejected-credentials error to raise the scoped-token possibility, so that the most likely first-run failure is not indistinguishable from a typo.
66. As a new user, I want saving to be impossible until the connection test passes, so that I cannot store a combination that was never verified.
67. As a new user, I want the terminal to simply wait while I go to the browser to mint a token, so that I do not lose half-entered input on the most-travelled path in setup.
68. As a new user, I want to be able to cancel setup at any step with nothing saved, so that I am never trapped in a form.
69. As a returning user, I want to reach setup from the TUI with one key and from the shell with one command, so that I can fix credentials without reinstalling.
70. As a returning user, I want the token field blank on every visit with a note that it is stored and where, so that changing only my email does not require minting a new token.
71. As a returning user, I want my token in the macOS Keychain rather than a file, so that my work credentials are stored the way secrets should be.
72. As a returning user, I want a file fallback when the Keychain cannot be used, and to be told plainly that it was used, so that a broken keyring does not block me.
73. As a returning user, I want an environment variable to override the stored token, so that CI and scripts can run without touching my Keychain.
74. As a returning user, I want credentials that stop working to show a persistent banner in the TUI and in the band above my Claude Code prompt, so that I notice before a day's worklogs pile up unsent.
75. As a returning user, I want my timers to keep running when credentials fail or are missing, so that an auth problem does not become lost work.
76. As a returning user, I want my stored token never auto-deleted on a failure, so that a transient Jira outage does not wipe my setup.
77. As a returning user, I want being unconfigured to behave like being offline, so that I can start timing before I have finished setting up Jira.

### Driving it from a shell or a script

78. As a developer, I want every action available as a subcommand, so that I can drive Punchcard from any shell, alias or script.
79. As a developer, I want an opt-in `--json` output with one stable envelope on every command, success or failure, so that programs can parse it without guessing.
80. As a developer, I want JSON never switched on just because stdout is not a TTY, so that `status | cut` in my shell prompt keeps working.
81. As a developer, I want distinct exit codes for distinct outcomes, so that a script can tell "nothing running" from "ticket not found" from "busy".
82. As a developer, I want Punchcard to never exit 2, so that it can run inside a Claude Code hook without blocking my action.
83. As a developer, I want a stop that succeeded locally but failed to sync to exit with its own code and `ok: true`, so that a caller runs `sync` instead of stopping again.
84. As a developer, I want a one-line `status --short` that prints nothing when idle, so that my shell prompt can show the running ticket.
85. As a developer, I want the side effects of a command (a paused ticket, a recovery, a sync result) reported as events, so that nothing happens behind my back.
86. As a developer, I want `stop` on a TTY to prompt for the duration and a comment, and to use the measured time with no prompt otherwise, so that scripts never hang.
87. As a developer, I want a `--dry-run` that previews the exact worklog a stop would send, so that I can check before committing.
88. As a developer, I want reads to never call Jira and never write, so that `status` is instant and safe to run anywhere.
89. As a developer, I want every change to carry who caused it (hand, prompt or agent), so that the TUI and the record can tell my own acts from an agent's.

### Driving it from Claude Code

90. As a developer, I want Claude Code to drive Punchcard through the same CLI I use, so that there is one contract and one version to keep in step.
91. As a developer, I want the plugin to be optional, so that the CLI and TUI are fully usable without it.
92. As a developer, I want my own submitted prompt in a session whose branch names a ticket to switch the timer to that ticket, so that moving between agents' worktrees moves my time with no extra act.
93. As a developer, I want typing in the prompt box to count as presence without switching anything, so that a forgotten timer stays alive only while I am actually around.
94. As a developer, I want a timer I set by hand to stay put when I prompt in another session, so that a hook never overrides a deliberate choice.
95. As a developer, I want a session whose branch names no ticket to give presence but never focus, so that an unkeyed session cannot start a timer on the wrong ticket.
96. As a developer, I want a band above my Claude Code prompt showing the running ticket, its time, whether it is held, unsent worklogs and auth problems, so that every session tells me whether I am on the clock.
97. As a developer, I want a `/punch` command inside Claude Code for start, pause, resume and stop, so that I can act by hand without leaving the session.
98. As a developer, I want an agent to be able to start, pause and resume an un-held timer without asking me, so that picking up a ticket costs it nothing.
99. As a developer, I want an agent's stop to be a proposal that shows me the exact worklog, so that nothing reaches Jira that I have not read.
100. As a developer, I want the call that actually sends an agent's worklog to be gated behind a yes/no I must answer, even under skip-permissions mode, so that an agent can never submit time on its own.
101. As a developer, I want that gate to deny the stop when no human is present, with the timer left running, so that an unattended agent can neither submit nor discard my time.
102. As a developer, I want the gate to hold even when the plugin is disabled, so that disabling the plugin never weakens it.
103. As a developer, I want a skill that teaches the agent the commands and the rules, so that it does not need context injected at every session start.
104. As a developer, I want closing, sleeping or deleting an Orca pane or worktree to do nothing to my timer, so that Punchcard behaves the same in any terminal.
105. As a developer, I want the band to tell me when the plugin and the binary are out of step or the binary is not on `PATH`, so that a half-upgraded install is visible rather than silently broken.

### The TUI

106. As a developer, I want a lazygit-style layout with one question per pane, so that I never have to decode what a number means.
107. As a developer, I want the Tickets pane to show one number per row, time today, plus a sync marker, so that a row reads at a glance.
108. As a developer, I want the Ticket pane to show the work stop will send as a headline, and to spell out its arithmetic as counted and not-counted stretches, so that I can see why the number is what it is.
109. As a developer, I want a Jira strip showing what is logged today, what is owed, and the last sync, with each row leading with its outcome, so that my standing with Jira is one glance away.
110. As a developer, I want the stop dialog to live in the Ticket pane with an editable duration, an optional comment and a live preview of what will be sent, so that the dialog never resizes the layout.
111. As a developer, I want a persistent, non-dismissible banner for the worst current condition (token rejected, not connected, Keychain error), so that I cannot miss a problem by dismissing it reflexively.
112. As a developer, I want the semantic colours to come from the 16 ANSI slots and every state to have a symbol, so that the TUI follows my theme and reads in 16 colours or none.
113. As a developer, I want blue reserved for sync, so that one colour always means "this concerns Jira".
114. As a developer, I want a context-sensitive key bar, so that the keys that apply now are always visible.
115. As a developer, I want `S` to suspend the TUI, run setup in the same terminal and redraw, so that there is one setup flow and no settings pane.
116. As a developer, I want the TUI to look right at 80×24 and at 120×32, so that it works in a tmux pane as well as a full window.

### Platform and distribution

117. As a developer, I want macOS to be the daily-driven, hand-tested platform, so that the tool is polished where I use it.
118. As a developer, I want Linux to build and pass the full suite in CI, so that a Linux box can run it without a port.
119. As a developer, I want config and data at XDG paths on both platforms, so that everything sits where a terminal user looks for it.
120. As a developer, I want to install with `brew install j-dsmith/tap/punchcard`, so that installation is one command with no quarantine prompt.
121. As a developer, I want `go install` to work as a fallback, so that I am never blocked by the tap.
122. As a developer, I want the plugin pinned to the binary's release, so that an install or update always gets the mod and skill that match the binary.
123. As a developer, I want `setup` to install the plugin and offer the `ask` rule, so that the agent integration is two confirmations away from a fresh install.
124. As a developer, I want uninstalling to leave my config, data and Keychain item alone, so that unsent worklogs survive a reinstall.

### Building it (first Go project)

125. As a first-time Go developer, I want each unfamiliar Go part (Bubble Tea's update loop, `flock`, the `security` CLI, Huh, `ExecProcess`, GoReleaser) introduced in its own slice, so that a failure has exactly one plausible cause.
126. As a first-time Go developer, I want every slice to end at a commit that builds, runs and is green in CI on both platforms, so that I always have a working tool to fall back to.
127. As a first-time Go developer, I want each slice to have one hand check I can perform, so that "done" is observable rather than a feeling.
128. As a first-time Go developer, I want the Jira API proven by curl before any Go is written, so that a network-slice failure is unambiguously a Go failure.
129. As a first-time Go developer, I want the captured real API responses as my fixtures, so that writing the wire types is transcription rather than guesswork.
130. As a first-time Go developer, I want the pure core and its tests to land in the first slice, so that every later slice builds on transitions that are already pinned.
131. As a first-time Go developer, I want credentials deferred until late, so that my first network call cannot fail ambiguously between a bad token and a bad Keychain read.
132. As a first-time Go developer, I want the plugin built before the TUI, so that I am dogfooding the CLI from inside Claude Code while the TUI slices are built.
133. As a developer whose code is written by agents and reviewed by me, I want tests wherever a hand check can't catch or can't afford a regression (the core transitions, concurrent writers, the parsed `--json` contract, the TUI's frames), so that I can tell from a green suite that an agent's change did not move the contract.
134. As a first-time Go developer, I want the standard library's test package with a small set of well-known helpers and no assertion framework, so that every tutorial and every agent already knows the idiom.

## Implementation Decisions

### Process model — [Process model, and what lastSeen means now](https://github.com/j-dsmith/punchcard/issues/6), ADR [0001](adr/0001-no-daemon-presence-based-lastseen.md)

- **No daemon.** The CLI is stateless and the TUI is one more writer of the same files. A running timer is an open `Segment` with a start time, and nothing needs to be alive for it to run. A timer with no process behind it is a feature: an agent can start it from a hook, and the TUI can be quit at any time.
- **`lastSeen` records presence, not liveness.** Presence is the user's own input: a TUI keystroke, a state-changing command, or typing or submitting a prompt in a Claude Code session. Reads (`status`, `ls`) are pure and never write. An open but untouched TUI is not presence. Agent activity is not presence. The 30-second heartbeat is gone; the heartbeat file is written on presence events, rate-limited to one write per 30 s and never waiting for the lock.
- **Staleness is derived, never stored.** A running timer is stale after 2 hours with no presence. The first act of presence that finds it stale caps the open segment at `lastSeen` (clamped to the segment's start if the clock went backwards), sets `recovered`, and only then records itself, because the gap can be seen only once. The command announces the recovery and `--json` carries a `recovered` event. **The recovered marker clears only at stop**, not at resume (a change from v1).
- **`status` on a stale timer** reports state `stale`, headlines the capped elapsed, and shows `lastSeen` with its age and that the next action will recover it. JSON carries `state: "stale"`, `lastSeen`, `elapsedSeconds` (capped) and `uncappedSeconds`.
- **One timer, the user's attention.** Parallel agents in several worktrees still mean one timer. **Focus** (the user's own prompt in a session whose branch names a ticket) moves an un-held timer. A timer set by **hand** is **held** and only another act by hand moves it. A session whose ticket cannot be told gives presence but never focus. `touch KEY` vouches only if KEY is the running ticket.
- **Every change is one indivisible transition**: recover first, then close any open segment before opening another. `start B` while A runs pauses A and says so.

### Concurrent writers — [Concurrent writers to state.json](https://github.com/j-dsmith/punchcard/issues/7), ADR [0002](adr/0002-flock-on-a-side-lock-no-cached-writes.md)

- **One exclusive advisory `flock` on a separate `state.lock`** serialises every writer. It is not on `state.json` itself, because the atomic temp-then-rename save gives that file a new inode on every write. The kernel releases the lock when its holder dies, so a crash can never leave a stale lock.
- **Every change is one critical section**: lock, read both files fresh, back up a corrupt file or refuse a newer version, recover, apply, write, unlock. **No writer applies a change to a copy it read earlier, the TUI included.** A TUI keystroke requests a transition; it never saves the TUI's model. There is therefore no version counter and no mtime check.
- **The lock is never held across a network call.** Syncing takes a second lock, `sync.lock`, without waiting, and holds it across the POST, so two writers can never both POST the same worklog. Marking the state under `state.lock` happens before and after, in two short critical sections.
- **A CLI that finds the lock held waits about 2 s, then exits 75.** Presence writes never wait: they try the lock without blocking and skip if it is held, which is safe because the holder is either a transition that records presence itself or another presence write.
- **v1's "in-memory state stays the truth on a failed write" is retired.** A change that cannot lock or write does not happen and says so (exit 75 or 1, or a TUI banner). A running timer is never affected: it is already on disk as an open segment.
- **The TUI learns of outside changes by polling, not watching.** On its 1 s tick it `stat`s `state.json` (inode and mtime), re-reads on change, and diffs old against new to find the changed rows. No watcher library.

### Stack and project shape — [Stack confirmation](https://github.com/j-dsmith/punchcard/issues/8), ADR [0003](adr/0003-go-and-bubble-tea-one-binary.md); [Build order](https://github.com/j-dsmith/punchcard/issues/15)

- **Go, Bubble Tea v2 with Lip Gloss v2 and Bubbles v2, one `punchcard` binary.** Subcommands are the CLI; the TUI lives in the same binary and the CLI path never starts the renderer. Module paths are under `charm.land/…/v2` (Bubble Tea v2.0.10, Lip Gloss v2.0.6, Bubbles v2.2.1; Huh v2.0.3 for setup). Go 1.26 through the `toolchain` directive, so fresh worktrees never drift.
- **Why not Ink:** the CLI runs on the hot path of every prompt in a keyed session; Go starts in about 4 ms against Node's 33 ms (190 ms with Ink loaded eagerly), Go reaches `flock` directly, and the binary is a few megabytes rather than 57. **Why not Rust:** no colour downsampling in Ratatui, and Go is the language a TypeScript developer can review critically soonest. The Bubble Tea prototype passed its checkpoint, so the Go-core-plus-Ink-TUI fallback is retired.
- **The mod is a thin client of `punchcard … --json`** and never imports the core: one seam, one version, one lock owner, and Claude Code's process never holds Punchcard's lock.
- **One module, one repo.** A single `cmd` entry point and internal packages for the core (types and the pure `apply`), the store, the Jira client, the CLI, the TUI, credentials and setup. Everything is internal so nothing outside the binary can import it. The Claude Code plugin (mod, skill, plugin manifest) lives in its own top-level directory in the same repo with the marketplace manifest at the root. The TUI prototype stays a reference on its branch and is never merged or promoted.
- **`cobra` for the command tree.** Exit codes are always set by Punchcard's own code, never cobra's. Other dependencies are the Charm set, `x/sys` for `flock`, `go-cmp`, `testscript` and `x/exp/golden`. No testify, no golangci-lint config.
- **Clock and filesystem are injected, not global.** `now` is a function value passed into the core and the TUI model; the TUI's tick message carries the time and `Update` never reads the wall clock. The only filesystem seam is the XDG environment.

### Domain model

[`CONTEXT.md`](../CONTEXT.md) is authoritative. The points that most shape the code:

- A **`TrackedTicket`** is on the list whether or not it has time against it. It may be **unverified** (added while Jira could not be reached; no summary; confirmed on the next contact).
- **A pause closes a `Segment`; it does not end the `TimeEntry`.** One entry produces exactly one `Worklog`, which gives the idempotency guard a single object to attach to. There is at most one open segment across every writer.
- **Every state is derived, none is stored**: running is an open final segment; paused is a closed final segment with no worklog; stopped is a non-null worklog; stale is running with no presence for 2 h; recovered is a stored flag on the entry (a fact about history, not a state). Held and Origin are stored facts about the running entry.
- **Today** is the local calendar day containing now. Time today splits a segment at midnight and counts only the part inside the day. A worklog's day is the local date of its entry's first segment start.
- **Origin** is who caused a change: `hand`, `prompt` or `agent`. Only `hand` holds. `hand` and `prompt` are presence; `agent` is not. Origin never decides recovery.
- Jira's API says *issue*; this domain says *ticket*. The word *issue* is confined to the wire types.
- **Types arrive at their use site.** Slice 1 lands the core types together with the transitions that use them and their tests; there is no test-free "define the model" commit.

### The pure core — [Test strategy for the Go stack](https://github.com/j-dsmith/punchcard/issues/16)

- The core is **one pure function**: `apply(state, command, now) → (state, events, error)`. Recovery runs in the pure layer; the store calls it before `apply`. Every transition (start, pause, resume, stop with duration and comment, add, remove, touch, mark worklog synced or failed, record auth failure) is a command, and every side effect nobody asked for (paused A, recovered, held, sync outcome) is an event.
- Derived views (elapsed, time today, list order, stale, the owed total, the preview a stop would send) are pure functions over the state and `now`.
- Durations: measured elapsed is the sum of segments. `timeSpentSeconds` is the measured elapsed truncated to the minute unless the user supplied a duration, which may exceed the measured elapsed. Under one minute is refused.

### CLI contract — [CLI command surface and the --json contract](https://github.com/j-dsmith/punchcard/issues/9), ADR [0004](adr/0004-cli-contract-exit-codes-json-envelope-origin.md)

- **Commands:** `start [KEY]`, `pause`, `resume [KEY]`, `stop [KEY]`, `status`, `ls`, `add KEY…`, `rm KEY`, `sync [KEY]`, `setup`, `version`. Hidden: `retry` (alias of `sync`), `touch [KEY] [--cwd DIR]` (the mod's presence call; `--cwd` derives the key from that directory's git branch), `--origin`. Bare `punchcard` opens the TUI on a TTY and prints `status` otherwise.
- **JSON is opt-in only** (`--json` or `PUNCHCARD_JSON=1`); stdout not being a TTY never switches it on. One envelope on stdout even on failure: `{v, ok, outcome, command, data, events, error}`. `error` is `{code, message, hint?}`. `v` versions the contract separately from the file schema; additive changes keep `v: 1`; `version --json` reports it. `--json` never prompts. State words follow `CONTEXT.md`: `running|paused|stale` for a timer, `pending|synced|failed` for a worklog.
- **Exit codes:** 0 ok (including idempotent no-ops; reads always 0), 1 internal, 3 `not_configured`, 4 `no_timer` (nothing to act on, or an ambiguous target), 5 `ticket_not_found` (404: missing or invisible), 6 `needs_confirmation` (`data.preview` is the exact worklog; rerun with `--yes`), 7 `sync_pending` (**partial success**, `ok: true`: the stop is on disk, the worklog is pending or failed; run `sync`, never `stop` again), 8 `auth_failed`, 64 usage, 65 `state_unreadable` (newer schema), 75 busy. **Exit 2 is never used.**
- **Origin:** `agent` when `CLAUDECODE` or `AI_AGENT` is set, `hand` otherwise; `--origin` overrides (the mod passes `prompt` for focus and `hand` for `/punch`). A command typed with `!` inside Claude Code counts as `agent`; accepted.
- **Transitions:** `start A` while A runs is a no-op with exit 0, and with origin `hand` it holds A. `start B` pauses A with an event. `pause` with nothing running exits 4. `resume` with no key resumes the most recently paused entry; `resume KEY` equals `start KEY`. `start KEY` adds an untracked ticket. `rm` refuses a ticket with a running or paused entry or an unsynced worklog (exit 4 with a hint); `--force` overrides.
- **Summary fetch:** only a ticket new to the list fetches its summary, with a budget of about 2 s. A 404 exits 5 and changes nothing. Unreachable or unconfigured starts anyway and leaves the ticket unverified until a later change to that ticket, or the TUI, confirms it. Reads never call Jira.
- **Stop:** with no key, the running entry, else the single paused entry; several paused entries exit 4 and list them; `stop KEY` targets one. On a TTY it prompts for the duration (prefilled) and a comment; with no TTY or with `--json` it uses the measured elapsed truncated to the minute. It POSTs inline; `--no-sync` leaves the worklog pending. A recovered entry with no TTY and no `--duration` is refused with exit 6. Under one minute is refused with exit 64 and a hint to pass `--duration`. `--duration` accepts `1h30m`, `1h`, `90m` or bare minutes, 1 m to 24 h; `h:mm` is rejected. With origin `agent` and no `--yes`, `stop` always exits 6 with `data.preview`. `--dry-run` always previews with exit 0.
- **Sync:** `sync` with no key sends every pending and failed worklog one at a time; exits 0 when all sent, 7 when some remain, 8 on a 401, 3 when unconfigured. Only `stop` (its own worklog) and `sync` ever POST.
- **Human output:** errors to stderr; colour only on a TTY with `NO_COLOR` unset. `status` shows the running ticket, paused tickets, the unsent count and, after a 401, the auth failure until a call succeeds. `status --short` is for shell prompts (`▶ ABC-123 1:12 · 2 unsent`, `⏸ …`, `! … stale`), prints nothing when idle and always exits 0. `ls` is ordered by most recent activity with key, state, open elapsed, unsynced count and summary. Every human-mode change prints a stderr warning while unsent worklogs exist.
- **Recents are the `ls` order**, with no separate store. `start` with no key opens a picker on a TTY and exits 64 otherwise.

### Agent integration — [Agent integration surface and unattended-action policy](https://github.com/j-dsmith/punchcard/issues/10), ADR [0005](adr/0005-agent-stop-gated-by-mod-and-ask-rule.md)

- **One optional plugin, `punchcard`, at user scope**, holding a **mod** and a model-invocable **skill**, kept in this repo and versioned with the binary. No MCP server, no settings hooks, no Orca-specific integration, no session-start context injection. The CLI and TUI are complete without it.
- **The mod:**
  - On `prompt.submit` with origin kind `composer` or `bridge`, runs `touch --cwd <session cwd> --origin prompt`: presence, and focus if the timer is not held. Switching happens on submit. `prompt.edit` is presence only, rate-limited, never waiting.
  - Draws a **band** above the prompt from `status --json`, polled every 10 s and right after its own calls: `▶ ABC-123 1:12 · held · <summary>`, `⏸ …`, `! ABC-123 recovered · fix at stop`, plus `· 2 unsent`, `· Jira auth failed`, `· here: DEF-9` when this session's key is not the running one, a version-skew warning, and `punchcard not found on PATH`. Idle with nothing owed shows no band. Events from its own calls are one-shot toasts.
  - Registers **`/punch`** with origin `hand` (so it holds): `start [KEY]` (no key means this branch's key), `pause`, `resume [KEY]`, `stop [duration] [-m comment]`. `/punch stop` shows the preview in a dialog with Submit, Keep pending (`--no-sync`) and Cancel; a recovered entry needs a duration.
  - Always passes `--origin`; `CLAUDECODE` is not guaranteed in its children.
- **The ticket key** is an explicit argument, else a key matching `[A-Z][A-Z0-9]+-\d+` in the git branch name of the session's directory, else nothing. A session with no key gives presence only; starting there takes an explicit key, which holds. A wrong guess that 404s exits 5 and changes nothing; offline adds it unverified.
- **Unattended-action policy.** Agent `start`/`pause`/`resume` are ungated and move only an un-held timer. An agent's stop is two steps: `stop` always exits 6 with the preview, which the agent shows; `stop --yes [--duration] [-m]` is **gated**. Unattended (`claude -p`, scheduled), the gated call is denied, the timer keeps running, and the skill tells the agent to say "run `/punch stop` when you're back".
- **Two gates, one dialog.** The mod's `tool.check` returns `ask` for any Bash command matching `punchcard stop … --yes` (raw command text, compound commands included), with a catch that also returns `ask` so a crash fails closed; it never returns `allow`, since a mod's `allow` overrides an ask rule. `setup` offers to add `Bash(punchcard stop *--yes*)` to `permissions.ask` in the user's Claude Code settings as the gate that holds with the plugin disabled. Both prompt under `--dangerously-skip-permissions` and both are denied under `-p` (verified on Claude Code 2.1.291).
- **Orca is just a terminal.** Closing, sleeping, archiving or deleting a pane or worktree does nothing to the timer.

### Persistence — v1 [persistence contract](v1-mac/wayfinder/issues/04-on-disk-persistence-contract.md), carried and amended by ADRs [0001](adr/0001-no-daemon-presence-based-lastseen.md), [0002](adr/0002-flock-on-a-side-lock-no-cached-writes.md) and [0006](adr/0006-token-via-security-cli-xdg-paths.md)

Two domain files plus two lock files under the XDG data directory (`~/.local/share/punchcard/` by default; the `XDG_*` variables are honoured on both platforms). Config and secrets live elsewhere; these files carry only domain data and one fact about Jira.

The state file, which encodes the schema more precisely than prose (carried from v1, with `held`, `origin` and `authFailedAt` added by this map):

```json
{
  "schemaVersion": 1,
  "authFailedAt": null,
  "trackedTickets": [
    {
      "key": "PUNCH-12",
      "summary": "Wire up the worklog POST",
      "unverified": false,
      "addedAt": "2026-08-09T09:02:41.000-0500",
      "entries": [
        {
          "id": "1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88",
          "segments": [
            { "startedAt": "2026-08-09T09:02:41.000-0500",
              "endedAt":   "2026-08-09T09:48:10.000-0500" },
            { "startedAt": "2026-08-09T10:15:00.000-0500" }
          ],
          "held": true,
          "origin": "hand",
          "recovered": false,
          "worklog": null
        }
      ]
    }
  ]
}
```

`worklog` is `null` until the entry is stopped, then one of three flat tagged shapes:

```json
{ "state": "pending", "startedAt": "…", "timeSpentSeconds": 1800, "comment": "Fixed the formatter" }
{ "state": "synced",  "startedAt": "…", "timeSpentSeconds": 1800, "comment": null,
  "worklogId": "110001", "syncedAt": "…" }
{ "state": "failed",  "startedAt": "…", "timeSpentSeconds": 1800, "comment": null,
  "message": "401 Unauthorized", "failedAt": "…" }
```

The heartbeat file holds `schemaVersion` and `lastSeen`, nothing else, and is written on presence events, rate-limited, never waiting for the lock.

- **Entries nest under their ticket.** A `TrackedTicket` is identified by its Jira key; a `TimeEntry` carries its own UUID, and that UUID is the idempotency key. **Never POST when the worklog state is `synced`**; the stored `worklogId` is what makes a retry after an ambiguous timeout safe.
- **`timeSpentSeconds` and `comment` are stored on the worklog**, not derived: the segments are what Punchcard measured, the worklog is what Jira was told, and a retry must not resurrect the pre-edit value.
- **`held` and `origin` live on the entry** and describe its current running stretch; `recovered` is set by recovery and cleared only by stop. `unverified` on the ticket clears on the first Jira confirmation. `authFailedAt` is a fact about Jira, not a timer state, set by any 401 and cleared only by a successful authenticated call or a fresh setup.
- **ISO-8601 strings with the Jira formatter everywhere** (`yyyy-MM-dd'T'HH:mm:ss.SSSZ`, numeric offset without a colon). Nil optionals are omitted, not written as `null`; an open segment simply has no `endedAt`. The file stays hand-readable because hand-editing it is a build-order hand check.
- **`schemaVersion: 1` from day one.** Load handling: missing or zero-byte is first run; undecodable is renamed to a timestamped corrupt backup and a non-blocking notice names the path; a newer version is refused with exit 65 and nothing written.
- **Write on every change, atomic replace (temp file then rename), under the lock.** Nothing is pruned.

### Jira integration — v1 [curl spike](v1-mac/wayfinder/issues/05-jira-api-curl-spike.md), proven against real Jira Cloud

- **Jira Cloud, `/rest/api/3/`, Basic auth with email plus a classic (unscoped) API token.** A scoped token returns 401 on every endpoint including `/myself`; the error copy names the possibility without asserting a mechanism nobody verified.
- Endpoints: `GET /serverInfo` (anonymous reachability, asserts Cloud), `GET /myself` (identity), `GET /issue/{key}?fields=summary` (always with `fields=summary`: 297 bytes against 19 KB), `POST /issue/{key}/worklog?adjustEstimate=leave` (201), `DELETE /issue/{key}/worklog/{id}?adjustEstimate=leave` (204, used only by the spike script and by hand).
- **The worklog body** carries `timeSpentSeconds`, `started` in the format above with a numeric offset (`+0000` accepted, `+00:00` rejected), and `comment` as an Atlassian Document Format `doc` holding one paragraph of plain text, omitted when empty. The committed `request-worklog.json` fixture is the shape to match.
- **Ids are strings**, issue and worklog alike; the account id is opaque. Jira echoes `started` in the account's timezone, so compare instants, never strings. A single error envelope (a message list plus a field-keyed map) decodes every failure. A 404 is ambiguous between missing and invisible.
- **The Jira interface has three calls**: fetch summary, post worklog, test connection (serverInfo then myself). A fake returning canned outcomes, including unreachable and 401, sits above it for every test; the real client is exercised by one `httptest` round-trip per call and, by hand, against a throwaway ticket whose worklog is deleted afterwards.
- A 401 from any call persists `authFailedAt` and leaves the worklog pending; it is a retryable outcome, never a terminal one.

### Credentials, config and paths — [Platform floor and credential storage](https://github.com/j-dsmith/punchcard/issues/11), ADR [0006](adr/0006-token-via-security-cli-xdg-paths.md)

- **Platform floor: macOS daily-driven and hand-tested; Linux must build and pass the full suite in CI.** Windows stays out.
- **Token lookup order on macOS:** `PUNCHCARD_TOKEN` (an override that setup never writes), then the macOS Keychain **written and read only through `/usr/bin/security`** (generic password, service `punchcard`, account = the configured email, about 1 s timeout, the secret passed on stdin or as the last argument, never logged), then a `0600` token file. On Linux: the env var, then the file. `status` reports which source was used.
- **Why `security` and not Security.framework:** a Keychain item's ACL trusts the application that created it. Apple's signed `security` binary creating and reading the item means no GUI prompt for any caller, including an unsigned Go binary rebuilt daily and a hook with nobody at the keyboard. Verified by probe: a detached, no-TTY child read the item in about 30 ms with no dialog.
- **The file is a fallback, never a silent default on macOS.** If the Keychain write fails, setup writes the file and says so. If both exist, the Keychain wins and `status` notes the stray file. Setup deletes every `punchcard` item before writing, so there is one credential at a time.
- **A Keychain error or timeout is not an auth failure.** With no fallback file, a command that needs Jira exits 3 naming the Keychain; `authFailedAt` stays unset.
- **Paths, XDG on both platforms:** config at `~/.config/punchcard/config.json` (`{schemaVersion, siteUrl, email}`, refuse-on-newer), the token fallback beside it at `~/.config/punchcard/token`, and state, heartbeat, locks and corrupt backups under `~/.local/share/punchcard/`. The `XDG_CONFIG_HOME` and `XDG_DATA_HOME` variables are honoured and are the test seam for the filesystem.
- **Only Jira-calling commands read the token** (`start` on a new ticket, `stop`, `sync`, `setup`); the every-prompt presence path never pays that cost.
- **Being unconfigured is treated like being offline.** `start` leaves the ticket unverified; `stop` exits 7 with error code `not_configured`; only `sync` exits 3; reads exit 0 and `status --json` carries `configured: false` plus the Keychain cause if any; the TUI stays fully usable under a banner.

### First run and setup — [First-run and settings in the terminal](https://github.com/j-dsmith/punchcard/issues/12)

- **One flow, `punchcard setup`**, an inline clack-style wizard under a block-letter wordmark, built with Huh. There is no settings pane; the TUI's `S` suspends the TUI, runs setup in the same terminal, and redraws re-reading config. No TTY on stdin exits 64 ("setup is interactive; run it in a terminal"). There is no flags-only setup, so a token never lands in shell history or within an agent's reach.
- **First run:** when there is no config file **and** no state file, a bare `punchcard` runs the wizard first; finishing or cancelling drops into the TUI. The state file's existence is the "already introduced" marker. Every later launch while unconfigured is the TUI under a persistent banner; there is no gate view.
- **Steps in validation order: site URL → email → token → automatic Test Connection → `Y/n` save.** The URL is normalised to `https://<host>` and echoed live. The token is masked, and on a revisit starts blank with "Stored in Keychain — leave blank to keep" naming the source. The token step prints the Atlassian token page as a terminal link plus the scoped-token warning, and the terminal simply waits while the user is in the browser. Test Connection runs serverInfo then myself: a URL failure returns to the URL step with the error, a 401 returns to the token step with the scoped-token copy, a pass shows "Connected as <name>". **Save is reachable only straight after a pass**; the flow is linear, so any edit walks back through the test. Cancel at any step writes nothing and leaves existing credentials intact.
- **Changing only the email** reads the old Keychain item, tests with the new email, then deletes the old item and rewrites it under the new account. With `PUNCHCARD_TOKEN` set, setup tests with the typed token, saves it, and warns that the env var overrides it; a blank token tests with the env token and says so. A fresh setup clears `authFailedAt`.
- **A re-run opens with a summary and a fork**: "Connected to <site> as <email> (token in Keychain)" then "Change credentials? (y/N)", defaulting to yes with the failure time when `authFailedAt` is set. No skips to the plugin and ask-rule steps.
- **Plugin step** (optional, `Y/n` defaulting to yes): runs the marketplace add and `claude plugin install punchcard -s user`, showing their output; skipped with the commands printed if `claude` is not on `PATH`; `✓` with no question if already installed; offers `claude plugin update` when the installed version differs from the binary's. A failure never undoes saved credentials.
- **Ask-rule step** (offered independently of the plugin): shows the exact JSON, then appends the rule to `permissions.ask` only if missing, keeps every other key, writes a `.bak`, and replaces the file atomically. A missing file is created with just that key; an unparseable one is left alone with the line printed to add by hand; an existing rule shows `✓`.
- **The banner** (TUI status line and mod band) has one slot, never dismissible, showing the worst condition: token rejected at <time> with N worklogs waiting, then not connected, then a Keychain error naming the cause. It clears only when the fact clears. Timers are never touched.

### TUI layout and interaction — [TUI layout and interaction](https://github.com/j-dsmith/punchcard/issues/13)

The settled layout, from the prototype at 80×24 (the prototype is a reference; its code is rewritten, not promoted):

```
╭─[1] Tickets · today 4h 29m────────────────────╮╭─[2] PUN-142─────────────────╮
│›▶ PUN-142  Wire flock on the side s… 1:12:00  ││Wire flock on the side       │
│ ‖ PUN-138  Stop dialog: duration pa…  1h 12m  ││state.lock                   │
│ · OPS-77   Rotate staging certs          45m ○││                             │
│ · PUN-120  Spike: Bubble Tea v2 lay…  1h 20m ✗││▶ RUNNING             1:12:00│
│ · DATA-9   ? unverified — Jira not …       —  ││  stop sends this as one     │
│ · PUN-151  Recents ordering in ls          —  ││  worklog — x                │
│ · ENG-4410 Investigate intermittent…       —  ││                             │
│                                               ││ 22:01–now   0:47:00 counted │
│                                               ││ 20:53–22:01 ‖ paused 1h 08m │
│                                               ││             not counted     │
│                                               ││ 20:28–20:53     25m counted │
│                                               ││                             │
│                                               ││WORKLOGS TODAY               │
│                                               ││  nothing yet                │
╰───────────────────────────────────────────────╯╰─────────────────────────────╯
╭─[3] Jira · logged today 30m · owed 1h 35m · last sync 17:49──────────────────╮
│ STATUS                        TICKET       TIME   STOPPED                    │
│ ✗ failed — 503 · r            PUN-120    1h 20m   16:08                      │
│ ○ not sent yet · r            OPS-77        15m   19:28                      │
│ ✓ logged 17:49                OPS-77        30m   17:48                      │
╰──────────────────────────────────────────────────────────────────────────────╯
 ✗ 1 worklog failed to reach Jira · r to retry · nothing is lost                
 space pause  x stop…  a add  d remove  r sync 2  S setup  ? help  q quit       
```

- **One question per pane, one unit of time per number. No header strip**: rows sort by recent activity, so the running ticket is always row 1.
- **[1] Tickets** (about 62% of the width): what am I tracking and where did today go. Rows in recents order; each has a cursor, a state symbol, the key, the summary, **one number = time today** (the running row ticks as `h:mm:ss`, others show `1h 12m` or `—`), and one sync marker (`✗` failed, `○` pending, blank). The pane title carries the day's total.
- **[2] Ticket** (right, full height): what happened on this one. The full summary; a headline that is **the work stop will send** (`▶ RUNNING 1:12:00`, `‖ PAUSED 1h 12m`, `! RECOVERED 2h 10m`); "stop sends this as one worklog — x"; the stretches newest first, each counted stretch with its length in the number column, each pause dimmed as `‖ paused 1h 08m, not counted` with its length in the label so it cannot be summed by eye, and a recovered entry adding `! away 4h, not counted`; then WORKLOGS TODAY for this ticket with the same outcome words as [3]. The stop and add dialogs take over this pane, so selection never resizes anything. Overlays use Lip Gloss's compositor.
- **[3] Jira**, a full-width strip along the bottom sized to its contents (at most about 5 rows, then `+N more`): logged today · owed · last sync (or `token rejected` / `not connected`), a dim column header, and rows that **lead with the outcome**: `✗ failed — 503 · r`, `○ not sent yet · r`, `✓ logged 17:49`. Owed worklogs from any day; sent ones only from today.
- **Status line**, just above the keys: the worst condition in the quiet banner form (one slot, never dismissible, worst first), otherwise the last event with its time and origin (`you` / `prompt` / `agent`). A transient notice (a toast, "syncing…") replaces it for about 5 s, the one exception to never dismissible.
- **Key bar**: context-sensitive, showing only the keys that apply now, switching to the dialog's keys inside a dialog.
- **Keys:** `space`/`enter` start, pause or resume the selected row (starting B pauses A); `p` pauses whatever is running from any row; `x` opens the stop dialog (editable duration, optional comment, live "→ sends 1h 12m" preview; enter sends, esc cancels and leaves the timer alone); `a` add with recents offered under the input; `d` remove, confirming with `y` and warning when unsent time would be lost; `r` syncs all pending and failed; `S` suspends into setup; `j k ↑ ↓ g G` move; `?` help; `q` quits with timers running.
- **Colour and symbols:** the semantic colours are the **16 ANSI slots** so they follow the theme; only greys come from the 256 cube and fall back to bright black. Running green bold `▶`; paused yellow `‖`; recovered magenta bold `!`; **sync reserved blue** (pending `○`, the `r` key; synced `✓` is dim, not blue); failure red `✗`; idle `·` and unverified `?` dim. Every state has a symbol so no-colour mode still distinguishes them; symbols are single-cell, no emoji, none whose width varies by terminal.
- **Changes made elsewhere:** on the 1 s tick, when the `stat` check finds `state.json` changed, the changed row shows a tag naming what and who for about 6 s (`started by agent`, `paused by prompt`, `stopped by hand`) and the status line logs it with its origin.
- **The TUI never writes from its model.** Every keystroke requests a transition through the store, and the model re-reads the result. The 1 s tick carries `now`; `Update` never reads the clock.
- **Defaults the build may revisit** (shipped in every prototype variant, never objected to, not put to the user directly): the key map, the 16-colour and no-colour palettes, the 6 s marker, and the stop dialog wrapping at 80 columns in the right pane. `S` → `ExecProcess` was smoke-tested headlessly but not in a real terminal; its hand check lives in the setup slice.

### Distribution — [Distribution](https://github.com/j-dsmith/punchcard/issues/14), ADR [0007](adr/0007-plugin-pinned-to-binary-release-tag.md)

- **A Homebrew tap, `j-dsmith/homebrew-tap`.** Pushing a `v*` tag runs GoReleaser in GitHub Actions, gated on the CI job, building darwin/arm64, darwin/amd64, linux/amd64 and linux/arm64 with `CGO_ENABLED=0`, and publishing tarballs and the formula with a PAT secret. `go install` is a free, undocumented fallback. Nothing is signed or notarised: neither path sets the quarantine flag. Browser-downloaded tarballs are unsupported, with the `xattr` workaround in the README.
- **The plugin is pinned to the binary's release tag.** The marketplace entry on `main` is a GitHub source with `ref: vX.Y.Z`, and the plugin manifest carries `version: X.Y.Z`. A release script bumps both, commits, then tags. Auto-update is off. The binary leads; after `brew upgrade` the band says the versions differ and to run `setup`, which offers `claude plugin update punchcard@punchcard`. Skew within one contract `v` is harmless.
- **Everything calls the bare `punchcard` on `PATH`**: the mod, the skill and the ask rule. Orca opens a login shell, so Claude Code inherits the Homebrew prefix. A missing binary shows in the band and makes presence a no-op.
- **Uninstall leaves config, data and the Keychain item**; the README lists the three removal commands. The plugin is removed separately.

### Build order — [Build order: the new walking skeleton](https://github.com/j-dsmith/punchcard/issues/15)

A slice is **a commit that builds, runs, is green in CI on both platforms, and has one hand check**. Never a refactor-only or scaffolding-only commit. Thirteen slices, one agent session each. Done already and not slices: the curl spike, the four research reports under [`docs/research/`](research/), and the TUI prototype.

| # | Slice | Lands | Isolates | Hand check |
|---|---|---|---|---|
| 1 | **Core + CI** | The core types and the pure `apply` with the whole fiddly table (start B pauses A, pause/resume, stop with truncate to the minute, staleness and recovery with the clamp, presence rate-limit, held vs focus, idempotency guard, time today split at midnight). `punchcard version` and `version --json`. CI matrix macOS + Linux: `vet`, `staticcheck`, `go test -race`. | Go toolchain directive, CI matrix | `punchcard version --json` on both CI runners |
| 2 | **Store + first commands** | The store: state and heartbeat files, `state.lock` via `flock`, atomic rename, schema version with refuse-on-newer and back-up-and-reset, XDG paths, lock → read fresh → recover → apply → write, presence writes that skip when busy. Plain-text `start KEY`, `pause`, `resume`, `status`. Tests against a temp XDG data dir; in-process contention with two opens of the lock. | `flock` | Start in one terminal, see it in another; sleep the machine, elapsed still right; push `lastSeen` 3 h back by editing the heartbeat file, `status` shows the entry recovered |
| 3 | **CLI contract** | Every command from ADR 0004 (`ls`, `add`, `rm`, local-only `stop`, stub `sync`, hidden `touch --cwd` and `--origin`), the JSON envelope, every exit code, origin from the environment, held, `status --short`. In-process envelope table per command; first `testscript` scripts (exit 75 under a held lock, never 2, no auto-JSON, origin from env). | `cobra`, `testscript` | `punchcard start --json \| jq`; hold the lock in a second shell and `start` exits 75 in about 2 s |
| 4 | **Jira read** | Wire types transcribed from the fixtures with a decode test each; the three-call Jira interface with its fake; one `httptest` round-trip. `start NEWKEY` fetches the summary with a 2 s budget: 404 exits 5 and changes nothing; unreachable leaves it unverified. Config read from the XDG config file (written by hand for now); token from `PUNCHCARD_TOKEN` only. | first real network call | `start` on the throwaway ticket shows its real summary; a typo exits 5 |
| 5 | **Stop + worklog** | `stop` prompts on a TTY, truncates, POSTs inline under `sync.lock`; pending / synced / failed; exit 7; recovered entry refused without a TTY or `--duration`; `sync`; 401 persists `authFailedAt`; `--yes`, `--dry-run` and `needs_confirmation` for origin `agent`. Encode test against the worklog fixture; the fake covers unreachable and 401. | — | Throwaway ticket receives a worklog, then delete it; cut the network, `stop` exits 7, `sync` sends it |
| 6 | **Plugin** | The mod (`touch --cwd … --origin prompt` on submit, band from `status --json` every 10 s, `/punch`, `tool.check` → `ask` for `stop … --yes`, fails closed) and the skill; the plugin and marketplace manifests. | mod API (JS) | On a branch named for the throwaway ticket, submitting a prompt starts the timer and the band shows it; an agent's `stop` previews and `stop --yes` prompts |
| 7 | **TUI a: frame + Tickets** | The Bubble Tea program, the 1 s tick carrying `now`, [1] Tickets on the real store, `space`/`p`/movement/`q`. Golden frames at 80×24 and 120×32 (empty, running, paused); the headless key run. Bare `punchcard` on a TTY opens it. | Bubble Tea update loop | Open the TUI, the running row ticks; `space` on another row pauses the first |
| 8 | **TUI b: Ticket pane + dialogs** | [2] Ticket with headline, counted / not counted / away stretches, WORKLOGS TODAY; the stop dialog with live preview, add with recents, remove with warning; compositor overlays. Goldens: recovered row, stop dialog. | compositor overlay | `x` sends a worklog to the throwaway ticket; a recovered entry shows `! away` and asks for a duration |
| 9 | **TUI c: Jira strip + status line** | [3] Jira strip, `r` sync, the status line with the worst-condition banner, 5 s toasts, the context-sensitive key bar, the 16-ANSI palette. Goldens: sync-failure banner, not-configured banner. | — | Fail a sync with a bad URL in config, `✗` row and banner appear; fix it, `r` clears both |
| 10 | **TUI d: changed elsewhere** | The 1 s `stat` of the state file, the old-against-new diff, the 6 s "started by agent" tag, origin on the status line. | — | `punchcard pause` from another terminal; the TUI shows `paused by hand` within a second |
| 11 | **Credentials** | `security` as a function seam with a timeout; the env → Keychain → file chain (Linux skips the keyring); Keychain error = exit 3; `authFailedAt` cleared only by a successful call or setup; the Keychain-error banner. Tests inject a seam that returns, errors or sleeps. | `/usr/bin/security` | Write the token with `security`, unset the env var, `stop` still POSTs; `status` from a no-TTY child shows no prompt |
| 12 | **Setup** | The Huh wizard under the wordmark, Test Connection, the re-run fork, plugin and ask-rule steps, first-run auto when no config and no state, no TTY → exit 64; `S` → `ExecProcess` from the TUI. | Huh, `ExecProcess` | Empty XDG dirs, bare `punchcard` runs the wizard and lands in the TUI; `S` suspends, setup runs, the TUI redraws with the banner gone |
| 13 | **Distribution** | GoReleaser on `v*` gated on CI, the tap, the release script bumping the plugin version and the marketplace ref together, version-skew and binary-not-found states in the band. | GoReleaser, tap | `brew install j-dsmith/tap/punchcard` on a clean `PATH`, then `setup` installs the plugin at the same version |

The orderings that are decisions, not accidents:

- **Slice 1 ships a binary.** A core-only commit has nothing to run; `version --json` is cheap and proves the toolchain directive and the CI matrix before anything leans on them.
- **The store lands with four thin commands** so it has a real hand check while a failure still has one plausible cause; the contract's mechanics are a different kind of work and get their own slice.
- **Recovery is hand-checked by editing the heartbeat file.** No hidden stale-threshold knob.
- **No temporary credential code.** The env var is first in the settled chain, so slices 4 through 10 run on a hand-written config file plus the env var; nothing to gitignore.
- **Plugin before the TUI.** The prototype already passed, so the TUI is not the risk; the mod is the surface nobody has used, and dogfooding the CLI from inside Claude Code during the TUI slices shakes the core and the contract earlier than golden frames can.
- **Credentials and setup last**, as in v1: the first network call fails for one reason only, and the TUI banner is built on two of its three conditions before the Keychain one exists.
- **Four TUI slices, not six**, each owning one unfamiliar thing; if slice 7 runs heavy, split the tick out.

**What `to-tickets` cuts:** one implementation ticket per slice, thirteen, all at build-from resolution. Each names what to build, its blocker (the previous slice; slice 11 needs only slice 5), the tests it must land, its one hand check, and the ADRs and resolutions it links. Blocking uses GitHub's native issue dependencies.

## Testing Decisions

**The rule: test what a hand check can't catch or can't afford.** v1 said "pure and fiddly only" because a first-time Swift developer did not want to maintain a suite for a personal tool. Three things changed the calculus: the code is agent-written and human-reviewed, so tests are how a reviewer knows an agent's change did not move the contract; several writers contend for the state file, which no hand check exercises reliably; and the `--json` envelope is parsed by other programs. The per-slice hand check still carries the walking skeleton.

**What makes a good test here:** it exercises external behaviour through a seam that already exists, with inputs and outputs that a reader can see whole. It never asserts on an internal structure or call sequence. A test that needs a real Keychain, real Jira or a real terminal is out of scope by construction; those are the hand checks.

**Seams, highest first.** Six, each one the natural boundary the architecture already has:

1. **The pure core**: `apply(state, command, now) → (state, events, error)` and the derived views. Table tests over in-memory values: recovery and capping including the clamp, staleness, the presence rate-limit, truncate to the minute, `start B` pauses A, held versus focus, the idempotency guard, time today across midnight. This is the whole fiddly list and it needs no files.
2. **The store**: one suite against a temp directory with the XDG data variable pointed at it: round-trip, corrupt-file backup, refuse-on-newer, rename-replace, recover-before-apply. **Concurrency** in-process: two goroutines with two separate opens of the lock file contend exactly as two processes do, covering "second writer waits", "presence write skips" and "released when the descriptor closes". No multi-process soak test; exit 75 is a CLI fact and lives in the scripts.
3. **The Jira edge**: a decode test per committed fixture (summary, 404 envelope, worklog POST, bad `started`), an encode test that the request body matches the worklog fixture shape for shape (which catches the string-id and `started` traps), and one `httptest` round-trip per client call. Above the client, the three-call Jira interface with a fake returning canned outcomes including unreachable and 401, so `start` going unverified and `stop` exiting 7 need no server.
4. **The CLI contract**: in-process for every command and exit code, calling the command's run function with an injected store, the fake Jira and a fixed `now`, comparing the envelope with `go-cmp`. `testscript` for process-level facts only: exit 75 while the script holds the lock, exit 2 never appearing, JSON off when stdout is not a TTY, origin inferred from the environment, plain-text output shape. The scripts are the executable form of ADR 0004.
5. **The TUI**: golden frames of `View()` at 80×24 and 120×32 through a colour-profile writer forced to ASCII, compared with the golden helper and refreshed with its update flag, one per named scenario (empty list, one running, one paused, a recovered row, the stop dialog, a sync-failure banner, the not-configured banner); and headless key runs that send key messages through `Update`, render after each, and assert no panic and every frame exactly the window height. Both call `Update` and `View` directly on a fake store with a fixed `now`. No `teatest`: it has no tagged v2 release, and driving the real program loop is Bubble Tea's to test. Suspend-and-restore and real terminal rendering are hand checks.
6. **Credentials**: the chain (env → Keychain → file; env → file on Linux) and the `authFailedAt` set and clear rules are pure once the Keychain read is a function seam. Tests inject one that returns, errors or sleeps past the timeout. The real `security` round-trip is a hand check; nothing in CI touches a Keychain.

**Harness:** the standard library's test package plus `go-cmp`, `testscript` and the golden helper. No testify. Table tests and subtests are the idiom. `now` is an injected function value, never a package global. Every test that touches files sets the XDG variables to a temp directory; the app already honours them, so that is the only filesystem seam.

**Prior art:** none in this repo; this is a greenfield first Go project, and the TUI prototype's frame dump and headless key run (about 4,900 frames, no crashes) are the pattern the TUI tests follow.

**CI:** GitHub Actions matrix of `macos-latest` and `ubuntu-latest` running `go vet`, `staticcheck` and `go test -race ./...`, landing in slice 1 so every later slice is green on Linux from the start, and required before a `v*` tag builds.

**Verification against real Jira** uses a throwaway ticket, and the worklog is deleted afterwards. The spike script demonstrates the create-then-delete cycle.

## Out of Scope

Carried from v1 and reaffirmed by the map:

- Shared or multi-user backend of any kind.
- OAuth 3LO; Basic auth with a classic API token is proven and sufficient.
- Automatic idle detection.
- Live ticket autocomplete against Jira search; keys are entered manually, with recents.
- In-app reporting or analytics.
- Remaining-estimate management; worklogs leave the estimate alone.
- Editing or deleting worklogs already synced to Jira.
- Pruning or archiving of history.
- Least-privilege scoped tokens; the 401-not-403 behaviour was observed and deliberately not investigated.
- Windows.

New with the process model, ruled out in [Process model, and what lastSeen means now](https://github.com/j-dsmith/punchcard/issues/6):

- Measuring an agent's wall-clock effort per ticket; a timer measures the user's attention, and agent effort never reaches Jira.
- Detecting terminal or pane focus; no such signal exists in Claude Code or Orca.

New with the rescope:

- Any GUI, menu-bar or notification-centre presence; the Xcode project and everything Swift, archived under [`docs/v1-mac/`](v1-mac/README.md).
- An MCP server (deferred: a server process and a second contract for a guarantee the gates already give).
- Settings hooks of any kind; the mod does everything they could and tells typed prompts from synthetic ones.
- Orca-specific integration (linked work items, archive hooks); Punchcard behaves the same in any terminal.
- A background sync of any kind.
- Linux Secret Service; it can be added later behind the same lookup.
- Code signing and notarisation; Homebrew and `go install` never quarantine.
- A settings pane in the TUI; `setup` is the only place credentials are edited.
- A flags-only `setup` (a `--token` flag would leak into shell history and an agent's reach).

## Further Notes

**This is the author's first Go project.** The author is fluent in TypeScript and React, new to Go, and works mostly through agents reviewing their output. That shaped the plan: the build order isolates each unfamiliar Go part in its own slice, the dependency set is small and well-trodden, the test idiom is the one every tutorial uses, and several decisions favour legibility over elegance. Preserve that bias: favour the boring, inspectable option.

**The map is complete.** All fifteen wayfinder tickets are resolved and the frontier is empty. Nothing remains to decide before building. The defaults listed under the TUI ("the build may revisit") are defaults, not open questions.

**The design philosophy, redesigned for the terminal:** one glance tells you whether you are on the clock; starting costs one keystroke; pause and stop look different; a recovered entry is flagged on its row until acted on; failures are visible banners, never blockers.

**Agent integration is the centre of gravity**, and the CLI is its only seam. Every decision about the contract (opt-in JSON, no exit 2, exit 7 as success with debt, origin from the environment) exists so that an agent can drive Punchcard without surprising the user, and every decision about the gate exists so that it can never submit time on its own.

**The captured fixtures are redacted and formats are preserved exactly**: ids remain strings, the account id keeps its colon, timestamps keep their offset without a colon. The spike script regenerates raw responses, and the redaction script must be re-run before committing.

**Facts verified on Claude Code 2.1.290–292** and relied on by the plugin: a plugin cannot ship permission rules; both `permissions.ask` rules and a mod's `tool.check` → `ask` prompt under `--dangerously-skip-permissions` and are denied under `-p`; a mod's `allow` overrides an ask rule; `tool.check` receives the raw command text; `prompt.submit` carries an origin kind but no cwd, and the session cwd is available by call; mods run in-process in the terminal CLI, Orca panes and the Desktop Code tab. If Claude Code changes any of these, the agent-integration decision is the one to revisit.
