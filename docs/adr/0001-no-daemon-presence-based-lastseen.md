# No daemon: a timer is an open segment, and `lastSeen` records presence, not liveness

Punchcard v2 has no background process. The CLI is stateless, and the TUI is one more writer of the same files. A running timer is an open segment with a start time, so no process needs to be alive for it to run. `lastSeen` no longer means "the app was alive". It means "the user was last present": their own keystrokes in the TUI, commands they ran, or prompts they typed in a Claude Code session. Recovery no longer waits for a relaunch. The first act of presence to find a timer **stale** (no presence for longer than a threshold, 2 h by default) caps it at `lastSeen` and flags it recovered.

We rejected a daemon because its heartbeat measures the wrong thing. It tells you the machine was awake, not that the user was working. v1's menu-bar heartbeat had the same blind spot: it only ever capped a crash, never a timer forgotten on an awake machine. A daemon would also cost a process to launch, find, keep alive and keep in step across upgrades, and Orca kills every process tree it launched, so a daemon would have to detach itself completely. Keeping a single writer does not need a daemon. A lock is enough, and that is the concurrent-writers decision.

## Considered Options

- **Daemon (spawned on demand, or a launchd/systemd user service).** Rejected for the reasons above.
- **TUI heartbeat only while open (v1 semantics on a new platform).** Rejected: a TUI left open in tmux keeps a forgotten timer alive all weekend. That is liveness again, under another name.
- **No `lastSeen`, no cap.** Rejected: it gives up the carried recovery rule, and the persistent marker that makes a wrong duration hard to miss.

## Consequences

- **The timer measures the user's attention, so there is one timer however many agents run in parallel.** Switching follows the user's own prompts, and a timer set by hand is **held** against them (see `CONTEXT.md`: Presence, Focus, Held). A prompt switches only when its focus changes (ADR 0009). Telling a typed prompt from a scheduled, subagent or peer one needs a Claude Code mod (`prompt.submit` `e.origin.kind`). Settings hooks cannot tell the difference.
- **Reads (`status`, `ls`) are pure.** They report a timer as `stale` with its capped elapsed, but never write.
- **Stretches longer than the threshold with no input anywhere get capped.** The duration edit at stop is where that gets corrected, and the recovered marker stays until stop (not until resume) so the user gets there.
- `heartbeat.json` is no longer written on a 30-second tick. It is written on presence events, rate-limited.

## Amendment, 2026-10-12: focus moves a timer set by hand

A timer set by hand is no longer **held** against the user's own prompts. A change of focus moves the running timer whoever set it; "held" is renamed **by hand** and means only that an agent cannot move it. See ADR 0010.
