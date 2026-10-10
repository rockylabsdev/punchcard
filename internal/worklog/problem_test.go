package worklog_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/j-dsmith/punchcard/internal/worklog"
)

// Why classifies a StopResult.SyncErr once, for the CLI's error.code and
// the TUI's words alike. The strings are the CLI contract's codes.
func TestWhyAWorklogWasNotSent(t *testing.T) {
	cases := []struct {
		err  error
		want worklog.Problem
		code string
	}{
		{fmt.Errorf("%w: no config.json", config.ErrNotConfigured), worklog.ProblemNotConfigured, "not_configured"},
		{fmt.Errorf("%w: %w", config.ErrNotConfigured, &config.KeychainError{Err: errors.New("locked")}), worklog.ProblemNotConfigured, "not_configured"},
		{tracker.ErrUnauthorized, worklog.ProblemAuthFailed, "auth_failed"},
		{&tracker.Refusal{Status: 401}, worklog.ProblemAuthFailed, "auth_failed"},
		{fmt.Errorf("%w: dial tcp", tracker.ErrUnreachable), worklog.ProblemUnreachable, "unreachable"},
		{&tracker.Refusal{Status: 503}, worklog.ProblemRejected, "sync_failed"},
		{store.ErrSyncBusy, worklog.ProblemBusy, "sync_busy"},
		{errors.New("disk full"), worklog.ProblemPending, "sync_pending"},
	}
	for _, c := range cases {
		t.Run(c.err.Error(), func(t *testing.T) {
			got := worklog.Why(c.err)
			if got != c.want || string(got) != c.code {
				t.Errorf("Why = %q, want %q (%s)", got, c.want, c.code)
			}
		})
	}
}
