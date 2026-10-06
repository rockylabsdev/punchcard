# v1 — macOS menu-bar plan (superseded)

This directory holds the planning artifacts for Punchcard's first incarnation: a SwiftUI macOS menu-bar app. No code was ever written against it.

On 2026-10-05 the project was rescoped to a terminal app (TUI + CLI) so it can live inside Claude Code and Orca IDE workflows. The brief for that rescope is at [`../wayfinder/brief.md`](../wayfinder/brief.md).

What is still authoritative from here:

- [`wayfinder/issues/05-jira-api-curl-spike.md`](wayfinder/issues/05-jira-api-curl-spike.md) — the Jira API contract, proven against real Jira Cloud. Platform-independent. Fixtures live at repo-root `docs/jira-api/`.
- [`wayfinder/issues/04-on-disk-persistence-contract.md`](wayfinder/issues/04-on-disk-persistence-contract.md) — the state-file shape. Carried forward; only the heartbeat semantics are reopened.
- The problem statement and user stories in [`spec.md`](spec.md).

Everything Swift-specific (tickets 01–03, the spec's implementation decisions, all of `issues/`) is superseded.
