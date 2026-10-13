package core

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AtForms names every form ParseAt reads, for hints.
const AtForms = "HH:MM (the latest such time, today or last night) or -D (that long ago: -45m, -1h30m)"

// ErrAtForm: the text is none of AtForms.
var ErrAtForm = errors.New("not a time")

var atClock = regexp.MustCompile(`^([0-9]{1,2}):([0-5][0-9])$`)

// ParseAt reads the TIME a backdated pause, resume or start names, as of
// now: HH:MM is the most recent such local time at or before now (so
// 23:50 at 00:10 is last night), and -D is D ago, D in ParseDuration's
// grammar. There are no dates: a correction is within the day. It fails
// with ErrAtForm.
func ParseAt(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		d, err := ParseDuration(rest)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %q: %v", ErrAtForm, s, err)
		}
		return now.Add(-d), nil
	}
	p := atClock.FindStringSubmatch(s)
	if p == nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrAtForm, s)
	}
	h, _ := strconv.Atoi(p[1])
	m, _ := strconv.Atoi(p[2])
	if h > 23 {
		return time.Time{}, fmt.Errorf("%w: %q", ErrAtForm, s)
	}
	y, mo, d := now.Date()
	t := time.Date(y, mo, d, h, m, 0, 0, now.Location())
	if t.After(now) {
		t = time.Date(y, mo, d-1, h, m, 0, 0, now.Location())
	}
	return t, nil
}
