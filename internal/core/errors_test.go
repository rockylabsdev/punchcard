package core_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestReasonIsARefusalInTheUsersWords(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w: 10:45 is before 10:00", core.ErrUsage), "10:45 is before 10:00"},
		{fmt.Errorf("%w: AA-1 has nothing paused", core.ErrNoTimer), "AA-1 has nothing paused"},
		{fmt.Errorf("AA-1 %w", core.ErrByHand), "AA-1 was set by hand"},
		{errors.New("disk full"), "disk full"},
	}
	for _, c := range cases {
		if got := core.Reason(c.err); got != c.want {
			t.Errorf("Reason(%q) = %q, want %q", c.err, got, c.want)
		}
	}
}
