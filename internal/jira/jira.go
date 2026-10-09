// Package jira is Punchcard's only door to Jira Cloud (/rest/api/3/,
// Basic auth with email plus a classic API token). Callers depend on the
// three-call Client interface; tests use Fake, and the real HTTP client
// is New. Jira's word "issue" is confined to wire.go.
package jira

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SummaryBudget is how long a summary fetch may take before the caller
// gives up and leaves the ticket unverified.
const SummaryBudget = 2 * time.Second

// Client is the whole Jira surface Punchcard uses.
type Client interface {
	// Summary fetches a ticket's summary. A 404 is ErrNotFound: the
	// ticket is missing or this account cannot see it.
	Summary(ctx context.Context, key string) (string, error)
	// PostWorklog sends one worklog to key and returns Jira's worklog id.
	PostWorklog(ctx context.Context, key string, w Worklog) (string, error)
	// TestConnection checks the site (serverInfo, must be Cloud), then
	// the credentials (myself), and names the account.
	TestConnection(ctx context.Context) (Account, error)
}

// Worklog is what PostWorklog tells Jira. Started is sent with a numeric
// offset; an empty Comment is omitted.
type Worklog struct {
	Started          time.Time
	TimeSpentSeconds int
	Comment          string
}

// Account is who the credentials authenticate as.
type Account struct {
	AccountID   string
	DisplayName string
	Email       string
}

// Outcomes every caller distinguishes. Match with errors.Is.
var (
	// ErrNotFound: a 404, ambiguous between missing and invisible.
	ErrNotFound = errors.New("not found or not visible to this account")
	// ErrUnauthorized: a 401 (bad email or token, or a scoped token).
	ErrUnauthorized = errors.New("credentials rejected by Jira")
	// ErrUnreachable: no HTTP answer (network, DNS, timeout).
	ErrUnreachable = errors.New("could not reach Jira")
	// ErrBadSite: TestConnection's serverInfo step failed or the site is
	// not Jira Cloud. It wraps the underlying cause too.
	ErrBadSite = errors.New("not a reachable Jira Cloud site")
)

// APIError is any non-2xx answer, decoded from Jira's one error
// envelope. A 404 matches ErrNotFound and a 401 ErrUnauthorized.
type APIError struct {
	Status   int
	Messages []string
	Fields   map[string]string
}

func (e *APIError) Error() string {
	parts := append([]string{}, e.Messages...)
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Jira answered %d", e.Status)
	}
	return fmt.Sprintf("Jira answered %d: %s", e.Status, strings.Join(parts, "; "))
}

func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == 404
	case ErrUnauthorized:
		return e.Status == 401
	}
	return false
}
