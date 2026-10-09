# Punchcard

Time tracking against Jira tickets from the terminal. One binary, `punchcard`, gives you a TUI (run it bare) and a CLI (`punchcard start ABC-123`, `punchcard stop`, `punchcard status`). A Claude Code plugin keeps the timer on your branch's ticket while you work with an agent.

macOS and Linux, on arm64 and amd64.

## Install

```sh
brew install j-dsmith/tap/punchcard
punchcard setup
```

`punchcard setup` asks for your Jira site, email and API token, tests the connection, and saves them. On macOS the token goes in the Keychain. If Claude Code is installed, setup also installs the `punchcard@punchcard` plugin at the same version as the binary.

### Upgrade

```sh
brew upgrade punchcard
punchcard setup
```

The binary updates first. Until you run `punchcard setup` again to update the plugin, the band above Claude Code's prompt says the plugin and binary versions differ.

### Without Homebrew

If you have Go installed:

```sh
go install github.com/j-dsmith/punchcard/cmd/punchcard@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. The plugin and Claude Code call the bare `punchcard` on `PATH`.

### Tarballs downloaded with a browser

Each [release](https://github.com/j-dsmith/punchcard/releases) has tarballs. These are not supported as an install method. The binary is not signed or notarised, and macOS quarantines files downloaded through a browser, so Gatekeeper refuses to run it. If you use one anyway, clear the quarantine flag after unpacking:

```sh
xattr -d com.apple.quarantine ./punchcard
```

Homebrew and `go install` never set this flag, so neither needs the workaround.

## Uninstall

```sh
brew uninstall punchcard
claude plugin uninstall punchcard@punchcard
```

Uninstalling leaves your config, your data and the Keychain item in place. Your data may hold worklogs that were never sent to Jira, so run `punchcard sync` first if `punchcard status` shows any unsent. To remove everything, run these three commands:

```sh
rm -rf ~/.config/punchcard                        # config and token file
rm -rf ~/.local/share/punchcard                   # timers and unsent worklogs
security delete-generic-password -s punchcard     # macOS Keychain item
```

If you set `XDG_CONFIG_HOME` or `XDG_DATA_HOME`, the first two are `$XDG_CONFIG_HOME/punchcard` and `$XDG_DATA_HOME/punchcard`.

## Releasing

Maintainers only. From a clean `main`:

```sh
scripts/release.sh 0.2.0
```

The script sets the plugin's `version` and the marketplace entry's `ref` to the new version, commits, and tags `v0.2.0`. It does not push. It prints the push command; pushing the tag runs the release workflow. That workflow runs CI first, then GoReleaser, which publishes the tarballs and the Homebrew formula.
