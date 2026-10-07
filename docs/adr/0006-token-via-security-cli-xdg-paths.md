# Credentials: the Jira token read through `/usr/bin/security`, never in-process, and XDG paths on macOS too

Punchcard v2 targets macOS (daily use, hand-tested) and Linux (must build and pass the test suite in CI, not hand-tested). The Jira token is found in this order. **`PUNCHCARD_TOKEN`** is an override that `setup` never writes. Next comes the **macOS Keychain, written and read only by shelling out to `/usr/bin/security`** (generic password, service `punchcard`, account = the configured email, about 1 s timeout). Last is a **`0600` file**. On Linux the chain is the env var and then the file. `status` reports which source was used. The Keychain is reached through the `security` binary rather than Security.framework because a Keychain item's ACL trusts the *application that created it*. If `/usr/bin/security`, which Apple signs, both writes and reads the item, any caller gets it with no prompt. An unsigned Go binary reading in-process becomes its own identity and risks a GUI prompt after every rebuild or upgrade. That prompt would land inside a hook, possibly with nobody there to answer it. A probe confirmed the no-prompt claim: a throwaway item read from a detached child process with no TTY and stdin closed came back in about 30 ms every time, with no dialog.

Config and data live at **XDG paths on both platforms**, honouring the `XDG_*` env vars, rather than at Go's `os.UserConfigDir` (`~/Library/Application Support` on macOS). Everything sits where a terminal user looks for it, and the same on both OSes, as `gh` does. The paths are:

- `$XDG_CONFIG_HOME/punchcard/config.json`: `{schemaVersion, siteUrl, email}`, nothing secret.
- `$XDG_CONFIG_HOME/punchcard/token`: the `0600` fallback, kept apart from config so that config can be `cat`'d or kept in dotfiles.
- `$XDG_DATA_HOME/punchcard/`: `state.json`, `heartbeat.json`, `state.lock`, `sync.lock`, and corrupt-file backups. These go under *data*, not *state*, because worklog history is not disposable.

## Considered Options

- **Keychain in-process (go-keyring's cgo path, Security.framework).** Rejected: a prompt for each binary identity, inside unattended hooks.
- **Linux Secret Service (go-keyring over D-Bus).** Rejected for now: no timeout of its own (it hangs on a locked keyring), and it fails on headless boxes anyway. It can be added later behind the same lookup.
- **`0600` file only, everywhere.** Rejected: it breaks user story 59 (token in the Keychain, not a file) on the platform used every day.
- **Env var only.** Rejected: a GUI-launched Orca may never pass shell env vars to hooks.
- **TOML config.** Rejected: JSON is in the standard library and matches `state.json`, and two fields need no comments.

## Consequences

- **The file is a fallback, never a silent default on macOS.** If the Keychain write fails, `setup` writes the file and says so. If both exist, the Keychain wins and `status` notes the stray file. `setup` deletes every `punchcard` item before writing, so there is one credential at a time.
- **A Keychain that errors or times out is not an auth failure.** With no fallback file, a command that needs Jira exits 3 `not_configured` and names the Keychain as the cause.
- **A 401 is persisted, since no process lives long enough to remember it.** It writes `authFailedAt` into `state.json` under the state lock. It is cleared only by a successful authenticated Jira call or a fresh `setup`. It never deletes or rewrites the token, never touches a timer, and leaves the worklog pending (stories 60–62). `status --json` exposes it for the TUI banner and the mod's band. It records a fact about Jira, not a derived timer state, so the carried rule "every state derived, none stored" still holds.
- **Reading the token costs about 30 ms on macOS.** Only Jira-calling commands pay it (`start` on a new ticket, `stop`, `sync`). The every-prompt presence path never reads the token.
