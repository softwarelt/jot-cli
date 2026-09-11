package store

import (
	"context"
	"fmt"
	"strings"
)

// TagCount is one row of `jot tags` output.
type TagCount struct {
	Name  string
	Count int
}

// NormalizeTag trims whitespace and lowercases (Unicode-aware) a raw tag as
// typed on the command line. It returns an error if the result is empty or
// still contains internal whitespace after trimming — tags must be single
// shell words (FR-3).
func NormalizeTag(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("tag must not be empty")
	}
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return "", fmt.Errorf("tag %q must not contain whitespace (use - or _ instead)", raw)
	}
	return strings.ToLower(trimmed), nil
}

// ListTags returns every tag ever used, each with a count of notes carrying
// it under the given deleted filter. Tags are never garbage-collected —
// one whose count drops to zero still appears, with count 0 (see DESIGN.md
// §3.1 / the product plan's schema callout).
func (s *Store) ListTags(ctx context.Context, deleted DeletedFilter) ([]TagCount, error) {
	query := `
		SELECT t.name, COUNT(n.id)
		FROM tags t
		LEFT JOIN note_tags nt ON nt.tag_name = t.name
		LEFT JOIN notes n ON n.id = nt.note_id AND ` + deletedClause("n.deleted_at", deleted) + `
		GROUP BY t.name
		ORDER BY t.name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer rows.Close()

	var out []TagCount
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
			return nil, fmt.Errorf("scanning tag count: %w", err)
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}
