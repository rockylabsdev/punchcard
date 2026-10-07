# PROTOTYPE — TUI layout and interaction (throwaway)

Answers [TUI layout and interaction](https://github.com/j-dsmith/punchcard/issues/13).
In-memory fake state, no Jira, no `state.json`. Lives only on `prototype/tui-layout`.

```sh
cd prototype/tui-layout && go run .            # -variant B to start elsewhere
go run . -dump 80x24                           # print every variant/scenario frame, no colour
go run . -smoke                                # headless key-sequence run over all variants
```

Three layouts of the same app, switched live:

| | Variant | Shape |
|---|---|---|
| A | Panes | lazygit: ticket list on the left; selected ticket's entries over the worklog queue on the right; stop/add take over the right column |
| B | Ledger | one dense table under a reverse-video "on the clock" strip; the selected row expands inline; stop/add are centred modals |
| C | Clock | the running entry in block digits owns the top; compact list plus "owed to Jira" tray below; stop/add are a drawer and the clock collapses to a line |

Prototype controls (the magenta bar, not part of the design):
`F2`/`ctrl+n` variant · `F3`/`ctrl+o` colour (auto, 256, 16, none, truecolor) ·
`F4`/`ctrl+e` scenario (normal, recovered, auth failed, not configured, empty) ·
`F5`/`ctrl+g` a change from elsewhere (agent start → prompt focus → CLI stop) ·
`F6`/`ctrl+f` clamp to 80×24 · `F1`/`ctrl+b` hide the bar.

Design keys under test: `space`/`enter` start·pause·resume the selected row, `p` pause whatever runs,
`x` stop (opens the dialog), `a` add (try `XXX-1`, `OFF-1`), `d` remove, `r` sync, `S` setup (suspends the TUI), `?` help, `q` quit.
