# Getting started

This page takes you from nothing to a week of worklogs: install, a first timer, a first stop, `punchcard log`. None of it needs an account or a setup step. Connecting Jira, if you use it, is the optional last section.

## 1. Install

The [README](../../README.md#install) has the install routes. In short: Homebrew once there is a release, or `go install` from a clone. Check it worked:

```
$ punchcard version
punchcard v0.0.0-20261009151055-725c73585087 (contract v1, go1.26.9 darwin/arm64)
```

A build from a clone prints a version like the one above. A release prints `punchcard v0.1.0 (…)`.

## 2. Start a timer

Start one on a ticket key. A key is any name you choose: letters, digits, `.`, `_` and `-`, starting with a letter or digit (`payroll`, `client-acme`, `ABC-123`). A ticket you have never used is added to your list:

```
$ punchcard start payroll
+ payroll added
▶ payroll running  0:00:00
```

Starting another ticket pauses this one and says so:

```
$ punchcard start payroll-q4
⏸ payroll paused
+ payroll-q4 added
▶ payroll-q4 running  0:00:00
```

A timer you start by hand is **held**: nothing automatic, such as the Claude Code plugin, will move it. See [Concepts](concepts.md#held).

Or run bare `punchcard` for the TUI, press `a` to add a ticket by key, and `space` on its row to start it. See [The TUI](tui.md).

The timer keeps running when you close the terminal or quit the TUI. It is a start time in a file, not a process.

## 3. Stop, and log the worklog

On a terminal, `stop` shows the measured time as the default, lets you correct it, and asks for an optional comment. In a script, give both:

```
$ punchcard stop --yes --duration 30 -m "pay run"
■ payroll stopped: 30m, logged
```

The duration takes `1h30m`, `1h`, `90m` or a bare number of minutes, from 1 minute to 24 hours. `1:30` is refused with a hint and the prompt asks again. Press ctrl-d at the duration prompt to cancel: `punchcard: not stopped; the timer is untouched`. `punchcard stop --dry-run` previews and changes nothing.

`logged` means the worklog is final: one record of the ticket, the time truncated to the minute, the moment the entry began, and your comment. With no tracker it stays on your machine and nothing is ever owed.

## 4. Read back your week

`punchcard log` lists worklogs, Monday of this week through today by default, as Markdown you can paste into a PR, an email or an invoice:

```
$ punchcard log
## 2026-10-09 Fri · 0h 30m

| key | summary | time | comment |
|---|---|---|---|
| payroll |  | 0:30 | pay run |

**Total: 0h 30m**
```

`--csv` is for a spreadsheet; `--json` is for a script or an agent:

```
$ punchcard log --csv
date,started,key,summary,minutes,comment,state,remote_id,entry_id,recovered
2026-10-09,16:54,payroll,,30,pay run,local,,3FEF1118-E02E-469C-B33C-B304E5652AE4,false
```

`--month` is the first of this month through today, and `--from 2026-10-01 --to 2026-10-31` is any range. Every format and range is in [The CLI](cli.md#log).

## 5. A ticket per repository (optional)

If a directory is always billed to the same ticket, put the key in a `.punchcard` file at its root:

```
$ cat .punchcard
# this repository is billed to
payroll
```

The Claude Code plugin then keeps the timer on `payroll` while you work in that repository, whatever branch is checked out. Without a file, it uses an `ABC-123`-shaped key in the git branch name. See [`.punchcard` files](claude-code.md#punchcard-files).

## 6. The Claude Code plugin, and a tracker (optional)

`punchcard setup` is where both are chosen. It needs a terminal: in a pipe or with `--json` it exits 64. Its first question is the tracker:

- **None: time stays on this machine.** What you have been doing so far.
- **Jira: stopped time becomes Jira worklogs.** Each stop also sends its worklog to Jira.

Choosing none goes straight to the Claude Code steps. This run was made through a pseudo-terminal with a blank home directory and no `claude` on the `PATH`:

```
┌  punchcard setup
◇  Tracker
│  None: time stays on this machine
│
│  Saved /…/.config/punchcard/config.json: no tracker, so stopped time is logged here
│  Claude Code (claude) is not on PATH. To add the punchcard plugin later, run:
│    claude plugin marketplace add j-dsmith/punchcard
│    claude plugin install punchcard@punchcard -s user
│  Claude Code asks you before an agent's punchcard stop --yes logs a worklog
│  if your settings have this rule:
│    { "permissions": { "ask": ["Bash(punchcard stop *--yes*)"] } }
◇  Add the ask rule to /…/.claude/settings.json?
│  Yes
│
│  ✓ Created /…/.claude/settings.json with the ask rule
│  Done. Run punchcard to open the TUI.
└
```

The two Claude Code steps:

- **Install the Claude Code plugin?** Offered when `claude` is on your `PATH`. See [Claude Code](claude-code.md). If `claude` is not found, setup prints the two commands to run later.
- **Add the ask rule?** Offered whether or not you installed the plugin. It puts one rule in your Claude Code settings so that an agent's `punchcard stop --yes` always asks you first. Setup shows the exact JSON, keeps every other key in the file, and writes a `.bak` first.

Nothing runs `setup` for you: a blank machine runs whatever command you give it.

### Connecting Jira

Only if you choose Jira at the first step. You need a Jira Cloud site, the email you sign in to Atlassian with, and a **classic** API token. (No site yet? [Testing without a real Jira](sandbox.md) runs a fake one on localhost.) Make the token at <https://id.atlassian.com/manage-profile/security/api-tokens>. Jira refuses a *scoped* token with the same 401 it gives a wrong one, so make the classic kind.

After the tracker step, in order. Each one must pass before the next appears, and nothing is written until the last.

1. **Jira site.** Paste anything: a bare host, a full URL, a board URL from your address bar, or just the site name. It is normalised to `https://<host>` and echoed back under the field as you type. A name with no dot becomes `<name>.atlassian.net`.
2. **Email.** The one you sign in to Atlassian with.
3. **API token.** Masked. The field shows the link to the token page and the scoped-token warning. The terminal waits while you go to the browser.
4. **Test Connection.** Automatic. It asks the site who it is (it must be Jira Cloud), then who you are. A bad site sends you back to step 1 with the error. A 401 sends you back to step 3 with a reminder that a scoped token is refused like a wrong one. A pass prints `✓ Connected as <your display name>`.
5. **Save these credentials?** Default yes. The tracker, site and email go to `config.json`. On macOS the token goes into the Keychain through Apple's `security` tool, as an item named `punchcard` for your email. If the Keychain write fails, or on Linux, the token goes to a file beside the config that only you can read, and setup says so.

Then the two Claude Code steps. This run was made against [fakejira](sandbox.md) with the token in `PUNCHCARD_TOKEN`, so the token field was left blank and nothing was saved for it:

```
┌  punchcard setup
◇  Tracker
│  Jira: stopped time becomes Jira worklogs
│
◇  Jira site
│  http://127.0.0.1:48801
│
◇  Email
│  tester@example.com
│
◇  API token
│  (blank)
│
│  Testing with PUNCHCARD_TOKEN…
│  ✓ Connected as Sandbox Tester
◇  Save these credentials?
│  Yes
│
│  Saved /…/.config/punchcard/config.json
│  No token saved: PUNCHCARD_TOKEN supplies it
│  Claude Code (claude) is not on PATH. To add the punchcard plugin later, run:
│    claude plugin marketplace add j-dsmith/punchcard
│    claude plugin install punchcard@punchcard -s user
│  Claude Code asks you before an agent's punchcard stop --yes logs a worklog
│  if your settings have this rule:
│    { "permissions": { "ask": ["Bash(punchcard stop *--yes*)"] } }
◇  Add the ask rule to /…/.claude/settings.json?
│  Yes
│
│  ✓ Created /…/.claude/settings.json with the ask rule
│  Done. Run punchcard to open the TUI.
└
```

Esc or ctrl-c before the credentials are saved writes nothing and prints `Nothing saved`. After that, esc leaves the saved credentials in place and prints `Stopped. Your Jira credentials are saved.`

Running `punchcard setup` again opens with the tracker prefilled. Keeping Jira shows a summary and a fork:

```
┌  punchcard setup
◇  Tracker
│  Jira: stopped time becomes Jira worklogs
│
│  Connected to http://127.0.0.1:48801 as tester@example.com (token from PUNCHCARD_TOKEN)
◇  Change credentials?
│  No
│
│  Claude Code (claude) is not on PATH. To add the punchcard plugin later, run:
│    claude plugin marketplace add j-dsmith/punchcard
│    claude plugin install punchcard@punchcard -s user
│  ✓ Ask rule in place (/…/.claude/settings.json)
│  Done. Run punchcard to open the TUI.
└
```

On a Mac after a normal setup the summary ends `(token in Keychain)`. Answering No to "Change credentials?" skips to the Claude Code steps. Answering Yes walks the same steps, with the site and email prefilled and the token blank, labelled "Stored in Keychain — leave blank to keep". If Jira has rejected your token since the last setup, the fork defaults to Yes and says when the rejection happened.

Choosing none on a re-run leaves Jira: the site and email stay in the file (so choosing Jira again finds them, and the token stays in the Keychain), and if any worklogs are still owed to Jira, setup offers to mark them local, showing how many and how much time. Declined, they stay owed until you choose Jira again and `sync`.

### Timing with Jira

With Jira, everything above works the same, and three things change. These runs were made against fakejira.

A ticket you have never used is looked up, and its summary shown. A key is Jira's shape, `ABC-123`, and is upper-cased for you:

```
$ punchcard start PUN-2
+ PUN-2 added
▶ PUN-2 running  0:00:00  Wire the worklog POST
$ punchcard start pun-3
⏸ PUN-2 paused
+ PUN-3 added
▶ PUN-3 running  0:00:00  Stop dialog: duration parsing and the live preview
```

A typo changes nothing:

```
$ punchcard start TYPO-999
punchcard: Jira has no ticket TYPO-999 that this account can see
  check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed.
```

If Jira cannot be reached, the key is added **unverified** (`+ DATA-9 added (not verified with Jira yet)`) and confirmed the next time Jira answers. See [Concepts](concepts.md#unverified).

A stop also sends the worklog. Jira's remaining estimate is left alone:

```
$ punchcard stop PUN-2 --yes --duration 25 -m "sandbox check"
■ PUN-2 stopped: 25m, sent to Jira
```

If Jira cannot be reached, the stop still happens and the worklog waits (exit 7):

```
$ punchcard stop PUN-3 --yes --duration 12
■ PUN-3 stopped: 12m, worklog failed
punchcard: the worklog is saved but not sent: Jira answered 503: Service unavailable
  run `punchcard sync` later; never stop again
```

Later, send everything that is owed:

```
$ punchcard sync
✓ PUN-3 12m sent (2026-10-09)
```

Nothing ever syncs in the background. Only `stop` (its own worklog) and `sync` talk to Jira about worklogs, and a worklog that has been sent is never sent twice. `punchcard log` lists Jira worklogs exactly as it lists local ones, with `state` `synced`, `pending` or `failed`.

## Next

- [Concepts](concepts.md): pause versus stop, held timers, what the `!` marker means.
- [Claude Code](claude-code.md): the plugin that moves the timer with your prompts.
