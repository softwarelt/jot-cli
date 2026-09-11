package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
	"github.com/softwarelt/jot-cli/internal/store"
)

// resolvePrefix looks up an id/prefix argument common to show/rm/restore:
// enforces the minimum prefix length, and on an ambiguous match prints the
// FR-9 disambiguation list itself before returning the error (so callers
// just need to propagate it for the exit code).
func resolvePrefix(cmd *cobra.Command, d *deps, st *store.Store, arg string) (store.Note, error) {
	if len(arg) < minPrefixLen {
		return store.Note{}, usageErrorf("id prefix must be at least %d characters", minPrefixLen)
	}
	note, err := st.GetByPrefix(cmd.Context(), arg)
	if err != nil {
		var amb *store.ErrAmbiguous
		if errors.As(err, &amb) {
			output.WriteAmbiguousMatches(cmd.OutOrStdout(), amb.Prefix, amb.Matches, d.location())
			return store.Note{}, amb
		}
		return store.Note{}, err
	}
	return note, nil
}

func newShowCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Full detail for one note, deleted or not",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := d.store()
			if err != nil {
				return err
			}
			note, err := resolvePrefix(cmd, d, st, args[0])
			if err != nil {
				return err
			}
			output.WriteNoteDetail(cmd.OutOrStdout(), note, d.location())
			return nil
		},
	}
	return cmd
}
