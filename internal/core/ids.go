package core

import (
	"crypto/rand"
	"fmt"
)

// NewEntryID returns a fresh uppercase UUIDv4 for a TimeEntry. It is the
// one impure helper here; callers pass its result in Start.NewEntryID
// so Apply itself stays pure.
func NewEntryID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails (Go 1.24+)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
