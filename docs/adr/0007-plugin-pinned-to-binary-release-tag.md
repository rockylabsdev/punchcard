# Distribution: a Homebrew tap, and the Claude Code plugin pinned to the binary's release tag

`punchcard` ships through a **Homebrew tap** (`j-dsmith/homebrew-tap`). Pushing a `v*` tag runs GoReleaser in GitHub Actions, gated on the Linux CI job. GoReleaser builds darwin/arm64, darwin/amd64, linux/amd64 and linux/arm64 with `CGO_ENABLED=0`, and publishes the tarballs and the formula. `go install` works as a free, undocumented fallback. Nothing is signed or notarised: Homebrew formula installs and `go install` never set `com.apple.quarantine`, and the Go linker ad-hoc signs darwin/arm64. The **plugin is pinned to the exact release of the binary**. In `.claude-plugin/marketplace.json` on `main`, the `punchcard` entry is a `github` source for `j-dsmith/punchcard` with `ref: vX.Y.Z`, and `plugin.json` carries `version: X.Y.Z`. A release script bumps both, commits, and then tags `vX.Y.Z`. So an install or update always gets the mod and skill from the same tag as the binary, never unreleased code from `main`.

## Considered Options

- **No `version`, so the plugin version is the commit SHA.** Rejected: every push to `main` becomes a plugin update that runs ahead of the released binary.
- **A `version` with a relative-path source.** Rejected: updates fire only on a bump, but an install between releases gets `main`'s mod under the old version number.
- **Marketplace auto-update.** Rejected: it is off by default for third-party marketplaces, and turning it on lets the plugin update before `brew upgrade`, which creates the opposite skew.
- **Homebrew `post_install` running `claude plugin update`.** Rejected: it runs in Homebrew's sandbox, may not find `claude`, and reaches into another tool's config.

## Consequences

- **The binary leads, and the plugin follows by hand.** After `brew upgrade`, the mod's band shows that the plugin and binary versions differ and says to run `punchcard setup`. `setup`'s plugin step offers `claude plugin update punchcard@punchcard`. Skew within one contract `v` is harmless because contract changes are additive (ADR 0004).
- **Everything calls the bare `punchcard` on `PATH`**: the mod, the skill's instructions to the agent, and the `permissions.ask` rule (ADR 0005), which matches only the bare name. Orca opens a login shell, so Claude Code inherits `/opt/homebrew/bin`. If the mod can't find the binary, the band shows `punchcard not found on PATH` and presence is a no-op.
- **Uninstall leaves the data.** `brew uninstall` removes only the binary. `~/.config/punchcard/`, `~/.local/share/punchcard/` (which may hold unsent worklogs) and the Keychain item `punchcard` stay, and the README lists the three removal commands. The plugin is removed separately with `claude plugin uninstall`.
- **Tarballs downloaded through a browser are unsupported.** They are quarantined; the README gives `xattr -d com.apple.quarantine` as the workaround.
