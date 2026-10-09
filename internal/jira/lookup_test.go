package jira_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// LookUp is the one mapping of a summary fetch to what the CLI and the
// TUI act on.
func TestLookUpSummary(t *testing.T) {
	opened := func(c jira.Client) func() (jira.Client, error) {
		return func() (jira.Client, error) { return c, nil }
	}
	failing := func(err error) func() (jira.Client, error) {
		return func() (jira.Client, error) { return nil, err }
	}
	cases := []struct {
		name string
		open func() (jira.Client, error)
		want jira.Lookup
	}{
		{"found", opened(&jira.Fake{Summaries: map[string]string{"AA-1": "Fix it"}}), jira.Lookup{Outcome: jira.LookupFound, Summary: "Fix it"}},
		{"a 404", opened(&jira.Fake{}), jira.Lookup{Outcome: jira.LookupNotFound}},
		{"a 401", opened(&jira.Fake{Err: jira.ErrUnauthorized}), jira.Lookup{Outcome: jira.LookupAuthFailed}},
		{"unreachable", opened(&jira.Fake{Err: jira.ErrUnreachable}), jira.Lookup{Outcome: jira.LookupUnknown}},
		{"a 503", opened(&jira.Fake{Err: &jira.APIError{Status: 503}}), jira.Lookup{Outcome: jira.LookupUnknown}},
		{"not configured", failing(fmt.Errorf("%w: no config.json", config.ErrNotConfigured)), jira.Lookup{Outcome: jira.LookupUnknown}},
		{"a broken config is a notice, then not configured", failing(errors.New("config.json: bad json")),
			jira.Lookup{Outcome: jira.LookupUnknown, Notice: "config.json: bad json; treating Jira as not configured"}},
		{"no Jira at all", nil, jira.Lookup{Outcome: jira.LookupUnknown}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := jira.LookUp(c.open, "AA-1")
			if d := cmp.Diff(c.want, got); d != "" {
				t.Errorf("LookUp (-want +got):\n%s", d)
			}
		})
	}
}
