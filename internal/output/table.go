// Package output renders store.Note/TagCount results for the CLI, in both
// the human-readable and --json forms (DESIGN.md §4.4).
package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/softwarelt/jot-cli/internal/store"
)

const bodyTruncateLen = 80

// shortIDLen is 13: the literal first 13 characters of the canonical UUID
// string (8 hex, a dash, 4 hex) — the full UUIDv7 timestamp field, dash
// included so this is always usable as-is as a lookup prefix (GetByPrefix
// matches on the literal stored string, dashes and all).
//
// An earlier version truncated to 8 characters, matching a git short hash;
// a smoke test caught that colliding across notes captured mere seconds
// apart — 8 hex characters only guarantees uniqueness across a ~65-second
// window (see DESIGN.md / the product plan's "what a shared prefix means"
// callout), which broke "Saved <id>" as something reliably actionable
// right after a typo. 13 was chosen because it's exactly where the UUIDv7
// timestamp field ends, not an arbitrary round number.
const shortIDLen = 13

// ShortID is the abbreviated id shown in listings and confirmations.
func ShortID(id string) string {
	if len(id) > shortIDLen {
		return id[:shortIDLen]
	}
	return id
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// WriteNotesTable renders the abbreviated list/search format: short id,
// timestamp (in loc), tags, truncated body.
func WriteNotesTable(w io.Writer, notes []store.Note, loc *time.Location) {
	if len(notes) == 0 {
		fmt.Fprintln(w, "No notes found.")
		return
	}
	for _, n := range notes {
		ts := n.CreatedAt.In(loc).Format("2006-01-02 15:04")
		tags := strings.Join(n.Tags, ", ")
		fmt.Fprintf(w, "%s  %s  %-24s  %s\n", ShortID(n.ID), ts, tags, truncate(n.Body, bodyTruncateLen))
	}
}

// WriteNoteDetail renders the full, untruncated view for `jot show`.
func WriteNoteDetail(w io.Writer, n store.Note, loc *time.Location) {
	fmt.Fprintf(w, "id:      %s\n", n.ID)
	fmt.Fprintf(w, "created: %s\n", n.CreatedAt.In(loc).Format("2006-01-02 15:04:05"))
	if n.DeletedAt != nil {
		fmt.Fprintf(w, "deleted: %s\n", n.DeletedAt.In(loc).Format("2006-01-02 15:04:05"))
	}
	if len(n.Tags) > 0 {
		fmt.Fprintf(w, "tags:    %s\n", strings.Join(n.Tags, ", "))
	}
	fmt.Fprintf(w, "\n%s\n", n.Body)
}

// WriteAmbiguousMatches renders the FR-9 disambiguation list: chronological
// (List/Search already sort newest-first, but GetByPrefix's matches come in
// ascending id order — oldest first — which is what's passed in here), with
// enough context to double as "what else was jotted around here".
func WriteAmbiguousMatches(w io.Writer, prefix string, matches []store.Note, loc *time.Location) {
	fmt.Fprintf(w, "%q matches %d notes — type more characters to narrow it down:\n\n", prefix, len(matches))
	WriteNotesTable(w, matches, loc)
}

// WriteTagsTable renders `jot tags` output.
func WriteTagsTable(w io.Writer, tags []store.TagCount) {
	if len(tags) == 0 {
		fmt.Fprintln(w, "No tags yet.")
		return
	}
	for _, t := range tags {
		fmt.Fprintf(w, "%-24s %d\n", t.Name, t.Count)
	}
}
