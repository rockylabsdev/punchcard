# Go and Bubble Tea v2, one binary, not Ink

Punchcard v2 is written in Go with Bubble Tea v2 (plus Lip Gloss and Bubbles) and ships as a single `punchcard` binary. Subcommands are the CLI; the TUI lives in the same binary, and the CLI path never starts the renderer. This reverses the brief's Ink default. With no daemon (ADR 0001), the CLI is spawned on the hot path of every prompt in a keyed session, through the Claude Code mod. The user put performance and responsiveness first, and Go starts in about 4 ms where Node takes about 33 ms (190 ms with Ink loaded eagerly). Go also reaches `flock` directly (ADR 0002), where Node core can't without a native addon.

## Considered Options

- **Ink (Node/TypeScript).** The user's daily language, and flexbox layout is the most ergonomic of the three. Rejected: a start-up budget that one stray top-level import can break, a native addon for `flock`, and 57 MB Bun binaries. Its one structural advantage, letting the TS mod import the core instead of shelling out, was rejected on its own merits. The mod is a thin client of `punchcard … --json`, so there is one seam, one version and one lock owner, and Claude Code's process never holds Punchcard's lock.
- **Go core and CLI, Ink TUI.** Viable because the TUI only requests transitions and never writes from a copy (ADR 0002), so it could shell out for every change. Kept as the **fallback**, not chosen: two languages, a Node runtime for the TUI, and the `state.json` types written twice, to avoid a problem not yet seen.
- **Rust (Ratatui).** Just as fast and smaller. Rejected: Ratatui with crossterm doesn't downsample colour, so the app would have to choose a palette itself for 16-colour terminals. Its ownership model is also the part of Rust the user has not yet got past, and the code is mostly written by agents and reviewed by the user, so the language they can read critically soonest wins.

## Consequences

- **Bubble Tea v2, not v1**: v2 has the colour downsampling this choice depends on. `go.mod` says `go 1.26` and Go's `toolchain` directive fetches the right compiler, so fresh worktrees never drift.
- **The TUI prototype is the checkpoint.** If a Bubble Tea prototype can't produce a clean, organised, btop/lazygit-like layout at 80×24 without fighting Lip Gloss's string-joining layout, the fallback is the Go core plus an Ink TUI. The switch costs a prototype, not a rewrite. The prototype also confirms that Lip Gloss, Bubbles and Huh have v2-compatible releases.
- **First Go project.** The user is fluent in TypeScript/React, new to Go, and works mainly through agents. Unfamiliar Go parts (Bubble Tea's update loop, `flock` via `x/sys/unix`, `/usr/bin/security`) each deserve their own slice.
