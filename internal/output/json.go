package output

import (
	"encoding/json"
	"io"

	"github.com/softwarelt/jot-cli/internal/store"
)

// SchemaVersion versions the --json output shape. The product plan flags
// this as a contract scripts will depend on once shipped — bump it, don't
// silently reshape a field, if the format ever needs to change.
const SchemaVersion = 1

const jsonTimeLayout = "2006-01-02T15:04:05.000Z"

type jsonNote struct {
	ID        string   `json:"id"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	DeletedAt *string  `json:"deleted_at"`
}

type notesEnvelope struct {
	SchemaVersion int        `json:"schema_version"`
	Notes         []jsonNote `json:"notes"`
}

// WriteNotesJSON renders notes as the versioned --json envelope used by
// `list` and `search`.
func WriteNotesJSON(w io.Writer, notes []store.Note) error {
	out := notesEnvelope{SchemaVersion: SchemaVersion, Notes: make([]jsonNote, 0, len(notes))}
	for _, n := range notes {
		jn := jsonNote{
			ID:        n.ID,
			Body:      n.Body,
			Tags:      n.Tags,
			CreatedAt: n.CreatedAt.UTC().Format(jsonTimeLayout),
			UpdatedAt: n.UpdatedAt.UTC().Format(jsonTimeLayout),
		}
		if jn.Tags == nil {
			jn.Tags = []string{}
		}
		if n.DeletedAt != nil {
			s := n.DeletedAt.UTC().Format(jsonTimeLayout)
			jn.DeletedAt = &s
		}
		out.Notes = append(out.Notes, jn)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

type jsonTagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type tagsEnvelope struct {
	SchemaVersion int            `json:"schema_version"`
	Tags          []jsonTagCount `json:"tags"`
}

// WriteTagsJSON renders tag counts as the versioned --json envelope used by
// `tags`.
func WriteTagsJSON(w io.Writer, tags []store.TagCount) error {
	out := tagsEnvelope{SchemaVersion: SchemaVersion, Tags: make([]jsonTagCount, 0, len(tags))}
	for _, t := range tags {
		out.Tags = append(out.Tags, jsonTagCount{Name: t.Name, Count: t.Count})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
