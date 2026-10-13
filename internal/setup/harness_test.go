package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// esc is a scripted answer that cancels the prompt.
const esc = "<esc>"

// scriptUI answers prompts from a script and records a transcript:
//
//	? Title [note…] = answer     a field ("" answer keeps the prefilled value)
//	? Title (Y/n) = y            a question ("" takes the default)
//	? Title {a, b} = b           a choice ("" keeps the prefilled option)
//	! message                    a validation error (the next answer retries)
//	  line                       something the wizard said
type scriptUI struct {
	t       *testing.T
	answers []string
	log     strings.Builder
	out     bytes.Buffer // command output
}

func (u *scriptUI) next(title string) (string, bool) {
	if len(u.answers) == 0 {
		u.t.Errorf("no answer left for %q; transcript so far:\n%s", title, u.log.String())
		return esc, false
	}
	a := u.answers[0]
	u.answers = u.answers[1:]
	return a, true
}

func (u *scriptUI) Ask(f Field) (string, error) {
	for {
		a, _ := u.next(f.Title)
		fmt.Fprintf(&u.log, "? %s", f.Title)
		for _, n := range f.Note {
			fmt.Fprintf(&u.log, " [%s]", n)
		}
		if a == esc {
			u.log.WriteString(" = <esc>\n")
			return "", ErrAborted
		}
		if a == "" {
			a = f.Value
		}
		shown := a
		if f.Secret && a != "" {
			shown = "•••"
		}
		fmt.Fprintf(&u.log, " = %s\n", shown)
		if f.Echo != nil {
			fmt.Fprintf(&u.log, "  echo %s\n", f.Echo(a))
		}
		if f.Validate != nil {
			if err := f.Validate(a); err != nil {
				fmt.Fprintf(&u.log, "! %v\n", err)
				continue
			}
		}
		return a, nil
	}
}

func (u *scriptUI) Confirm(q Question) (bool, error) {
	a, _ := u.next(q.Title)
	yn := "y/N"
	if q.Default {
		yn = "Y/n"
	}
	fmt.Fprintf(&u.log, "? %s (%s)", q.Title, yn)
	for _, n := range q.Note {
		fmt.Fprintf(&u.log, " [%s]", n)
	}
	switch a {
	case esc:
		u.log.WriteString(" = <esc>\n")
		return false, ErrAborted
	case "":
		if q.Default {
			a = "y"
		} else {
			a = "n"
		}
	}
	fmt.Fprintf(&u.log, " = %s\n", a)
	return a == "y", nil
}

func (u *scriptUI) Choose(c Choice) (string, error) {
	a, _ := u.next(c.Title)
	vals := make([]string, len(c.Options))
	for i, o := range c.Options {
		vals[i] = o.Value
	}
	fmt.Fprintf(&u.log, "? %s {%s}", c.Title, strings.Join(vals, ", "))
	for _, n := range c.Note {
		fmt.Fprintf(&u.log, " [%s]", n)
	}
	switch a {
	case esc:
		u.log.WriteString(" = <esc>\n")
		return "", ErrAborted
	case "":
		a = c.Value
	}
	fmt.Fprintf(&u.log, " = %s\n", a)
	for _, v := range vals {
		if v == a {
			return a, nil
		}
	}
	u.t.Errorf("answer %q is not one of %v", a, vals)
	return "", ErrAborted
}

func (u *scriptUI) Say(lines ...string) {
	for _, l := range lines {
		fmt.Fprintf(&u.log, "  %s\n", l)
	}
}

func (u *scriptUI) Output() io.Writer { return &u.out }

// fakeKeychain is the Keychain: items by account, and scripted errors.
type fakeKeychain struct {
	mu                        sync.Mutex
	items                     map[string]string
	readErr, writeErr, delErr error
	calls                     []string
}

func (k *fakeKeychain) Read(email string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls = append(k.calls, "read "+email)
	if k.readErr != nil {
		return "", k.readErr
	}
	v, ok := k.items[email]
	if !ok {
		return "", config.ErrNoKeychainItem
	}
	return v, nil
}

func (k *fakeKeychain) Write(email, token string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls = append(k.calls, "write "+email+" "+token)
	if k.writeErr != nil {
		return k.writeErr
	}
	if k.items == nil {
		k.items = map[string]string{}
	}
	k.items[email] = token
	return nil
}

func (k *fakeKeychain) DeleteAll() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls = append(k.calls, "deleteAll")
	if k.delErr != nil {
		return k.delErr
	}
	k.items = nil
	return nil
}

// fakeJira is Test Connection: sites it knows, tokens it accepts.
type fakeJira struct {
	sites  map[string]bool   // site → reachable Cloud site
	tokens map[string]string // email+" "+token → display name
	calls  []string
}

func (j *fakeJira) connect(_ context.Context, site, email, token string) (tracker.Account, error) {
	j.calls = append(j.calls, site+" "+email+" "+token)
	if !j.sites[site] {
		return tracker.Account{}, fmt.Errorf("%w: %w", tracker.ErrBadSite, tracker.ErrUnreachable)
	}
	name, ok := j.tokens[email+" "+token]
	if !ok {
		return tracker.Account{}, &tracker.Refusal{Status: 401}
	}
	return tracker.Account{AccountID: "A1", DisplayName: name, Email: email}, nil
}

