// Package cli builds the cobra command tree for every jot subcommand.
// It is only ever invoked once cmd/jot/main.go's dispatch (DESIGN.md §4.1)
// has already decided the first argument is a reserved subcommand word —
// this package never has to guess between "capture" and "subcommand".
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/softwarelt/jot-cli/internal/config"
	"github.com/softwarelt/jot-cli/internal/store"
)

// Version is set at build time via -ldflags (DESIGN.md §8).
var Version = "dev"

// deps threads config and a lazily-opened store through to every subcommand.
// The store is only opened on first use, so `jot config`, `jot --help`, and
// friends never create ~/.jot as a side effect (FR-14 is about capture and
// data commands, not about asking for help).
type deps struct {
	cfg config.Config
	db  *store.Store
}

func (d *deps) store() (*store.Store, error) {
	if d.db != nil {
		return d.db, nil
	}
	if err := config.EnsureHome(d.cfg.JotHome); err != nil {
		return nil, fmt.Errorf("creating %s: %w", d.cfg.JotHome, err)
	}
	st, err := store.Open(d.cfg.Storage.Path)
	if err != nil {
		return nil, err
	}
	d.db = st
	return st, nil
}

func (d *deps) location() *time.Location {
	return d.cfg.Location()
}

// Execute builds the command tree and runs it against args, whose first
// element is already known to be a reserved subcommand word (or a help/
// flag token cobra itself understands).
func Execute(args []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jot: %v\n", err)
		os.Exit(1)
	}

	d := &deps{cfg: cfg}

	root := &cobra.Command{
		Use:   "jot",
		Short: "Capture and search quick notes from the terminal",
		Long: `jot captures a quick note straight from the command line — no
subcommand needed for the most common action:

  jot "<note text>" [tag...]

The subcommands below cover everything else: browsing, searching, and
managing what's already been captured.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newListCmd(d),
		newSearchCmd(d),
		newShowCmd(d),
		newTagsCmd(d),
		newRmCmd(d),
		newRestoreCmd(d),
		newConfigCmd(d),
	)
	root.SetArgs(args)

	// Close explicitly (rather than via defer) since os.Exit below would
	// otherwise skip a deferred close entirely.
	execErr := root.Execute()
	if d.db != nil {
		d.db.Close()
	}

	if execErr != nil {
		code := codeForError(execErr)
		if code != 3 { // the ambiguous-match list was already printed by the subcommand
			fmt.Fprintf(os.Stderr, "jot: %v\n", execErr)
		}
		os.Exit(code)
	}
}
