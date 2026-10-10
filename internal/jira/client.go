package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// Open is the real client from the config file and the token lookup
// (PUNCHCARD_TOKEN, then kc, then the token file; kc is
// config.SystemKeychain() outside tests, nil on Linux). It fails with
// config.ErrNotConfigured, config.ErrNewerSchema, a *config.KeychainError
// or a malformed-file error before any network call. Only Jira-calling
// commands call it; reads and the presence path never do.
func Open(getenv func(string) string, kc config.Keychain) (tracker.Client, error) {
	cfg, tok, err := config.Credentials(getenv, kc)
	if err != nil {
		return nil, err
	}
	return New(cfg.SiteURL, cfg.Email, tok.Value), nil
}

// httpClient is the real client: one site, one email and token.
type httpClient struct {
	base       string // https://<host>/rest/api/3
	email, tok string
	http       *http.Client
}

// New is the real client for siteURL (e.g. https://example.atlassian.net,
// a trailing slash is fine) with Basic auth. Callers bound each call with
// the context; the transport also gives up after 30 s.
func New(siteURL, email, token string) tracker.Client {
	return &httpClient{
		base:  strings.TrimRight(siteURL, "/") + "/rest/api/3",
		email: email,
		tok:   token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *httpClient) Summary(ctx context.Context, key string) (string, error) {
	var v wireIssue
	if err := c.do(ctx, "GET", "/issue/"+url.PathEscape(key)+"?fields=summary", nil, &v); err != nil {
		return "", err
	}
	return v.Fields.Summary, nil
}

func (c *httpClient) PostWorklog(ctx context.Context, key string, w tracker.Worklog) (string, error) {
	var v wireWorklog
	if err := c.do(ctx, "POST", "/issue/"+url.PathEscape(key)+"/worklog?adjustEstimate=leave", newWireWorklogRequest(w), &v); err != nil {
		return "", err
	}
	return v.ID, nil
}

func (c *httpClient) TestConnection(ctx context.Context) (tracker.Account, error) {
	var si wireServerInfo
	if err := c.do(ctx, "GET", "/serverInfo", nil, &si); err != nil {
		return tracker.Account{}, fmt.Errorf("%w: %w", tracker.ErrBadSite, err)
	}
	if si.DeploymentType != "Cloud" {
		return tracker.Account{}, fmt.Errorf("%w: deployment type %q", tracker.ErrBadSite, si.DeploymentType)
	}
	var me wireMyself
	if err := c.do(ctx, "GET", "/myself", nil, &me); err != nil {
		return tracker.Account{}, err
	}
	return tracker.Account{AccountID: me.AccountID, DisplayName: me.DisplayName, Email: me.EmailAddress}, nil
}

// do sends one request and decodes a 2xx body into out. A transport
// failure is tracker.ErrUnreachable; any other status is an *APIError decoded
// from the error envelope (an undecodable body leaves it empty).
func (c *httpClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("%w: %v", tracker.ErrBadSite, err)
	}
	req.SetBasicAuth(c.email, c.tok)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", tracker.ErrUnreachable, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", tracker.ErrUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e wireError
		_ = json.Unmarshal(b, &e)
		return &APIError{Status: resp.StatusCode, Messages: e.ErrorMessages, Fields: e.Errors}
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}
