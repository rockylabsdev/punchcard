package jira

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// fixture reads one committed response from the curl spike.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "jira-api", "responses", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decode[T any](t *testing.T, name string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(fixture(t, name), &v); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return v
}

func TestDecodeIssueSummary(t *testing.T) {
	got := decode[wireIssue](t, "issue-summary.json")
	want := wireIssue{ID: "100001", Key: "EX-1234"}
	want.Fields.Summary = "Example ticket summary for fixture purposes"
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("(-want +got)\n%s", d)
	}
}

func TestDecodeErrorEnvelopes(t *testing.T) {
	cases := []struct {
		file string
		want wireError
	}{
		{"issue-404.json", wireError{
			ErrorMessages: []string{"Issue does not exist or you do not have permission to see it."},
			Errors:        map[string]string{},
		}},
		{"worklog-post-badstarted.json", wireError{
			ErrorMessages: []string{},
			Errors:        map[string]string{"started": `Invalid date format. Please enter the date in the format "yyyy-MM-dd'T'HH:mm:ss.SSSZ".`},
		}},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			if d := cmp.Diff(c.want, decode[wireError](t, c.file)); d != "" {
				t.Errorf("(-want +got)\n%s", d)
			}
		})
	}
}

func TestDecodeWorklogPost(t *testing.T) {
	got := decode[wireWorklog](t, "worklog-post.json")
	want := wireWorklog{ID: "110001", IssueID: "100001", Started: "2026-08-08T14:28:01.000-0500", TimeSpentSeconds: 300}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("(-want +got)\n%s", d)
	}
	// Jira echoes started in the account's zone: compare instants.
	at, err := time.Parse(startedLayout, got.Started)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 8, 8, 19, 28, 1, 0, time.UTC); !at.Equal(want) {
		t.Errorf("started = %v, want %v", at, want)
	}
}

func TestDecodeConnection(t *testing.T) {
	si := decode[wireServerInfo](t, "serverinfo.json")
	if d := cmp.Diff(wireServerInfo{BaseURL: "https://example.atlassian.net", DeploymentType: "Cloud"}, si); d != "" {
		t.Errorf("serverInfo (-want +got)\n%s", d)
	}
	me := decode[wireMyself](t, "myself.json")
	want := wireMyself{AccountID: "712020:11111111-2222-3333-4444-555555555555", EmailAddress: "user@example.com", DisplayName: "Example User"}
	if d := cmp.Diff(want, me); d != "" {
		t.Errorf("myself (-want +got)\n%s", d)
	}
}

// The request body matches the committed request fixture shape for
// shape; the bad-started fixture's colon offset is what Jira rejects.
func TestEncodeWorklogRequest(t *testing.T) {
	started := time.Date(2026, 8, 8, 19, 28, 1, 0, time.UTC)
	got, err := json.Marshal(newWireWorklogRequest(Worklog{Started: started, TimeSpentSeconds: 300, Comment: "Punchcard slice-0 spike — delete me"}))
	if err != nil {
		t.Fatal(err)
	}
	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture(t, "request-worklog.json"), &wantV); err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(wantV, gotV); d != "" {
		t.Errorf("(-fixture +encoded)\n%s", d)
	}

	bad := decode[wireWorklogRequest](t, "request-worklog-badstarted.json")
	if _, err := time.Parse(startedLayout, bad.Started); err == nil {
		t.Errorf("startedLayout accepts the rejected %q", bad.Started)
	}
}

func TestEncodeWorklogRequestOmitsEmptyComment(t *testing.T) {
	got, err := json.Marshal(newWireWorklogRequest(Worklog{Started: time.Date(2026, 8, 8, 19, 28, 1, 0, time.UTC), TimeSpentSeconds: 60}))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"timeSpentSeconds":60,"started":"2026-08-08T19:28:01.000+0000"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
