package cli

import (
	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
)

func newTagsCmd(d *deps) *cobra.Command {
	var asJSON, includeDeleted, deletedOnly bool

	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List distinct tags with counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			deleted, err := deletedFilterFrom(includeDeleted, deletedOnly)
			if err != nil {
				return err
			}

			st, err := d.store()
			if err != nil {
				return err
			}

			counts, err := st.ListTags(cmd.Context(), deleted)
			if err != nil {
				return err
			}

			if asJSON {
				return output.WriteTagsJSON(cmd.OutOrStdout(), counts)
			}
			output.WriteTagsTable(cmd.OutOrStdout(), counts)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "output as JSON")
	cmd.Flags().BoolVar(&includeDeleted, "include-deleted", false, "count across all notes")
	cmd.Flags().BoolVar(&deletedOnly, "deleted-only", false, "count only soft-deleted notes")

	return cmd
}
