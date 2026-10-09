package cli

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// linePick is the default picker: a numbered list on stderr, one line of
// answer on stdin (a number, or a key typed in full; empty cancels).
func (a *app) linePick(choices []Choice) (string, error) {
	if len(choices) > 9 {
		choices = choices[:9]
	}
	for i, c := range choices {
		fmt.Fprintf(a.env.Stderr, "  %d) %s  %s\n", i+1, c.Key, c.Summary)
	}
	fmt.Fprint(a.env.Stderr, "Start which? (number or KEY, empty cancels) ")
	line, _ := bufio.NewReader(a.env.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if n, err := strconv.Atoi(line); err == nil {
		if n < 1 || n > len(choices) {
			return "", fmt.Errorf("%d is not one of the choices", n)
		}
		return choices[n-1].Key, nil
	}
	return line, nil // core refuses a bad key with usage
}

// branchPattern finds a ticket key anywhere in a branch name
// (feature/ABC-123-thing).
var branchPattern = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

// branchKey is the ticket key in dir's current git branch, or "" when
// dir is not a repository, HEAD is detached, or the branch names no key.
func branchKey(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--short", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	key := branchPattern.FindString(strings.TrimSpace(string(out)))
	if !core.ValidKey(key) {
		return ""
	}
	return key
}
