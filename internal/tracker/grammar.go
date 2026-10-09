package tracker

import (
	"regexp"
	"strings"
)

// Grammar is what a ticket key looks like under the tracker in effect.
// It is resolved once per invocation and handed to the core, which
// validates keys with it; the edges (the CLI, the TUI) normalise what was
// typed with it first. The zero Grammar is Slug.
type Grammar struct {
	pattern *regexp.Regexp // nil: the slug
	upper   bool
	example string
}

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Slug is the grammar with no tracker: a letter or digit, then letters,
// digits, '.', '_' and '-'. Case-sensitive, kept as typed.
var Slug = Grammar{}

// NewGrammar is a tracker's own grammar: keys must match pattern
// (anchored), and with upper they are upper-cased on entry. example is
// a key to show in a refusal.
func NewGrammar(pattern string, upper bool, example string) Grammar {
	return Grammar{pattern: regexp.MustCompile(pattern), upper: upper, example: example}
}

// Valid reports whether key is a key under g, as stored.
func (g Grammar) Valid(key string) bool {
	if g.pattern == nil {
		return slugPattern.MatchString(key)
	}
	return g.pattern.MatchString(key)
}

// Normalize is what typed becomes on entry: trimmed, and upper-cased
// when the grammar says so. It does not validate.
func (g Grammar) Normalize(typed string) string {
	k := strings.TrimSpace(typed)
	if g.upper {
		k = strings.ToUpper(k)
	}
	return k
}

// Equal reports whether g and o accept and normalise keys alike.
func (g Grammar) Equal(o Grammar) bool {
	src := func(x Grammar) string {
		if x.pattern == nil {
			return ""
		}
		return x.pattern.String()
	}
	return src(g) == src(o) && g.upper == o.upper && g.example == o.example
}

// Example is a valid key, for hints ("like PUN-123").
func (g Grammar) Example() string {
	if g.example == "" {
		return "payroll"
	}
	return g.example
}
