# Punchcard user guide

Punchcard times work against tickets from the terminal and logs each stretch of work as a worklog on your machine. A ticket is any key you choose (`payroll`, `ABC-123`), and `punchcard log` reports a week or a month of worklogs as Markdown, CSV or JSON. One binary, `punchcard`, is both a full-screen TUI (run it bare) and a CLI (`punchcard start payroll`). A tracker is optional: with Jira set up, each worklog is also sent to Jira. An optional Claude Code plugin keeps the timer on the ticket your session is for (a `.punchcard` file, or the key in your git branch) while you work with an agent.

Every example in these pages was run against the code on this branch, in throwaway directories, with a fake Jira where Jira was needed. Where something could not be run, the page says so.

| Page | Read it when |
| --- | --- |
| [Getting started](getting-started.md) | You want to install it, time a ticket, read back your week, and optionally connect Jira. |
| [Concepts](concepts.md) | You want to know what "by hand", "recovered", "local" or "unverified" means, in one scenario each. |
| [The TUI](tui.md) | You want every key, symbol and colour of the full-screen app. |
| [The CLI](cli.md) | You want every command, flag, exit code and the `--json` envelope, including [`log`](cli.md#log). |
| [Claude Code](claude-code.md) | You want the plugin: [`.punchcard` files](claude-code.md#punchcard-files), the band above the prompt, `/punch`, and why an agent's stop always asks you. |
| [Files and troubleshooting](files-and-troubleshooting.md) | Something is wrong, or you want to see what is on disk. |
| [Testers' guide](testers.md) | A friend asked you to try it and report what broke. |
| [Testing without a real Jira](sandbox.md) | You want to try the Jira tracker against a fake Jira on localhost, with no account or token. |

Install, upgrade, uninstall and releasing are in the top-level [README](../../README.md). The design and the reasons behind it are in [docs/spec.md](../spec.md), [docs/spec-local-first.md](../spec-local-first.md) and the decision records under [docs/adr/](../adr/). The vocabulary is in [CONTEXT.md](../../CONTEXT.md).
