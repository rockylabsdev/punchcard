# Research: Orca IDE surface (wayfinder #4)

**Question.** Orca IDE (https://www.onorca.dev, open-source, by stablyai) runs a fleet of Claude Code and Codex agents in parallel git worktrees and claims to read each repo's `.claude/` hooks. What surface does it expose, and is it its own integration layer or does it fall out of Claude Code hooks for free?

**Sources inspected (2026-10-05).**

- Source: `stablyai/orca` at commit `9756daeced4eda1dc888f14fefeeed1762aa5822` (2026-10-05, `package.json` version **1.4.214**). The website docs are generated from `docs/site/content/docs/**/*.mdx` in the same commit, so file paths below are the primary text behind the `https://www.onorca.dev/docs/...` pages; the live pages were also fetched and agree.
- Only Orca's docs and code were used. Cloned code was read, not run.

Paths below are relative to the Orca repo root unless they start with `https://`.

---

## 1. What Orca itself exposes

### 1a. Agent status hooks (working / waiting / done): Orca-managed, installed into Claude's **user-level** settings

Orca's "agent status hooks" are ordinary Claude Code `hooks` entries that Orca writes into **`~/.claude/settings.json`** (user scope, not the repo's `.claude/settings.json`):

- Config path: `getConfigPath()` returns `join(homedir(), '.claude', 'settings.json')` in `src/main/claude/hook-settings.ts`.
- Each entry is a `type: "command"` hook with `timeout: 10` (`MANAGED_HOOK_TIMEOUT_SECONDS = 10`, `src/main/agent-hooks/installer-utils.ts:47`) whose command is a wrapper that runs `~/.orca/agent-hooks/claude-hook.sh` if present, otherwise drains stdin and prints `{}` (`src/main/agent-hooks/runtime-home-hook-command.ts`; `getSharedManagedScriptPath` in `src/main/agent-hooks/installer-utils.ts:106`).
- Events registered for Claude (`CLAUDE_MANAGED_EVENTS`, `src/main/claude/claude-managed-hook-events.ts`): `SessionStart`, `UserPromptSubmit`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `TeammateIdle`, `PreToolUse` (`matcher: "*"`), `PostToolUse` (`*`), `PostToolUseFailure` (`*`), `PermissionRequest` (`*`), `PostCompact`, and `SessionEnd` (only for Claude >= 2.1.261). `PreCompact` is deliberately not registered. Each event is gated on the installed Claude CLI's version because older Claude versions discard the whole settings file on an unknown event name (`src/main/claude/claude-hook-event-versions.ts`).
- Orca also installs a `statusLine` command (`~/.orca/agent-hooks/claude-statusline.sh`) used for usage and rate-limit display, but only if the slot is empty and the user has not previously deleted it (`installManagedStatusLine` in `src/main/claude/hook-service.ts`; `applyManagedStatusLine` in `src/main/claude/hook-settings.ts`).
- Merge semantics: Orca **appends** its own entry to each event's array after removing any previous Orca entry; user entries under the same event are left intact (`applyManagedHooks`, `src/main/claude/hook-settings.ts`). `removeManagedHooks` strips only commands matching Orca's script name. Source comment: "Events whose Orca entry must go; user entries under them always stay."
- What the hook does: prints `{}` to stdout first (so permission-style hooks fail open), captures the full Claude JSON payload from stdin unchanged, then `curl -X POST http://127.0.0.1:${ORCA_AGENT_HOOK_PORT}/hook/claude` with `X-Orca-Agent-Hook-Token` and the pane/tab/worktree identity, with 0.5 s connect and 1.5 s max timeouts; on failure it spools the event to `~/.orca/agent-hooks/spool/pane-<id>.jsonl` (`src/main/claude/hook-script.ts`, `src/main/agent-hooks/hook-post-command.ts`, `buildPosixHookSpoolLines` in `src/main/agent-hooks/hook-stdin-contract.ts`). The hook exits 0 without posting when `ORCA_PANE_KEY` / `ORCA_AGENT_HOOK_PORT` / `ORCA_AGENT_HOOK_TOKEN` are absent (Claude running outside an Orca pane) and when `CLAUDE_JOB_DIR` is set (a backgrounded Claude job).
- Toggle: Settings > Agents > Agent status hooks, or `orca agent hooks status|on|off --json`; `off` removes the entries and stops reinstalling them (`docs/site/content/docs/agents/hooks-memory.mdx`, `src/cli/specs/agent-hooks.ts`).
- Status is also inferred from the terminal's OSC title sequence, which Claude Code emits (`docs/site/content/docs/model/agents-sessions.mdx`: "State is detected from the terminal's OSC title sequence and agent hooks").

