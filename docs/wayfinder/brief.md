# Punchcard v2 — rescope brief for a fresh wayfinder run

## What is changing, and why

Punchcard v1 was planned as a macOS menu-bar app in SwiftUI (`docs/v1-mac/spec.md`, `docs/v1-mac/wayfinder/map.md`, nine implementation tickets in `docs/v1-mac/issues/`). **The platform is being replaced, not the product.** Punchcard becomes a **terminal app**: a full-screen TUI plus a scriptable CLI, sharing one core.

The reason is workflow fit. I live in the terminal, in Claude Code sessions and in the Orca IDE. A menu-bar app sits outside that loop; a terminal app sits inside it. The decisive new requirement is that **Claude Code can drive Punchcard as naturally as I can** — start a timer when an agent picks up a ticket, stop and submit when it finishes — without me leaving the session.

## Destination for this map

A spec-ready understanding of terminal-native Punchcard, so `to-spec` can rewrite `docs/spec.md` and `to-tickets` can recut `docs/issues/`. The map is done when the stack, the process model, the CLI contract, the agent-integration surface, and the TUI shape are decided.

## What carries forward unchanged — do not re-litigate

- **The problem statement and the product.** Time work against a Jira ticket, sync it as a worklog, local file is the source of truth, Jira is a destination that may be down. User stories 1–62 in the v1 spec survive almost verbatim once "menu bar / popover / click" is read as "terminal / pane / keystroke". Story 2 (no Dock icon, no window) becomes "no GUI at all".
- **The domain model** in root `CONTEXT.md`: `TrackedTicket`, `TimeEntry`, `Segment`, `Worklog`, `lastSeen`. One `TimeEntry` → exactly one `Worklog`. Elapsed is derived from timestamps, never a ticking counter. Only its first sentence ("macOS menu-bar app") needs changing.
- **The settled product decisions**: one running timer; pause vs stop as distinct acts; manual key entry plus recents; editable duration and optional comment at stop; truncate to the minute; `POST /rest/api/3/issue/{key}/worklog` with `adjustEstimate=leave`; idempotency guard of entry UUID + stored `worklogId`, never POST when `synced`; failed sync stays pending and retryable and never blocks a timer; JSON-file persistence; no v1 reporting.
- **Ticket 05 (Jira API curl spike)** — fully platform-independent and proven against real Jira Cloud: Basic auth with a **classic unscoped token** (scoped tokens 401), `started` format `yyyy-MM-dd'T'HH:mm:ss.SSSZ`, one error envelope, string ids, fixtures in `docs/jira-api/`. Carry into the new map's Decisions-so-far as-is.
- **Ticket 04 (persistence contract)** — carry the shape (`state.json` + separate `heartbeat.json`, `schemaVersion`, refuse-on-newer, back-up-and-reset on corrupt, atomic writes, entries nested under tickets, recovery capped at `lastSeen` with a persistent row marker, no pruning). Only the *heartbeat semantics* reopen, see below.
- **The design philosophy** of the menu-bar app, now redesigned for the terminal: one glance tells you whether you are on the clock; starting costs one keystroke; pause and stop look different; a recovered entry is flagged on its row until acted on; failures are visible banners, never blockers. Favour the boring, inspectable option.

## What is invalidated

- Ticket 01 (Swift/SwiftUI bootstrap), ticket 02 (Swift walking-skeleton build order), ticket 03 (Settings `Window` scene, Keychain), the Implementation Decisions section of the v1 spec, and all nine `docs/v1-mac/issues/` tickets. They are archived under `docs/v1-mac/` with a README marking them superseded.
- The "first Swift project" constraint that shaped v1's build order and test strategy. It no longer applies. Replace it with whatever familiarity note fits the chosen stack.

## Direction

**TUI.** Keyboard-driven, full-screen, in the spirit of btop and lazygit: panes, single-key bindings, a status bar that shows the bindings, a running row that is unmistakable. Reference mood only; the layout is a prototype ticket.

