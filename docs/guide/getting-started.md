# Getting started

This page takes you from nothing to a worklog in Jira: install, `punchcard setup`, a first timer, a first stop.

## 1. Install

The [README](../../README.md#install) has the install routes. In short: Homebrew once there is a release, or `go install` from a clone. Check it worked:

```
$ punchcard version
punchcard v0.0.0-20261009151055-725c73585087 (contract v1, go1.26.9 darwin/arm64)
```

A build from a clone prints a version like the one above. A release prints `punchcard v0.1.0 (…)`.

## 2. Connect it to Jira with `punchcard setup`

You need a Jira Cloud site, the email you sign in to Atlassian with, and a **classic** API token. (No site yet? [Testing without a real Jira](sandbox.md) runs a fake one on localhost.) Make the token at <https://id.atlassian.com/manage-profile/security/api-tokens>. Jira refuses a *scoped* token with the same 401 it gives a wrong one, so make the classic kind.

Run the wizard in a terminal. It needs one: `punchcard setup` in a pipe or with `--json` exits 64.

```
$ punchcard setup

●●●··●··●·●··●··●●··●··●··●●···●●··●●●··●●●·
●··●·●··●·●●·●·●··●·●··●·●··●·●··●·●··●·●··●
●··●·●··●·●●·●·●····●··●·●····●··●·●··●·●··●
●●●··●··●·●·●●·●····●●●●·●····●●●●·●●●··●··●
●····●··●·●·●●·●····●··●·●····●··●·●·●··●··●
●····●··●·●··●·●··●·●··●·●··●·●··●·●··●·●··●
●·····●●··●··●··●●··●··●··●●··●··●·●··●·●●●·

┌  punchcard setup
│ Jira site
│
│ > acme.atlassian.net
```

The steps, in order. Each one must pass before the next appears, and nothing is written until the last.

1. **Jira site.** Paste anything: a bare host, a full URL, a board URL from your address bar, or just the site name. It is normalised to `https://<host>` and echoed back under the field as you type. A name with no dot becomes `<name>.atlassian.net`.
2. **Email.** The one you sign in to Atlassian with.
3. **API token.** Masked. The field shows the link to the token page and the scoped-token warning. The terminal waits while you go to the browser.
4. **Test Connection.** Automatic. It asks the site who it is (it must be Jira Cloud), then who you are. A bad site sends you back to step 1 with the error. A 401 sends you back to step 3 with a reminder that a scoped token is refused like a wrong one. A pass prints `✓ Connected as <your display name>`.
5. **Save these credentials?** Default yes. The site and email go to `config.json`. On macOS the token goes into the Keychain through Apple's `security` tool, as an item named `punchcard` for your email. If the Keychain write fails, or on Linux, the token goes to a file beside the config that only you can read, and setup says so.
6. **Install the Claude Code plugin?** Offered when `claude` is on your `PATH`. See [Claude Code](claude-code.md). If `claude` is not found, setup prints the two commands to run later.
7. **Add the ask rule?** Offered whether or not you installed the plugin. It puts one rule in your Claude Code settings so that an agent's `punchcard stop --yes` always asks you first. Setup shows the exact JSON, keeps every other key in the file, and writes a `.bak` first.

Esc or ctrl-c at steps 1 to 5 writes nothing and prints `Nothing saved`. After step 5, esc leaves the saved credentials in place and prints `Stopped. Your Jira credentials are saved.`

This is what cancelling at the first prompt looks like, run through a pseudo-terminal:

```
┌  punchcard setup
│ Jira site
│
│ > acme.atlassian.net
└  Nothing saved
```

Running `punchcard setup` again opens with a summary and a fork. This run was made against a fake Jira with the token in a file and Claude Code pointed at an empty settings directory; on a Mac after a normal setup the first line ends `(token in Keychain)`:

```
┌  punchcard setup
│  Connected to http://127.0.0.1:48801 as tester@example.com (token in /…/punchcard/token)
◇  Change credentials?
│  No
│
◇  Install the Claude Code plugin?
│  No
│
│  Claude Code asks you before an agent's punchcard stop --yes reaches Jira
│  if your settings have this rule:
│    { "permissions": { "ask": ["Bash(punchcard stop *--yes*)"] } }
◇  Add the ask rule to /…/settings.json?
│  Yes
│
│  ✓ Created /…/settings.json with the ask rule
│  Done. Run punchcard to open the TUI.
└
```

Answering No to "Change credentials?" skips to the plugin and ask-rule steps. Answering Yes walks the same five steps, with the site and email prefilled and the token blank, labelled "Stored in Keychain — leave blank to keep". If Jira has rejected your token since the last setup, the fork defaults to Yes and says when the rejection happened.

**A true first run does this for you.** With no config file and no state file, a bare `punchcard` in a terminal runs the wizard before opening the TUI. Finishing or cancelling both land you in the TUI. It is offered once: the state file it writes is the marker.

Check the connection without touching Jira:

```
$ punchcard status
Nothing running.
Jira token: file
```

That run kept the token in a file. On a Mac after setup the last line reads `Jira token: Keychain`.

## 3. Start a timer

Start one on a ticket key. A ticket you have never used is added to your list and its summary is fetched from Jira:

```
$ punchcard start DEMO-1
+ DEMO-1 added
▶ DEMO-1 running  0:00:00  held  Add the worklog POST
```

`held` means you started it by hand, so nothing automatic will move it. Starting another ticket pauses this one and says so:

```
$ punchcard start DEMO-2
⏸ DEMO-1 paused
+ DEMO-2 added (not verified with Jira yet)
▶ DEMO-2 running  0:00:00  held
```

(That second run was made with no Jira connection, so the ticket is "not verified": it has no summary yet and will be confirmed the next time Jira can be reached.)

A typo on a connected Jira changes nothing:

```
$ punchcard start TYPO-999
punchcard: Jira has no ticket TYPO-999 that this account can see
  check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed.
```

Or run bare `punchcard` for the TUI, press `a` to add a ticket by key, and `space` on its row to start it. See [The TUI](tui.md).

The timer keeps running when you close the terminal or quit the TUI. It is a start time in a file, not a process.

## 4. Stop, and send the worklog to Jira

Stopping is the one act that reaches Jira. On a terminal, `stop` shows the measured time as the default, lets you correct it, and asks for an optional comment:

```
$ punchcard stop
DEMO-2 Duration [40m]: 
Comment (optional): Fixed the formatter
■ DEMO-2 stopped: 40m, sent to Jira
```

The duration takes `1h30m`, `1h`, `90m` or a bare number of minutes, from 1 minute to 24 hours. `1:30` is refused with a hint and the prompt asks again. Press ctrl-d at the duration to cancel: `punchcard: not stopped; the timer is untouched`.

Preview first if you like. This changes nothing:

```
$ punchcard stop --dry-run
DEMO-1: would send 47m, started 2026-10-09 09:33
```

What Jira gets: one worklog on the ticket, with the time truncated to the minute, started at the moment the entry began, and your comment as plain text. The ticket's remaining estimate is left alone.

If Jira cannot be reached, the stop still happens and the worklog waits:

```
$ punchcard stop
■ DEMO-2 stopped: 12m, worklog failed
punchcard: the worklog is saved but not sent: Jira answered 503: Service unavailable
  run `punchcard sync` later; never stop again
```

Later, send everything that is owed:

```
$ punchcard sync
✓ DEMO-2 12m sent (2026-10-09)
```

Nothing ever syncs in the background. Only `stop` (its own worklog) and `sync` talk to Jira about worklogs, and a worklog that has been sent is never sent twice.

## Next

- [Concepts](concepts.md): pause versus stop, held timers, what the `!` marker means.
- [Claude Code](claude-code.md): the plugin that moves the timer with your prompts.
