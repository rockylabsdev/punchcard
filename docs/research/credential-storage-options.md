# Credential storage without Keychain-from-a-signed-app

Research for wayfinder ticket #5 (part of map #1). Question: where does a terminal Punchcard keep the Jira classic API token (Basic auth) so that it works **unattended from a Claude Code hook** (including inside Orca, which launches Claude with permissions bypassed), on macOS and possibly Linux?

Sources are primary (man pages / help text, library source and READMEs, specs, vendor docs). Points I could not check against a primary source before the deadline are marked **[not verified]**. Inferences I drew from verified facts are marked **(inference)**.

---

## 1. macOS Keychain via the `security` CLI

### What the tool does (verified locally, macOS 26 / Darwin 25.5)

`security add-generic-password -h` says:

- `-T` "Specify an application which may access this item (multiple -T options are allowed)".
- `-A` "Allow any application to access this item without warning (insecure, not recommended!)".
- `-U` "Update item if it already exists".
- "**By default, the application which creates an item is trusted to access its data without warning.** You can remove this default access by explicitly specifying an empty app pathname: `-T \"\"`".
- "Use of the -p or -w options is insecure. Specify -w as the last option to be prompted." (A `-w <secret>` argument shows up in the process list.)

`security find-generic-password -s SERVICE -a ACCOUNT -w` prints only the password to stdout.

`man security`, `set-generic-password-partition-list`: the partition list is "an extra parameter in the ACL which limits access to the item based on an application's code signature. You must present the keychain's password to change a partition list."

### Who gets prompted, and when

