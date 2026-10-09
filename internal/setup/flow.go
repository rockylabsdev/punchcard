package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// TokenPage is where Atlassian mints a classic API token.
const TokenPage = "https://id.atlassian.com/manage-profile/security/api-tokens"

// Run is the whole wizard; it returns the exit code. Esc or ctrl+c at
// any credentials step writes nothing and says so.
func Run(d Deps) int {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Connect == nil {
		d.Connect = func(ctx context.Context, site, email, token string) (jira.Account, error) {
			return jira.New(site, email, token).TestConnection(ctx)
		}
	}
	w := wizard{Deps: d}
	saved, err := w.credentials()
	switch {
	case errors.Is(err, ErrAborted):
		w.UI.Say("Nothing saved")
		return exitOK
	case errors.Is(err, config.ErrNewerSchema):
		w.UI.Say("✗ "+err.Error(), "Upgrade punchcard to change its settings; nothing was changed.")
		return exitNewer
	case errors.Is(err, errNotSaved):
		w.UI.Say("Nothing saved")
		return exitOK
	case err != nil:
		w.UI.Say("✗ " + err.Error())
		return exitInternal
	}
	if err := w.plugin(); errors.Is(err, ErrAborted) {
		return w.stopped(saved)
	}
	if err := w.askRule(); errors.Is(err, ErrAborted) {
		return w.stopped(saved)
	}
	w.UI.Say("Done. Run punchcard to open the TUI.")
	return exitOK
}

// stopped: esc in the plugin or ask-rule step. Credentials already
// saved stay saved.
func (w wizard) stopped(saved bool) int {
	if saved {
		w.UI.Say("Stopped. Your Jira credentials are saved.")
	} else {
		w.UI.Say("Stopped. Nothing changed.")
	}
	return exitOK
}

// errNotSaved: the user said no at the save question.
var errNotSaved = errors.New("not saved")

type wizard struct{ Deps }

// stored is the token setup would keep on a blank answer: the Keychain
// item or file for the configured email, never PUNCHCARD_TOKEN.
func (w wizard) stored(email string) (config.Secret, error) {
	if email == "" {
		return config.Secret{}, nil
	}
	t := config.Tokens{Getenv: withoutEnvToken(w.Getenv)}
	if w.Keychain != nil {
		t.Keychain = w.Keychain.Read
	}
	sec, err := t.Lookup(email)
	if errors.Is(err, config.ErrNotConfigured) {
		return config.Secret{}, nil
	}
	return sec, err
}

func withoutEnvToken(getenv func(string) string) func(string) string {
	return func(k string) string {
		if k == "PUNCHCARD_TOKEN" {
			return ""
		}
		return getenv(k)
	}
}

// where names a stored token's home in words.
func (w wizard) where(s config.Secret) string {
	if s.Source == config.SourceKeychain {
		return "Keychain"
	}
	p, _ := config.TokenPath(w.Getenv) // a stored file token implies a config dir
	return p
}

// credentials is the re-run fork, then site → email → token → test →
// save. saved reports whether anything was written.
func (w wizard) credentials() (saved bool, err error) {
	old, lerr := config.Load(w.Getenv)
	switch {
	case errors.Is(lerr, config.ErrNewerSchema):
		return false, lerr
	case lerr != nil:
		old = config.Config{} // missing or broken: a first setup
	}
	stored, kerr := w.stored(old.Email)
	if kerr != nil {
		w.UI.Say("⚠ " + kerr.Error())
	}
	if old.Email != "" {
		change, err := w.fork(old, stored)
		if err != nil || !change {
			return false, err
		}
	}
	envTok := w.Getenv("PUNCHCARD_TOKEN")
	f := form{site: old.SiteURL, email: old.Email}
	step := stepSite
	for {
		switch step {
		case stepSite:
			if err := w.askSite(&f); err != nil {
				return false, err
			}
			step = stepEmail
		case stepEmail:
			if err := w.askEmail(&f); err != nil {
				return false, err
			}
			step = stepToken
		case stepToken:
			if err := w.askToken(&f, stored, envTok); err != nil {
				return false, err
			}
			step = stepTest
		case stepTest:
			acct, back := w.test(f, stored, envTok)
			if back != stepSave {
				step = back
				continue
			}
			name := acct.DisplayName
			if name == "" {
				name = f.email
			}
			w.UI.Say("✓ Connected as " + name)
			ok, err := w.UI.Confirm(Question{Title: "Save these credentials?", Default: true})
			if err != nil {
				return false, err
			}
			if !ok {
				return false, errNotSaved
			}
			return true, w.save(f, stored, envTok)
		}
	}
}

