package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
)

func newRestoreCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Undo a soft delete",
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
			if err := st.Restore(cmd.Context(), note.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Restored %s.\n", output.ShortID(note.ID))
			return nil
		},
	}
	return cmd
}
