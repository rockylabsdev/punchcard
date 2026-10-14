package core_test

import (
	"errors"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestParseAtReadsAClockTimeOrAnAgo(t *testing.T) {
	now := at("10:50:20").Time
	cases := []struct {
		in   string
		want core.Timestamp
	}{
		{"10:00", at("10:00")},
		{"9:05", at("09:05")},
		{"10:50", at("10:50")},
		{"-45m", at("10:05:20")},
		{"-1h30m", at("09:20:20")},
		{"-1:30", at("09:20:20")},
		{" -20 ", at("10:30:20")},
	}
	for _, c := range cases {
		got, err := core.ParseAt(c.in, now)
		if err != nil {
			t.Errorf("ParseAt(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want.Time) {
			t.Errorf("ParseAt(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseAtAClockTimeLaterThanNowIsYesterday(t *testing.T) {
	now := day(10, "00:10").Time
	got, err := core.ParseAt("23:50", now)
	if err != nil {
		t.Fatal(err)
	}
	if want := day(9, "23:50"); !got.Equal(want.Time) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestParseAtRefusesAnythingElse(t *testing.T) {
	for _, in := range []string{"", "10", "45m", "+45m", "24:00", "10:60", "2026-08-09 10:00", "10:00am", "-0", "-25h", "now"} {
		if _, err := core.ParseAt(in, at("10:50").Time); !errors.Is(err, core.ErrAtForm) {
			t.Errorf("ParseAt(%q) err = %v, want ErrAtForm", in, err)
		}
	}
}