type step int

const (
	stepSite step = iota
	stepEmail
	stepToken
	stepTest
	stepSave
)

// form is what has been typed so far; a walk back keeps it.
type form struct{ site, email, token string }

// fork is a re-run's first question: change the credentials, or keep
// them and go on to the plugin and ask rule.
func (w wizard) fork(old config.Config, stored config.Secret) (bool, error) {
	where := "no token stored"
	switch {
	case stored.Value != "":
		where = "token in " + w.where(stored)
	case w.Getenv("PUNCHCARD_TOKEN") != "":
		where = "token from PUNCHCARD_TOKEN"
	}
	w.UI.Say(fmt.Sprintf("Connected to %s as %s (%s)", old.SiteURL, old.Email, where))
	q := Question{Title: "Change credentials?"}
	if w.Store != nil {
		if wd, _, err := w.Store.Read(); err == nil && !wd.State.AuthFailedAt.IsZero() {
			at := wd.State.AuthFailedAt.In(w.Now().Location())
			q.Default = true
			q.Note = []string{"Jira rejected the token at " + at.Format("15:04 on Mon 2 Jan")}
		}
	}
	return w.UI.Confirm(q)
}

func (w wizard) askSite(f *form) error {
	v, err := w.UI.Ask(Field{
		Title: "Jira site", Placeholder: "acme.atlassian.net", Value: f.site,
		Echo: func(s string) string {
			if n, err := NormalizeSite(s); err == nil {
				return "→ " + n
			}
			return ""
		},
		Validate: func(s string) error { _, err := NormalizeSite(s); return err },
	})
	if err != nil {
		return err
	}
	f.site, _ = NormalizeSite(v)
	return nil
}

func (w wizard) askEmail(f *form) error {
	v, err := w.UI.Ask(Field{
		Title: "Email", Placeholder: "you@example.com", Value: f.email,
		Note:     []string{"The email you sign in to Atlassian with"},
		Validate: validEmail,
	})
	if err != nil {
		return err
	}
	f.email = strings.TrimSpace(v)
	return nil
}

func validEmail(s string) error {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return errors.New("enter the email you sign in to Atlassian with")
	case !strings.Contains(s, "@") || strings.ContainsAny(s, " \t\"\\"):
		return errors.New("that does not look like an email address")
	}
	return nil
}