// fakeClaude is `claude` on PATH, or not.
type fakeClaude struct {
	found bool
	list  string            // `plugin list --json` output
	fail  map[string]error  // by the joined args
	said  map[string]string // more output, by the joined args
	calls []string
}

func (c *fakeClaude) Path() string {
	if !c.found {
		return ""
	}
	return "/usr/local/bin/claude"
}

func (c *fakeClaude) Run(_ context.Context, out io.Writer, args ...string) error {
	cmd := strings.Join(args, " ")
	c.calls = append(c.calls, cmd)
	fmt.Fprintf(out, "ran %s\n", cmd)
	fmt.Fprint(out, c.said[cmd])
	return c.fail[cmd]
}

func (c *fakeClaude) Output(_ context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	c.calls = append(c.calls, cmd)
	if err := c.fail[cmd]; err != nil {
		return nil, err
	}
	return []byte(c.list), nil
}

// rig is one wizard run's world: temp XDG and Claude dirs, the fakes.
type rig struct {
	t      *testing.T
	env    map[string]string
	ui     *scriptUI
	kc     *fakeKeychain
	linux  bool // no Keychain
	jira   *fakeJira
	claude *fakeClaude
	store  *store.Store
	now    time.Time
}

const (
	site  = "https://acme.atlassian.net"
	email = "me@acme.test"
	tok   = "tok-good"
)

func newRig(t *testing.T) *rig {
	dir := t.TempDir()
	r := &rig{
		t: t,
		env: map[string]string{
			"HOME":              filepath.Join(dir, "home"),
			"XDG_CONFIG_HOME":   filepath.Join(dir, "config"),
			"XDG_DATA_HOME":     filepath.Join(dir, "data"),
			"CLAUDE_CONFIG_DIR": filepath.Join(dir, "claude"),
		},
		kc: &fakeKeychain{},
		jira: &fakeJira{
			sites:  map[string]bool{site: true},
			tokens: map[string]string{email + " " + tok: "Jamey Smith"},
		},
		claude: &fakeClaude{},
		now:    time.Date(2026, 10, 9, 14, 30, 0, 0, time.UTC),
	}
	r.store = store.New(filepath.Join(dir, "data", "punchcard"), func() time.Time { return r.now })
	return r
}

func (r *rig) getenv(k string) string { return r.env[k] }

// run runs the wizard with these answers and returns the exit code.
func (r *rig) run(answers ...string) int {
	r.t.Helper()
	r.ui = &scriptUI{t: r.t, answers: answers}
	code := Run(r.deps(r.ui))
	if len(r.ui.answers) > 0 {
		r.t.Errorf("answers left over: %q\ntranscript:\n%s", r.ui.answers, r.ui.log.String())
	}
	return code
}

// deps is the wizard's world on ui.
func (r *rig) deps(ui UI) Deps {
	d := Deps{
		UI: ui, Getenv: r.getenv, Connect: r.jira.connect, Claude: r.claude,
		Store: r.store, Version: "v0.1.0", Now: func() time.Time { return r.now },
	}
	if !r.linux {
		d.Keychain = r.kc
	}
	return d
}

func (r *rig) transcript() string { return r.ui.log.String() }

// has fails unless the transcript contains every one of want.
func (r *rig) has(want ...string) {
	r.t.Helper()
	for _, w := range want {
		if !strings.Contains(r.transcript(), w) {
			r.t.Errorf("transcript lacks %q:\n%s", w, r.transcript())
		}
	}
}

func (r *rig) lacks(not ...string) {
	r.t.Helper()
	for _, w := range not {
		if strings.Contains(r.transcript(), w) {
			r.t.Errorf("transcript has %q:\n%s", w, r.transcript())
		}
	}
}

// configured writes config.json as an earlier setup would have.
func (r *rig) configured(site, email string) {
	r.t.Helper()
	if err := config.Save(r.getenv, config.Config{SiteURL: site, Email: email}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) config() (config.Config, bool) {
	c, err := config.Load(r.getenv)
	return c, err == nil
}

func (r *rig) tokenPath() string {
	p, err := config.TokenPath(r.getenv)
	if err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *rig) configPath() string {
	p, err := config.Path(r.getenv)
	if err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *rig) tokenFile() (string, bool) {
	b, err := os.ReadFile(r.tokenPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return strings.TrimSpace(string(b)), true
}

// file is config.json as written, whatever Load would make of it.
func (r *rig) file() config.Config {
	r.t.Helper()
	b, err := os.ReadFile(r.configPath())
	if err != nil {
		r.t.Fatal(err)
	}
	var c config.Config
	if err := json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	return c
}

func (r *rig) settings() (string, bool) {
	b, err := os.ReadFile(SettingsPath(r.getenv))
	if err != nil {
		return "", false
	}
	return string(b), true
}
