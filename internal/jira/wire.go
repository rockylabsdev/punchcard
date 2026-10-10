package jira

import "github.com/j-dsmith/punchcard/internal/tracker"

// Wire types, transcribed from the curl spike's fixtures in
// docs/jira-api/responses/. Jira's word "issue" stays in this file; the
// rest of Punchcard says "ticket". Ids are strings, always.

// startedLayout is Jira's yyyy-MM-dd'T'HH:mm:ss.SSSZ: a numeric offset
// with no colon ("+0000" accepted, "+00:00" rejected).
const startedLayout = "2006-01-02T15:04:05.000-0700"

// wireIssue is GET /issue/{key}?fields=summary.
type wireIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
	} `json:"fields"`
}

// wireError is the one error envelope every failure decodes into: a
// message list plus a field-keyed map.
type wireError struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

// wireWorklogRequest is the POST /issue/{key}/worklog body.
type wireWorklogRequest struct {
	TimeSpentSeconds int     `json:"timeSpentSeconds"`
	Started          string  `json:"started"`
	Comment          *adfDoc `json:"comment,omitempty"`
}

// adfDoc is an Atlassian Document Format doc holding one paragraph of
// plain text.
type adfDoc struct {
	Type    string    `json:"type"`
	Version int       `json:"version"`
	Content []adfNode `json:"content"`
}

type adfNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text,omitempty"`
	Content []adfNode `json:"content,omitempty"`
}

func newWireWorklogRequest(w tracker.Worklog) wireWorklogRequest {
	r := wireWorklogRequest{TimeSpentSeconds: w.TimeSpentSeconds, Started: w.Started.Format(startedLayout)}
	if w.Comment != "" {
		r.Comment = &adfDoc{Type: "doc", Version: 1, Content: []adfNode{
			{Type: "paragraph", Content: []adfNode{{Type: "text", Text: w.Comment}}},
		}}
	}
	return r
}

// wireWorklog is the 201 response to the worklog POST (fields we use).
// Started is echoed in the account's timezone: compare instants.
type wireWorklog struct {
	ID               string `json:"id"`
	IssueID          string `json:"issueId"`
	Started          string `json:"started"`
	TimeSpentSeconds int    `json:"timeSpentSeconds"`
}

// wireServerInfo is GET /serverInfo (anonymous reachability).
type wireServerInfo struct {
	BaseURL        string `json:"baseUrl"`
	DeploymentType string `json:"deploymentType"`
}

// wireMyself is GET /myself (identity).
type wireMyself struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}
