package tracker_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// LookUp is the one mapping of a summary fetch to what the CLI and the
// TUI act on.
func TestLookUpSummary(t *testing.T) {
	opened := func(c tracker.Client) func() (tracker.Client, error) {
		return func() (tracker.Client, error) { return c, nil }
	}
	failing := func(err error) func() (tracker.Client, error) {
		return func() (tracker.Client, error) { return nil, err }
	}
	cases := []struct {
		name string
		open func() (tracker.Client, error)
		want tracker.Lookup
	}{
		{"found", opened(&tracker.Fake{Summaries: map[string]string{"AA-1": "Fix it"}}), tracker.Lookup{Outcome: tracker.LookupFound, Summary: "Fix it"}},
		{"a 404", opened(&tracker.Fake{}), tracker.Lookup{Outcome: tracker.LookupNotFound}},
		{"a 401", opened(&tracker.Fake{Err: tracker.ErrUnauthorized}), tracker.Lookup{Outcome: tracker.LookupAuthFailed}},
		{"unreachable", opened(&tracker.Fake{Err: tracker.ErrUnreachable}), tracker.Lookup{Outcome: tracker.LookupUnknown}},
		{"a 503", opened(&tracker.Fake{Err: &tracker.Refusal{Status: 503}}), tracker.Lookup{Outcome: tracker.LookupUnknown}},
		{"not configured", failing(fmt.Errorf("%w: no config.json", tracker.ErrNotConfigured)), tracker.Lookup{Outcome: tracker.LookupUnknown}},
		{"a broken config is a notice, then not configured", failing(errors.New("config.json: bad json")),
			tracker.Lookup{Outcome: tracker.LookupUnknown, Notice: "config.json: bad json; treating the tracker as not configured"}},
		{"no tracker configured", failing(tracker.ErrNoTracker), tracker.Lookup{Outcome: tracker.LookupUnknown}},
		{"no Jira at all", nil, tracker.Lookup{Outcome: tracker.LookupUnknown}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tracker.LookUp(c.open, "AA-1")
			if d := cmp.Diff(c.want, got); d != "" {
				t.Errorf("LookUp (-want +got):\n%s", d)
			}
		})
	}
}
