package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/output"
	"github.com/softwarelt/jot-cli/internal/store"
)

func newRmCmd(d *deps) *cobra.Command {
	var hard, yes bool

	cmd := &cobra.Command{
		Use:   "rm <id>",
		Short: "Soft-delete a note, or permanently purge with --hard",
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

			if hard {
				return runHardDelete(cmd, d, st, note, yes)
			}
			return runSoftDelete(cmd, st, note)
		},
	}

	cmd.Flags().BoolVar(&hard, "hard", false, "permanently delete instead of soft-delete")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt (required with --hard when not interactive)")
	return cmd
}

func runSoftDelete(cmd *cobra.Command, st *store.Store, note store.Note) error {
	if err := st.SoftDelete(cmd.Context(), note.ID); err != nil {
		if errors.Is(err, store.ErrAlreadyDeleted) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s is already deleted.\n", output.ShortID(note.ID))
			return nil
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s. Restore with: jot restore %s\n", output.ShortID(note.ID), output.ShortID(note.ID))
	return nil
}

func runHardDelete(cmd *cobra.Command, d *deps, st *store.Store, note store.Note, yes bool) error {
	if !yes {
		confirmed, err := confirmHardDelete(cmd, note, d)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled — no changes made.")
			return nil
		}
	}
	if err := st.HardDelete(cmd.Context(), note.ID); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Permanently deleted %s.\n", output.ShortID(note.ID))
	return nil
}
