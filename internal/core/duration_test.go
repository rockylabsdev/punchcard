package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestParseDurationReadsEveryForm(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"1h30m", 90 * time.Minute},
		{"1h 30m", 90 * time.Minute},
		{" 1h  20m ", 80 * time.Minute},
		{"1H20M", 80 * time.Minute},
		{"1h", time.Hour},
		{"90m", 90 * time.Minute},
		{"90", 90 * time.Minute},
		{"1", time.Minute},
		{"1:30", 90 * time.Minute},
		{"0:45", 45 * time.Minute},
		{"24:00", 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"1440", 24 * time.Hour},
	}
	for _, c := range cases {
		got, err := core.ParseDuration(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
}

func TestParseDurationRefusesWhatAWorklogCannotHold(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		{"", core.ErrDurationForm},
		{"30s", core.ErrDurationForm},
		{"1.5h", core.ErrDurationForm},
		{"1:5", core.ErrDurationForm},
		{"1:60", core.ErrDurationForm},
		{"1:30:00", core.ErrDurationForm},
		{"-5", core.ErrDurationForm},
		{"m", core.ErrDurationForm},
		{"soon", core.ErrDurationForm},
		{"0", core.ErrDurationRange},
		{"0m", core.ErrDurationRange},
		{"0:00", core.ErrDurationRange},
		{"25h", core.ErrDurationRange},
		{"24h1m", core.ErrDurationRange},
		{"1441", core.ErrDurationRange},
	}
	for _, c := range cases {
		if got, err := core.ParseDuration(c.in); !errors.Is(err, c.want) {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
}