Apple, *Access Control Lists* (https://developer.apple.com/documentation/security/access-control-lists): each legacy (file-based, non-iCloud) keychain item has an ACL listing trusted apps per operation. "If so [the caller is trusted], the system grants access. Otherwise, the system prompts the user for confirmation. The user may choose to Deny, Allow, or Always Allow the access. In the latter case, the system adds the app to the list of trusted apps".

Applied to the three callers in the ticket:

- **Item written and read through `/usr/bin/security`** (from Node, Go, Rust, or a shell hook, doesn't matter). The *creating application* is `/usr/bin/security`, so it is trusted by default, and later reads through the same Apple-signed binary should get **no prompt, ever**, whatever process spawned it. **(inference** from the help text plus Apple's ACL doc. I did not test it, because the ticket forbids touching real keychain items.**)** This is exactly what `zalando/go-keyring` and so `gh` rely on (see section 3).
- **Item read in-process through Security.framework by an unsigned or ad-hoc-signed binary** (for example a Rust/Go binary using the framework directly, or `node` loading a native addon such as `@napi-rs/keyring`). Here the trusted app is that binary. A different binary reading the item gets the GUI prompt. "Always Allow" adds it to the ACL. Whether a rebuilt or upgraded unsigned binary (new hash), or a switch of `node` version via nvm/Homebrew, is still treated as the same trusted app and so avoids a new prompt is **[not verified]**. The partition-list text above ("based on an application's code signature") suggests identity is tied to the signature, which is the prompt-storm risk. **(inference)**
- **Locked keychain / no GUI session** (for example SSH into the Mac). The relevant error is `errSecInteractionNotAllowed`. Locally, `security error -25308` gives "User interaction is not allowed." That a hook-launched process in a logged-in GUI session sees an unlocked login keychain is **[not verified]**. I expect it, since Claude Code and Orca run inside the user's GUI login session.

### Can the ACL be made non-interactive?

Yes, in two ways, both from the help text:

1. Write with `security` and read with `security`, so the default creator trust covers every read (recommended).
2. Add specific readers with `-T /path/to/binary`, or use `-A` for any app (Apple labels `-A` insecure).

Changing the partition list needs the keychain password, so it is not something to script into setup.

### Rotation by hand

`security add-generic-password -U -s punchcard -a <email> -w` (prompts for the value), or Keychain Access.app. Easy to find under the service name.

---

## 2. Cross-platform keyring libraries

### Node: `keytar` is dead; `@napi-rs/keyring` is the successor

- `atom/node-keytar` is **archived** (GitHub API: `archived: true`, last push 2022-12-12). https://github.com/atom/node-keytar
- `@napi-rs/keyring` (https://github.com/Brooooooklyn/keyring-node) describes itself as the "https://github.com/hwchen/keyring-rs Node.js binding via https://napi.rs". Its `Cargo.toml` depends on `keyring-core` 1.x, plus `apple-native-keyring-store` with feature `keychain` on macOS, and on Linux `dbus-secret-service-keyring-store` (vendored, pure-Rust crypto) **and** `linux-keyutils-keyring-store`.
- README, Linux backend selection: "the Secret Service (gnome-keyring, KWallet, keepassxc, ...) is tried first, and the binding **silently falls back** to the kernel keyutils keyring when no Secret Service is available." You can pin the store with `{ linux: { store: 'secret-service' } }`, and then "the constructor throws if that store is unavailable". Warning in the README: keyutils "is completely in-memory and **will not persist across reboots**."
- On macOS it goes through Security.framework in-process (per its `Cargo.toml`), so the caller identity is the `node` binary. That makes it the second case in section 1. Prompt behaviour across Node upgrades is **[not verified]**.

### Go: `zalando/go-keyring` (what `gh` uses)

README (https://github.com/zalando/go-keyring): "The OS X implementation depends on the `/usr/bin/security` binary". On Linux/BSD it "depends on the Secret Service dbus interface, which is provided by GNOME Keyring", and "It's expected that the default collection `login` exists".

Source, `keyring_darwin.go`:

- `Get` runs `/usr/bin/security find-generic-password -s <svc> -wa <user>`.
- `Set` runs `/usr/bin/security -i` and writes `add-generic-password -U -s … -a … -w …` to **stdin**, so the secret stays out of argv. Values are stored base64 with a `go-keyring-base64:` prefix. The limit is 4096 bytes per command (`ErrSetDataTooBig`).

Source, `secret_service/secret_service.go`:

- `NewSecretService()` calls `dbus.SessionBus()` and returns its error. With no session bus (headless, SSH, cron), this **fails immediately with an error**. The exact message is **[not verified]**.
- Collection is `/org/freedesktop/secrets/collection/login`, else the `aliases/default` alias.
- `handlePrompt` calls `Prompt()` and then blocks on `signal := <-promptSignal` with **no timeout**. If the collection is locked and nobody can answer the unlock dialog, the call **hangs**. This is the reason `gh` wraps every keyring call in a 60 s timeout (section 3).

### Rust: `keyring` (keyring-rs)

- Now `open-source-cooperative/keyring-rs` (https://github.com/open-source-cooperative/keyring-rs). The crate is a wrapper. "the library/API parts have been moved into the keyring-core crate and the credential stores all in separate crates." Its default `v1` feature keeps the old API.
- docs.rs v1 module (https://docs.rs/keyring/latest/keyring/v1/index.html): "On macOS, the secure credential store is Keychain Services." "On *nix operating systems, the secure credential store is the Secret Service." The v1 docs do not describe headless fallback **[not verified]**.
- `apple-native-keyring-store` (https://docs.rs/apple-native-keyring-store): the `keychain` store (legacy file keychain) "available to all applications". The `protected` store is the data-protection keychain, where "Some of its features are only available to applications with provisioning profiles". "This crate has no default features." For an unsigned CLI, use `keychain` (in-process, so same ACL caveat as Node).
- `linux-keyutils-keyring-store` (https://docs.rs/linux-keyutils-keyring-store): "completely in-memory and will not persist across reboots". It is recommended "for a headless linux box, or one that doesn't come with gnome-keyring". The docs advise treating it as a secure cache.

### Secret Service behaviour (the Linux backend for all three)

freedesktop Secret Service API (https://specifications.freedesktop.org/secret-service/latest/):

- §8 Locking and Unlocking (`unlocking.html`): "The secrets of locked items cannot be accessed." `Unlock()` "may also return a prompt object … it must be acted upon". "A service may lock previously unlocked items for any reason at any time … (such as the computer sleeping)."
- §9 Prompts (`prompts.html`): "the service may need to prompt the user for additional information, such as a master password. **The prompts are displayed by the service** on behalf of the client application." Prompts are bound to the caller's D-Bus connection.
- §4 Aliases: the `default` alias names the default collection.

**What fails on headless Linux / SSH, and how loudly:**

- No session bus means a fast, loud error from go-keyring and keyring-rs. In `@napi-rs/keyring` there is a *silent* fallback to keyutils, so the token disappears on reboot.
- A bus with a locked collection and no display means the prompt cannot be answered. go-keyring hangs with no timeout. Behaviour of the Rust/Node bindings here is **[not verified]**.
- KWallet: reachable only through its Secret Service compatibility layer for these libraries. No library above ships a native kwallet backend in the configurations read **[not verified for keyring-rs optional stores]**.

---

## 3. A `0600` config file under an XDG path, and how `gh`, `aws`, `npm`, `op` do it

### XDG Base Directory spec (https://specifications.freedesktop.org/basedir/latest/)

- "If `$XDG_CONFIG_HOME` is either not set or empty, a default equal to `$HOME/.config` should be used."
- `$XDG_STATE_HOME` defaults to `$HOME/.local/state`.
- `$XDG_RUNTIME_DIR` "Its Unix access mode MUST be 0700."

### `gh` in detail (the closest precedent)

Read from `cli/cli` (gh 2.86.0 installed locally) and `cli/go-gh`.

**Storage backend.** `internal/keyring/keyring.go` (https://github.com/cli/cli/blob/trunk/internal/keyring/keyring.go) is "a simple wrapper that adds timeouts to the zalando/go-keyring package". `Set`, `Get` and `Delete` each run in a goroutine and give up after **60 seconds** with `TimeoutError`.

**Item name.** `keyringServiceName(hostname)` returns `"gh:" + hostname` (for example service `gh:github.com`). The account is the username. An empty-username item holds the *active* token (`internal/config/config.go`, https://github.com/cli/cli/blob/trunk/internal/config/config.go).

**Write path: keyring with file fallback** (`AuthConfig.Login`):

1. If secure storage is on (the default), `keyring.Set("gh:<host>", user, token)`.
2. On success, remove any old `oauth_token` from the config file.
3. If secure storage is off (`--insecure-storage`) **or the keyring write failed**, write `hosts.<host>.users.<user>.oauth_token` into `hosts.yml` and report `insecureStorageUsed = true`.

The help text for `gh auth login` (`pkg/cmd/auth/login/login.go`) says: "the authentication token will be stored securely in the system credential store. If a credential store is not found or there is an issue using it gh will fallback to writing the token to a plain text file. See `gh auth status` for its stored location." The flag is `--insecure-storage`: "Save authentication credentials in plain text instead of credential store."

**Read path, in precedence order** (`AuthConfig.ActiveToken`, which calls `go-gh` `auth.TokenFromEnvOrConfig`):

1. `GH_TOKEN`, then `GITHUB_TOKEN` (for github.com / ghe.com / localhost). Enterprise hosts use `GH_ENTERPRISE_TOKEN`, then `GITHUB_ENTERPRISE_TOKEN`.
2. Plain-text `oauth_token` in `hosts.yml`.
3. Keyring item for the active user, then the legacy unkeyed item.

`gh help environment`: `GH_TOKEN` "Setting this avoids being prompted to authenticate and takes precedence over previously stored credentials." The login help says env vars are "most suitable for 'headless' use of gh such as in automation."

**File location and mode** (`cli/go-gh` `pkg/config/config.go`, https://github.com/cli/go-gh/blob/trunk/pkg/config/config.go):

- `ConfigDir()` is `$GH_CONFIG_DIR`, else `$XDG_CONFIG_HOME/gh`, else `~/.config/gh`. The fallback file is `hosts.yml` there.
- `writeFile` makes the directory `0771` and opens the file with mode **`0600`**.

**Escape hatch for other tools.** `go-gh` `tokenFromGh` shells out to `gh auth token --secure-storage --hostname <host>`, so one binary owns the keyring item.

**Why gh is quiet from hooks on macOS (inference).** gh writes and reads only through `/usr/bin/security` (via go-keyring). The item's creator and every later reader are the same Apple-signed binary, so there is no ACL prompt, whichever terminal or hook launched `gh`.

### `aws` CLI

https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html says "These files are **plaintext** files". Credentials live in `~/.aws/credentials` (INI, `[default]` / `[name]` profiles).

https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html:

- Env vars override profile values; command-line parameters override both.
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` override the file.
- `AWS_SHARED_CREDENTIALS_FILE` (default `~/.aws/credentials`) and `AWS_CONFIG_FILE` relocate the files.

No OS keyring. The file mode aws uses is **[not verified]**.

### `npm`

https://docs.npmjs.com/cli/v10/configuring-npm/npmrc:

- Per-user config is "`$HOME/.npmrc` (or the `userconfig` param …)".
- `_authToken` and other credentials "must all be scoped to a specific registry. This ensures that npm will never send credentials to the wrong host."
- "Environment variables can be replaced using `${VARIABLE_NAME}`". So `//registry/:_authToken=${NPM_TOKEN}` keeps the secret in the environment.

The page does not mention file permissions. No OS keyring.

### 1Password `op`

- Desktop-app integration: "Authenticate 1Password CLI the same way you unlock your device, like with your fingerprint, face, Apple Watch…" (https://www.1password.dev/cli/app-integration/). It is interactive by design.
- Manual sign-in: `op` "creates a session token and sets the `OP_SESSION` environment variable"; "Session tokens expire after 30 minutes of inactivity" (https://www.1password.dev/cli/sign-in-manually/).
- Automation: "Set the `OP_SERVICE_ACCOUNT_TOKEN` environment variable to the service account token" (https://www.1password.dev/service-accounts/use-with-1password-cli/).
- Where `op` keeps its config file, and whether it persists the service-account token, is **[not verified]**.

Lesson: the password-manager CLI itself treats env vars as *the* unattended path and biometric prompts as the interactive path.

---

## 4. Environment variable (`PUNCHCARD_TOKEN`)

**Precedent.** `gh` (`GH_TOKEN`, highest precedence), `aws` (`AWS_*` override the file), `npm` (`${NPM_TOKEN}` interpolation in `.npmrc`), and `op` (`OP_SERVICE_ACCOUNT_TOKEN`). All cited above.

**As the only store.** It never prompts and works on any OS. It does not survive anything by itself: the user has to put it in a shell rc or secret manager. Whether it reaches the hook depends on how Claude Code / Orca were launched. A GUI-launched Orca may not source `~/.zshrc`, so the variable can be **missing in exactly the hook context we care about** **(inference; Orca's environment inheritance is [not verified])**.

**As an override on top of a store.** This is what every surveyed tool does. It is the clean escape hatch for CI, SSH, or debugging, and it costs one `getenv`.

---

## Criteria table

| Option | Unattended from a hook, no prompt storm | Headless Linux | Survives reinstall | Find and rotate by hand |
|---|---|---|---|---|
| 1a. Keychain, written **and** read via `/usr/bin/security` | Yes, creator-trusted, no prompt (inference, gh precedent) | n/a (macOS only) | Yes, item lives in login keychain | Yes: `security … -U`, Keychain Access |
| 1b. Keychain via in-process framework (Node addon / unsigned Rust/Go binary) | Risky: prompt per new caller identity; behaviour across rebuilds/upgrades [not verified] | n/a | Yes, but a new binary may re-prompt | Yes |
| 2. Keyring lib on Linux (Secret Service) | Yes if the collection is unlocked in a desktop session; go-keyring **hangs** on an unanswerable prompt | **No**: no session bus means an error; `@napi-rs/keyring` silently falls back to keyutils (lost on reboot) | Yes (Secret Service); No (keyutils after reboot) | `secret-tool`, Seahorse; awkward |
| 3. `0600` file under `$XDG_CONFIG_HOME/punchcard/` | Yes, never prompts | Yes | Yes, if the uninstall leaves `~/.config` alone | Yes, the most obvious |
| 4. Env var only | Yes, if it reaches the hook's environment (not guaranteed for GUI-launched Orca) | Yes | Depends on where the user set it | Yes, in the shell rc |
| 4'. Env var as override on top of 1a/3 | Yes | Yes | Inherits the base store | Yes |

## Takeaways for the platform-floor-and-credentials decision

- **Disqualified for unattended hooks:** option **1b** (in-process Keychain from an unsigned binary or Node addon) because of the prompt-storm risk, and the **Linux Secret Service path without a timeout and fallback** (it hangs or fails on headless boxes). `@napi-rs/keyring`'s silent keyutils fallback is unsafe as the only store, because it loses the token on reboot.
- **The `gh` pattern carries over directly:** env var override, then a store, with a plain `0600` XDG file as the fallback whenever the keyring errors or times out, and `status` reporting which source was used. Using `/usr/bin/security` as the macOS backend is what keeps it prompt-free. Any stack can shell out to it the way go-keyring does, so this does not decide the stack.
