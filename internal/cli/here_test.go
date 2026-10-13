package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/j-dsmith/punchcard/internal/config"
)

// gitRepo is a fresh repository on branch with one empty commit, inside
// its own temp dir so a test can write a file above the repository root.
func gitRepo(t *testing.T, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", branch}, {"commit", "-q", "--allow-empty", "-m", "x"}} {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Env = []string{"HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func punchcardFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// here runs status --json --cwd dir and returns data.here and the
// envelope's notices.
func (h *harness) here(dir string) (any, []any) {
	h.t.Helper()
	r := h.run("status", "--json", "--cwd", dir)
	if r.code != 0 {
		h.t.Fatalf("status exit %d: %s", r.code, r.stderr)
	}
	env := decode(h.t, r.stdout)
	data := env["data"].(map[string]any)
	here, present := data["here"]
	if !present {
		h.t.Fatalf("status --cwd has no here: %s", r.stdout)
	}
	notices, _ := env["notices"].([]any)
	return here, notices
}

func key(k, source string) any { return map[string]any{"key": k, "source": source} }

func TestThePunchcardFileNamesTheKeyAndTouchFocusesOnIt(t *testing.T) {
	repo := gitRepo(t, "main")
	punchcardFile(t, filepath.Join(repo, ".punchcard"), "payroll\n")
	h := newHarness(t)
	h.tracker = config.TrackerNone
	h.env["CLAUDECODE"] = "1"

	if got, _ := h.here(repo); !reflect.DeepEqual(got, key("payroll", "file")) {
		t.Errorf("here = %v", got)
	}

	h.do("09:00", "start", "billing")
	h.now = at("09:10")
	r := h.run("touch", "--cwd", repo, "--origin", "prompt")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	wantLines(t, r.stdout, "▶ payroll started")
}

// #41: a session on PUN-1 asks its agent to start OPS-77; the next prompt
// in that session has the same focus, so it leaves OPS-77 running.
func TestAnUnchangedFocusLeavesAnAgentsStartRunning(t *testing.T) {
	repo := gitRepo(t, "main")
	punchcardFile(t, filepath.Join(repo, ".punchcard"), "PUN-1\n")
	h := newHarness(t)
	h.tracker = config.TrackerNone
	h.env["CLAUDECODE"] = "1"

	h.do("09:00", "touch", "--cwd", repo, "--origin", "prompt")
	h.do("09:05", "start", "OPS-77")
	h.now = at("09:10")
	r := h.run("touch", "--cwd", repo, "--origin", "prompt")
	if r.code != 0 || r.stdout != "" {
		t.Fatalf("second touch: exit %d stdout %q stderr %q, want a silent 0", r.code, r.stdout, r.stderr)
	}
	r = h.run("status")
	if !strings.Contains(r.stdout, "OPS-77") || strings.Contains(r.stdout, "PUN-1") {
		t.Errorf("status after the second touch:\n%s", r.stdout)
	}
}

func TestTheNearestPunchcardWinsAndNoneAboveTheRepository(t *testing.T) {
	repo := gitRepo(t, "main")
	sub := filepath.Join(repo, "services", "billing")
	punchcardFile(t, filepath.Join(filepath.Dir(repo), ".punchcard"), "outside\n")
	punchcardFile(t, filepath.Join(sub, "README"), "")
	h := newHarness(t)
	h.tracker = config.TrackerNone

	if got, _ := h.here(sub); got != nil {
		t.Errorf("a .punchcard above the repository root counted: here = %v", got)
	}
	punchcardFile(t, filepath.Join(repo, ".punchcard"), "# the whole repo\n\n  payroll  \n")
	punchcardFile(t, filepath.Join(repo, "services", ".punchcard"), "billing\n")
	if got, _ := h.here(sub); !reflect.DeepEqual(got, key("billing", "file")) {
		t.Errorf("from services/billing: here = %v, want services/.punchcard's", got)
	}
	if got, _ := h.here(repo); !reflect.DeepEqual(got, key("payroll", "file")) {
		t.Errorf("at the root, past a comment and a blank line: here = %v", got)
	}
}

func TestWithNoPunchcardTheBranchNamesTheKey(t *testing.T) {
	repo := gitRepo(t, "feature/ABC-123-login")
	h := newHarness(t)
	if got, _ := h.here(repo); !reflect.DeepEqual(got, key("ABC-123", "branch")) {
		t.Errorf("here = %v", got)
	}
	if got, _ := h.here(t.TempDir()); got != nil {
		t.Errorf("not a repository, no file: here = %v, want null", got)
	}
}

func TestAnInvalidPunchcardFallsThroughToTheBranchWithANotice(t *testing.T) {
	for _, c := range []struct {
		name    string
		tracker config.Tracker
		line    string
	}{
		{"no tracker", config.TrackerNone, "pay roll"},
		{"Jira", config.TrackerJira, "payroll"},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := gitRepo(t, "feature/ABC-123-login")
			punchcardFile(t, filepath.Join(repo, ".punchcard"), c.line+"\n")
			h := newHarness(t)
			h.tracker = c.tracker
			got, notices := h.here(repo)
			if !reflect.DeepEqual(got, key("ABC-123", "branch")) {
				t.Errorf("here = %v, want the branch's key", got)
			}
			if len(notices) != 1 || !strings.Contains(notices[0].(string), filepath.Join("repo", ".punchcard")) ||
				!strings.Contains(notices[0].(string), "is not a key") {
				t.Errorf("notices = %v, want one naming the file and why", notices)
			}
			r := h.run("status", "--cwd", repo)
			if !strings.Contains(r.stderr, filepath.Join("repo", ".punchcard")) {
				t.Errorf("human stderr lacks the notice: %q", r.stderr)
			}
		})
	}
}

