package store

import (
	"errors"
	"fmt"
)

// ErrInvalidInput marks an error as caused by bad user input (an empty note,
// a malformed tag) rather than an internal failure — callers map it to a
// usage-error exit code (2) rather than a general one (1). Wrap it with
// fmt.Errorf("...: %w", ErrInvalidInput) so errors.Is still matches through
// whatever additional context is added.
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound means no note matched the given id or prefix at all.
var ErrNotFound = errors.New("note not found")

// ErrAlreadyDeleted means SoftDelete was called on a note that is already
// soft-deleted — the caller should treat this as a no-op, not a failure
// (see the edge cases in the product plan).
var ErrAlreadyDeleted = errors.New("note is already deleted")

// ErrNotDeleted means Restore was called on a note that isn't soft-deleted.
var ErrNotDeleted = errors.New("note is not deleted")

// ErrAmbiguous means a prefix matched more than one note. Matches is every
// match, ordered by id (== chronologically, since UUIDv7 sorts that way) —
// see FR-9 and DESIGN.md §3.4.
type ErrAmbiguous struct {
	Prefix  string
	Matches []Note
}

func (e *ErrAmbiguous) Error() string {
	return fmt.Sprintf("%q matches %d notes", e.Prefix, len(e.Matches))
}
