package cli

import (
	"fmt"
	"time"
)

// parseDateArg accepts exactly RFC3339 or YYYY-MM-DD for --since/--until —
// deliberately nothing looser (DESIGN.md §9: silently accepting ambiguous
// input is worse than rejecting it). A bare date is interpreted as
// midnight UTC that day.
func parseDateArg(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD, got %q", s)
}