func TestUnderJiraAPunchcardKeyIsUpperCased(t *testing.T) {
	repo := gitRepo(t, "main")
	punchcardFile(t, filepath.Join(repo, ".punchcard"), "abc-123\n")
	h := newHarness(t)
	if got, notices := h.here(repo); !reflect.DeepEqual(got, key("ABC-123", "file")) || notices != nil {
		t.Errorf("here = %v, notices %v", got, notices)
	}
}

func TestStatusWithoutCwdHasNoHere(t *testing.T) {
	h := newHarness(t)
	r := h.run("status", "--json")
	if _, present := decode(t, r.stdout)["data"].(map[string]any)["here"]; present {
		t.Errorf("here without --cwd: %s", r.stdout)
	}
}

// #48: a branch token not in capitals counts only when its project is
// already on the list, and is spelled as that project is.
func TestALowercaseBranchKeyCountsWhenItsProjectIsTracked(t *testing.T) {
	for _, c := range []struct {
		name    string
		tracker config.Tracker
		tracked []string
		branch  string
		want    any
	}{
		{"lowercase, KA tracked", config.TrackerJira, []string{"KA-1"}, "ka-6629-ship-it", key("KA-6629", "branch")},
		{"after a slash", config.TrackerJira, []string{"KA-1"}, "feature/ka-123-x", key("KA-123", "branch")},
		{"mixed case", config.TrackerJira, []string{"KA-1"}, "Ka-77-x", key("KA-77", "branch")},
		{"lowercase, KA not tracked", config.TrackerJira, []string{"OPS-1"}, "ka-6629-ship-it", nil},
		{"capitals, KA not tracked", config.TrackerJira, []string{"OPS-1"}, "KA-6629-ship-it", key("KA-6629", "branch")},
		{"release-2026", config.TrackerJira, []string{"KA-1"}, "release-2026", nil},
		{"fix-2", config.TrackerJira, []string{"KA-1"}, "fix-2", nil},
		{"release-2026, RELEASE tracked", config.TrackerJira, []string{"RELEASE-1"}, "release-2026", key("RELEASE-2026", "branch")},
		{"fix-2, FIX tracked", config.TrackerJira, []string{"FIX-1"}, "fix-2", key("FIX-2", "branch")},
		{"no tracker, ka-1 tracked", config.TrackerNone, []string{"ka-1"}, "ka-6629-x", key("ka-6629", "branch")},
		{"no tracker, KA-1 tracked", config.TrackerNone, []string{"KA-1"}, "ka-6629-x", key("KA-6629", "branch")},
		{"the first token that counts", config.TrackerJira, []string{"KA-1"}, "fix-2-ka-6629", key("KA-6629", "branch")},
		{"capitals inside a word, as before", config.TrackerJira, []string{"OPS-1"}, "fixKA-12", key("KA-12", "branch")},
		{"two spellings: the first on the list", config.TrackerNone, []string{"ka-1", "KA-2"}, "Ka-6629-x", key("ka-6629", "branch")},
		{"two spellings, the other first", config.TrackerNone, []string{"KA-2", "ka-1"}, "ka-6629-x", key("KA-6629", "branch")},
		{"a slug has no project", config.TrackerNone, []string{"payroll"}, "payroll-12", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := gitRepo(t, c.branch)
			h := newHarness(t)
			h.tracker = c.tracker
			for _, k := range c.tracked {
				h.do("09:00", "start", k)
			}
			got, notices := h.here(repo)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("here = %v, want %v", got, c.want)
			}
			if notices != nil {
				t.Errorf("notices = %v, want none", notices)
			}
		})
	}
}

func TestTouchFocusesOnALowercaseBranchKeyOfATrackedProject(t *testing.T) {
	repo := gitRepo(t, "ka-6629-x")
	h := newHarness(t)
	h.env["CLAUDECODE"] = "1"
	h.do("09:00", "start", "KA-1")
	h.do("09:05", "start", "OPS-1")
	h.now = at("09:10")
	r := h.run("touch", "--origin", "prompt", "--cwd", repo)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	wantLines(t, r.stdout, "▶ KA-6629 started")
}
