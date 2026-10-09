# Agent integration: one optional plugin, and an agent's stop is proposed then gated

Claude Code drives Punchcard through one optional plugin, `punchcard`, installed at user scope and versioned with the binary. It holds a **mod** and a **skill**. There is no MCP server and there are no settings hooks. The mod turns the user's own prompts into presence and focus (`touch --cwd <dir> --origin prompt` on a `composer` or `bridge` submit; typing is presence only), draws the timer in the band above the prompt, adds `/punch` (origin `hand`), and gates the agent's stop. The CLI and the TUI work fully without the plugin.

**An agent proposes a stop and the user disposes.** With origin `agent`, `punchcard stop` without `--yes` always exits 6 (`needs_confirmation`) with `data.preview`, the exact worklog, and the agent shows it. Only `stop --yes` reaches Jira, and that call is **gated**: Claude Code puts a yes/no to the human. With no human present (`claude -p`, a scheduled task) the gated call is **denied**, never silently performed, and the timer stays running until the user stops it by hand or it goes stale. Exit 6 is the policy; the gate is the enforcement.

**Two gates, one dialog.** The mod's `tool.check` returns `ask` for a Bash command matching `punchcard stop … --yes`, with a `.catch` that also returns `ask` so a crash fails closed. `setup` offers to add `Bash(punchcard stop *--yes*)` to the `permissions.ask` list in `~/.claude/settings.json` as a second gate that holds when the plugin is disabled. Both prompt under `--dangerously-skip-permissions` (Orca's default) and both are denied under `-p`, verified on Claude Code 2.1.291. The mod never returns `allow` for that command, because a mod's `allow` overrides an ask rule.

## Considered Options

- **An agent never stops; only the user does.** Rejected: "stop and submit without leaving the session" is the headline use case, and a gated stop costs one keypress.
- **An agent's stop parks the worklog awaiting approval.** Rejected: a new domain state to do what a dialog already does.
- **An MCP `stop` tool with `requiresUserInteraction`.** Deferred: a long-lived server and a second contract beside `--json`, for a guarantee the two gates already give.
- **One gate on every `punchcard stop`.** Rejected: the dialog shows only the command line, not the worklog. Gating only `--yes`, after the preview, means the human approves what they have just read.
- **Mod gate only / ask rule only.** Rejected: the mod gate vanishes with the plugin; the rule needs a settings edit and misses unusual spellings (a full path, an `env` prefix).
- **Orca's linked work item as a key source.** Rejected: Punchcard must work the same in any terminal. The key comes from an explicit argument or the git branch name, nothing else.

## Consequences

- **A session whose branch carries no key gives presence but never focus.** Starting a ticket there takes an explicit key, which holds the timer.
- **An agent's `start`/`pause`/`resume` are ungated** and move only an un-held timer, the same rule as focus from a prompt.
- **Closing, sleeping or deleting a terminal or worktree does nothing to the timer**, in Orca or anywhere else.
- **No context is injected at session start.** The model-invocable skill teaches the agent the commands and the rules (an agent's stop needs `--yes`; exit 7 means `sync`, never `stop` again).
- **The mod relies on nothing implicit:** it passes `--origin` on every call, since `CLAUDECODE` is not guaranteed in its child processes.

## Amendment, 2026-10-09: auto mode

A hand check on Claude Code 2.1.292 in **auto mode** (the permission mode whose decider is a classifier) ran an agent's `punchcard stop --yes` with no dialog. The mod API is explicit: an `ask` verdict "puts it to the mode's decider (the dialog, the auto-mode classifier, a headless host)", so the mod's `tool.check` → `ask` is a dialog only in the default and bypass modes. The settings ask rule is documented to force a prompt in every mode, but the user had declined it.

The mod therefore gates in two layers. `tool.check` still answers `ask` (and now keeps an `ask` that cites the user's own rule, rule and all). A `tool.call` hook on the same commands asks the engine's verdict with `$.tool.check` and, where that verdict would not reach a person (auto mode, or a mode not yet seen, with no user rule and no deny), raises its own dialog with `$.ui.ask`, Send to Jira or Keep the timer running, and refuses the call unless the person chose Send. Under `claude -p` that dialog rejects, so the call is refused with the timer running. The permission mode reaches the mod on the classic hook inputs (`SessionStart`, `UserPromptSubmit`). The design's guarantee is unchanged; what changed is that the `ask` verdict alone was never enough to carry it.
