package jira

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a Client with canned outcomes, for every test above the HTTP
// client. The zero value knows no tickets (every Summary is a 404) and
// accepts every worklog.
type Fake struct {
	// Summaries are the tickets Jira knows, key → summary. Any other key
	// is a 404 (ErrNotFound).
	Summaries map[string]string
	// Err, when set, is what every call returns instead: ErrUnreachable,
	// ErrUnauthorized, an *APIError (e.g. a 503), ...
	Err error
	// Account is what TestConnection names.
	Account Account

	mu       sync.Mutex
	calls    []string
	worklogs []PostedWorklog
}

// PostedWorklog is one worklog the Fake accepted.
type PostedWorklog struct {
	Key     string
	ID      string
	Worklog Worklog
}

// Calls lists every call made, in order: "summary KEY", "worklog KEY",
// "test".
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// Worklogs lists the worklogs accepted so far. Ids are "W1", "W2", ...
func (f *Fake) Worklogs() []PostedWorklog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PostedWorklog(nil), f.worklogs...)
}

func (f *Fake) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.Err
}

func (f *Fake) Summary(ctx context.Context, key string) (string, error) {
	if err := f.record("summary " + key); err != nil {
		return "", err
	}
	s, ok := f.Summaries[key]
	if !ok {
		return "", &APIError{Status: 404, Messages: []string{"Issue does not exist or you do not have permission to see it."}}
	}
	return s, nil
}

func (f *Fake) PostWorklog(ctx context.Context, key string, w Worklog) (string, error) {
	if err := f.record("worklog " + key); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("W%d", len(f.worklogs)+1)
	f.worklogs = append(f.worklogs, PostedWorklog{Key: key, ID: id, Worklog: w})
	return id, nil
}

func (f *Fake) TestConnection(ctx context.Context) (Account, error) {
	if err := f.record("test"); err != nil {
		return Account{}, err
	}
	return f.Account, nil
}
