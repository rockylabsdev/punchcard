# Research: Claude Code integration surface (hooks, skills, MCP)

Resolves wayfinder ticket #3. Answers, point by point, what Claude Code exposes that Punchcard could hang off, so ticket #10 (agent integration surface and unattended-action policy) can choose which layers ship in v1.

## Sources and currency

Inspected on **2026-10-05**. The surface moves fast; re-check before relying on a detail.

| Source | What was used | Version evidence |
| :- | :- | :- |
| Claude Code docs, `code.claude.com/docs/en/*` (hooks, hooks-guide, skills, slash-commands, mcp, permissions, permission-modes, settings, env-vars, statusline, headless, tools-reference, cli-reference, plugins-reference) | Every claim below unless marked otherwise | Pages carry no visible "last updated" date. The newest Claude Code version any inspected page cites is **v2.1.288** (env-vars, tools-reference, claude-directory); the hooks reference cites up to v2.1.274 |
| MCP specification, revision **2025-06-18**: `modelcontextprotocol.io/specification/2025-06-18/server/tools` and `.../client/elicitation` | Tool declaration, elicitation protocol, human-in-the-loop requirements | Note: the Claude Code MCP page refers to a newer "protocol revision 2026-07-28" for form/URL elicitation capabilities; that revision was not inspected |
| Installed CLI: `claude --version` → **2.1.290**; `claude --help`, `claude mcp --help`, `claude mcp add --help` | Flags the docs are thin on (`--permission-prompts`, `--bare`, `--strict-mcp-config`, `mcp add --scope`) | 2.1.290 |
| Published settings schema, `https://json.schemastore.org/claude-code-settings.json` (title "Claude Code Settings") | Hook handler fields, `permissions.ask`, MCP approval keys | Lists 31 hook events; the docs list 33 (schema lacks `PreModelSwitch`/`PostModelSwitch`), so the schema lags the docs slightly |
| Empirical, this machine | `CLAUDE*` env vars present in a Bash-tool child; top-level keys of session transcript `.jsonl` lines | Marked **[observed, undocumented]** where the docs do not back it |

Where a claim could not be verified it is marked **[not verified]**.

---

## 1. Hooks

Reference: https://code.claude.com/docs/en/hooks (all section anchors below are on that page unless stated). Guide: https://code.claude.com/docs/en/hooks-guide.

### 1.1 Complete event list

33 events as of the inspected reference. "Block" = can stop the action; "Inject" = can add text Claude sees; "Ask" = can force a human permission prompt.

| Event | Fires when | Block | Inject | Ask |
| :- | :- | :- | :- | :- |
| `SessionStart` | session starts/resumes; matchers `startup`, `resume`, `clear`, `compact`, `fork` | No | Yes (stdout plain text or `additionalContext`) | No |
| `Setup` | `--init`, `--init-only`, `--maintenance` runs; matchers `init`, `maintenance` | No | No | No |
| `UserPromptSubmit` | a prompt is submitted, before Claude sees it. Also fires on scheduled tasks, a background subagent reporting back, and cross-session messages | Yes (exit 2 / `decision: "block"`) | Yes | No |
| `UserPromptExpansion` | a typed `/command` expands into a prompt; matcher = command/skill name | Yes | Yes | No |
| `PreToolUse` | before a tool call; matcher = tool name (`Bash`, `mcp__server__tool`, regex) | Yes (`permissionDecision: "deny"` / exit 2) | Yes (`additionalContext`) | **Yes** (`permissionDecision: "ask"`) |
| `PermissionRequest` | Claude Code is about to show a permission prompt | decides allow/deny instead of the user | No | No (it *answers* prompts) |
| `PermissionDenied` | auto mode denies a call; can return `retry: true` | No | No | No |
| `PostToolUse` | after a tool succeeds | No (tool already ran; `decision: "block"` only feeds Claude a reason) | Yes (`additionalContext`, `updatedToolOutput`) | No |
| `PostToolUseFailure` | after a tool fails | No | Yes | No |
| `PostToolBatch` | a batch of parallel tool calls resolves | Yes | Yes | No |
| `Notification` | Claude Code emits a notification; matchers `permission_prompt`, `idle_prompt`, `auth_success`, `elicitation_dialog`, `elicitation_url_dialog`, `elicitation_complete`, `elicitation_response`, `agent_needs_input`, `agent_completed`, `quota_auto_resume_*` | No ("Notification hooks can't block or modify notifications") | No | No |
| `MessageDisplay` | assistant text is displayed; can replace display text only | No | No | No |
| `SubagentStart` / `SubagentStop` | a subagent spawns / finishes; matcher = agent type | Stop: yes | Yes | No |
| `TaskCreated` / `TaskCompleted` | task-list items created / marked done | Yes | No | No |
| `Stop` | "the main Claude Code agent has finished responding. Does not run if the stoppage occurred due to a user interrupt" | Yes (keeps Claude working; 8-consecutive cap, `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`) | Yes | No |
| `StopFailure` | turn ends on an API error; output ignored | No | No | No |
| `TeammateIdle` | agent-team teammate about to idle (experimental) | Yes | No | No |
| `InstructionsLoaded` | a CLAUDE.md / rules file loads; audit only | No | No | No |
| `ConfigChange` | a settings/skills file changes mid-session; matchers `user_settings`, `project_settings`, `local_settings`, `policy_settings`, `skills` | Yes (except `policy_settings`) | No | No |
| `CwdChanged` / `DirectoryAdded` | working dir changes / `--add-dir` | No | No | No |
| `FileChanged` | a watched file changes; matcher = literal filenames | No | No | No |
| `WorktreeCreate` / `WorktreeRemove` | `--worktree` or subagent `isolation: worktree`; any non-zero exit fails it | Yes | No | No |
| `PreCompact` / `PostCompact` | before / after context compaction | Pre: yes | No | No |
| `PreModelSwitch` / `PostModelSwitch` | before / after `/model` | Pre: yes (`ask` prompts only in interactive `/model`) | Post: yes | Pre: limited |
| `Elicitation` / `ElicitationResult` | an MCP server requests user input / the user answered; matcher = server name | Yes (deny) | No | No (it *replaces* the dialog) |
| `SessionEnd` | session terminates; matchers `clear`, `resume`, `logout`, `prompt_input_exit`, `other` | No ("can't block session termination") | No | No |

