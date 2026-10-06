# Concurrent writers: one `flock` on a side lock file, and nobody writes from a cached copy

No daemon means several writers to `state.json` and `heartbeat.json`: the TUI, every CLI call, and the Claude Code mod through the CLI. They are serialised by an exclusive advisory `flock` on a separate `state.lock`, not on `state.json` itself, because the atomic temp-then-rename write gives `state.json` a new inode on every save and a lock on the old one protects nothing. The kernel releases `flock` when its holder dies, so a crash can never leave a stale lock.

Every change is one critical section: lock, read both files fresh, back up a corrupt file or refuse a newer version, recover, apply, write, unlock. **No writer applies a change to a copy it read earlier — the TUI included.** A TUI keystroke asks for a transition; it does not save the TUI's model. That makes the read-modify-write race impossible by construction, so `state.json` carries no version counter or mtime check. The lock is never held across a network call; syncing to Jira takes a second lock, `sync.lock`, without waiting and holds it across the POST, so two writers can never both POST the same pending worklog.

## Considered Options

- **`flock` on `state.json`.** Rejected: defeated by the rename.
- **mkdir lock with mtime staleness (`proper-lockfile`).** Rejected: a crashed holder blocks every writer until the staleness timeout, and it is cooperative guesswork where the kernel offers a guarantee.
- **No lock, optimistic version counter with retry.** Rejected: more code on every writer, and a retry loop where a short wait is enough.
- **A stored `syncing` worklog state** instead of `sync.lock`. Rejected: a crash would strand it on disk.

## Consequences

- **The stack must reach `flock`.** Go and Rust have it; Node core does not, so Ink needs a native addon.
- **v1's "in-memory state stays the truth when a write fails" is retired.** With several writers a divergent in-memory model is the stale-write bug. A change that cannot lock or write simply does not happen and says so (CLI exit 75 `EX_TEMPFAIL` after a ~2 s wait, or exit 1; a TUI banner). A running timer is unaffected: it is already on disk as an open segment.
- **Presence writes never wait.** They are rate-limited to one per 30 s against the `lastSeen` on disk and try the lock without blocking, skipping if it is held. That is safe because the holder is either a transition, which recovers and records presence itself, or another presence write.
- **The TUI learns of outside changes by polling, not watching.** On its existing 1 s render tick it `stat`s `state.json` (inode + mtime), re-reads on change, and diffs old against new to know which rows changed. No watcher library, no platform differences.
