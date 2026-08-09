# 05 — Survive quit and crash

**What to build:** The list and its accumulated time survive quitting and relaunching. If the app dies while a timer is running, the next launch caps that stretch at the last moment the app was known alive, leaves it paused rather than silently running, and flags the row so the user notices it needs checking — even if they relaunch hours later.

**Blocked by:** 04 — Track multiple tickets.

**Status:** ready-for-agent

Build slice 5 of the [spec](../spec.md). The full contract is [wayfinder ticket 04](../wayfinder/issues/04-on-disk-persistence-contract.md); the schema is reproduced in the [spec](../spec.md) and encodes the decisions more precisely than prose.

- [ ] **Two files** in the sandbox container's Application Support directory: a state file, and a separate tiny heartbeat file holding only a schema version and `lastSeen`
- [ ] Config lives in neither — these files carry only domain data, no secrets, no settings
- [ ] `schemaVersion` present from the first write
- [ ] Entries **nest under their ticket**, making an orphan unrepresentable
- [ ] Dates are ISO-8601 strings using the **same formatter built for Jira** in ticket 02
- [ ] Nil optionals are **omitted**, not written as `null` — an open segment simply has no end key, and that absence is what recovery keys off
- [ ] Write on **every state change**, using an **atomic replace** — no debouncing, and never relying on a quit hook
- [ ] A **failed write must not stop a running timer**: in-memory state stays the truth, a persistent banner shows the risk, the next change retries
- [ ] Heartbeat written every **30 seconds, only while a timer is running**
- [ ] **Recovery:** on launch, cap the open segment at `lastSeen` and mark the entry recovered; it is then paused *by derivation*, with no separate state change
- [ ] **Recovery clamp:** if `lastSeen` predates the segment's start (clock change), clamp to the start rather than persisting a negative duration
- [ ] The recovered flag is a **persistent marker on the row**, cleared when the user resumes or stops it — *not* an alert at launch
- [ ] Missing or zero-byte file → first run, empty state, no error
- [ ] Undecodable file → renamed to a timestamped backup, fresh start, non-blocking notice **naming the backup path**
- [ ] Unknown newer `schemaVersion` → refuse to load, blocking notice, nothing written
- [ ] Nothing is pruned
- [ ] **Test:** recovery capping as a pure function — `lastSeen` inside the open segment; `lastSeen` before its start (the clamp); no open segment at all
- [ ] **Test:** state-file round-trip
- [ ] **Manual check:** quit mid-run, relaunch, see the entry capped and flagged as paused
