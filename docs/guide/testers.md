# Testers' guide

Thanks for trying this. Punchcard is a terminal time tracker: a timer that lives in your shell, a full-screen view of your tickets, a log of where the week went, and a Claude Code plugin that keeps the timer on the ticket your repository or branch names. It works on its own with no account anywhere; Jira is an optional tracker it can send worklogs to. This page gets you installed, walks you through the things that should work, and says what to send when they do not. You should not need to ask me anything; if you do, that is a bug in this page, so tell me that too.

Read the [Getting started](getting-started.md) page first if you like, but everything you need is here.

## What you need

- **macOS** (Apple Silicon or Intel) or Linux. On Linux the token goes in a file instead of the Keychain; everything else is the same.
- **Go 1.26 or newer** to build it. `brew install go` gives you a current Go. An older Go from 1.21 up downloads 1.26 by itself when it builds this.
- **Access to the repository.** It is private. I will add you as a collaborator; you need an SSH key on your GitHub account (`ssh -T git@github.com` should greet you).
- **Claude Code** 2.1.29x or newer for the plugin part (`claude --version`), and a git checkout whose branch name contains a ticket key, such as `feature/ABC-123-try-punchcard`.

Only for the optional Jira part (scenarios 4 to 14):

- **A Jira Cloud site** you can log time on (or, to try everything first without one, the fake Jira in [Testing without a real Jira](sandbox.md)), the email you sign in to Atlassian with, and a **classic API token** from <https://id.atlassian.com/manage-profile/security/api-tokens>. Not a scoped token: Jira refuses those with the same error as a wrong one.
- **One or two throwaway tickets** on that site. Punchcard will post worklogs to them. Delete the worklogs afterwards in Jira: open the ticket, find the work log (the clock icon or the "Work log" tab under Activity), and delete each entry. Punchcard never deletes anything in Jira.

Punchcard writes to `~/.local/share/punchcard` and, once you run `punchcard setup`, `~/.config/punchcard`. With Jira on a Mac it also keeps one Keychain item called `punchcard`. [Files and troubleshooting](files-and-troubleshooting.md) lists everything and how to remove it all.

## Install

There is no release yet, so build from a clone:

```sh
git clone git@github.com:j-dsmith/punchcard.git
cd punchcard
go install ./cmd/punchcard
export PATH="$(go env GOPATH)/bin:$PATH"   # put this line in your shell's rc file too
punchcard version
```

Expect `punchcard v0.0.0-<date>-<commit> (contract v1, go1.26.9 darwin/arm64)`, with your platform. If `punchcard` is not found, the `PATH` line is the problem.

The same thing without a clone also works, because the repository is private the `GOPRIVATE` part is required:

```sh
GOPRIVATE=github.com/j-dsmith go install github.com/j-dsmith/punchcard/cmd/punchcard@latest
```

Both were run while writing this page.

## The test script

Do these in order; later ones build on earlier ones. Each says what to do and what to expect. Note anything that differs, even wording. Scenarios 1 to 3 need nothing but the binary. Scenarios 4 to 14 add Jira; without one, most of them still work with your own keys (pausing, drift, a forgotten timer, two writers, the TUI), and 15 to 17 need no Jira. After a scenario that posts a worklog to Jira, delete it there.

### 1. The machine-readable envelope

```sh
punchcard version --json
```

Expect one line of JSON starting `{"v":1,"ok":true,"outcome":"ok","command":"version"`. Before any setup, `punchcard status` says `Nothing running.` and nothing else, and `punchcard status --json` has `"tracker":"none"`.

### 2. Without a tracker: start, stop, log

```sh
punchcard start payroll
punchcard start invoices
punchcard stop invoices --yes --duration 25 -m "March invoices"
punchcard status
punchcard log
punchcard log --csv
punchcard sync; echo "exit $?"
```

Expect `+ payroll added` and `▶ payroll running`, then `⏸ payroll paused` before `▶ invoices running`. A key is any word of letters, digits, `.`, `_` and `-`: `punchcard start "pay roll"` is refused with exit 64. The stop prints `■ invoices stopped: 25m, logged`: with no tracker a worklog is final the moment it stops, and nothing is ever owed. `status` shows `payroll` paused. `log` prints this week as Markdown, a heading per day (`## <date> <day> · 0h 25m`), a `key | summary | time | comment` table and `**Total: 0h 25m**`; paste it into a GitHub comment and check it renders as a table. `log --csv` prints a header row and one row with `state` `local`; open it in Numbers or Excel and check the columns. `sync` exits 3 with `there is no tracker to send to`. Also try `punchcard log --month` and `punchcard log --from <a date> --to <a date>`.

