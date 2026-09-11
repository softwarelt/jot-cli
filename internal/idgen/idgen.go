// Package idgen isolates jot's one dependency on the uuid library, per
// DESIGN.md §2.
package idgen

import "github.com/google/uuid"

// NewID returns a new UUIDv7 string: time-ordered in its leading
// characters, globally unique, safe as a primary key across devices if jot
// ever grows a sync feature (see the product plan's schema rationale).
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Only fails if the system's randomness source is broken, which is
		// unrecoverable for a tool that identifies every note by id.
		panic("jot: failed to generate id: " + err.Error())
	}
	return id.String()
}