func (w wizard) askToken(f *form, stored config.Secret, envTok string) error {
	note := []string{
		"Make a classic API token (not a scoped one) at " + link(TokenPage),
		"A scoped token looks valid but Jira rejects it.",
	}
	blankOK := false
	switch {
	case envTok != "":
		note = append(note, "PUNCHCARD_TOKEN is set — leave blank to test with it")
		blankOK = true
	case stored.Value != "":
		note = append(note, "Stored in "+w.where(stored)+" — leave blank to keep")
		blankOK = true
	}
	v, err := w.UI.Ask(Field{
		Title: "API token", Secret: true, Value: f.token, Note: note,
		Validate: func(s string) error {
			s = strings.TrimSpace(s)
			switch {
			case s == "" && !blankOK:
				return errors.New("paste an API token")
			case strings.ContainsAny(s, " \t\"\\"):
				return errors.New("an API token has no spaces, quotes or backslashes")
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	f.token = strings.TrimSpace(v)
	return nil
}

// test runs Test Connection with what was typed (a blank token means
// PUNCHCARD_TOKEN if set, else the stored one). It returns the step to
// go on to: stepSave on a pass, else the step that holds the culprit.
func (w wizard) test(f form, stored config.Secret, envTok string) (jira.Account, step) {
	token := f.token
	switch {
	case token != "":
	case envTok != "":
		token = envTok
		w.UI.Say("Testing with PUNCHCARD_TOKEN…")
	default:
		token = stored.Value
		w.UI.Say("Testing with the stored token…")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ConnectBudget)
	defer cancel()
	acct, err := w.Connect(ctx, f.site, f.email, token)
	switch {
	case err == nil:
		return acct, stepSave
	case errors.Is(err, jira.ErrUnauthorized):
		w.UI.Say("✗ Jira rejected these credentials.",
			"Check the email, and that the token is a classic one: a scoped token",
			"is refused like a wrong one. Make a classic token at "+link(TokenPage))
		return acct, stepToken
	case errors.Is(err, jira.ErrBadSite), errors.Is(err, jira.ErrUnreachable):
		w.UI.Say("✗ "+f.site+" is not a reachable Jira Cloud site", "  "+strings.TrimPrefix(err.Error(), jira.ErrBadSite.Error()+": "))
		return acct, stepSite
	}
	w.UI.Say("✗ Test Connection failed: " + err.Error())
	return acct, stepSite
}

// save writes config.json, then the token: every punchcard Keychain
// item deleted, the token written through security, the file only as a
// fallback (or on Linux). A fresh setup clears authFailedAt.
func (w wizard) save(f form, stored config.Secret, envTok string) error {
	if err := config.Save(w.Getenv, config.Config{SiteURL: f.site, Email: f.email}); err != nil {
		return err
	}
	saved, _ := config.Path(w.Getenv) // Save succeeded, so the path exists
	w.UI.Say("Saved " + saved)
	token := f.token
	if token == "" {
		token = stored.Value
	}
	switch {
	case token == "":
		w.UI.Say("No token saved: PUNCHCARD_TOKEN supplies it")
	default:
		if err := w.saveToken(f.email, token); err != nil {
			return err
		}
		if envTok != "" {
			w.UI.Say("⚠ PUNCHCARD_TOKEN is set and overrides the saved token while it is set")
		}
	}
	w.clearAuthFailure()
	return nil
}

func (w wizard) saveToken(email, token string) error {
	path, _ := config.TokenPath(w.Getenv) // config.Save succeeded first, so it exists
	if w.Keychain == nil {
		if err := config.WriteTokenFile(w.Getenv, token); err != nil {
			return err
		}
		w.UI.Say("Token saved in " + path + " (readable only by you)")
		return nil
	}
	err := w.Keychain.DeleteAll()
	if err == nil {
		err = w.Keychain.Write(email, token)
	}
	if err == nil {
		if rerr := config.RemoveTokenFile(w.Getenv); rerr != nil {
			w.UI.Say("⚠ Could not remove the old token file " + path + ": " + rerr.Error())
		}
		w.UI.Say("Token saved in the macOS Keychain")
		return nil
	}
	if ferr := config.WriteTokenFile(w.Getenv, token); ferr != nil {
		return fmt.Errorf("could not save the token: Keychain: %v; file: %w", err, ferr)
	}
	w.UI.Say("⚠ Could not write to the macOS Keychain ("+err.Error()+")",
		"  saved the token in "+path+" (readable only by you) instead")
	return nil
}

// clearAuthFailure: tested credentials make a recorded 401 history. It
// writes only when one is recorded, so setup never creates state.json
// (the first-run marker).
func (w wizard) clearAuthFailure() {
	if w.Store == nil {
		return
	}
	wd, _, err := w.Store.Read()
	if err != nil || wd.State.AuthFailedAt.IsZero() {
		return
	}
	if _, err := w.Store.Apply(core.ClearAuthFailure{}); err != nil {
		w.UI.Say("⚠ Could not clear the rejected-token banner: " + err.Error())
	}
}

// link is url as an OSC 8 terminal hyperlink (its text is the url, so a
// terminal without links shows it plainly).
func link(url string) string {
	return "\x1b]8;;" + url + "\x1b\\" + url + "\x1b]8;;\x1b\\"
}