Run bare `punchcard` too: expect the TUI with no banner, and the bottom pane titled `[3] Today · logged 25m`. With an empty list (try it with `HOME` pointed at an empty directory) the Tickets pane also says `no tracker · S to set one up`.

### 3. A `.punchcard` file names the ticket

In a git checkout on a branch named for a ticket (`feature/ABC-123-try-punchcard`):

```sh
punchcard status --json --cwd "$PWD"
echo client-acme > .punchcard
punchcard status --json --cwd "$PWD"
echo 'not a key!' > .punchcard
punchcard status --json --cwd "$PWD"
rm .punchcard
```

Expect `"here":{"key":"ABC-123","source":"branch"}` first, then `"here":{"key":"client-acme","source":"file"}`. A subdirectory finds the same file. The invalid one prints a notice naming the file, `"not a key!" is not a key (like payroll); using the branch`, and `here` falls back to the branch. The plugin (scenario 15) uses this same answer.

## With Jira (optional)

Scenarios 4 to 14 connect Punchcard to Jira and try what that adds: summaries fetched from Jira, worklogs sent to it, and what happens when it cannot be reached.

### 4. Setup, including a wrong token

Run `punchcard setup`. The first question is the tracker: choose **Jira**. Type your site any way you like (`acme.atlassian.net`, the full URL, a board URL from the address bar) and watch it echo the normalised `https://…` form under the field. Type your email. **First paste a wrong token** (change one character): expect `✗ Jira rejected these credentials.` with the scoped-token note, and the wizard back at the token step. Paste the right one: expect `✓ Connected as <your name>` and `Save these credentials?`. Save.

Then: **answer No to "Install the Claude Code plugin?"** (it cannot work until there is a release; scenario 15 covers the plugin another way) and **Yes to "Add the ask rule"**. Expect `✓ Created …/settings.json with the ask rule` or `✓ Added the ask rule (old file kept as …/settings.json.bak)`.

Then `punchcard status`: expect `Nothing running.` and `Jira token: Keychain` (`file` on Linux).

Also try: run `punchcard setup` again and press Esc at the first question. Expect `Nothing saved` and nothing changed. Run it once more, keep Jira, answer No to "Change credentials?", No, and expect `✓ Ask rule in place`.

### 5. A first timer, with the real summary

```sh
punchcard start ABC-123        # your throwaway ticket
```

Expect `+ ABC-123 added` and `▶ ABC-123 running  0:00:00  held  <the ticket's real summary>`. Then a typo:

```sh
punchcard start ABC-999999
punchcard ls
```

Expect exit 5 with `Jira has no ticket ABC-999999 that this account can see` and `ls` unchanged.

### 6. Switching, pausing, resuming

```sh
punchcard start ABC-124        # your second ticket
punchcard status
punchcard pause
punchcard resume
punchcard resume ABC-123
punchcard status
```

Expect: starting the second ticket prints `⏸ ABC-123 paused` before `▶ ABC-124 running`. `pause` pauses it, `resume` resumes it, `resume ABC-123` pauses `ABC-124` and starts `ABC-123`. `status` lists one running and one paused line, and the running one says `held`.

### 7. No process, no drift

With `ABC-123` running, open a second terminal and run `punchcard status` there: same answer. Close the first terminal completely. Put the laptop to sleep for a couple of minutes, wake it, run `punchcard status`. Expect the elapsed time to include the sleep and to match the wall clock since you started. (Time is measured from timestamps, not counted by a process.)

### 8. Stop sends a worklog

Wait until the timer shows at least one minute, then on a terminal:

```sh
punchcard stop --dry-run
punchcard stop
```

The dry run prints `ABC-123: would send Nm, started <date time>` and changes nothing. The stop prompts `ABC-123 Duration [Nm]:`; press Enter to keep it, type a comment, Enter. Expect `■ ABC-123 stopped: Nm, sent to Jira`. Open the ticket in Jira: the worklog is there with your comment and the time rounded down to the minute. Delete it.

