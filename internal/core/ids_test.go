package core_test

import (
	"regexp"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestNewEntryIDIsAnUppercaseUUIDv4(t *testing.T) {
	re := regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`)
	a, b := core.NewEntryID(), core.NewEntryID()
	for _, id := range []string{a, b} {
		if !re.MatchString(id) {
			t.Errorf("%q is not an uppercase UUIDv4", id)
		}
	}
	if a == b {
		t.Errorf("two ids collided: %s", a)
	}
}
