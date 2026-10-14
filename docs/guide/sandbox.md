# Testing without a real Jira

Punchcard needs no tracker: with none, every surface works and worklogs stay on your machine. This page is for trying the optional Jira tracker without a real one. `fakejira` is a stand-in Jira Cloud on localhost. It answers the four calls Punchcard makes, knows six tickets, prints every request it gets, and can be switched into failure modes while it runs. With it you can try every surface with Jira as the tracker, setup, the CLI, the TUI and the Claude Code plugin, with no account and no token, and nothing you do leaves your machine.

## Start it

It comes in the release tarball next to `punchcard` (see the [testers' guide](testers.md#install)):

```sh
fakejira
```

From a clone of the repository, `go run ./cmd/fakejira` does the same.

```
fakejira on http://127.0.0.1:8181 (mode ok)

  punchcard setup   site: http://127.0.0.1:8181   email: anyone@example.com   token: anything
  tickets: DATA-9, OPS-77, PUN-1, PUN-2, PUN-3, WEB-412 (any other key is a 404)
  modes:   curl 'http://127.0.0.1:8181/_fake/mode?set=401'   (ok, 401, 503, slow)
  posted:  curl http://127.0.0.1:8181/_fake/worklogs
```

Leave it running in its own terminal. Every request shows there as it happens, which is the quickest way to see what reached "Jira":

```
11:43:21 200 GET /rest/api/3/issue/PUN-2
11:43:21 404 GET /rest/api/3/issue/NOPE-1
11:43:21   worklog 110001 on PUN-2: 1500s "sandbox check"
11:43:21 201 POST /rest/api/3/issue/PUN-2/worklog
```

`-addr` changes the address; `-mode` sets the starting mode.

## Keep it away from your real setup

Punchcard reads config and data from `~/.config/punchcard` and `~/.local/share/punchcard`, and `setup` writes the token into the macOS Keychain. For a sandbox, point all of that elsewhere in the shell you test from:

```sh
export XDG_CONFIG_HOME=~/punchcard-sandbox/config
export XDG_DATA_HOME=~/punchcard-sandbox/data
export PUNCHCARD_TOKEN=sandbox
export DISABLE_AUTOUPDATER=1
```

`DISABLE_AUTOUPDATER` is for Claude Code, not Punchcard. Claude Code installs its own updates under `XDG_DATA_HOME` and points `~/.local/bin/claude` at the new copy. Without the variable, a `claude` started from this shell can update itself into the sandbox, and deleting the sandbox then leaves `claude` pointing at nothing. If that has already happened, point it back at your normal copy: `ln -sfn ~/.local/share/claude/versions/<version> ~/.local/bin/claude`.

With `PUNCHCARD_TOKEN` set, `setup` lets you leave the token field blank, tests with the environment's token, and writes nothing to the Keychain: it says `No token saved: PUNCHCARD_TOKEN supplies it`. Delete the two directories when you are done and nothing remains. First check that `readlink ~/.local/bin/claude` does not point into them.

Anything that must see the sandbox, the TUI, a second terminal, Claude Code, has to be started from a shell with those variables exported.

## Set it up

```sh
punchcard setup
```

At the first question, Tracker, choose `Jira: stopped time becomes Jira worklogs`. Then site `http://127.0.0.1:8181`, any email, token left blank. Test Connection passes with `✓ Connected as Sandbox Tester`. Say No to the plugin install (see below) and yes or no to the ask rule as you like; the rule goes into your real Claude Code settings unless `CLAUDE_CONFIG_DIR` is set.

Then the Jira parts of [Getting started](getting-started.md) and the [testers' guide](testers.md) work against the fake: `punchcard start PUN-2` fetches the real-looking summary, a typo exits 5, `stop` posts a worklog, and bare `punchcard` opens the TUI.

## Break it on purpose

Switch modes while the TUI or a session is open and watch the surfaces react:

| Mode | What happens | What to look for |
| --- | --- | --- |
| `ok` | Everything succeeds. | `✓ logged` rows, `sent to Jira`. |
| `401` | Every call is rejected as unauthenticated. | `stop` exits 7 with `auth_failed`; `status` shows `! Jira rejected the token`; the TUI banner `✗ Jira rejected the token at …`; the band `Jira auth failed`; `sync` exits 8. Switch back to `ok` and `sync`: the marker clears. |
| `503` | Worklog POSTs fail; reads still work. | `stop` exits 7 with `worklog failed`; `✗ failed — 503 · r` in the TUI's day's log; `r` or `sync` after `ok` sends it. |
| `slow` | Every call takes 20 seconds, longer than any budget. | A start adds the ticket unverified; `stop` says `could not reach the tracker` after 10 s; the TUI shows `✗ failed — unreachable`. |

```sh
curl 'http://127.0.0.1:8181/_fake/mode?set=503'
curl 'http://127.0.0.1:8181/_fake/mode?set=ok'
```

For "Jira is down" rather than slow, stop `fakejira` with ctrl-c: connections are refused at once.

Worklogs the fake accepted, and a way to clear them:

```sh
curl http://127.0.0.1:8181/_fake/worklogs
curl -X DELETE http://127.0.0.1:8181/_fake/worklogs
```

## The Claude Code plugin against it

Start Claude Code from the sandbox shell, so it inherits the three variables, in a checkout whose branch names one of the fake's tickets:

```sh
git checkout -b feature/PUN-3-sandbox
claude --plugin-dir /path/to/punchcard/plugin
```

The band, `/punch`, the agent's gated stop and focus between sessions all work as described in [Claude Code](claude-code.md); the worklogs land in `fakejira`'s log instead of a real ticket. Note that the plugin's permission dialog and the ask rule are Claude Code's, so the sandbox changes nothing about them.

## Limits

The fake is six fixed tickets and four modes. It does not check the token, paginate, rate-limit, or answer any call Punchcard does not make. A pass against it says the surfaces behave; the [testers' guide](testers.md) still ends with a real ticket for the one thing it cannot show, that a real Jira accepts the worklog body.
