package setup

import "testing"

func TestNormalizeSite(t *testing.T) {
	cases := []struct {
		in, want string
		bad      bool
	}{
		{in: "acme.atlassian.net", want: "https://acme.atlassian.net"},
		{in: "  https://acme.atlassian.net/  ", want: "https://acme.atlassian.net"},
		{in: "http://ACME.atlassian.net", want: "https://acme.atlassian.net"},
		{in: "https://acme.atlassian.net/jira/software/projects/PUN/boards/3?selectedIssue=PUN-1", want: "https://acme.atlassian.net"},
		{in: "acme.atlassian.net/browse/PUN-12", want: "https://acme.atlassian.net"},
		{in: "acme", want: "https://acme.atlassian.net"},
		{in: "jira.example.com", want: "https://jira.example.com"},
		// http is kept only on localhost: a Jira stand-in (cmd/fakejira).
		{in: "http://127.0.0.1:8181", want: "http://127.0.0.1:8181"},
		{in: "http://localhost:8181/", want: "http://localhost:8181"},
		{in: "localhost:8181", want: "https://localhost:8181"},
		{in: "", bad: true},
		{in: "   ", bad: true},
		{in: "https://", bad: true},
		{in: "acme corp.atlassian.net", bad: true},
		{in: "ftp://acme.atlassian.net", bad: true},
	}
	for _, c := range cases {
		got, err := NormalizeSite(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("NormalizeSite(%q) = %q, want an error", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("NormalizeSite(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}
