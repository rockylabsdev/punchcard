# The CLI contract: one opt-in JSON envelope, exit codes that avoid 2, and Origin read from the environment

The `punchcard` CLI is the only seam between Punchcard and an agent. The Claude Code mod and Claude's own Bash calls are thin clients of `punchcard … --json` and never import the core. So the contract is meant to last. JSON is **opt-in only** (`--json` or `PUNCHCARD_JSON=1`), and stdout not being a TTY never switches it on. Every command writes the same envelope to stdout, failures included: `{v, ok, outcome, command, data, events, error}`. `v` is the contract version. Additive changes keep `v: 1`. `outcome` names the exit code as a string. `events` carries the side effects nobody asked for (a paused ticket, a recovery, a sync result). `--json` never prompts.

## Exit codes

| code | outcome | meaning |
|---|---|---|
| 0 | `ok` | done, or an idempotent no-op; reads always exit 0 |
| 1 | `internal` | unexpected error or failed write |
| 3 | `not_configured` | no credentials when a Jira call was needed |
| 4 | `no_timer` | nothing running or paused to act on, or an ambiguous target |
| 5 | `ticket_not_found` | Jira 404: missing, or not visible to this account |
| 6 | `needs_confirmation` | refused until it is confirmed; `data.preview` is the exact worklog; rerun with `--yes` |
| 7 | `sync_pending` | **partial success**: the change is on disk, the worklog is pending or failed |
| 8 | `auth_failed` | Jira returned 401 |
| 64 | `usage` | bad arguments (sysexits `EX_USAGE`) |
| 65 | `state_unreadable` | `state.json` written by a newer version (sysexits `EX_DATAERR`) |
| 75 | `busy` | state lock held after ~2 s (ADR 0002, `EX_TEMPFAIL`) |

**Exit 2 is never used.** A Claude Code hook that exits 2 blocks the user's action, and `usage` would be the obvious thing to put on 2. **Exit 7 is a success with debt.** `ok` is `true`, the stop happened, and only the sync is owed. A caller that sees 7 must run `sync`, never `stop` again.

## Origin

Every change has an **Origin** (see `CONTEXT.md`): `hand`, `prompt` or `agent`. The CLI infers `agent` when `CLAUDECODE` or `AI_AGENT` is set (Claude Code sets both on its agent's shell), and assumes `hand` otherwise. A hidden `--origin` flag overrides the inference: the mod passes `prompt` for focus from the user's own prompt, and `hand` for its own `/punch` command. Only `hand` sets **held**. `agent` is not presence. A command the user types with `!` inside Claude Code inherits the agent's environment and counts as `agent`. We accept that, because the TUI and a plain shell remain the ways to act by hand.

## Considered Options

- **JSON whenever stdout is not a TTY.** Rejected: it surprises `status | cut` in a shell prompt, and agents pass `--json` anyway.
- **Non-zero for a stop whose sync failed, without marking it a success.** Rejected: an agent that retries `stop` on failure would hit "nothing running", or stop the wrong entry. The stop did happen.
- **A confirmation token instead of `--yes`.** Rejected: an agent can copy a token as easily as a flag. The real human gate is a Claude Code `permissions.ask` rule or an MCP `requiresUserInteraction` tool, which the agent-integration ticket decides.
- **Origin from an explicit flag only.** Rejected: every agent call would have to remember the flag, and a forgotten flag would make agent work look like the user's presence and hold timers.

## Consequences

- Reads (`status`, `ls`) never call Jira and never write. Only `stop` (once, for its own worklog) and `sync` POST. Nothing syncs in the background.
- `start KEY` on a ticket that isn't on the list fetches the summary once, with a ~2 s budget. On a 404 it exits 5 and changes nothing. If Jira can't be reached, it starts anyway and the ticket is **unverified**.
