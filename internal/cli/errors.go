package cli

import (
	"errors"
	"fmt"

	"github.com/softwarelt/jot-cli/internal/store"
)

// minPrefixLen is the shortest id prefix show/rm/restore will accept — long
// enough to type quickly, short enough to rarely collide (product plan,
// command surface section).
const minPrefixLen = 4

// usageError marks an error as the user's mistake (bad flags, malformed
// input) rather than an internal failure — codeForError maps it to exit
// code 2 (DESIGN.md §4.3).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usageErrorf(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// codeForError maps an error returned from a command's RunE to the process
// exit code it should produce (DESIGN.md §4.3).
func codeForError(err error) int {
	if err == nil {
		return 0
	}
	var ue *usageError
	if errors.As(err, &ue) {
		return 2
	}
	if errors.Is(err, store.ErrInvalidInput) {
		return 2
	}
	var amb *store.ErrAmbiguous
	if errors.As(err, &amb) {
		return 3
	}
	return 1
}
