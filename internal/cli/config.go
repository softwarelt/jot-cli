package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show resolved config (edit ~/.jot/config.toml to change it)",
		Long: `Prints the fully resolved configuration jot is currently using,
along with the path of the config file it was loaded from (if any).

jot has no "config set" command. To change a setting, edit that file
directly — it's a plain TOML file — and run any jot command again; changes
take effect immediately, no restart needed.

Resolution order: CLI flag > environment variable > config file > built-in
default. See JOT_HOME / JOT_CONFIG if you keep the file somewhere other than
the default ~/.jot/config.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := d.cfg
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "jot home:                    %s\n", cfg.JotHome)
			if cfg.ConfigFilePath != "" {
				fmt.Fprintf(w, "config file:                 %s\n", cfg.ConfigFilePath)
			} else {
				fmt.Fprintln(w, "config file:                 (none found — using built-in defaults)")
			}
			fmt.Fprintf(w, "storage.path:                %s\n", cfg.Storage.Path)
			fmt.Fprintf(w, "display.timezone:            %s\n", cfg.Display.Timezone)
			fmt.Fprintf(w, "display.default_list_limit:  %d\n", cfg.Display.DefaultListLimit)
			return nil
		},
	}
	return cmd
}