**Stack.** My default is **Ink** (React for the terminal, Node/TypeScript). Claude Code's own TUI is built with Ink, which makes it a well-trodden path for an app that coexists with it. Note that btop (C++) and lazygit (Go) are not Ink apps; they are the *feel* I want, not the toolchain. I am open to a different stack if the process-model or distribution answer argues for one.

**CLI.** First-class, not an afterthought: `punchcard start KEY`, `pause`, `resume`, `stop [--duration --comment]`, `status`, `add`, `rm`, `ls`, `sync`/`retry`, `setup`. Stable, machine-readable output (`--json`) so agents can parse it. The exact surface is a ticket.

**Agent integration.** This is the new centre of gravity. Candidates, from simplest up: plain CLI with JSON output; Claude Code hooks (session start/stop, tool events); a Punchcard skill or slash command; an MCP server. Decide which layers exist in v1 and what the agent is allowed to do unattended (start and pause freely; stop-and-submit probably wants a human confirm, since that is the one path that reaches Jira). I am open to architecture ideas here. Orca IDE integration is a separate, unresearched question: find out what surface it exposes before deciding anything.

## The fog — what the map must resolve

1. **Process model, and what `lastSeen` means now.** This is the first ticket. The menu-bar app was one long-lived process with a 30s heartbeat. A CLI `start` that writes a timestamp and exits has no process to heartbeat. Is a timer with no running process a feature (an agent can start it, no TUI needed) or a hazard (forgotten over a weekend)? Options: stateless CLI + TUI with heartbeat only while the TUI is open; a small background daemon the TUI and CLI both talk to; a hybrid. The recovery-cap rule depends on this answer.
2. **Concurrent writers.** TUI, CLI, and an agent may all touch `state.json`. Locking, atomic replace, and how the TUI learns of a change it did not make (poll vs watch).
3. **Stack confirmation.** Ink/TypeScript vs Go (Bubble Tea) vs Rust (Ratatui), judged against the process-model and distribution answers, not aesthetics.
4. **CLI command surface and the `--json` contract.** Exit codes, output shape, and the stop-and-submit confirmation flow when there is no interactive user.
5. **Agent integration surface.** Which of CLI / hooks / skill / MCP ship in v1, and the unattended-action policy. Research what Claude Code hooks and Orca expose.
6. **Credential storage without Keychain.** macOS Keychain via the `security` CLI, a cross-platform keyring library, an 0600 config file, or an env var. Decide alongside the platform floor.
7. **Platform floor.** A terminal app makes Linux nearly free. Decide whether v1 is still macOS-only or macOS + Linux. Windows stays out.
8. **First-run and settings in the terminal.** The Settings window becomes `punchcard setup` or a TUI pane. The Test Connection logic (serverInfo, then myself; Save gated on a pass; scoped-token warning in helper text and in the 401 copy) carries over intact.
9. **TUI layout and interaction.** Prototype ticket: panes, keybindings, how a running row reads in 16- and 256-colour terminals, what the status bar shows. The two v1 micro-calls (running-row controls, blue reserved for sync) are reframed as terminal colour and glyph semantics.
10. **Distribution.** npm global / npx, Homebrew, or a single binary, driven by the stack choice.
11. **Build order.** A new walking skeleton once 1–5 are settled. Keep v1's discipline: every slice ends at a commit that runs and has one hand check; the curl spike is already done.

## Out of scope — carried over and extended

Multi-user backend; OAuth 3LO; automatic idle detection; live ticket autocomplete; in-app reporting; remaining-estimate management; editing or deleting synced worklogs; pruning; scoped tokens; Windows. Newly out: any GUI, menu-bar, or notification-centre presence; the Xcode project and everything Swift.

## Notes for the map's Notes block

- Skills to consult: `/grilling`, `/domain-modeling`, `/prototype`, `/research`.
- Keep `CONTEXT.md` the single authority on vocabulary; sharpen it in place.
- Favour recommendations with rationale over open questions. The product is settled; this run is about the terminal shape and the agent seam.