Two consequences for Punchcard:

- **There is no "task finished" event.** `Stop` fires at the end of *every* assistant turn, not when the agent considers a ticket done. `SessionEnd` is the only end-of-work signal and it is best-effort: its hooks share a **1.5-second default budget** (raisable per-hook `timeout` up to 60 s, or `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS`), cannot block, and have their JSON output discarded (#sessionend). On SIGTERM in `-p` mode Claude Code still runs `SessionEnd` and nothing else (https://code.claude.com/docs/en/headless#stop-a-run-with-sigterm).
- **`SessionStart` is the natural "inject timer status" point**: plain stdout from a `SessionStart` hook is added to Claude's context (#exit-code-0), so `punchcard status --json` output can be injected at every startup/resume/clear/compact.

### 1.2 stdin JSON shape

Every hook receives one JSON object on stdin (#common-input-fields):

```json
{
  "session_id": "abc123",
  "prompt_id": "550e8400-...",            // v2.1.196+, correlates with OTel events
  "transcript_path": "~/.claude/projects/<proj>/<session>.jsonl",
  "cwd": "/path/where/hook/was/invoked",
  "scratchpad_dir": "...",                 // v2.1.257+
  "permission_mode": "default|plan|acceptEdits|auto|dontAsk|bypassPermissions",
  "effort": {"level": "low|medium|high|xhigh|max"},
  "hook_event_name": "PreToolUse",
  "agent_id": "...", "agent_type": "..."   // only in subagent / --agent context
}
```

Event-specific additions that matter here:

| Event | Extra fields |
| :- | :- |
| `SessionStart` | `source` (`startup`/`resume`/`clear`/`compact`/`fork`), `model`, optional `session_title`, and on resume cache-expiry fields |
| `UserPromptSubmit` | `prompt` (the text), optional `session_title` |
| `PreToolUse` / `PostToolUse` / `PermissionRequest` / `PermissionDenied` | `tool_name`, `tool_input` (for Bash: `command`, `description`, `timeout`, `run_in_background`), `tool_use_id`; `PostToolUse` adds `tool_response`; MCP tools add an `mcp_server` object; `PermissionRequest` adds `permission_suggestions` |
| `Stop` / `SubagentStop` | `stop_hook_active`, `last_assistant_message`, `background_tasks[]`, `session_crons[]` (#stop-input) |
| `Notification` | `message`, `title`, `notification_type` |
| `SessionEnd` | `reason` |
| `Elicitation` | `server`, `tool_name`, `form` |

`transcript_path` is "written asynchronously, may lag"; the docs tell Stop hooks to use `last_assistant_message` rather than read the transcript.

### 1.3 Exit-code and stdout semantics

Source: #exit-code-0, #exit-code-2, #other-exit-codes, #json-output.

- **Exit 0**: success. If stdout starts with `{` and ends with `}` it is parsed as JSON control output. Plain-text stdout is **added to Claude's context only for `UserPromptSubmit`, `UserPromptExpansion`, `SessionStart`, `PostModelSwitch`**; for every other event stdout goes to the debug log. Stderr on exit 0 "goes to the debug log only, never the transcript, and Claude never sees it".
- **Exit 2**: blocking error on events that can block. "Even a JSON `permissionDecision` of `"allow"` can't override it." Stderr becomes the blocking reason; where it lands (Claude vs user vs nowhere) is per event. For `PreToolUse` it blocks the tool call and Claude sees the reason.
- **Any other exit code**: non-blocking error; the action proceeds; the transcript shows a `<hook name> hook error` notice with the first stderr line. Valid JSON on stdout still takes effect.
- **Universal JSON fields**: `continue` (false = Claude stops processing), `stopReason`, `systemMessage` (shown to the user as a brief terminal notification), `terminalSequence` (allow-listed OSC escapes, e.g. a desktop/tab notification), `suppressOutput` (accepted, no effect: hook stdout is never shown in the transcript anyway).
- **Event-specific decision fields**:
  - `PreToolUse`: `hookSpecificOutput.permissionDecision` = `allow` | `deny` | `ask` | `defer`, plus `permissionDecisionReason`, `updatedInput` (rewrite the tool's arguments), `additionalContext`.
  - `PostToolUse`: `additionalContext`, `updatedToolOutput`.
  - `Stop`/`SubagentStop`/`UserPromptSubmit`/`PostToolBatch`/`PreCompact`/`ConfigChange`: top-level `decision: "block"` + `reason`.
  - `SessionStart`/`SubagentStart`: `additionalContext`, `sessionTitle`, `initialUserMessage` (non-interactive), `watchPaths`, `reloadSkills`.
  - `PermissionRequest`: `hookSpecificOutput.decision.behavior` = `allow` | `deny`, `updatedInput`, `updatedPermissions`, `message`, `interrupt`.
  - `Elicitation`: `action` = `accept` | `decline` | `cancel`, `content`.
- `additionalContext` is capped at 10,000 characters; longer text is written to a file and Claude gets the path plus a 2,000-char preview.
- Hook stdout/`additionalContext` is injected "as a system reminder that starts with the hook's name; Claude reads both. Neither channel produces a visible transcript entry."

### 1.4 Can a hook block, inject context, or prompt the user?

- **Block**: yes, per the table above. For tool calls, a `PreToolUse` exit 2 or `deny` "stops the tool call before permission rules are evaluated, so the block applies even when an allow rule would otherwise let the call proceed" (https://code.claude.com/docs/en/permissions#extend-permissions-with-hooks).
- **Inject**: yes, on `SessionStart`, `UserPromptSubmit`, `Pre/PostToolUse`, `Stop`, and others.
- **Prompt the user**: a hook **cannot render its own question**. The only way a hook reaches a human is `PreToolUse` returning `permissionDecision: "ask"`, which escalates to the **standard permission dialog** for that tool call, labelled `[settings]`, `[plugin:<name>]` or `[skill]` by origin. "A hook's `"ask"` also forces a permission prompt in auto mode: the classifier can still deny the tool call, but it can't approve the call silently" (#pretooluse-decision-control). `Notification` hooks are explicitly side-effect only. There is no hook that opens a free-form yes/no dialog.
- `PermissionRequest` hooks run the other direction: they *answer* prompts. "In sessions that can't show a prompt, such as background subagents in non-interactive mode, Claude Code still runs these hooks, and if no hook returns a decision, it denies the tool call" (#permissionrequest).

### 1.5 Hook handler types and fields

Schema `$defs.hookCommand` and #hook-handler-fields: `type` = `command` | `prompt` | `agent` | `http` | `mcp_tool`. Common fields: `matcher` (on the group), `if` (permission-rule filter, tool events only, e.g. `"if": "Bash(punchcard *)"`), `timeout` (default 600 s; 30 s on `UserPromptSubmit`; 10 s on `MessageDisplay`; 1.5 s shared on `SessionEnd`), `statusMessage`, `once` (skill frontmatter only). Command hooks: `command`, optional `args` (exec form, no shell), `async` (background; "no deduplication across multiple firings"), `asyncRewake`, `shell` (`bash` = login shell, or `powershell`). "Command hooks execute shell commands with your full user permissions" (#security-considerations).

Hook processes receive `CLAUDE_PROJECT_DIR` (stable project root even inside a worktree, while `cwd` in the JSON moves to the worktree), `CLAUDECODE=1`, `CLAUDE_CODE_CHILD_SESSION=1`, `CLAUDE_CODE_SESSION_ID` (matches `session_id`), `CLAUDE_PID`, `CLAUDE_EFFORT`, and on `SessionStart`/`Setup`/`CwdChanged`/`FileChanged` a `CLAUDE_ENV_FILE` path whose `export` lines persist into later Bash commands (https://code.claude.com/docs/en/env-vars; hooks #persist-environment-variables).

### 1.6 Where hooks are configured, and whether a project hook can ship in-repo

Source: #configuration, hooks-guide #configure-hook-location, https://code.claude.com/docs/en/settings#settings-files-and-who-they-affect.

| Location | Scope | Shareable |
| :- | :- | :- |
| `~/.claude/settings.json` | every project on this machine | no |
| `.claude/settings.json` | this project; "In a git repository, commit it so teammates get it" | **yes, committed** |
| `.claude/settings.local.json` | this project, you only; Claude Code adds it to global git excludes when it creates the file | no |
| Managed settings (`managed-settings.json`, MDM, console) | org-wide; base layer | admin |
| Plugin `hooks/hooks.json` | wherever the plugin is enabled | yes, bundled |
| Skill frontmatter `hooks:` | registered when the skill is invoked, persist for the rest of the session (`once: true` to self-remove) | yes, in the skill |
| Subagent frontmatter | while the subagent runs; `Stop` becomes `SubagentStop` | yes |

Hooks **merge** across levels rather than replace. `disableAllHooks: true` (any level, subject to precedence; `--settings '{"disableAllHooks": true}'` wins for one run) turns them off; `--bare` and `--safe-mode` skip them (`claude --help`). Settings files are watched and reloaded mid-session, and `ConfigChange` fires on each edit. `/hooks` is a read-only browser labelling each hook's origin.

**A project hook shipped in `.claude/settings.json` runs** — including, per https://code.claude.com/docs/en/permissions#what-runs-before-you-trust-a-folder, in a `claude -p` run in a folder never trusted ("Used") — whereas project `permissions.allow` rules wait for the workspace-trust dialog. For a personal tool like Punchcard the more useful fact is the inverse: a hook in **`~/.claude/settings.json` fires in every repo** the user opens, so Punchcard does not need to touch each project's `.claude/` to get its hooks installed; a plugin is the other global vehicle (section 2.4).

---

## 2. Skills and slash commands

Reference: https://code.claude.com/docs/en/skills. Slash commands: https://code.claude.com/docs/en/slash-commands.

### 2.1 Where skills live; global installation

| Location | Path | Loads in |
| :- | :- | :- |
| Personal | `~/.claude/skills/<name>/SKILL.md` | "All your projects on this machine, but not Cowork or cloud sessions" |
| Project | `.claude/skills/<name>/SKILL.md` | sessions in this repository (commit to share) |
| Plugin | `<plugin>/skills/<name>/SKILL.md` | wherever the plugin is enabled, as `/plugin-name:skill-name` |
| Enterprise | managed settings dir | org-wide |
| Nested | `<subdir>/.claude/skills/...` | sessions at or below `<subdir>` |

Precedence on name clash: Enterprise > Personal > Project > Bundled > Built-in. So **yes, a skill installed under `~/.claude/skills/` works in every repo the user opens** on that machine. `.claude/commands/<name>.md` is the legacy form: "Custom commands have been merged into skills. A file at `.claude/commands/deploy.md` and a skill at `.claude/skills/deploy/SKILL.md` both create `/deploy` and work the same way."

### 2.2 `SKILL.md` frontmatter

All optional; names are lowercase-hyphenated (#frontmatter-reference):

`name` (defaults to directory name) · `description` (what and when; Claude uses it to decide to auto-invoke; `description`+`when_to_use` truncated at 1,536 chars) · `when_to_use` · `argument-hint` · `arguments` (named positional args) · `disable-model-invocation` (true = only the human can run it; "for deployments, sends, side effects you control") · `user-invocable` (false = Claude-only background knowledge) · `allowed-tools` (pre-approve tools **for the invoking turn only**) · `disallowed-tools` (remove tools while active, e.g. `AskUserQuestion` for a background loop) · `model` · `effort` · `context: fork` + `agent` + `background` (run in an isolated subagent) · `hooks` (registered on invoke, persist for the session) · `paths` (globs gating auto-activation) · `shell` · `metadata` · `license` · `compatibility`.

Substitutions in the body and in `allowed-tools` Bash rules: `$ARGUMENTS`, `$ARGUMENTS[N]`/`$N`, `$name`, `${CLAUDE_SESSION_ID}`, `${CLAUDE_EFFORT}`, `${CLAUDE_SKILL_DIR}`, `${CLAUDE_PROJECT_DIR}`, and in plugin skills `${CLAUDE_PLUGIN_ROOT}`/`${CLAUDE_PLUGIN_DATA}`.

### 2.3 How a skill invokes a CLI

Three distinct mechanisms, with different permission behaviour:

1. **Dynamic context injection** — `` !`punchcard status --json` `` inline, or a `` ```! `` fenced block. "The command runs before the skill content is sent to Claude. The output replaces the placeholder." Rules (#inject-dynamic-context): runs on the user's machine; 2-minute timeout; stderr merged into stdout; **non-zero exit aborts the whole invocation** (exit 1 from grep/diff/git is tolerated; append `|| true` otherwise); "Injected commands never prompt for permission while the skill renders. Claude Code checks each one against your permission rules first. A command a deny rule matches aborts the invocation"; grant it with `allowed-tools: Bash(punchcard *)` in the same frontmatter. Can be globally disabled with `disableSkillShellExecution: true`. This is a **silent, deterministic** CLI call — the right tool for reading state, wrong for anything that should be confirmed.
2. **Instructions to Claude** — the body tells Claude to run `punchcard start $0`; Claude calls the `Bash` tool and the normal permission flow applies. `allowed-tools` pre-approves listed rules for that turn only ("The grant clears when you send your next message"); the documented pattern `allowed-tools: Bash(${CLAUDE_SKILL_DIR}/scripts/render.sh *)` lets a skill run a bundled script without a prompt. The `Skill` tool itself is listed as "Permission required: Yes" in https://code.claude.com/docs/en/tools-reference and is controllable with `Skill(name)` / `Skill(name *)` rules.
3. **Frontmatter hooks** — a skill can register e.g. a `PreToolUse` hook on invoke that persists for the session (hooks #hooks-in-skills-and-agents).

Invocation: `/name args` at the start of a message runs immediately; Claude may auto-invoke unless `disable-model-invocation: true`; in `-p` mode "User-invoked skills and custom commands work. Include `/skill-name` in the prompt string" (https://code.claude.com/docs/en/headless). Skill content is loaded once and stays in context; after compaction the most recent invocation is retained (5,000 tokens per skill, 25,000 combined).

### 2.4 Plugins as the global bundle

https://code.claude.com/docs/en/plugins-reference: a plugin directory carries `skills/`, `commands/`, `agents/`, `hooks/hooks.json`, `.mcp.json`, `bin/` ("Files here are on the Bash tool's `PATH` while the plugin is enabled, so Claude runs them as bare commands"), and optional `.claude-plugin/plugin.json`. Hooks and MCP servers in a plugin get `${CLAUDE_PLUGIN_ROOT}` / `${CLAUDE_PLUGIN_DATA}`. A plugin can be loaded for one session with `--plugin-dir <path>` (`claude --help`) or installed from a marketplace and toggled via `enabledPlugins`. **[not verified]**: the install/marketplace page (`plugins/install`) and whether an installed plugin is user-scoped by default were not inspected. A plugin is the one artefact that can ship Punchcard's skill, its hooks, and its MCP server together.

---

## 3. MCP

Reference: https://code.claude.com/docs/en/mcp. Spec: https://modelcontextprotocol.io/specification/2025-06-18/server/tools.

### 3.1 Registering a stdio server

```bash
claude mcp add [--transport stdio] [-e KEY=value] [--scope local|user|project] <name> -- <command> [args...]
# e.g. claude mcp add --scope user punchcard -- punchcard mcp
```

Everything after `--` is passed to the server untouched (#option-3-add-a-local-stdio-server). `claude mcp add-json <name> '<json>'` takes an `mcpServers` entry directly. Scopes (#mcp-installation-scopes):

| Scope | Stored in | Loads in | Approval |
| :- | :- | :- | :- |
| `local` (default) | `~/.claude.json`, keyed by project | this project only | none |
| `user` | `~/.claude.json` | **all projects on this machine** | none |
| `project` | `.mcp.json` at repo root (commit it) | this project, for everyone | interactive sessions prompt "Review and approve"; `claude mcp reset-project-choices` resets; `-p`, SDK and cloud sessions "load project-scoped servers without asking"; `enableAllProjectMcpServers` / `enabledMcpjsonServers` / `disabledMcpjsonServers` settings; committed approvals are ignored until the folder is trusted |

Precedence when the same name exists in several: local > project > user > plugin > claude.ai connector. `.mcp.json` shape:

```json
{ "mcpServers": { "punchcard": { "type": "stdio", "command": "punchcard", "args": ["mcp"], "env": { "X": "${X:-default}" } } } }
```

`${VAR}` and `${VAR:-default}` expand in `command`, `args`, `env`, `url`, `headers`. The server process receives `CLAUDE_PROJECT_DIR` (stable project root), `CLAUDECODE=1`, `CLAUDE_CODE_SESSION_ID` (the ID it was spawned with; "An MCP server subprocess retains the ID it was spawned with"), but **not** `CLAUDE_CODE_CHILD_SESSION` because servers "are long-lived and outlive the session that spawned them" (https://code.claude.com/docs/en/env-vars). Claude Code answers `roots/list` with the launch directory plus `--add-dir` directories.

`--mcp-config <file-or-json>` and `--strict-mcp-config` load servers for one run (`claude --help`); `--bare` connects only `--mcp-config` servers. `claude mcp serve` exposes Claude Code itself as a stdio server (not relevant here).

### 3.2 Declaring tools

Per the spec, a server declares `capabilities.tools` (optionally `listChanged`), answers `tools/list` with `Tool` objects — `name`, optional `title`, `description`, `inputSchema` (JSON Schema object), optional `outputSchema`, optional `annotations` — and handles `tools/call`, returning `content[]` and/or `structuredContent`, with `isError: true` for execution errors. "Clients MUST consider tool annotations to be untrusted unless they come from trusted servers."

Claude Code specifics (https://code.claude.com/docs/en/mcp):
- Tools are named `mcp__<server>__<tool>` (plugin-bundled: `mcp__plugin_<plugin>_<server>__<tool>`); that name is what permission rules, `allowed-tools`, subagent `tools`, and hook matchers use.
- Tool search is on by default on Claude 4.5+ models: only tool names and **server instructions** load at startup; definitions are deferred until needed. "Add clear, descriptive server instructions." Each description and the instructions are truncated at 2,048 chars. `alwaysLoad: true` on the server entry, or `_meta["anthropic/alwaysLoad"]: true` on a tool, exempts it.
- Schema hygiene: top-level property names 1–64 chars of `[A-Za-z0-9_.-]`; schema must validate against JSON Schema 2020-12; root-level `anyOf/oneOf/allOf` are flattened; offending tools are excluded individually (v2.1.216+).
- Output: 25,000-token default limit (`MAX_MCP_OUTPUT_TOKENS`), warning at 10,000; per-tool `_meta["anthropic/maxResultSizeChars"]`.
- `_meta["anthropic/requiresUserInteraction"]: true` — see 3.3.
- MCP prompts appear as slash commands `/mcp__server__prompt` (listed as `/server:prompt (MCP)`), arguments split on whitespace. MCP resources are `@server:uri` mentions.

### 3.3 Can an MCP tool require a permission prompt?

Three layers, from weakest to strongest:

1. **Permission mode + rules.** In Manual mode only reads run without asking (https://code.claude.com/docs/en/permission-modes#available-modes: `default` → "Reads only"), so an MCP tool call prompts unless an allow rule matches; the permissions page confirms MCP tool prompts exist ("On most permission prompts, including Bash, PowerShell, file, and MCP tool prompts, ... press Tab to open a comment field"). Rules: `mcp__punchcard` (whole server), `mcp__punchcard__*`, `mcp__punchcard__stop`. Evaluation order is "deny, then ask, then allow. The first match in that order determines the outcome, and rule specificity doesn't change the order" (https://code.claude.com/docs/en/permissions#manage-permissions). So `"ask": ["mcp__punchcard__stop"]` plus `"allow": ["mcp__punchcard__*"]` prompts for stop and nothing else. **Caveat**: parameter-scoped rules (`mcp__x__y(param:value)`) are *not* supported for MCP tools from settings files ("Claude Code ... skips any `mcp__` rule that has parentheses"); only `--disallowedTools` accepts them. In auto mode, allow/ask/deny rules resolve first and "Everything else goes to the classifier".
2. **`PreToolUse` hook returning `ask`** (section 1.4): forces the dialog even in auto mode; still subject to deny/ask rules.
3. **`_meta["anthropic/requiresUserInteraction"]: true` on the tool** (#require-approval-for-a-specific-tool): "Claude Code shows that tool's permission prompt on every call, even in `acceptEdits`, `auto`, and `bypassPermissions` permission modes, and doesn't offer a "don't ask again" option for it. Allow rules that match the tool don't skip the prompt either. In `dontAsk` mode, which never prompts, Claude Code denies the call instead." "The prompt has to reach a person": with `--permission-prompt-tool` an `allow` is converted to a deny; only the Agent SDK's `canUseTool` callback may approve it. Remote Control withholds one-tap approval for it. "Use this for tools whose permission prompt is itself the point, such as a consent or access-grant step where auto-approval would mean no human ever agreed."

Separately, **elicitation** (#respond-to-mcp-elicitation-requests; spec `client/elicitation`): mid-call the server can send `elicitation/create` with a message and a flat `requestedSchema` (string/number/boolean/enum), and "Claude Code displays an interactive dialog and passes your response back to the server" (form mode, or URL mode for browser flows). Responses are `accept` (with content) / `decline` / `cancel`. A boolean field is a literal yes/no. Caveats: an `Elicitation` hook can auto-answer "skipping the dialog entirely"; with `--permission-prompts none` "Any MCP elicitation request that no `Elicitation` hook answers is cancelled"; the spec says "Servers MUST NOT use elicitation to request sensitive information". A `Notification` hook with matcher `elicitation_dialog` fires after the dialog has waited about six seconds.

### 3.4 What the user sees when an agent calls a tool

- A permission prompt, when one is due, "shows what Claude is about to do, followed by your options" (https://code.claude.com/docs/en/permissions#permission-system); the Bash example shows the description, the command, "This command requires approval", and Yes / Yes-and-don't-ask-again / Yes-and-switch-to-auto / No. For a hook-forced `ask` the prompt carries a `[settings]`/`[plugin:x]`/`[skill]` label. `requiresUserInteraction` and org `ask` prompts omit the remember option. The spec requires clients to "Show tool inputs to the user before calling the server".
- When a `PermissionRequest` hook auto-approves, "The transcript shows "Allowed by PermissionRequest hook" where the dialog would have appeared" (hooks-guide #auto-approve-specific-permission-prompts).
- `/mcp` shows connected servers and tool counts; a tool blocked by the org is marked `disabled by your organization`.
- Hook stdout never appears in the transcript; `systemMessage` appears as a brief notification.
- **[not verified]**: the exact rendering of an MCP tool call row in the transcript (label text, collapsed/expanded state) is not described in the inspected pages.

---

## 4. Knowing the ticket

**There is no Jira-key field anywhere in the surface.** Nothing in the hook JSON, the environment, or the settings identifies a ticket. What exists:

| Signal | Where | Usable how |
| :- | :- | :- |
| `cwd` / `CLAUDE_PROJECT_DIR` | every hook; Bash and MCP child env | A hook can run `git branch --show-current` in `cwd` itself and regex a key out of a **branch-name convention** (e.g. `feature/PROJ-123-...`). The docs' own status-line example does exactly this with `git branch --show-current` (https://code.claude.com/docs/en/statusline). Inside a `--worktree` session `cwd` is the worktree and `CLAUDE_PROJECT_DIR` stays the root |
| Status-line JSON | `statusLine` command only (not hooks) | Carries `workspace.repo.{host,owner,name}`, `workspace.git_worktree`, `worktree.branch`, `pr.number`/`pr.url` — but no branch name for the main checkout, and it is a display hook, debounced at 300 ms, not an event hook |
| `prompt` | `UserPromptSubmit` | Regex the human's prompt for `[A-Z][A-Z0-9]+-\d+`; a hook can also inject "timer is running on X" via `additionalContext` |
| `tool_input` | `PreToolUse`/`PostToolUse` | If the agent calls `punchcard start PROJ-123` (Bash) or `mcp__punchcard__start {key}` the key is in `tool_input`; likewise if a Jira MCP/connector tool (`mcp__claude_ai_<server>__<tool>`, or any `mcp__*jira*` server) is called, its arguments carry the key |
| `last_assistant_message` | `Stop` | Text of the final turn; keys can be regexed but this is heuristic |
| `session_title`, `-n <name>`, `sessionTitle` output | `SessionStart`/`UserPromptSubmit` input; `SessionStart`/`UserPromptSubmit` hook output; `claude -n` (`claude --help`) | A convention of naming sessions after tickets would surface the key to hooks; a hook can set the title itself |
| `session_id` / `CLAUDE_CODE_SESSION_ID` | all hooks; Bash/hook/MCP env | Correlation key for Punchcard's own state (e.g. "entry started by session X"), not a ticket |
| transcript `.jsonl` lines | `transcript_path` | **[observed, undocumented]**: every line on this machine carries top-level `gitBranch`, `cwd`, `sessionId`, `version` fields (`gitBranch: "main"` here). None of the inspected pages document the line format; treat as unstable |

Conclusion: **the agent must be told, or tell Punchcard, the key explicitly** (skill argument `/punchcard:start PROJ-123`, or the model passing it to the CLI/MCP tool); the only passive inference is a branch-name convention computed by the hook from `cwd`, which is the user's own convention to adopt. Note that Punchcard's own state removes most of the need: once a timer is running, `pause`/`stop` need no key, and `punchcard status --json` can be injected at `SessionStart` so the agent knows what is running.

---

## 5. Unattended vs confirmed

Which layers can ask the human a yes/no before acting, and which run silently.

| Layer | Can it make a human confirm? | How, and what breaks it |
| :- | :- | :- |
| **Plain CLI via the Bash tool** | **Yes, enforceably**, via a `permissions.ask` rule such as `"ask": ["Bash(punchcard stop *)"]` | Ask rules are in the list of "Actions no mode auto-approves ... in any mode, including `bypassPermissions`" (https://code.claude.com/docs/en/permission-modes#actions-no-mode-auto-approves); content-scoped ask rules "still force a prompt" under sandbox auto-allow; precedence is deny > ask > allow regardless of scope or specificity; ask/deny rules in a project `.claude/settings.json` are **not** gated by workspace trust (unlike allow). Caveat: rules match "the command text Claude writes" after compound-command splitting, so `punchcard stop` via a wrapper or script is not caught; "isn't a security boundary around the program". `start`/`pause` can be made silent with `"allow": ["Bash(punchcard *)"]` |
| **Hooks** | **Yes, but only as a permission prompt**: `PreToolUse` → `permissionDecision: "ask"` | Forces the standard dialog even in auto mode; labelled by origin. Cannot pose its own question, cannot block on `SessionEnd`, `Stop` runs silently (stdout never shown). `PermissionRequest` hooks can *auto-approve* prompts, so a user-installed hook could remove the confirmation; and deny/ask rules still override a hook's `allow` |
| **Skills / slash commands** | **Only by persuasion**: the skill text can instruct Claude to call `AskUserQuestion` before stopping, or tell it never to stop without being asked | `AskUserQuestion` is multiple-choice, "Permission required: No", "Questions stay open until you answer them" unless `askUserQuestionTimeout` is set (then auto-continues with whatever was selected); it is never auto-approved in any mode, but it is **model-discretionary**, not enforced. `disable-model-invocation: true` on a stop skill makes it human-only. `!`-injected commands never prompt |
| **MCP tool** | **Yes, enforceably**, via `_meta["anthropic/requiresUserInteraction"]: true` on the `stop` tool; or via `ask` rules on `mcp__punchcard__stop`; or via elicitation mid-call | `requiresUserInteraction` prompts in every mode including `bypassPermissions`, no remember option, allow rules don't skip it, `--permission-prompt-tool` cannot approve it. Elicitation gives a server-driven yes/no form but a user's `Elicitation` hook can auto-answer it and `--permission-prompts none` cancels it |
| **Unattended runs** (`claude -p`, `dontAsk`, `--permission-prompts none`, SDK with no host) | Nothing can confirm; everything that would prompt is **denied, not silently approved** | `dontAsk`: "auto-denies every call that would otherwise prompt", including ask rules, `AskUserQuestion`, and `requiresUserInteraction` tools "even if you've allowed them". `-p` with no host: "these requests are denied either way"; `--permission-prompts none` additionally removes `AskUserQuestion` and cancels unanswered elicitations. `-p --dangerously-skip-permissions`: "the few calls that would still prompt are denied instead" |

Layers that run **silently** by design: `SessionStart`/`Stop`/`SessionEnd`/`PostToolUse` command hooks (no transcript entry; `systemMessage` is the only user-visible channel), skill `!` injection, any tool matched by an allow rule, anything the auto-mode classifier approves, and MCP tools in `acceptEdits`/`auto` with a matching allow rule.

The decisive fact for the stop-and-submit policy: **two mechanisms survive every permission mode and degrade to "deny" rather than "silently do it" when no human is present — a `permissions.ask` rule on the CLI command, and `requiresUserInteraction` on an MCP tool.** Both are declared in a file Punchcard controls (the user's `~/.claude/settings.json`, or the tool's own `tools/list` entry). A skill alone cannot enforce confirmation; a hook can only escalate to the same permission dialog those two mechanisms already produce.

---

## Appendix: CLI flags relevant to the integration (from `claude --help`, 2.1.290)

- `--permission-mode <acceptEdits|auto|bypassPermissions|manual|dontAsk|plan>`; `--permission-prompts <host|none>` ("none: ... anything that would prompt is denied automatically").
- `--allowedTools`, `--disallowedTools` (permission-rule syntax, e.g. `"Bash(punchcard *)"`).
- `--bare` (skip hooks, plugins, MCP from settings, CLAUDE.md; `CLAUDE_CODE_SIMPLE=1`), `--safe-mode` (all customisations off), `--disable-slash-commands` (all skills off), `--setting-sources user,project,local`.
- `--mcp-config <files...>`, `--strict-mcp-config`; `claude mcp add --scope local|user|project`; `claude mcp reset-project-choices`; `claude mcp serve`.
- `--session-id <uuid>`, `-n/--name <name>` (session display name), `--worktree`, `--bg` (background session; `claude agents`/`attach`/`logs`).
- `--include-hook-events` (stream hook lifecycle in `stream-json` output).
