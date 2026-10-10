package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/tracker"
)

// PunchcardFile is the file a directory names its ticket in.
const PunchcardFile = ".punchcard"

// Here is a directory's key and where it came from.
type Here struct {
	Key    string     `json:"key"`
	Source HereSource `json:"source"`
}

// HereSource is where a directory's key came from.
type HereSource string

const (
	HereFile   HereSource = "file"   // a .punchcard
	HereBranch HereSource = "branch" // the git branch name
)

// resolveHere is the one answer to "what is dir's key". The nearest
// .punchcard at dir or an ancestor wins, looking no higher than the git
// repository root (or the filesystem root outside a repository). If its
// first meaningful line is a key under g, that is the key; if not, a
// notice names the file and why, and the branch rule decides: the first
// ABC-123-shaped token in the current branch. Nil when neither names a
// key. The plugin has no key rule of its own; it asks status for this.
func resolveHere(dir string, g tracker.Grammar) (*Here, []string) {
	if dir == "" {
		return nil, nil
	}
	var notices []string
	if path, line, ok := nearestPunchcard(dir); ok {
		k := g.Normalize(line)
		switch {
		case k == "":
			notices = append(notices, fmt.Sprintf("%s names no key; using the branch", path))
		case !g.Valid(k):
			notices = append(notices, fmt.Sprintf("%s: %q is not a key (like %s); using the branch", path, line, g.Example()))
		default:
			return &Here{Key: k, Source: HereFile}, nil
		}
	}
	if k := branchKey(dir); k != "" && g.Valid(g.Normalize(k)) {
		return &Here{Key: g.Normalize(k), Source: HereBranch}, notices
	}
	return nil, notices
}

// nearestPunchcard finds the .punchcard nearest dir and its first line
// that is neither blank nor a # comment, trimmed ("" if it has none).
// Symlinks are resolved first so dir compares equal to the root git
// reports (a temp dir under /var is /private/var on macOS).
func nearestPunchcard(dir string) (path, line string, found bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", "", false
	}
	if r, err := filepath.EvalSymlinks(d); err == nil {
		d = r
	}
	root := gitRoot(d)
	for {
		p := filepath.Join(d, PunchcardFile)
		if f, err := os.Open(p); err == nil {
			defer f.Close()
			return p, firstLine(f), true
		}
		parent := filepath.Dir(d)
		if d == root || parent == d {
			return "", "", false
		}
		d = parent
	}
}

// firstLine is the first non-blank, non-# line, trimmed. Only the start
// of the file is read: a huge or binary file costs one buffer.
func firstLine(f *os.File) string {
	s := bufio.NewScanner(f)
	for s.Scan() {
		if l := strings.TrimSpace(s.Text()); l != "" && !strings.HasPrefix(l, "#") {
			return l
		}
	}
	return ""
}

// gitRoot is dir's repository root, symlinks resolved, or "" outside a
// repository.
func gitRoot(dir string) string {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(out)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	return root
}

// branchPattern finds a ticket key anywhere in a branch name
// (feature/ABC-123-thing). It stays this shape whatever the tracker.
var branchPattern = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

// branchKey is the ticket key in dir's current git branch, or "" when
// dir is not a repository, HEAD is detached, or the branch names no key.
func branchKey(dir string) string {
	out, err := git(dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return branchPattern.FindString(strings.TrimSpace(out))
}

// git runs one git query in dir. touch runs on every prompt, so a git
// that hangs is cut off.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}
