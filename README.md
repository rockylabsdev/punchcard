# Punchcard

A time tracker for the terminal. Time a ticket, stop it, and the time is a worklog on your machine; `punchcard log` turns a week of them into a timesheet. One binary, `punchcard`, gives you a TUI (run it bare) and a CLI. A Claude Code plugin keeps the timer on the ticket your session is for while you work with an agent.

```
$ punchcard start payroll
+ payroll added
▶ payroll running  0:00:00
$ punchcard stop --yes --duration 30 -m "pay run"
■ payroll stopped: 30m, logged
$ punchcard log
## 2026-10-09 Fri · 0h 30m

| key | summary | time | comment |
|---|---|---|---|
| payroll |  | 0:30 | pay run |

**Total: 0h 30m**
```

No account and no setup: a ticket is any key you choose. A tracker is optional. Jira is the one Punchcard knows today: choose it in `punchcard setup` and each stop also sends its worklog to Jira.

macOS and Linux, on arm64 and amd64.

**Documentation:** the [user guide](docs/guide/README.md) covers getting started, every command and key, the Claude Code plugin, troubleshooting, and a [testers' guide](docs/guide/testers.md).

## Install

**Before the first release** there is no tap, no release tarball and no installable plugin. Build from a clone (the repository is private, so you need access and an SSH key on GitHub):

```sh
git clone git@github.com:j-dsmith/punchcard.git && cd punchcard
go install ./cmd/punchcard
```

Or without a clone: `GOPRIVATE=github.com/j-dsmith go install github.com/j-dsmith/punchcard/cmd/punchcard@latest`. Either way, make sure `$(go env GOPATH)/bin` is on your `PATH`. Load the plugin per session with `claude --plugin-dir /path/to/punchcard/plugin` until `punchcard setup` can install it. The rest of this section describes the released install.

```sh
brew install j-dsmith/tap/punchcard
punchcard start payroll
```

That is all a trackerless install needs. Run `punchcard setup` when you want the Claude Code plugin or a tracker. Its first question is the tracker: none, or Jira. With Jira it asks for your site, email and API token, tests the connection, and saves them (on macOS the token goes in the Keychain). Either way, if Claude Code is installed, setup then offers the `punchcard@punchcard` plugin at the same version as the binary.

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

Make sure `$(go env GOPATH)/bin` is on your `PATH`. The plugin and Claude Code call the bare `punchcard` on `PATH`. Go 1.26 is required; an older Go downloads it on first build. While the repository is private, set `GOPRIVATE=github.com/j-dsmith` so Go fetches it over git instead of the public proxy.

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

Uninstalling leaves your config and your data in place, and with Jira the Keychain item too. Your data is your worklogs: export them first if you want them (`punchcard log --from 2026-01-01 --csv > worklogs.csv`). With Jira, your data may also hold worklogs never sent, so run `punchcard sync` first if `punchcard status` shows any unsent. To remove everything:

```sh
rm -rf ~/.config/punchcard                        # config (and, with Jira, the token file)
rm -rf ~/.local/share/punchcard                   # timers and worklogs
security delete-generic-password -s punchcard     # with Jira on macOS: the Keychain item
```

If you set `XDG_CONFIG_HOME` or `XDG_DATA_HOME`, the first two are `$XDG_CONFIG_HOME/punchcard` and `$XDG_DATA_HOME/punchcard`. If `punchcard setup` added the ask rule, it is the `Bash(punchcard stop *--yes*)` entry under `permissions.ask` in `~/.claude/settings.json`. See [Files and troubleshooting](docs/guide/files-and-troubleshooting.md).

## Releasing

Maintainers only. From a clean `main`:

```sh
scripts/release.sh 0.2.0
```

The script sets the plugin's `version` and the marketplace entry's `ref` to the new version, commits, and tags `v0.2.0`. It does not push. It prints the push command; pushing the tag runs the release workflow. That workflow runs CI first, then GoReleaser, which publishes the tarballs and the Homebrew formula.
