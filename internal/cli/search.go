package cli

import (
	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
)

func newSearchCmd(d *deps) *cobra.Command {
	var (
		tag                         string
		asJSON                      bool
		includeDeleted, deletedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "search <term>",
		Short: "Keyword search over note bodies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deleted, err := deletedFilterFrom(includeDeleted, deletedOnly)
			if err != nil {
				return err
			}

			st, err := d.store()
			if err != nil {
				return err
			}

			notes, err := st.Search(cmd.Context(), args[0], tag, deleted)
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
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "output as JSON")
	cmd.Flags().BoolVar(&includeDeleted, "include-deleted", false, "also include soft-deleted notes")
	cmd.Flags().BoolVar(&deletedOnly, "deleted-only", false, "show only soft-deleted notes")

	return cmd
}
