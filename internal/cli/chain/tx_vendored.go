package cli

import "github.com/spf13/cobra"

// adoptVendoredTxCmd installs akt's transaction boundary throughout a command
// subtree owned by a dependency. Cobra runs only the closest persistent hook,
// so each descendant is wrapped to keep a future upstream hook from bypassing
// context endpoint and failure handling.
func adoptVendoredTxCmd(cmd *cobra.Command) *cobra.Command {
	applyTxPreRun(cmd)
	return cmd
}

func applyTxPreRun(cmd *cobra.Command) {
	if inner := cmd.PersistentPreRunE; inner != nil {
		cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			if err := TxPersistentPreRunE(c, args); err != nil {
				return err
			}
			return inner(c, args)
		}
	} else {
		cmd.PersistentPreRunE = TxPersistentPreRunE
	}

	for _, child := range cmd.Commands() {
		applyTxPreRun(child)
	}
}
