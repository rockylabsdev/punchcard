package core

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The duration a person types for a worklog, wherever they type it: the
// TUI's stop dialog, `stop --duration` and the terminal stop prompt.
const (
	// MinDuration and MaxDuration bound a typed duration.
	MinDuration = time.Minute
	MaxDuration = 24 * time.Hour
	// DurationForms names every form ParseDuration reads, for hints.
	DurationForms = "1h30m, 1h 30m, 1:30, 90m or minutes"
)

var (
	// ErrDurationForm: the text is none of DurationForms. Seconds and
	// fractions are refused, since a worklog is whole minutes.
	ErrDurationForm = errors.New("not a duration")
	// ErrDurationRange: a duration under MinDuration or over MaxDuration.
	ErrDurationRange = errors.New("duration out of range")
)

var (
	minutesPattern = regexp.MustCompile(`^[0-9]+$`)
	hmPattern      = regexp.MustCompile(`^(?:([0-9]+)h)?(?:([0-9]+)m)?$`)
	clockPattern   = regexp.MustCompile(`^([0-9]+):([0-5][0-9])$`)
)

// ParseDuration reads a typed duration: 1h30m, 1h 30m, 1h, 90m, 90 (bare
// minutes) or 1:30 (hours and minutes, never minutes and seconds), case
// and spaces ignored, from MinDuration to MaxDuration. It fails with
// ErrDurationForm or ErrDurationRange.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	var hours, mins string
	switch {
	case minutesPattern.MatchString(s):
		mins = s
	case clockPattern.MatchString(s):
		p := clockPattern.FindStringSubmatch(s)
		hours, mins = p[1], p[2]
	case s != "" && hmPattern.MatchString(s):
		p := hmPattern.FindStringSubmatch(s)
		hours, mins = p[1], p[2]
	default:
		return 0, ErrDurationForm
	}
	d := time.Duration(atoi(hours))*time.Hour + time.Duration(atoi(mins))*time.Minute
	if d < MinDuration || d > MaxDuration {
		return 0, ErrDurationRange
	}
	return d, nil
}

// atoi reads a run of digits, saturating so a huge number lands out of
// range instead of overflowing; "" is 0.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil && s != "" {
		return int(MaxDuration/time.Minute) + 1
	}
	return n
}
