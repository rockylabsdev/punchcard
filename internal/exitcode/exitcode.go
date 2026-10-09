// Package exitcode is the one table of punchcard's exit codes (ADR 0004),
// shared by the CLI and the setup wizard. Exit 2 is never used: it
// blocks a Claude Code hook.
package exitcode

const (
	OK                = 0
	Internal          = 1
	NotConfigured     = 3
	NoTimer           = 4
	TicketNotFound    = 5
	NeedsConfirmation = 6
	SyncPending       = 7
	AuthFailed        = 8
	Usage             = 64
	StateUnreadable   = 65 // also a config written by a newer punchcard
	Busy              = 75
)
