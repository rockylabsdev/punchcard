# Research: TUI stack comparison on process-model and distribution criteria (wayfinder #2)

**Question.** Compare Ink (Node/TypeScript), Bubble Tea (Go), and Ratatui (Rust) on the criteria the process-model, concurrency, and distribution decisions will be judged on. Facts only; the stack ticket decides.

**Inspected 2026-10-06.** Versions: Ink **v8.0.0** (2026-10-03, `engines.node >=22`), Bubble Tea **v2.0.10** (2026-09-24, module `charm.land/bubbletea/v2`, `go 1.26.0` in its go.mod), Ratatui **0.30.2**, Node docs **v26.10.0**, fsnotify **v1.10.1**, notify **8.2.0**, keyring-rs **v4.2.0**, @napi-rs/keyring **v2.1.0**, go-keyring **v0.2.8**. Local machine: Apple M1, macOS (Darwin 25.5), Node 24.5.0, Bun 1.2.1, Go 1.25.6, rustc 1.88.0.

**Constraints carried in from sibling research.**
- Orca SIGTERMs then SIGKILLs the agent's whole descendant tree (about 2 s) and every process group on the PTY's tty (5 s). A daemon survives only if fully detached: new session (`setsid` or double-fork), reparented to pid 1, no controlling tty, state outside the worktree (`docs/research/orca-ide-surface.md` on `research/orca-ide-surface`, section 2).
- Claude Code hooks run the CLI on events, so per-invocation cold start is paid on every hooked event (`docs/research/claude-code-integration-surface.md` on `research/claude-code-integration-surface`).

---

## Criteria × stack

| Criterion | Ink (Node/TS) | Bubble Tea (Go) | Ratatui (Rust) |
| --- | --- | --- | --- |
| 1. Background process | `spawn(..., {detached: true, stdio: 'ignore'}).unref()` gives a new session (setsid) on POSIX. No in-process `fork()`/`setsid()`; the daemon is a re-exec of node. Signal listeners for SIGTERM/SIGINT/SIGHUP. Workable. | `exec.Cmd` + `SysProcAttr{Setsid: true}` (re-exec self). No fork-without-exec. `os/signal.Notify`. Workable. | `CommandExt::process_group` stable; `setsid` is **nightly-only**, so stable needs `unsafe pre_exec` + `libc::setsid` or the `nix` crate. Signals via `signal-hook`/`tokio::signal` (not in std). Workable. |
| launchd / systemd user unit | Stack-agnostic. Both managers want a **foreground, non-daemonizing** process (launchd forbids `fork`/`setsid`; systemd recommends `Type=exec`). Same for all three. | same | same |
| 2. File watching | `fs.watch` (inotify / FSEvents), documented "not 100% consistent across platforms", filename not guaranteed. Third-party wrappers exist (not inspected). | fsnotify: inotify on Linux, **kqueue on macOS (no FSEvents)**; docs say watch the parent dir because of atomic-rename writes; no built-in debounce. | notify: **FSEvents** (default) on macOS, inotify on Linux, PollWatcher fallback; separate debouncer crates. |
| Advisory locking | **No locking API in `node:fs`.** Needs a native addon or a userland scheme (`proper-lockfile` = mkdir + mtime staleness, not flock). | `syscall.Flock` (or `golang.org/x/sys/unix`). | `std::fs::File::lock/try_lock/lock_shared` = `flock` on Unix, **stable since 1.89.0**. |
| Atomic rename | `fs.rename` (POSIX rename(2) semantics; Node doc text on replacement not verified in this pass). | `os.Rename` replaces non-directory target. | `std::fs::rename` replaces target (rename(2)). |
| 3. Single binary | Node SEA (Stability 1.1, `node --build-sea` since v25.5; must re-sign on macOS; macOS **arm64 only** in CI; cross-target disables code cache/snapshot). `bun build --compile` (8 targets incl. cross). `pkg` **deprecated/archived**. Local sizes: bun output **57-59 MB**; node binary 117 MB (SEA base). | Native static binary, trivial cross-compile. Local: **2.9 MB** (stripped, Bubble Tea v1.3.10). Linker ad-hoc-signs on darwin/arm64 (observed). | Native binary. Local: **0.58 MB** (stripped, ratatui 0.30.2). Linker ad-hoc-signs (observed). |
| macOS signing | SEA: docs require `codesign --remove-signature` then `codesign --sign -`. Bun: docs require codesign with JIT entitlements (Bun ≥1.2.4); Bun 1.2.1 output locally still carried Bun's Developer ID signature and **failed strict validation**, yet ran from a terminal. | Ad-hoc linker signature out of the box. | Same as Go. |
| 4. Cold start (local, median of 20, warm cache, prints one JSON line) | node plain: **33 ms**; node importing ink+react: **188 ms**; bun-compiled plain: **18 ms**; bun-compiled ink: **56 ms**. | **3.9 ms** | **3.0 ms** |
| 5. Keyring | keytar **archived** (2022). `@napi-rs/keyring` (binding to keyring-rs, "100% compatible node-keytar alternative"; native prebuilds). | `zalando/go-keyring`: macOS shells out to `/usr/bin/security`; Linux = Secret Service over D-Bus; avoids cgo. | `keyring` crate: native macOS Keychain; Linux Secret Service (D-Bus) **or kernel keyutils**. |
| Headless | Inherits keyring-rs (keyutils option on Linux). | Secret Service only on Linux: needs a D-Bus session + keyring daemon (implied by docs, not explicitly stated). | keyutils backend available for headless Linux; persistence semantics **not verified**. |
| 6. Rendering | chalk auto-detects level 0-3 (16/256/truecolor) and downsamples; `FORCE_COLOR` override. Flexbox `Box` (Yoga), borders, `useWindowSize` re-renders on resize, `alternateScreen`, `incrementalRendering`. | v2: "cell-based renderer, built-in color downsampling" (colorprofile); `WindowSizeMsg` on start and every resize; layout via Lip Gloss strings. | Immediate mode; `Color` 16/Indexed/Rgb; **no downsampling with crossterm** ("display will be unpredictable", Terminal.app named) - app must pick palette; constraint `Layout`. |
| 7. Testing | `ink-testing-library` (v4.0.0, 2024): `lastFrame()`, `frames`, `stdin.write()`, `rerender`. Last pushed 2024-06. | `teatest` in `charmbracelet/x/exp` (**experimental**, has a `v2` subdir): golden files, `Send`/`Type`, `WaitFor`, `FinalOutput`. | `TestBackend` in-crate: `assert_buffer*`, `resize`, `Display` for snapshots (insta recommended by ratatui.rs). Drive keys by calling your own event handler. |
| 8. Precedents (TUI + scriptable JSON) | Claude Code (`claude -p --output-format json`); Gemini CLI, GitHub Copilot CLI listed as Ink users. | MinIO `mc` (Bubble Tea in go.mod, global `--json` "JSON lines" flag). **lazygit is not Bubble Tea** (in-tree gocui on tcell v3). | OpenAI Codex CLI (`codex-rs/tui` on ratatui 0.30.2; `codex exec --json` JSONL). |

