package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "data.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCreateNote(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	n, err := st.CreateNote(ctx, "Alex is blocked", []string{"Alex", " 1:1 ", "alex"})
	require.NoError(t, err)
	require.NotEmpty(t, n.ID)
	require.Equal(t, "Alex is blocked", n.Body)
	// FR-2: normalized (trimmed + lowercased) and deduplicated ("Alex"/"alex" collapse).
	require.ElementsMatch(t, []string{"alex", "1:1"}, n.Tags)
	require.False(t, n.CreatedAt.IsZero())
	require.Equal(t, n.CreatedAt, n.UpdatedAt)
}

func TestCreateNote_EmptyBody(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateNote(context.Background(), "   ", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateNote_RejectsWhitespaceTag(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateNote(context.Background(), "note", []string{"two words"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidInput)

	// FR-3 edge case: nothing should have been written on a rejected tag.
	notes, err := st.List(context.Background(), ListParams{Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Empty(t, notes)
}

func TestNormalizeTag(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Bug", "bug", false},
		{"  wip  ", "wip", false},
		{"", "", true},
		{"   ", "", true},
		{"two words", "", true},
		{"tab\tinside", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeTag(tc.in)
		if tc.wantErr {
			require.Error(t, err, "input %q", tc.in)
			continue
		}
		require.NoError(t, err, "input %q", tc.in)
		require.Equal(t, tc.want, got)
	}
}

func TestList_TagFilterIsNormalized(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	_, err := st.CreateNote(ctx, "tagged", []string{"Bug"})
	require.NoError(t, err)

	notes, err := st.List(ctx, ListParams{Tag: "BUG", Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Len(t, notes, 1)
}

func TestList_SinceUntilInclusive(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	_, err := st.CreateNote(ctx, "note one", nil)
	require.NoError(t, err)
	created, err := st.List(ctx, ListParams{Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Len(t, created, 1)
	ts := created[0].CreatedAt

	notes, err := st.List(ctx, ListParams{Since: &ts, Until: &ts, Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Len(t, notes, 1, "exact timestamp should match inclusively on both bounds")

	before := ts.Add(-time.Millisecond)
	notes, err = st.List(ctx, ListParams{Until: &before, Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Empty(t, notes)
}

func TestList_Limit(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	for i := 0; i < 5; i++ {
		_, err := st.CreateNote(ctx, "note", nil)
		require.NoError(t, err)
	}
	notes, err := st.List(ctx, ListParams{Limit: 2, Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Len(t, notes, 2)
}

func TestSearch_CaseInsensitiveAndEscaped(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	_, err := st.CreateNote(ctx, "Root cause: retry storm (50% loss)", []string{"incident"})
	require.NoError(t, err)

	notes, err := st.Search(ctx, "RETRY STORM", "", ActiveOnly)
	require.NoError(t, err)
	require.Len(t, notes, 1)

	// A literal "%" in the search term must not act as a wildcard.
	notes, err = st.Search(ctx, "50% loss", "", ActiveOnly)
	require.NoError(t, err)
	require.Len(t, notes, 1)

	notes, err = st.Search(ctx, "nonexistent", "", ActiveOnly)
	require.NoError(t, err)
	require.Empty(t, notes)
}

func TestDeletedFilters(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	active, err := st.CreateNote(ctx, "active note", nil)
	require.NoError(t, err)
	deleted, err := st.CreateNote(ctx, "deleted note", nil)
	require.NoError(t, err)
	require.NoError(t, st.SoftDelete(ctx, deleted.ID))

	cases := []struct {
		filter DeletedFilter
		want   int
	}{
		{ActiveOnly, 1},
		{DeletedOnly, 1},
		{IncludeDeleted, 2},
	}
	for _, tc := range cases {
		notes, err := st.List(ctx, ListParams{Deleted: tc.filter})
		require.NoError(t, err)
		require.Lenf(t, notes, tc.want, "filter=%v", tc.filter)
	}

	notes, err := st.List(ctx, ListParams{Deleted: ActiveOnly})
	require.NoError(t, err)
	require.Equal(t, active.ID, notes[0].ID)
}

func TestSoftDeleteRestoreHardDelete(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	n, err := st.CreateNote(ctx, "note", []string{"tag1"})
	require.NoError(t, err)

	require.NoError(t, st.SoftDelete(ctx, n.ID))
	require.ErrorIs(t, st.SoftDelete(ctx, n.ID), ErrAlreadyDeleted)

	require.NoError(t, st.Restore(ctx, n.ID))
	require.ErrorIs(t, st.Restore(ctx, n.ID), ErrNotDeleted)

	require.NoError(t, st.HardDelete(ctx, n.ID))
	_, err = st.GetByPrefix(ctx, n.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// Hard delete removes the note, but never the tag itself (product plan:
	// tags are never garbage-collected).
	tags, err := st.ListTags(ctx, IncludeDeleted)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.Equal(t, "tag1", tags[0].Name)
	require.Equal(t, 0, tags[0].Count)
}

func TestSoftDeleteHardDeleteNotFound(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	require.ErrorIs(t, st.SoftDelete(ctx, "does-not-exist"), ErrNotFound)
	require.ErrorIs(t, st.HardDelete(ctx, "does-not-exist"), ErrNotFound)
	require.ErrorIs(t, st.Restore(ctx, "does-not-exist"), ErrNotFound)
}

func TestGetByPrefix(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	n, err := st.CreateNote(ctx, "unique note", nil)
	require.NoError(t, err)

	got, err := st.GetByPrefix(ctx, n.ID[:8])
	require.NoError(t, err)
	require.Equal(t, n.ID, got.ID)

	_, err = st.GetByPrefix(ctx, "zzzznotfound")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetByPrefix_Ambiguous_IncludesDeleted(t *testing.T) {
	// FR-9: show/rm/restore lookups ignore deleted status entirely.
	ctx := context.Background()
	st := openTestStore(t)

	a, err := st.CreateNote(ctx, "note a", nil)
	require.NoError(t, err)
	b, err := st.CreateNote(ctx, "note b", nil)
	require.NoError(t, err)
	require.NoError(t, st.SoftDelete(ctx, b.ID))

	// Find a prefix shared by both (their common leading characters).
	shared := commonPrefix(a.ID, b.ID)
	require.GreaterOrEqual(t, len(shared), 1)

	_, err = st.GetByPrefix(ctx, shared)
	var amb *ErrAmbiguous
	require.True(t, errors.As(err, &amb))
	require.Len(t, amb.Matches, 2, "ambiguous match must include the soft-deleted note")

	// Ascending id order (== chronological, since UUIDv7 sorts that way).
	require.Equal(t, a.ID, amb.Matches[0].ID)
	require.Equal(t, b.ID, amb.Matches[1].ID)
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}

func TestListTags_NeverGarbageCollected(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	n, err := st.CreateNote(ctx, "note", []string{"solo"})
	require.NoError(t, err)
	require.NoError(t, st.HardDelete(ctx, n.ID))

	tags, err := st.ListTags(ctx, ActiveOnly)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.Equal(t, "solo", tags[0].Name)
	require.Equal(t, 0, tags[0].Count)
}

// TestConcurrentCreateNote covers the acceptance criterion from the product
// plan: N concurrent captures produce N notes, zero lost, thanks to WAL
// mode + busy_timeout (NFR-4).
func TestConcurrentCreateNote(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.CreateNote(ctx, "concurrent note", []string{"stress"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	notes, err := st.List(ctx, ListParams{Deleted: ActiveOnly, Limit: n + 1})
	require.NoError(t, err)
	require.Len(t, notes, n)
}
