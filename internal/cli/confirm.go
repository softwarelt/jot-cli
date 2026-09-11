package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/softwarelt/jot-cli/internal/output"
	"github.com/softwarelt/jot-cli/internal/store"
)

// confirmHardDelete implements the rm --hard confirmation flow from
// DESIGN.md §4.5: show what's about to be destroyed, require typing "yes"
// in full (not y/N — the one truly destructive action gets a little extra
// friction on purpose, same reasoning as --hard having no short flag).
// Only called when --yes was not passed; the caller has already rejected
// --hard-without---yes on a non-TTY stdin.
func confirmHardDelete(cmd *cobra.Command, note store.Note, d *deps) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, usageErrorf("--hard requires --yes when not running interactively")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "About to permanently delete:")
	fmt.Fprintf(out, "  %s  %q", output.ShortID(note.ID), truncateForConfirm(note.Body))
	if len(note.Tags) > 0 {
		fmt.Fprintf(out, "  (%s)", strings.Join(note.Tags, ", "))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out)
	fmt.Fprint(out, `This cannot be undone. Type "yes" to confirm: `)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "yes"), nil
}

func truncateForConfirm(body string) string {
	r := []rune(strings.ReplaceAll(body, "\n", " "))
	if len(r) <= 60 {
		return string(r)
	}
	return string(r[:60]) + "…"
}
