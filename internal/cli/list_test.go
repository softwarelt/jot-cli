package cli

import (
	"testing"

	"github.com/softwarelt/jot-cli/internal/store"
)

func TestDeletedFilterFrom(t *testing.T) {
	cases := []struct {
		name                        string
		includeDeleted, deletedOnly bool
		want                        store.DeletedFilter
		wantErr                     bool
	}{
		{"neither flag", false, false, store.ActiveOnly, false},
		{"include-deleted", true, false, store.IncludeDeleted, false},
		{"deleted-only", false, true, store.DeletedOnly, false},
		{"both flags is a usage error", true, true, store.ActiveOnly, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deletedFilterFrom(tc.includeDeleted, tc.deletedOnly)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if codeForError(err) != 2 {
					t.Errorf("codeForError = %d, want 2 (usage error)", codeForError(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("deletedFilterFrom(%v, %v) = %v, want %v", tc.includeDeleted, tc.deletedOnly, got, tc.want)
			}
		})
	}
}
