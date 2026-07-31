package cli

import "github.com/spf13/cobra"

// withoutEmptyVendoredGroup removes a dependency-owned placeholder only while
// it has no child actions. If a future dependency release adds an action, the
// group remains mounted automatically.
func withoutEmptyVendoredGroup(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name && !child.HasSubCommands() {
			parent.RemoveCommand(child)
			break
		}
	}

	return parent
}

// adoptVendoredTxCmd installs akt's transaction boundary throughout a command
// subtree owned by a dependency. Cobra runs only the closest persistent hook,
// so each descendant is wrapped to keep a future upstream hook from bypassing
// context endpoint and failure handling.
func adoptVendoredTxCmd(cmd *cobra.Command) *cobra.Command {
	applyTxPreRun(cmd)
	return cmd
}

func applyTxPreRun(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		applyTxPreRun(child)
	}

	// Pure groups do not construct or broadcast transactions. Leaving preflight
	// on them makes an unknown child initialize a client before the group's
	// ValidateCmd can report the bad path.
	if cmd.HasSubCommands() {
		return
	}

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
}
