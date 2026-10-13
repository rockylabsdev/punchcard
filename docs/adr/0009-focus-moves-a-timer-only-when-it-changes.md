# Focus moves a timer only when it changes

A prompt's focus moves an un-held running timer only when the prompt's session ticket differs from the **last focus**, the session ticket of the user's previous keyed prompt (see `CONTEXT.md`). Before this, every prompt re-asserted its session's ticket. That undid any switch the user made from inside that session that was not **held**. In #41 the user asked the agent to "start OPS-77" in a session on PUN-1, and the next prompt moved the timer back to PUN-1. The same happened to a `!` command and to an agent's own start. Edge-triggered focus fixes every one of those routes at once. The timer still follows the user between sessions, and **held** keeps meaning "set by hand".

Every keyed prompt records the last focus, including when the timer is idle or held. So does a `punchcard touch KEY` typed by hand, which is the same presence call; an agent's touch does not. If it didn't, a stale last focus would make the next prompt look like a change and bring the bug back. A prompt from a session with no ticket leaves it alone. Hand starts, agent starts, pauses and stops never write it, and it never expires: a reset of any kind would make the next prompt a "change" and pull the timer back.

## Considered Options

- **An agent's start at the user's request is held (the agent passes `--origin hand`).** Rejected: a held timer stops following the user between sessions until they act by hand. The CLI would have to trust the agent's claim, and a `!` command would still be undone.
- **Suppress focus for the rest of the turn or session after an agent start.** Rejected: it needs per-session state, which Punchcard does not keep (ADR 0001), and "turn" has no meaning to the CLI.
- **Keep it as is and teach `/punch start`.** Rejected: saying "start OPS-77" in plain words would mean less than the slash command, and the undo surprises the user.

## Consequences

- Focus no longer pulls the timer back to the session's ticket. To return, the user says so, or uses `/punch start`. The status line already shows "`KEY` not running · /punch start".
- One more stored value, `lastFocus` in `state.json`, written on presence. It is additive: a file without it reads as empty, which differs from every key, so the first prompt after the upgrade behaves as before.
- A prompt that finds the state lock busy is skipped, as presence always was, so its last focus is not recorded. A quick prompt from another session's ticket can then go unseen as a change. This is rare and self-correcting at the next change of focus, so the touch does not wait for the lock.

## Amendment, 2026-10-12: edge-triggering now applies to a timer set by hand too

Because every keyed prompt records the last focus but a start by hand does not, a timer started by hand stays put through further prompts in the current session and moves at the user's first prompt in a session on another ticket. ADR 0010 lifts the "un-held" qualifier: a change of focus moves the running timer whoever set it. "Held" is renamed **by hand** and keeps only its guard against agents.
