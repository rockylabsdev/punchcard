package worklog

import (
	"errors"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/store"
)

// Problem is why a stopped worklog was not sent: the one classification
// of StopResult.SyncErr that the CLI (as error.code) and the TUI (in its
// own words) both report. The strings are the CLI contract's codes.
type Problem string

const (
	ProblemNotConfigured Problem = "not_configured" // no config, token or Keychain
	ProblemAuthFailed    Problem = "auth_failed"    // a 401
	ProblemUnreachable   Problem = "unreachable"    // no HTTP answer
	ProblemRejected      Problem = "sync_failed"    // Jira answered with an error
	ProblemBusy          Problem = "sync_busy"      // another punchcard is sending
	ProblemPending       Problem = "sync_pending"   // anything else (recording the outcome failed)
)

// Why classifies a SyncErr.
func Why(err error) Problem {
	var api *jira.APIError
	switch {
	case errors.Is(err, config.ErrNotConfigured):
		return ProblemNotConfigured
	case errors.Is(err, jira.ErrUnauthorized):
		return ProblemAuthFailed
	case errors.Is(err, jira.ErrUnreachable):
		return ProblemUnreachable
	case errors.As(err, &api):
		return ProblemRejected
	case errors.Is(err, store.ErrSyncBusy):
		return ProblemBusy
	default:
		return ProblemPending
	}
}
