# Concepts

Each term below is used throughout Punchcard and in [CONTEXT.md](../../CONTEXT.md). Every one comes with one concrete scenario.

## Ticket and the list

A **ticket** is a Jira issue, named by its key (`ABC-123`). Your **list** is the tickets you have put on it, whether or not they have time against them. `punchcard ls` and the TUI's Tickets pane show the list, most recently active first. Starting a ticket that is not on the list adds it.

*Scenario:* Monday morning you run `punchcard add PUN-142 PUN-138 OPS-77` to queue the day. All three sit on the list with no time. You start `PUN-142` and it moves to the top.

## Entry, segment and worklog

An **entry** is one start-to-stop stretch of work on a ticket. Pausing does not end it. An entry is a list of **segments**, each one an uninterrupted stretch of counting, and the entry's time is their sum. Stopping the entry produces exactly one **worklog**: the record sent to Jira.

*Scenario:* You start `PUN-142` at 08:14, pause at 08:15, resume at 08:20 and stop at 08:41. That is one entry with two segments and 22 minutes measured. Jira gets one worklog of 22 minutes, not two.

The TUI shows the arithmetic: segments as `counted`, the gaps between them as `‖ paused 5m, not counted`.

## One timer that follows your attention

At most one ticket runs at a time. Starting another pauses the running one and says so. The timer measures *your* attention, not an agent's work: five agents on five tickets still make one timer, and a day's worklogs never add up to more hours than you lived.

*Scenario:* `PUN-142` is running. You run `punchcard start OPS-77`. Output: `⏸ PUN-142 paused`, then `▶ OPS-77 running`. In the TUI, `space` on another row does the same.

## Pause versus stop

**Pause** suspends counting and keeps the entry open. Resuming continues the same entry. Nothing reaches Jira. **Stop** ends the entry, asks you to confirm the duration, and sends the worklog. Stop is the only path to Jira.

*Scenario:* You go for coffee: `punchcard pause`, then `punchcard resume` when you are back. The 22 minutes you were away are not counted and the entry is still one entry. At the end of the task: `punchcard stop`, and Jira gets the worklog.

In the TUI, `space` on the running row pauses it, `p` pauses from any row, and `x` opens the stop dialog. The two are different keys on purpose.

## Held

A timer you start **by hand** (the CLI in your own shell, the TUI, or `/punch` in Claude Code) is **held**. A held timer never moves on its own: your prompts in Claude Code and an agent's `start` leave it alone. Only another act by hand moves it. A timer started by an agent, or moved by one of your prompts, is not held and follows your focus from session to session.

*Scenario:* An agent started `PUN-142` for you, so it is not held. You switch to a Claude Code session on branch `feature/OPS-77-certs` and send a prompt: the timer moves to `OPS-77`. Later you run `punchcard start PUN-142` yourself. Now it is held, and prompting in the `OPS-77` session no longer moves it. `punchcard status` shows the word `held` on the running line.

An agent that tries to move a held timer gets exit 4 with error code `held`: "the user started it by hand; only they can move or pause it".

## Presence, lastSeen, stale and recovered

**Presence** is evidence you are at the keyboard: a key in the TUI, a command you run, typing or submitting a prompt in Claude Code. An agent's activity, an idle open TUI, and anything that only reads state are not presence. The last moment of presence is **lastSeen**, kept in `heartbeat.json` and rewritten at most every 30 seconds.

A running timer with no presence for more than **2 hours** is **stale**. Nothing changes while it is only read: `status` shows it as stale with the time capped at lastSeen and tells you the next action will recover it. The first change that finds it stale **recovers** it: the open segment is cut off at lastSeen, the timer is left paused, and the entry is flagged **recovered**. The flag stays until you stop the entry, so you get the chance to correct the duration.

*Scenario:* You start `DEMO-1` at 07:21, press a key at 07:31, and walk away. At 10:31 nothing has happened since 07:31:

```
$ punchcard status
! DEMO-1 stale  0:10:00 (capped; 3:10:00 uncapped)
  last seen 07:31 (3h00m ago); the next action will recover it
```

The next change recovers it:

```
$ punchcard pause
! DEMO-1 recovered: no presence since 07:31, so it was capped there; fix the duration at stop
⏸ DEMO-1 paused  0:10:00  (recovered: check the duration at stop)
```

The `!` marker means recovered. It shows in `status`, `ls`, `status --short` (`! DEMO-1 stale` while still stale), the TUI row and headline (`! RECOVERED`), and the Claude Code band (`! DEMO-1 recovered · fix at stop`). A recovered entry's measured time is only a guess, so stopping it in a script or from an agent needs an explicit `--duration`; the TUI dialog and the terminal prompt ask you to type one.

The TUI shows the cut-off stretch as `! away 2h 22m, not counted`.

Why 2 hours and not a daemon watching you: [ADR 0001](../adr/0001-no-daemon-presence-based-lastseen.md).

## Unverified

A ticket added while Jira could not be reached (offline, not set up, or a 401) is **unverified**: it has no summary and might be a typo. Time against it counts normally. It is confirmed the next time a change to it reaches Jira. A key Jira says does not exist is never added at all.

*Scenario:* On a train you run `punchcard start DATA-9`. Output: `+ DATA-9 added (not verified with Jira yet)`. `ls` shows `(unverified)` in the summary column and the TUI shows `? unverified — Jira not reached yet`. Back online, `punchcard start DATA-9` (or `space` on its row) fetches the summary and the marker clears.

## Today

**Today** is the local calendar day. A ticket's time today is the part of its segments inside that day. A segment that runs over midnight is split there, so a timer left running overnight does not inflate tomorrow. A worklog belongs to the day its entry started.

*Scenario:* You start at 23:30 and stop at 00:30. The TUI's Tickets pane shows 30 minutes for the ticket today and showed 30 minutes yesterday. The worklog is dated yesterday, with 1 hour.

## Pending, synced and failed worklogs

A stopped entry's worklog is **pending** until Jira accepts it (**synced**, with Jira's worklog id stored) or refuses it (**failed**, with the message). Pending and failed are both "unsent" and both retried by `sync` or the TUI's `r`. A synced worklog is never sent again.

*Scenario:* You stop with Jira down. The worklog is failed with "could not reach Jira". `status` says `1 unsent worklog(s), 0:25:00`, every later change warns about it, and the TUI's Jira strip shows `✗ failed — unreachable · r`. An hour later `punchcard sync` sends it and the row becomes `✓ logged 11:02`.

A 401 is different: Jira rejected your credentials. The worklog stays pending, the token is left alone, and `status`, the TUI banner and the Claude Code band say `Jira rejected the token` until a call succeeds or you run `setup` again.

## Origin

Every change records who caused it: **hand** (you, in a shell or the TUI), **prompt** (your own prompt in a Claude Code session, relayed by the plugin) or **agent** (an agent acting on its own). Only hand holds a timer. Hand and prompt are presence; agent is not. The CLI assumes agent when it runs inside Claude Code (the `CLAUDECODE` or `AI_AGENT` variable is set) and hand otherwise. A command you type with `!` inside Claude Code inherits the agent's environment and counts as agent.

*Scenario:* The TUI is open while an agent starts `OPS-77` from a Claude Code session. Within a second the row reads `started by agent` and the status line says `agent started OPS-77`.

The TUI shows `you` for hand.
