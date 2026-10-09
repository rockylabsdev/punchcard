// Package jira is the Jira Cloud tracker (/rest/api/3/, Basic auth with
// email plus a classic API token): one implementation of
// tracker.Client, and Jira's key grammar. Callers depend on the tracker
// package; only the code that constructs a Jira client imports this one.
// Jira's word "issue" is confined to wire.go.
package jira

import (
	"strings"

	"github.com/j-dsmith/punchcard/internal/tracker"
)

// Grammar is Jira's key: a project key (a letter, then letters or
// digits) and a number, ABC-123, upper-cased on entry.
var Grammar = tracker.NewGrammar(`^[A-Z][A-Z0-9]+-[0-9]+$`, true, "PUN-123")

// APIError is any non-2xx answer, decoded from Jira's one error
// envelope. It is a *tracker.Refusal to every caller (errors.As), and a
// 404 matches tracker.ErrNotFound and a 401 tracker.ErrUnauthorized.
type APIError struct {
	Status   int
	Messages []string
	Fields   map[string]string
}

// refusal is e in the tracker's terms.
func (e *APIError) refusal() *tracker.Refusal {
	parts := append([]string{}, e.Messages...)
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return &tracker.Refusal{Status: e.Status, Message: strings.Join(parts, "; ")}
}

func (e *APIError) Error() string { return e.refusal().Error() }

func (e *APIError) Is(target error) bool { return e.refusal().Is(target) }

// As makes an *APIError a *tracker.Refusal for callers.
func (e *APIError) As(target any) bool {
	if r, ok := target.(**tracker.Refusal); ok {
		*r = e.refusal()
		return true
	}
	return false
}
