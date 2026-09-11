package cli

import (
	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
	"github.com/softwarelt/jot-cli/internal/store"
)

func deletedFilterFrom(includeDeleted, deletedOnly bool) (store.DeletedFilter, error) {
	if includeDeleted && deletedOnly {
		return store.ActiveOnly, usageErrorf("--include-deleted and --deleted-only are mutually exclusive")
	}
	switch {
	case deletedOnly:
		return store.DeletedOnly, nil
	case includeDeleted:
		return store.IncludeDeleted, nil
	default:
		return store.ActiveOnly, nil
	}
}

func newListCmd(d *deps) *cobra.Command {
	var (
		tag                         string
		since, until                string
		limit                       int
		asJSON                      bool
		includeDeleted, deletedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Browse and filter notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return usageErrorf("--limit must be a positive number")
			}

			deleted, err := deletedFilterFrom(includeDeleted, deletedOnly)
			if err != nil {
				return err
			}

			st, err := d.store()
			if err != nil {
				return err
			}

			params := store.ListParams{Tag: tag, Limit: limit, Deleted: deleted}

			if since != "" {
				t, err := parseDateArg(since)
				if err != nil {
					return usageErrorf("--since: %v", err)
				}
				params.Since = &t
			}
			if until != "" {
				t, err := parseDateArg(until)
				if err != nil {
					return usageErrorf("--until: %v", err)
				}
				params.Until = &t
			}

			notes, err := st.List(cmd.Context(), params)
			if err != nil {
				return err
			}

			if asJSON {
				return output.WriteNotesJSON(cmd.OutOrStdout(), notes)
			}
			output.WriteNotesTable(cmd.OutOrStdout(), notes, d.location())
			return nil
		},
	}

	cmd.Flags().StringVarP(&tag, "tag", "t", "", "filter by tag")
	cmd.Flags().StringVarP(&since, "since", "s", "", "only notes created on/after this date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVarP(&until, "until", "u", "", "only notes created on/before this date (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().IntVarP(&limit, "limit", "n", d.cfg.Display.DefaultListLimit, "maximum notes to show")
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "output as JSON")
	cmd.Flags().BoolVar(&includeDeleted, "include-deleted", false, "also include soft-deleted notes")
	cmd.Flags().BoolVar(&deletedOnly, "deleted-only", false, "show only soft-deleted notes")

	return cmd
}