**Disqualifications.** None outright. Closest to disqualifying:
- **Ink + per-event hook invocation**: a CLI that eagerly imports Ink/React costs ~190 ms per invocation under plain Node on an M1. Mitigable (lazy `import()` of Ink only on the TUI path brings the CLI path to ~33 ms; bun-compiled ~18-56 ms), but it is a design constraint, not free.
- **Ink + locking**: no flock in Node core; whichever concurrency design is chosen must use a native addon or a mkdir/rename scheme.
- **`pkg`** is disqualified as a Node packaging route (archived, deprecated).

---

## 1. Background process

**Stack-agnostic facts first.**
- launchd (macOS per-user agents live in `~/Library/LaunchAgents`): "You must not daemonize your process. This includes calling the `daemon` function, calling `fork` followed by `exec`, or calling `fork` followed by `exit`." and "Do not call `setsid` to create a new session." Apple recommends on-demand launch over `KeepAlive` (https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html).
- systemd: "It is recommended to use `Type=exec` for long-running services" (systemd.service(5), source `man/systemd.service.xml` in https://github.com/systemd/systemd; the freedesktop.org HTML page returned 403 to the fetcher). `loginctl enable-linger`: "a user manager is spawned for the user at boot and kept around after logouts" (`man/loginctl.xml`, same repo).
- Consequence: under a service manager, every stack runs the daemon as a plain foreground process; self-detaching is only needed for the "CLI spawns its own daemon" path, which is the path Orca's tree-kill forces to be a full `setsid` detach.
- No stack has a pidfile primitive; a pidfile is a file plus a lock (criterion 2).

**Ink / Node.** `options.detached: true` on non-Windows: "the child process will be made the leader of a new process group and session ... See setsid(2)"; must use non-inherited stdio, else "the child process will remain attached to the controlling terminal"; `subprocess.unref()` lets the parent exit (https://nodejs.org/api/child_process.html). `child_process.fork()` "does not clone the current process" (same page); `process` has no `setsid`/daemon function (https://nodejs.org/api/process.html). Signals: SIGTERM/SIGINT have default handlers that are removed when a listener is installed; SIGKILL cannot be caught (same page). So: daemon = `spawn(process.execPath, [script, 'daemon'], {detached: true, stdio: 'ignore'})`. A setsid'd child has no controlling tty, which meets the Orca requirement.

**Bubble Tea / Go.** `syscall.SysProcAttr` on darwin has `Setsid bool // Create session`, `Setpgid`, `Noctty` (Go source `src/syscall/exec_libc2.go`, Go 1.25.6). Go has no fork-without-exec; re-exec `os.Args[0]` with `Setsid: true` and `Process.Release()` (https://pkg.go.dev/os). Signals via `signal.Notify` / `NotifyContext`; SIGHUP "is sent when a program loses its controlling terminal" (https://pkg.go.dev/os/signal).

**Ratatui / Rust.** `CommandExt::process_group` stable since 1.64; `pre_exec` stable since 1.34 but `unsafe` (async-signal-safe only); `CommandExt::setsid` is "Nightly-only experimental API" (#105376) (https://doc.rust-lang.org/std/os/unix/process/trait.CommandExt.html). Stable Rust therefore needs `unsafe { cmd.pre_exec(|| { libc::setsid(); Ok(()) }) }` or the `nix` crate. Signal handling is not in std; `signal-hook` 0.4.5 ("easier and safe Unix signal handling", https://docs.rs/signal-hook/latest/signal_hook/) or `tokio::signal`.

None of these is awkward enough to disqualify a daemon design.

## 2. File watching, locking, atomic rename

- **Node:** `fs.watch` uses inotify on Linux and FSEvents on macOS; caveats: "not 100% consistent across platforms", the filename argument is not guaranteed (https://nodejs.org/api/fs.html#caveats). `node:fs` exposes **no** flock/lockf/advisory lock API (same page, searched). The common userland lock `proper-lockfile` "utilizes the `mkdir` strategy" with periodic mtime refresh and a stale threshold (default 10 s, min 5 s) (https://github.com/moxystudio/node-proper-lockfile) - a cooperative lock, not flock, and a crashed holder blocks others until stale. Third-party watcher wrappers (e.g. chokidar) were not inspected.
- **Go:** fsnotify v1.10.1: Linux inotify, macOS/BSD **kqueue**, FSEvents "needs support in x/sys/unix". It advises against watching individual files because editors "write to a temporary file which is then moved to a destination"; "watch the parent directory and use `Event.Name` to filter". One write can produce many Write events; no built-in debounce; not recursive (https://pkg.go.dev/github.com/fsnotify/fsnotify). kqueue on macOS needs an open fd per watched file. Locking: `syscall.Flock(fd, how)`; the syscall docs steer new code to `golang.org/x/sys` (https://pkg.go.dev/syscall#Flock). `os.Rename`: "If newpath already exists and is not a directory, Rename replaces it" (https://pkg.go.dev/os#Rename).
- **Rust:** notify 8.2.0: FSEvents (default) or kqueue on macOS, inotify on Linux, PollWatcher; caveat that editors "truncate the file on save, some create a new one and replace the old one"; debouncing via `notify-debouncer-mini`/`-full` (https://docs.rs/notify/latest/notify/). `File::lock`/`try_lock`/`lock_shared`/`unlock` map to `flock` on Unix, **since 1.89.0** (https://doc.rust-lang.org/std/fs/struct.File.html) - note the local toolchain here is 1.88.0, so this needs a toolchain bump (or the `fs4`/`fs2` crates, not inspected). `std::fs::rename` replaces `to` if it exists (rename(2)) (https://doc.rust-lang.org/std/fs/fn.rename.html).
- All three: watch the **directory** holding `state.json`, not the file, given the atomic temp-then-rename write already settled in v1 ticket 04.

## 3. Single-binary distribution and macOS signing

- **Node SEA:** "Stability: 1.1 - Active development"; `node --build-sea sea-config.json` since v25.5.0; ESM via `"mainFormat": "module"`; macOS steps `codesign --remove-signature` then `codesign --sign -`; CI covers macOS **arm64 only**; cross-platform SEAs require `useCodeCache` and `useSnapshot` false (https://nodejs.org/api/single-executable-applications.html). A SEA is the node executable plus a blob; the local node 24.5.0 binary is 117 MB.
- **Bun:** `--compile` with `--target` for linux x64/arm64 (glibc+musl), darwin x64/arm64, windows x64/arm64; "Bun's binary is still way too big"; codesign with a JIT entitlements plist, "Codesign support requires Bun v1.2.4 or newer"; `--bytecode` for faster startup (https://bun.com/docs/bundler/executables). Local: 56.8 MB (plain) / 58.6 MB (Ink). Bundling Ink 8 failed until `react-devtools-core` (an Ink optional peer imported by `ink/build/devtools.js`) was installed. With Bun 1.2.1 the output kept Bun's own Developer ID signature and `codesign -v` reported "failed strict validation"; it still ran from the terminal.
- **pkg:** "`pkg` has been deprecated with `5.8.1` as the last release"; repo archived (https://github.com/vercel/pkg).
- **Go / Rust:** native executables, no runtime. Local stripped sizes 2.9 MB (Go + Bubble Tea v1.3.10) and 0.58 MB (Rust + ratatui 0.30.2, trivial program; a real app will be larger). `codesign -dv` on both shows `flags=0x20002(adhoc,linker-signed)`, i.e. the linker ad-hoc signs (observed locally; I did not find the Go release-note line stating it).
- **Gatekeeper:** "When a user downloads and opens an app ... Gatekeeper verifies that the software is from an identified developer, is notarized ..." and asks approval "before opening downloaded software for the first time"; "all software in macOS is checked for known malicious content the first time it's opened" (https://support.apple.com/guide/security/gatekeeper-and-runtime-protection-sec5599b66df/web). The Apple page does not explicitly cover locally built or non-quarantined binaries, nor a terminal-vs-hook launch distinction. **Not verified:** whether a quarantined unsigned CLI launched by a Claude Code hook (no UI to answer a prompt) is blocked vs prompted. Practical inference (not sourced): building from source, `go install`/`cargo install`, or Homebrew-from-source avoids the quarantine attribute, so the question only bites for downloaded prebuilt binaries.

## 4. Cold start

Local measurement, Apple M1, median of 20 runs after one warm-up, each program prints `{"running":false}` and exits (`python3` `subprocess.run` timing, includes fork/exec):

| Program | Median | Min |
| --- | --- | --- |
| `node plain.mjs` | 33.0 ms | 30.4 ms |
| `node ink.mjs` (static `import` of ink 8 + react 19, TUI not rendered) | 187.5 ms | 182.7 ms |
| `bun --compile` plain | 18.4 ms | 17.9 ms |
| `bun --compile` ink | 55.5 ms | 50.1 ms |
| Go binary importing Bubble Tea | 3.9 ms | 3.3 ms |
| Rust binary using ratatui | 3.0 ms | 2.8 ms |

Not measured: Node SEA, `node --build-snapshot`, compile cache. The Ink figure is import cost; a Node CLI that loads Ink only via dynamic `import()` on the TUI path should sit near the 33 ms plain figure. Bun docs cite `--bytecode` halving a larger app's cold start (https://bun.com/docs/bundler/executables). Hooks have generous timeouts (default 600 s; `SessionEnd` shares 1.5 s), so none of these breaks a hook; the cost is latency added to every hooked tool event.

## 5. Keyring

- **Node:** keytar (atom/node-keytar) archived, last release v7.9.0 (2022-02). `@napi-rs/keyring` v2.1.0 (2026-09-13): "keyring-rs Node.js binding via napi.rs", "Including 100% compatible node-keytar alternative"; Linux store selectable between Secret Service and keyutils (https://github.com/Brooooooklyn/keyring-node). Native prebuilds mean the addon must ship with any SEA/bun binary.
- **Go:** go-keyring v0.2.8: macOS shells out to `/usr/bin/security`; Linux/BSD uses Secret Service over D-Bus (GNOME Keyring, expects `login` collection); written to avoid C bindings for static binaries (https://github.com/zalando/go-keyring).
- **Rust:** keyring v4.2.0: `apple-native-keyring-store`, `linux-keyutils-keyring-store`, `dbus-secret-service-keyring-store` (https://docs.rs/keyring/latest/keyring/).
- **Headless:** none of the pages states headless behaviour explicitly. Secret Service needs a D-Bus session bus and an unlocked collection, which a headless SSH/Linux box usually lacks; keyutils (Rust and the napi binding) avoids D-Bus but its persistence across reboot was **not verified**. On macOS, all three ultimately reach the login Keychain; behaviour when invoked from a hook with no GUI session (e.g. over SSH) was **not verified**.

## 6. Terminal rendering

- **Ink:** text colour via chalk; chalk "Auto-detects color support", levels 0-3 (basic, 256, truecolor), overridable with `FORCE_COLOR` / `--color` (https://github.com/chalk/chalk). `Box` is flexbox ("like `<div style="display: flex">`") via Yoga, with borders; `useWindowSize()` "re-renders the component whenever the terminal is resized"; `alternateScreen` option; `incrementalRendering`; non-TTY mode skips cursor/resize handling and writes only the final frame (https://github.com/vadimdemedes/ink README). A bottom status bar is a `Box` row pinned with flex layout; keybinding hints are plain `Text`.
- **Bubble Tea v2:** "a high-performance cell-based renderer, built-in color downsampling, declarative views, high-fidelity keyboard and mouse handling" (https://github.com/charmbracelet/bubbletea); depends on `charmbracelet/colorprofile` (go.mod). `WindowSizeMsg` "sent to Update once initially and then on every terminal resize" (`screen.go`). Layout is string composition (Lip Gloss joins); the `bubbles/help` component renders key-binding help (not inspected in this pass). v2's go.mod says `go 1.26.0`; the local toolchain is 1.25.6.
- **Ratatui:** `Color` covers 16 named, `Indexed(u8)`, `Rgb`; for Rgb: "If the terminal does not support true color, code using the TermwizBackend will fallback to the default text color. Crossterm and Termion do not have this capability and the display will be unpredictable (e.g. Terminal.app may display glitched blinking text)" (https://github.com/ratatui/ratatui/blob/main/ratatui-core/src/style/color.rs). Immediate-mode, constraint-based `Layout` with automatic adaptation to size (https://ratatui.rs/). A status bar is one `Layout` row of `Line`/`Span`s. Relevant to brief item 9 (16/256-colour running row): Ratatui leaves palette degradation to the app; Ink and Bubble Tea downsample.

## 7. Testing a TUI

- **Ink:** `ink-testing-library` v4.0.0 (2024-05; repo last pushed 2024-06): `render()` returns `lastFrame()`, `frames`, `stdin.write()`, `rerender`, `unmount` (https://github.com/vadimdemedes/ink-testing-library). Snapshot with the test runner's own snapshot facility (Vitest/Jest), not built in.
- **Bubble Tea:** `teatest` in `github.com/charmbracelet/x/exp/teatest` (experimental; repo has a `v2/` subdirectory for Bubble Tea v2): `RequireEqualOutput` golden files, `Send`, `Type`, `WaitFor`, `FinalOutput`, `FinalModel` (https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest). The Elm-style `Update(msg)` is also unit-testable directly.
- **Ratatui:** `TestBackend` "renders to an memory buffer", `assert_buffer`, `assert_buffer_lines`, `assert_cursor_position`, `resize`, `Display` for snapshotting (https://docs.rs/ratatui/latest/ratatui/backend/struct.TestBackend.html); ratatui.rs recommends `insta` snapshots (https://ratatui.rs/; the recipe page timed out). There is no keystroke driver; tests call the app's own event-handling function.

## 8. Precedents mixing a TUI with a scriptable JSON CLI

- **Ink:** Claude Code - `claude -p ... --output-format json` (and `stream-json`) (https://code.claude.com/docs/en/headless); Ink's README lists Claude Code, Gemini CLI, and GitHub Copilot CLI as users. Gemini CLI's JSON output flag was not verified in this pass.
- **Bubble Tea:** MinIO `mc` - Bubble Tea, Bubbles, Lip Gloss in `go.mod`; global flag `json`, "enable JSON lines formatted output" (`cmd/flags.go`, https://github.com/minio/mc). Which `mc` subcommands render Bubble Tea UIs was not traced. Bubble Tea README's "in the wild" also lists chezmoi, gh-dash, Trufflehog, Glow.
- **Ratatui:** OpenAI Codex CLI - `codex-rs/tui` depends on ratatui 0.30.2 (https://github.com/openai/codex); `codex exec --json` makes "stdout ... a JSON Lines (JSONL) stream" (https://learn.chatgpt.com/docs/non-interactive-mode).
- **Correction to the brief:** lazygit is **not** a Bubble Tea app. Its `go.mod` has `gdamore/tcell/v3` and no Charm dependency; the UI is an in-tree fork of gocui at `pkg/gocui` (https://github.com/jesseduffield/lazygit). It still demonstrates the feel; it is not a Bubble Tea precedent.

## Unverified / open

- Gatekeeper behaviour for a quarantined, unsigned binary launched non-interactively from a hook.
- Keychain access from a hook in a non-GUI (SSH) session; keyutils persistence.
- Node `fs.rename` doc wording (assumed rename(2)).
- Node SEA cold start and size for an Ink app (not built).
