# Punchcard user guide

Punchcard times work against Jira tickets from the terminal and sends the time to Jira as worklogs. One binary, `punchcard`, is both a full-screen TUI (run it bare) and a CLI (`punchcard start ABC-123`). An optional Claude Code plugin keeps the timer on the ticket named in your git branch while you work with an agent.

Every example in these pages was run against the code on this branch, in throwaway directories, with a fake Jira where Jira was needed. Where something could not be run, the page says so.

| Page | Read it when |
| --- | --- |
| [Getting started](getting-started.md) | You want to install it, connect it to Jira and log your first worklog. |
| [Concepts](concepts.md) | You want to know what "held", "recovered" or "unverified" means, in one scenario each. |
| [The TUI](tui.md) | You want every key, symbol and colour of the full-screen app. |
| [The CLI](cli.md) | You want every command, flag, exit code and the `--json` envelope. |
| [Claude Code](claude-code.md) | You want the plugin: the band above the prompt, `/punch`, and why an agent's stop always asks you. |
| [Files and troubleshooting](files-and-troubleshooting.md) | Something is wrong, or you want to see what is on disk. |
| [Testers' guide](testers.md) | A friend asked you to try it and report what broke. |
| [Testing without a real Jira](sandbox.md) | You want to try every surface against a fake Jira on localhost, with no account or token. |

Install, upgrade, uninstall and releasing are in the top-level [README](../../README.md). The design and the reasons behind it are in [docs/spec.md](../spec.md) and the decision records under [docs/adr/](../adr/). The vocabulary is in [CONTEXT.md](../../CONTEXT.md).
