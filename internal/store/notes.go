package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/softwarelt/jot-cli/internal/idgen"
)

// timeLayout is the one and only timestamp format ever written to or read
// from the database: RFC3339, UTC, fixed millisecond precision. Every
// created_at/updated_at/deleted_at value goes through nowStamp/parseStamp
// exclusively (DESIGN.md §3.3) — this is what makes plain string comparison
// sort and range-filter correctly.
const timeLayout = "2006-01-02T15:04:05.000Z"

func nowStamp() string {
	return time.Now().UTC().Format(timeLayout)
}

func parseStamp(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

// Note is one row of notes, with its tags loaded alongside it.
type Note struct {
	ID        string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time // nil unless soft-deleted
	Tags      []string
}

// DeletedFilter controls whether a listing/search/count includes
// soft-deleted notes, only soft-deleted notes, or (the default) neither.
type DeletedFilter int

const (
	ActiveOnly     DeletedFilter = iota // default everywhere
	IncludeDeleted                      // active + soft-deleted together
	DeletedOnly                         // soft-deleted only
)

func deletedClause(col string, f DeletedFilter) string {
	switch f {
	case IncludeDeleted:
		return "1=1"
	case DeletedOnly:
		return col + " IS NOT NULL"
	default:
		return col + " IS NULL"
	}
}

// escapeLike escapes a user-supplied string for safe use inside a SQL LIKE
// pattern, so a literal search term is never accidentally treated as a
// wildcard.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ListParams filters a call to List.
type ListParams struct {
	Tag     string     // "" = no tag filter
	Since   *time.Time // nil = no lower bound
	Until   *time.Time // nil = no upper bound
	Limit   int        // <= 0 = no limit
	Deleted DeletedFilter
}

// CreateNote stores a new note. Raw tags are normalized here (FR-2/FR-3);
// duplicates within one call are deduplicated silently.
func (s *Store) CreateNote(ctx context.Context, body string, rawTags []string) (Note, error) {
	if strings.TrimSpace(body) == "" {
		return Note{}, fmt.Errorf("note text must not be empty: %w", ErrInvalidInput)
	}

	id := idgen.NewID()
	ts := nowStamp()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notes (id, body, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, body, ts, ts,
	); err != nil {
		return Note{}, fmt.Errorf("inserting note: %w", err)
	}

	seen := make(map[string]bool, len(rawTags))
	var tags []string
	for _, raw := range rawTags {
		tag, err := NormalizeTag(raw)
		if err != nil {
			return Note{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags (name, created_at) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`,
			tag, ts,
		); err != nil {
			return Note{}, fmt.Errorf("inserting tag %q: %w", tag, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO note_tags (note_id, tag_name) VALUES (?, ?)`,
			id, tag,
		); err != nil {
			return Note{}, fmt.Errorf("linking tag %q: %w", tag, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Note{}, fmt.Errorf("committing note: %w", err)
	}

	createdAt, _ := parseStamp(ts)
	return Note{ID: id, Body: body, CreatedAt: createdAt, UpdatedAt: createdAt, Tags: tags}, nil
}

// List returns notes matching p, newest first.
func (s *Store) List(ctx context.Context, p ListParams) ([]Note, error) {
	query := `SELECT DISTINCT n.id, n.body, n.created_at, n.updated_at, n.deleted_at FROM notes n`
	var args []any
	var where []string

	if p.Tag != "" {
		tag, err := NormalizeTag(p.Tag)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		query += ` JOIN note_tags nt ON nt.note_id = n.id`
		where = append(where, "nt.tag_name = ?")
		args = append(args, tag)
	}

	where = append(where, deletedClause("n.deleted_at", p.Deleted))

	if p.Since != nil {
		where = append(where, "n.created_at >= ?")
		args = append(args, p.Since.UTC().Format(timeLayout))
	}
	if p.Until != nil {
		where = append(where, "n.created_at <= ?")
		args = append(args, p.Until.UTC().Format(timeLayout))
	}

	query += " WHERE " + strings.Join(where, " AND ")
	query += " ORDER BY n.created_at DESC, n.id DESC"
	if p.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, p.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	defer rows.Close()

	notes, err := scanNotes(rows)
	if err != nil {
		return nil, err
	}
	return s.loadTags(ctx, notes)
}

// Search does a case-insensitive substring match over note bodies (v1's
// search — see the product plan's search-sequencing decision for the
// planned FTS5 fast-follow, which only touches this package).
func (s *Store) Search(ctx context.Context, term, tag string, deleted DeletedFilter) ([]Note, error) {
	query := `SELECT DISTINCT n.id, n.body, n.created_at, n.updated_at, n.deleted_at FROM notes n`
	var args []any
	var where []string

	if tag != "" {
		norm, err := NormalizeTag(tag)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		query += ` JOIN note_tags nt ON nt.note_id = n.id`
		where = append(where, "nt.tag_name = ?")
		args = append(args, norm)
	}

	// LIKE is already case-insensitive for ASCII by default in SQLite;
	// COLLATE has no effect on LIKE's own case-folding (verified directly
	// against sqlite3), so it's deliberately not used here — see FR-8's
	// ASCII-only case-folding caveat.
	where = append(where, "n.body LIKE ? ESCAPE '\\'")
	args = append(args, "%"+escapeLike(term)+"%")
	where = append(where, deletedClause("n.deleted_at", deleted))

	query += " WHERE " + strings.Join(where, " AND ")
	query += " ORDER BY n.created_at DESC, n.id DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("searching notes: %w", err)
	}
	defer rows.Close()

	notes, err := scanNotes(rows)
	if err != nil {
		return nil, err
	}
	return s.loadTags(ctx, notes)
}

// GetByPrefix always searches across ALL notes regardless of deleted_at —
// per FR-9, a specific known id doesn't need a deleted-status filter, so
// there is deliberately no DeletedFilter parameter here. On more than one
// match it returns *ErrAmbiguous with every match ordered by id (i.e.
// chronologically — see FR-9's disambiguation-list requirement).
func (s *Store) GetByPrefix(ctx context.Context, prefix string) (Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, created_at, updated_at, deleted_at FROM notes
		 WHERE id LIKE ? ESCAPE '\' ORDER BY id ASC`,
		escapeLike(prefix)+"%",
	)
	if err != nil {
		return Note{}, fmt.Errorf("looking up note: %w", err)
	}
	defer rows.Close()

	notes, err := scanNotes(rows)
	if err != nil {
		return Note{}, err
	}

	switch len(notes) {
	case 0:
		return Note{}, ErrNotFound
	case 1:
		full, err := s.loadTags(ctx, notes)
		if err != nil {
			return Note{}, err
		}
		return full[0], nil
	default:
		full, err := s.loadTags(ctx, notes)
		if err != nil {
			return Note{}, err
		}
		return Note{}, &ErrAmbiguous{Prefix: prefix, Matches: full}
	}
}

func (s *Store) getExact(ctx context.Context, id string) (Note, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, body, created_at, updated_at, deleted_at FROM notes WHERE id = ?`, id)

	var n Note
	var createdAt, updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(&n.ID, &n.Body, &createdAt, &updatedAt, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Note{}, ErrNotFound
		}
		return Note{}, fmt.Errorf("looking up note: %w", err)
	}
	n.CreatedAt, _ = parseStamp(createdAt)
	n.UpdatedAt, _ = parseStamp(updatedAt)
	if deletedAt.Valid {
		t, _ := parseStamp(deletedAt.String)
		n.DeletedAt = &t
	}
	return n, nil
}

// SoftDelete marks a note deleted without removing it (FR-11). Calling it on
// an already-deleted note returns ErrAlreadyDeleted — the CLI layer treats
// that as a no-op with a message, not a failure.
func (s *Store) SoftDelete(ctx context.Context, id string) error {
	note, err := s.getExact(ctx, id)
	if err != nil {
		return err
	}
	if note.DeletedAt != nil {
		return ErrAlreadyDeleted
	}
	ts := nowStamp()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE notes SET deleted_at = ?, updated_at = ? WHERE id = ?`, ts, ts, id,
	); err != nil {
		return fmt.Errorf("soft-deleting note: %w", err)
	}
	return nil
}

// HardDelete permanently removes a note and, via ON DELETE CASCADE, its
// note_tags links (FR-12). Tag rows themselves are never removed — see
// ListTags.
func (s *Store) HardDelete(ctx context.Context, id string) error {
	if _, err := s.getExact(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("hard-deleting note: %w", err)
	}
	return nil
}

// Restore clears deleted_at on a soft-deleted note (FR-13). Calling it on a
// note that isn't deleted returns ErrNotDeleted.
func (s *Store) Restore(ctx context.Context, id string) error {
	note, err := s.getExact(ctx, id)
	if err != nil {
		return err
	}
	if note.DeletedAt == nil {
		return ErrNotDeleted
	}
	ts := nowStamp()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE notes SET deleted_at = NULL, updated_at = ? WHERE id = ?`, ts, id,
	); err != nil {
		return fmt.Errorf("restoring note: %w", err)
	}
	return nil
}

func scanNotes(rows *sql.Rows) ([]Note, error) {
	var notes []Note
	for rows.Next() {
		var n Note
		var createdAt, updatedAt string
		var deletedAt sql.NullString
		if err := rows.Scan(&n.ID, &n.Body, &createdAt, &updatedAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
		n.CreatedAt, _ = parseStamp(createdAt)
		n.UpdatedAt, _ = parseStamp(updatedAt)
		if deletedAt.Valid {
			t, _ := parseStamp(deletedAt.String)
			n.DeletedAt = &t
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// loadTags fills in Tags for each note in place, in one query regardless of
// how many notes there are.
func (s *Store) loadTags(ctx context.Context, notes []Note) ([]Note, error) {
	if len(notes) == 0 {
		return notes, nil
	}

	ids := make([]string, len(notes))
	idx := make(map[string]int, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
		idx[n.ID] = i
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf(
		`SELECT note_id, tag_name FROM note_tags WHERE note_id IN (%s) ORDER BY tag_name`,
		placeholders,
	)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("loading tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var noteID, tag string
		if err := rows.Scan(&noteID, &tag); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		notes[idx[noteID]].Tags = append(notes[idx[noteID]].Tags, tag)
	}
	return notes, rows.Err()
}