Also try a stop within a minute of starting: `punchcard stop` prompts `Duration:` with no default; an empty Enter asks again; type `5m`. Delete that worklog too.

### 9. Offline: the stop still happens, the worklog waits

Start `ABC-123`, then turn Wi-Fi off. Run:

```sh
punchcard stop --duration 5m -m "offline test"
punchcard status
punchcard start ABC-124
```

Expect the stop to print `■ ABC-123 stopped: 5m, worklog failed` and `the worklog is saved but not sent: could not reach the tracker …` with exit 7. `status` shows `1 unsent worklog(s), 0:05:00`. The start works and warns `punchcard: 1 unsent worklog(s), 0:05:00; run punchcard sync`. Turn Wi-Fi on:

```sh
punchcard sync
```

Expect `✓ ABC-123 5m sent (<date>)`. Delete it in Jira. Then `punchcard stop --duration 1m` to clear `ABC-124` (and delete that one).

### 10. A forgotten timer is capped, and you fix the duration

Start `ABC-123`. Open `~/.local/share/punchcard/heartbeat.json` in an editor and move `lastSeen` back by three hours (change the hour in the timestamp, keep the format). Then:

```sh
punchcard status
punchcard pause
punchcard stop
```

Expect `status` to show `! ABC-123 stale … (capped; … uncapped)` and `last seen HH:MM (3h00m ago); the next action will recover it`. `pause` prints `! ABC-123 recovered: no presence since HH:MM, so it was capped there; fix the duration at stop`. `stop` says `ABC-123 was recovered: its time stops at the last presence. Correct the duration if needed.` and asks for the duration (if you edited the file within a minute of starting, there is no default); type `2m`. Delete the worklog.

### 11. Two writers at once

In one terminal, hold the lock for 30 seconds:

```sh
python3 -c 'import fcntl,time,os; f=open(os.path.expanduser("~/.local/share/punchcard/state.lock"),"a+"); fcntl.flock(f,fcntl.LOCK_EX); time.sleep(30)'
```

In another, within those 30 seconds:

```sh
punchcard start ABC-123; echo "exit $?"
punchcard status
```

Expect the start to wait about 2 seconds, print `state is locked by another punchcard` and `try again`, and exit 75. `status` still answers at once. After the 30 seconds, the same start works.

### 12. The TUI

Run bare `punchcard`. Expect three boxed panes: Tickets on the left, the selected ticket on the right, the day's log along the bottom (`[3] Today · logged …`), then a status line and a key bar. [The TUI](tui.md) shows every frame.

- The running row ticks once a second in `h:mm:ss`.
- `j`/`k` move. `space` on the other ticket's row: the running one pauses and this one starts; the key bar's `space` label changes to `pause`.
- `x`: the stop dialog, with the measured time prefilled and `→ sends Nm to Jira as one worklog`. Edit the duration to `3m`, `tab` to the comment, type one, `enter`. Expect the status line `stopped ABC-… · logged 3m` and a `✓ logged HH:MM` row in the day's log. Delete it in Jira later.
- `a`: type a key that does not exist and press enter: `not found — or not visible to this Jira account`, dialog stays open. `esc`. `a` again, `↓` to pick a recent one, enter: `is already on your list`.
- `d` on a row with time on it: a warning that its unsent time would be lost. Any key but `y` keeps it.
- `?` shows the help; any key closes it. `q` quits. `punchcard status` afterwards shows the timer still running.

### 13. The TUI sees changes from outside

Keep the TUI open in one terminal. In another run `punchcard pause`, then `punchcard resume`. Expect the row to show `paused by hand` and then `resumed by hand` within a second, and the status line to log `HH:MM you paused ABC-…`.

### 14. A failed send shows in the TUI and `r` fixes it

Quit the TUI. Edit `~/.config/punchcard/config.json` and change `siteUrl` to `https://nowhere.example.invalid`. Open the TUI, start a ticket, wait a minute, `x`, enter. Expect the status line `stopped ABC-… · 1m not sent: Jira not reachable · nothing is lost`, then the banner `✗ 1 worklog failed to reach Jira · r to retry · nothing is lost`, and a `✗ failed — unreachable · r` row in the day's log. Put the real `siteUrl` back (keep the TUI open), press `r`: expect `○ syncing…`, then `sent 1 worklog to Jira`, the row `✓ logged HH:MM`, the banner gone. Delete the worklog.