**These are inbound to Orca.** They report Claude's state to Orca's UI. Orca does not expose an outbound "agent became working/waiting/done" callback that a third-party tool can subscribe to; the only consumer of the HTTP endpoint is Orca's own hook server (`src/main/agent-hooks/server.ts`, `src/relay/agent-hook-server.ts`). The same mechanism exists for Codex and about twenty other agents (`AGENT_HOOK_TARGETS`, `src/shared/agent-hook-types.ts`).

### 1b. Repository hooks: worktree setup and archive scripts

- `orca.yaml` at the repo root (committed) accepts `scripts.setup` (runs in the new worktree after creation) and `scripts.archive` (runs in the worktree **before** archive/removal, only when hooks are enabled; the CLI needs `--run-hooks`; a failed archive hook blocks removal unless the caller explicitly accepts the failure). See `docs/site/content/docs/model/orca-yaml.mdx`; implementation in `src/main/hooks.ts` (`HOOK_TIMEOUT = 120_000` ms, SIGTERM then force-kill of the hook's process tree) and `src/main/runtime/runtime-registered-local-worktree-removal.ts` (archive hook runs before the PTY stop and before `git worktree remove`).
- A per-repo local equivalent lives in Settings > Repository ("Auto-run commands on worktree create", "Per-repo base ref and hooks"), `docs/site/content/docs/settings.mdx:146-147`. A "Command source & orca.yaml" setting chooses YAML-only, local-only, or both.
- `setupAgentStartupPolicy: wait-for-setup` delays agent launch until setup succeeds.
- The setup runner receives `ORCA_ROOT_PATH` (primary checkout), `ORCA_WORKTREE_PATH` (new worktree), `ORCA_WORKSPACE_NAME` (worktree directory basename), plus compatibility aliases `CONDUCTOR_ROOT_PATH` / `GHOSTX_ROOT_PATH` (`docs/site/content/docs/model/orca-yaml.mdx`, `src/main/setup-hook-env-vars.ts`).
- `defaultTabs` opens extra terminals (with optional commands) once per new worktree. `issueCommand` is a prompt template for linked GitHub/GitLab items with `{{artifact_url}}` and legacy `{{issue}}` placeholders (the docs scope it to GitHub/GitLab; it is not documented for Jira).
- `.worktreeinclude` copies gitignored files (e.g. `.env`) into each new worktree; `worktree.sharedDirectories` links big directories like `node_modules`.

There is **no post-worktree-delete or post-sleep hook** beyond `scripts.archive`, and `scripts.archive` runs only on archive/removal with hooks enabled, not on sleep or hibernation (see section 4).

### 1c. CLI

`orca` (`orca-ide` on Linux) is a full CLI that talks to the running Orca runtime and is already on `PATH` inside Orca-launched terminals (`docs/site/content/docs/cli/overview.mdx`, `docs/site/content/docs/cli/reference.mdx`). Relevant commands:

- `orca status --json`, `orca worktree list|ps|current|show|create|set|rm`, `orca terminal list|read|send|wait|create|split|close`, `orca agent hooks status|on|off`, `orca automations ...`, `orca linear ...`, browser automation, computer use, emulator control.
- Selectors: `active` / `current` (the enclosing Orca worktree from the shell's cwd or terminal context), `id:`, `path:`, `branch:`, `issue:<n>`.
- `orca worktree set --worktree active --comment "..." --workspace-status in-progress` is the documented "checkpoint" pattern for agents to report progress (`docs/site/content/docs/cli/worktree-checkpoints.mdx`).
- `orca worktree create --name ... --agent claude --prompt "..." --issue N --pr N --linear-issue X --gitlab-issue N --gitlab-mr N`. There is **no `--jira-issue` flag** on `create` or `set` in this version (`src/cli/specs/core.ts:69`, `src/cli/specs/worktree-set.ts:8`).

### 1d. API

The CLI is a thin client over an RPC server inside the Orca runtime (`src/main/runtime/rpc/methods/*`); `orca serve --port 6768` runs it headless. The RPC is not documented as a public API; the documented contract is the CLI with `--json`.

### 1e. MCP

Orca does **not** run an MCP server of its own. "MCP" in Orca's docs means: (a) Settings > Integrations > MCP registers third-party MCP endpoints and mirrors them into the agents' own MCP configs (`docs/site/content/docs/cli/skills.mdx` "MCP servers"; Codex mirror in `src/main/codex/config-toml-mcp-servers.ts`), and (b) `orca linear save-issue` etc. mimic the Linear MCP's write semantics over the CLI. No `src/main/mcp*` directory exists.

### 1f. Skills and plugins

- Orca ships agent skills (`skills/orca-cli`, `skills/orchestration`, `skills/orca-linear`, ...) installable with `npx skills add https://github.com/stablyai/orca --skill orca-cli` or `orca skills install --skill orca-cli --agent claude-code,codex`; a stub `SKILL.md` lands in the agent's skills directory and `orca skills get <name>` serves the version-matched guide (`docs/site/content/docs/cli/skills.mdx`).
- An app-level plugin system exists (`stablyai/orca-plugins` marketplace: themes, keybindings, language packs). Nothing in the docs or `src/main/plugins/` describes a plugin API for agent-lifecycle events.

### 1g. Launch defaults that matter for Punchcard

- Orca launches Claude with `--dangerously-skip-permissions` by default ("the worktree itself is the sandbox"); editable per agent in Settings > Agents (`docs/site/content/docs/model/agents-sessions.mdx` "Launch defaults", `src/shared/tui-agent-permissions.ts:7`).
- Orca pre-trusts the worktree folder in `~/.claude.json` so Claude does not show the folder-trust prompt (`src/main/claude/claude-folder-trust-file.ts`).
- Orca does **not** pass `--settings` or `--setting-sources` to Claude (no occurrences under `src/main`), and it only sets `CLAUDE_CONFIG_DIR` when the user picked a non-default managed account home (`src/main/claude/claude-config-dir-pin.ts`). So Claude's normal settings resolution (user, then project `.claude/settings.json`, then `.claude/settings.local.json`) applies unchanged.
- Resume: hibernated or restarted sessions relaunch with `claude --resume <id>` and "the launch command, arguments, and private environment captured when Orca first opened the agent" (`docs/site/content/docs/agents/hibernation.mdx`).

---

## 2. How Orca treats Claude Code hooks

**The repo's `.claude/settings.json` hooks run unchanged inside an Orca-launched agent.** Evidence:

- Docs: "Orca reads each repo's `.claude/` and `.codex/` configuration. Hooks you already have will run when Orca launches the agent in a worktree of that repo." (`docs/site/content/docs/agents/hooks-memory.mdx`, "Per-repo hooks"). The word "reads" is generous: there is no code in `src/main/claude/` that parses a repo's `.claude/settings.json`; Orca simply launches `claude` with the worktree as cwd and lets Claude load its own project settings. Orca's only writes are to `~/.claude/settings.json` (1a) and `~/.claude.json` (folder trust).
- Orca never writes to the project's `.claude/settings.json` or `.claude/settings.local.json` (grep over `src/main` for `.claude/settings` and `settings.local.json` returns only test fixtures and comments).
- Orca does not pass `--settings`, `--setting-sources`, `--disallowedTools`, or similar at launch (1g), so no settings source is suppressed.

**Events Orca adds.** Its own user-level entries on the thirteen events listed in 1a, run alongside (not instead of) the project's hooks for the same event. Claude runs all matching hooks for an event; Orca's `{}` stdout and exit 0 make it a no-op for `PermissionRequest`, `PreToolUse`, `UserPromptSubmit`, and `Stop`, so it does not alter decisions a project hook makes.

**Events Orca suppresses.** None. The only subtractive behaviour is `orca agent hooks off`, which removes Orca's own entries and leaves the user's.

**Caveats for a Punchcard hook.**

- The agent's `~/.claude/settings.json` will contain Orca's entries; a Punchcard hook that inspects the merged hook list should expect them.
- Orca's hook is a 10 s-timeout shell script firing on every tool call; it is independent of any Punchcard hook, but a Punchcard hook on `PreToolUse` / `PostToolUse` runs alongside it on every tool call.
- Because Orca launches with `--dangerously-skip-permissions`, a `PermissionRequest` hook will not fire in an Orca-launched session (Claude does not ask). Punchcard's "stop-and-submit needs a human confirm" policy therefore cannot rely on Claude's permission prompt when running under Orca; it must be enforced by Punchcard itself (the CLI refusing an unattended stop-and-submit without an explicit flag, or a TUI confirmation).
- A session resumed by Orca (hibernation, Restart chip) re-runs `SessionStart` with `source: "resume"`; a Punchcard `SessionStart` hook should be idempotent.

---

## 3. Ticket association and injected environment variables

### 3a. Does an Orca worktree carry a ticket?

Yes, as **worktree metadata**, including Jira:

- `WorkspaceLinkedItem = { provider: 'github' | 'gitlab' | 'linear' | 'jira', type: 'issue' | 'pr' | 'mr', number, title, url, linearIdentifier?, jiraIdentifier?, repoId? }` and `Worktree.linkedWorkItem` (`src/shared/worktree/types.ts:12-21`, `src/shared/worktree/meta-types.ts:51`).
- Jira linking is UI-only in this version: create a worktree from the Jira drawer, paste a `https://.../browse/ABC-123` URL into the name field, or pick an issue via Jira search; Orca "fills the workspace name, links the issue, and shows the key + summary on the worktree card with View on Jira" (`docs/site/content/docs/review/jira.mdx`). The workspace display name and branch seed are derived from the Jira key plus a compacted title (`getLinkedWorkItemWorkspaceName`; `workItemIdentity` returns `jiraIdentifier.toUpperCase()`, `src/shared/workspace-name.ts:158-195`). The Jira key is therefore usually present in the **branch name** and **worktree directory name**, but that is a convention of the composer, not a contract: the user can override the branch in the Advanced drawer (`docs/site/content/docs/model/worktrees.mdx`, "Naming the branch"), and "Auto-Rename Branch From Work" can rename an auto-generated creature branch after the agent's first message (`src/main/agent-hooks/first-work-branch-rename.ts`; `docs/site/content/docs/settings.mdx:38`), though that rename targets only `isAutoGeneratedCreatureBranchName` branches, not issue-derived ones.
- Reading the link programmatically: `orca worktree show --worktree active --json` returns the `RuntimeWorktreeRecord`, which is `Worktree & {...}` and so includes `linkedWorkItem` (`src/shared/runtime-worktree-contracts.ts:114`). The human-readable formatter prints `linkedIssue` (GitHub number) only (`src/cli/workspace-format.ts`). Writing the link from the CLI is possible for GitHub, Linear, and GitLab but **not Jira** (1c).
- The Jira key is **not** injected into the agent's environment and **not** appended to the agent's prompt by Orca. The `issueCommand` template (`{{artifact_url}}`) is documented for GitHub/GitLab items in the desktop composer; nothing in `src/main` routes `jiraIdentifier` into a prompt or env var (the only non-UI consumers are workspace naming, URL matching, and metadata normalisation).

### 3b. Environment variables Orca injects into an agent's process

Verified in the PTY spawn-env assembly (`src/main/ipc/pty/ipc/spawn-env.ts:131-150`, `src/main/ipc/pty/host-env/spawn-env-keys.ts`, `src/main/daemon/pty-subprocess/spawn-environment.ts`, `src/main/runtime/orca-runtime-core.ts:277-282`):

| Variable | Meaning | Where |
| --- | --- | --- |
| `ORCA_WORKTREE_ID` | Orca's worktree id; shape is `repoId::absolute-worktree-path` (comment in `src/main/runtime/structured-session-worktree-teardown.ts`) | `spawn-env.ts:136` |
| `ORCA_PANE_KEY` | stable id of the terminal pane (`tabId:leafId`) | `spawn-env.ts:132` |
| `ORCA_TAB_ID` | tab id | `spawn-env.ts:134` |
| `ORCA_AGENT_LAUNCH_TOKEN` | per-launch token used to attribute hook posts | `local-pty-launch-helpers.ts:12` |
| `ORCA_AGENT_HOOK_PORT`, `ORCA_AGENT_HOOK_TOKEN`, `ORCA_AGENT_HOOK_ENV`, `ORCA_AGENT_HOOK_VERSION`, `ORCA_AGENT_HOOK_TRANSPORT`, `ORCA_AGENT_HOOK_ENDPOINT` | the loopback hook server coordinates; `ORCA_AGENT_HOOK_ENDPOINT` points at `{userData}/agent-hooks/endpoint.env`, re-sourced on every hook call so sessions survive an Orca restart | `spawn-env-keys.ts`, `hooks-memory.mdx` |
| `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` | set to Orca's identity and version | `spawn-environment.ts:178` |
| `ORCA_AGENT_SESSION_ID`, `ORCA_STRUCTURED_SESSION` | only for "structured" (native-chat / orchestration) sessions, not terminal agents | `structured-session-child-identity-env.ts:61` |

Not injected into agent terminals in this version: `ORCA_WORKTREE_PATH`, `ORCA_WORKSPACE_NAME`, and `ORCA_ROOT_PATH` are documented and verified only for the **setup-script runner** (`src/main/setup-hook-env-vars.ts`); `src/main/pty/wsl-orca-env.ts` forwards them to WSL guests if present, but I found no code that sets them on a regular agent PTY. Treat `ORCA_WORKTREE_ID` (which embeds the path) and `$PWD` as the reliable identity signals, and `TERM_PROGRAM` as the "am I inside Orca" check.

Orca also strips inherited `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_SESSION_ID`, and `CLAUDE_CODE_BRIDGE_SESSION_ID` so a Claude started from an Orca pane is not treated as a nested child (`CLAUDE_CHILD_SESSION_STAMP_ENV_KEYS`, `spawn-env-keys.ts`).

### 3c. Implication

The only machine-readable path from an Orca agent to "which Jira ticket is this?" is: Claude hook payload (`cwd`) or `$ORCA_WORKTREE_ID`, then `orca worktree show --worktree active --json`, then `linkedWorkItem.jiraIdentifier` (present only when the worktree was created from Jira), with a fallback of parsing an `ABC-123` pattern out of the branch or directory name. Neither is guaranteed; Punchcard must still accept an explicit key.

---

## 4. Process lifecycle: what happens to a background process when Orca stops or discards the worktree

### 4a. Terminal close, worktree sleep, hibernation, Restart

Every one of these goes through the same PTY shutdown path, `shutdownLocalPty` in `src/main/providers/local-pty-termination.ts`:

1. For an agent PTY (or any `immediate` close), Orca first snapshots the PTY root's descendant tree via `ps -axo pid,ppid,pgid,lstart` and a **ppid walk** from the shell (`captureDescendantSnapshot`, `collectDescendantRows` in `src/main/pty-descendant-termination.ts`).
2. It sends **`SIGTERM` to every descendant** in that snapshot, then after `DESCENDANT_KILL_GRACE_MS = 2_000` sends **`SIGKILL`** to survivors whose pid, start time, and pgid still match (`terminateDescendantSnapshot`).
3. It signals the root: `SIGTERM` for a graceful close, escalating to `forceKillPosixPtyProcessGroups` (which `SIGKILL`s **every process group attached to the PTY's controlling tty**) after `LOCAL_PTY_GRACEFUL_FORCE_TIMEOUT_MS = 5_000`, or immediately for an immediate close (`killLocalPtyProcess`, `armLocalPtyForceKill`; `src/main/pty/posix-pty-process-groups.ts`). Closing the pty also delivers the kernel's `SIGHUP` to the foreground job.
4. Docs confirm the intent: "Close stops the process as part of removing the terminal", and bulk close "returns a failing `unverifiable` result rather than claiming the processes exited" when a stop cannot be confirmed (`docs/site/content/docs/cli/reference.mdx`, "Terminals"). Sleep "closes active panels" (`docs/site/content/docs/model/worktrees.mdx:115`); hibernation "quietly stop[s] those terminals" and later relaunches `claude --resume <id>` (`docs/site/content/docs/agents/hibernation.mdx`).

**Consequence for a Punchcard daemon.** A daemon started by the agent as a plain child (same session, in the ppid tree, or in a process group on the PTY's tty) **will be SIGTERM'd and then SIGKILL'd within about 2 s** when the terminal is closed, the worktree is slept or hibernated, or the agent is restarted. The only way to survive is to be fully detached before the sweep: double-fork or `setsid` so the daemon is reparented to pid 1 **and** has no controlling tty. Orca's own comments acknowledge this boundary ("a dead root's descendants reparent to pid 1 and become unfindable"; the group kill only targets groups whose `tty` matches the PTY's, `getPosixPtyProcessGroups`). A detached daemon is therefore invisible to Orca, which is both the survival mechanism and the hazard: Orca will not clean it up.

### 4b. Worktree delete and archive

`removeRuntimeRegisteredLocalWorktree` (`src/main/runtime/runtime-registered-local-worktree-removal.ts`) runs, in order: the `scripts.archive` hook (if enabled, with the worktree as cwd, before anything is torn down), a clean-tree preflight (dirty worktrees are refused unless `--force`), `stopPtys()` which calls `killAllProcessesForWorktree` (`src/main/runtime/worktree-teardown.ts`), then `git worktree remove` and branch deletion. `killAllProcessesForWorktree` sweeps three registries (renderer leaves, provider session list, local pty registry) with a 10 s budget (`WORKTREE_PROCESS_SWEEP_TIMEOUT_MS`), and for a destructive removal a stop that cannot be proven **blocks the deletion** unless an explicit Force Delete or `--force` waives it (`allowUnverifiedStop`). Each PTY it finds dies via 4a.

A detached daemon whose cwd is the worktree keeps running with its cwd unlinked. If it holds its state file inside the worktree it will keep writing to an orphaned inode. For Punchcard this is a reason to keep `state.json` in the user's data directory (as v1 already planned), not under the repo.

### 4c. App quit

`killAllLocalPtys` (`src/main/providers/local-pty-termination.ts`) runs synchronously at quit: agent PTYs get `forceKillPosixPtyProcessGroups` (SIGKILL on every group on the tty) with `proc.kill('SIGKILL')` as fallback; plain shells get `proc.kill()` (SIGHUP). There is no graceful window at quit and no hook fired at quit.

### 4d. Orca restart without quitting the agent

Orca can run PTYs in a daemon subprocess so terminals survive an Orca app restart; hook endpoints are persisted to `{userData}/agent-hooks/endpoint.env` for the same reason (`hooks-memory.mdx`, "Surviving a restart"). This affects only Orca's own hook plumbing; it does not change the kill semantics above.

### 4e. What Orca does not do

- No pre-close, pre-sleep, or pre-delete hook is delivered to the agent or the repo except `scripts.archive` on archive/delete with hooks enabled. There is no "worktree is about to be killed" signal a Punchcard daemon could listen for other than `SIGTERM` itself.
- Claude's own `SessionEnd` hook does fire on a graceful `SIGTERM` to Claude (Claude handles the signal). Orca registers its own `SessionEnd` entry only for Claude >= 2.1.261, and that registration does not affect whether the event reaches the project's `SessionEnd` hook. On the 2 s SIGKILL escalation, nothing fires.

---

## Direct answer: is Orca its own integration layer, or does it fall out of Claude Code hooks for free?

**It falls out of Claude Code hooks for free; Orca is not an integration layer Punchcard needs to target.**

- Orca runs the stock `claude` binary with the worktree as cwd, no `--settings` override, and normal settings resolution. A Punchcard hook declared in the repo's `.claude/settings.json` (or the user's) fires identically inside and outside Orca. Orca's own status hooks are additive user-level entries that coexist with it.
- Orca offers **no outbound event surface** (no agent-lifecycle callback, no MCP server, no plugin API for agents) that Punchcard could subscribe to instead of Claude's hooks. Its "agent status hooks" are inbound to Orca's UI.
- The only Orca-specific signals worth using are cheap and optional: `TERM_PROGRAM` to detect Orca, `ORCA_WORKTREE_ID` (or hook `cwd`) plus `orca worktree show --worktree active --json` to recover a linked `jiraIdentifier` when the worktree was created from a Jira issue, and `orca worktree set --comment` as a nice-to-have progress checkpoint. All three can live inside the Punchcard CLI or a Claude hook as a best-effort key guess; none justify an Orca-specific layer.

Two constraints Orca does impose on the agent-integration design:

1. **The unattended-action policy cannot rely on Claude's permission prompt**, because Orca launches with `--dangerously-skip-permissions`. Any "stop-and-submit needs a human" rule must be enforced by Punchcard (the CLI refuses an unattended submit without an explicit opt-in; the TUI confirms).
2. **A Punchcard daemon must not be a child of the agent's terminal** if it should outlive terminal close, sleep, hibernation, restart, or worktree delete; Orca SIGTERMs then SIGKILLs the whole descendant tree within about 2 s and the process groups on the tty within 5 s. Either make the daemon independent (`setsid` or double-fork, state in the user data directory rather than the repo) or design the process model so that a dead daemon is harmless (timestamps on disk, `lastSeen` cap), which is the process-model ticket's question, not Orca's.

**Gaps the docs and code do not settle.** Whether `ORCA_WORKTREE_PATH` / `ORCA_WORKSPACE_NAME` reach agent terminals on some hosts (only the setup runner is documented); whether the desktop composer's `issueCommand` prompt template applies to Jira items (the docs say GitHub/GitLab). Neither changes the answer.
