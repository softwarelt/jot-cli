package cli

import "testing"

func TestParseDateArg(t *testing.T) {
	valid := []string{
		"2026-09-10",
		"2026-09-10T14:23:01Z",
		"2026-09-10T14:23:01-07:00",
	}
	for _, v := range valid {
		if _, err := parseDateArg(v); err != nil {
			t.Errorf("parseDateArg(%q) = %v, want no error", v, err)
		}
	}

	// DESIGN.md §9: exactly RFC3339 or YYYY-MM-DD, nothing looser.
	invalid := []string{"yesterday", "09/10/2026", "2026-9-10", "last week", ""}
	for _, v := range invalid {
		if _, err := parseDateArg(v); err == nil {
			t.Errorf("parseDateArg(%q) = nil error, want a rejection", v)
		}
	}
}
