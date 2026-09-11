// Command jot is a local-first CLI for capturing quick notes with tags,
// timestamps, and keyword search. See DESIGN.md for the full design.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/softwarelt/jot-cli/internal/cli"
	"github.com/softwarelt/jot-cli/internal/config"
	"github.com/softwarelt/jot-cli/internal/output"
	"github.com/softwarelt/jot-cli/internal/store"
)

// reserved holds every word that dispatches to a cobra subcommand instead
// of being captured as note text (DESIGN.md §4.1 / product plan FR-18).
// version is handled separately below since cobra doesn't reliably give us
// a "-v" shorthand for free.
var reserved = map[string]bool{
	"list": true, "search": true, "show": true, "tags": true,
	"rm": true, "restore": true, "config": true,
	"help": true, "-h": true, "--help": true,
}

type action int

const (
	actionCapture action = iota
	actionDefaultList
	actionVersion
	actionSubcommand
)

type dispatch struct {
	action action
	args   []string // meaning depends on action: capture body+tags, or subcommand argv
}

// classify implements FR-18 / DESIGN.md §4.1 as a pure function so the
// dispatch rule itself — not just its side effects — is directly unit
// testable (see main_test.go).
func classify(args []string) dispatch {
	switch {
	case len(args) > 0 && args[0] == "--":
		// `--` forces literal capture unconditionally — even over a
		// reserved word or a leading dash. The FR-18 escape hatch.
		return dispatch{action: actionCapture, args: args[1:]}
	case len(args) == 0:
		return dispatch{action: actionDefaultList}
	case args[0] == "-v" || args[0] == "--version" || args[0] == "version":
		return dispatch{action: actionVersion}
	case reserved[args[0]]:
		return dispatch{action: actionSubcommand, args: args}
	default:
		return dispatch{action: actionCapture, args: args}
	}
}

func main() {
	d := classify(os.Args[1:])
	switch d.action {
	case actionCapture:
		runCapture(d.args)
	case actionDefaultList:
		runDefaultList()
	case actionVersion:
		fmt.Println(cli.Version)
	case actionSubcommand:
		cli.Execute(d.args)
	}
}

func runCapture(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, `jot: note text must not be empty

usage: jot "<note text>" [tag...]`)
		os.Exit(2)
	}

	_, st := mustOpen()

	// Close explicitly, before any exit below — os.Exit skips deferred calls.
	note, err := st.CreateNote(context.Background(), args[0], args[1:])
	st.Close()
	if err != nil {
		if errors.Is(err, store.ErrInvalidInput) {
			fmt.Fprintf(os.Stderr, "jot: %v\n", err)
			os.Exit(2)
		}
		fail(err)
	}
	fmt.Printf("Saved %s\n", output.ShortID(note.ID))
}

func runDefaultList() {
	cfg, st := mustOpen()

	notes, err := st.List(context.Background(), store.ListParams{
		Limit:   cfg.Display.DefaultListLimit,
		Deleted: store.ActiveOnly,
	})
	st.Close()
	if err != nil {
		fail(err)
	}
	output.WriteNotesTable(os.Stdout, notes, cfg.Location())
}

func mustOpen() (config.Config, *store.Store) {
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if err := config.EnsureHome(cfg.JotHome); err != nil {
		fail(err)
	}
	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		fail(err)
	}
	return cfg, st
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "jot: %v\n", err)
	os.Exit(1)
}