### 15. The Claude Code plugin

This works with or without Jira; where the two differ, both are given. Until there is a release the plugin is loaded per session from your clone. In a git checkout on a branch named for a ticket (for example `git checkout -b feature/ABC-123-punchcard`; with Jira, use your throwaway ticket):

```sh
claude --plugin-dir /path/to/your/clone/punchcard/plugin
```

Expect a band above the prompt within a few seconds. Because this is a build from a clone (a pseudo-version, not a release) it never says `plugin 0.1.0 ≠ punchcard …`; if it does, that is a bug. Because nothing is running yet it also says `ABC-123 not running · /punch start`; with another ticket running it would say `timer on <that ticket> … · here: ABC-123` instead. With no tracker the band never mentions Jira, a token or a connection. Put a `.punchcard` naming another key in the checkout (scenario 3) and the band names that key instead of the branch's within ten seconds; remove it again.

- Type `/punch start` and send it. Expect `▶ ABC-123 0:00 · held` in the band, followed by the ticket's summary with Jira (and a `+ ABC-123 added` toast if the ticket was not on your list yet).
- `/punch pause`, then `/punch resume`. The band follows.
- Ask the agent, in words: "stop my punchcard timer with the comment 'plugin test'". Expect it to show you a preview (ticket, duration, comment) **before** anything is sent, then to run `punchcard stop --yes …`, at which point Claude Code shows a permission dialog mentioning Punchcard. Approve it. Expect the agent to report the time logged and the band to go back to showing nothing (or just the version line). With Jira the worklog is in Jira; delete it.
- Do the stop again, but **decline** the dialog. Expect the timer still running (`punchcard status` in a shell) and the agent to tell you so, without trying again.
- Start Claude Code with `claude --dangerously-skip-permissions --plugin-dir …` and ask for a stop once more. Expect the dialog anyway.
- Press shift+tab until the status line says `auto mode on`, and ask for a stop once more. Expect a dialog from the plugin itself this time. With no tracker it asks `Punchcard: this command stops the timer and logs the worklog. Log it?`, with **Log it** and **Keep the timer running**; with Jira, `Punchcard: this command stops the timer and sends the worklog to Jira. Send it?`, with **Send to Jira** and **Keep the timer running**. Choose Keep: the timer is untouched and the agent says so. (If you added the ask rule in setup, Claude Code's own dialog appears instead; either way, exactly one.)

### 16. Your prompts move the timer between sessions

This needs a timer that was **not** started by hand. In the session from scenario 15, ask the agent "start the punchcard timer on this ticket". Expect the band `▶ ABC-123 0:00` without `held`. Now open a second Claude Code session (same `--plugin-dir`) in a checkout on a branch named for your second ticket, `feature/ABC-124-x`, and send any prompt, even "hi". Expect `punchcard status` in a shell to show `ABC-124` running and `ABC-123` paused, and the second session's band to show `▶ ABC-124 …`. If the TUI is open, its rows say `started by prompt` and `paused by prompt`.

Then in the first session send `/punch start` (by hand: held) and prompt again in the second. Expect `ABC-123` to stay running: a held timer never follows a prompt.

Stop whatever is running with `/punch stop` (with Jira, delete the worklogs). With no tracker the `/punch stop` dialog offers Submit and Cancel; with Jira it adds Keep pending.

### 17. Clean up

See [Resetting completely](files-and-troubleshooting.md#resetting-completely) if you want it gone, or keep it and tell me what you think.

## What to send me when something breaks

For each problem, as much of this as you can:

1. The exact command you ran, and everything it printed, including the exit code. In a shell: `punchcard …; echo "exit $?"`. For the TUI, a screenshot and the window size (`tput cols; tput lines`).
2. The output of `punchcard version` and `punchcard status --json`.
3. Your terminal app (Terminal, iTerm2, Ghostty, Warp, the VS Code terminal…), your OS version, and for plugin problems `claude --version` and the exact `claude …` command you launched with.
4. If you are willing, `~/.local/share/punchcard/state.json`.
5. What you expected instead, in one line.

**If you set up Jira, remove your Jira site URL and your email before you send anything.** They appear in `status --json` (as `notConfigured` messages), in `config.json`, in error messages that quote paths, and ticket summaries may name things you cannot share. Your API token never appears in any of these, but do not send `~/.config/punchcard/token` or anything from the Keychain.
