// Package core is Punchcard's domain: the persisted types and one pure
// transition function, Apply, plus derived views over the same values.
//
// Nothing here touches a file, the network or the wall clock. The caller
// (the store) reads state and heartbeat fresh under the lock, passes them
// with the current time to Apply, and writes back what it returns.
//
// Vocabulary follows CONTEXT.md: TrackedTicket, TimeEntry, Segment,
// Worklog, presence, focus, origin, lastSeen, stale.
package core
