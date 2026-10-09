package setup

import (
	"errors"
	"net/url"
	"strings"
)

// NormalizeSite turns whatever was pasted (a bare host, a full URL, a
// board URL from the address bar, a bare Atlassian site name) into
// https://<host>. A name with no dot is taken as <name>.atlassian.net.
func NormalizeSite(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("enter your Jira site, e.g. acme.atlassian.net")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("that is not a URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("the site must be an https:// address")
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", errors.New("enter your Jira site, e.g. acme.atlassian.net")
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == ':') {
			return "", errors.New("the site's host name has characters a host name cannot have")
		}
	}
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		host += ".atlassian.net"
	}
	return "https://" + host, nil
}
