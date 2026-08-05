package cli

import (
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output"
)

// KeysCmds registers a sub-tree of commands to interact with
// local private key storage.
//
// NOTE: this tree is NOT registered on the akt root command -- `akt context
// keys` (internal/cli/keys) is the key-management surface, and it takes its
// keyring from the context system rather than from flags. KeysCmds is kept as
// a clean copy of the upstream subtree for reference and for the commands
// below that other trees reuse; it therefore registers its own keyring flags,
// which the root's global --keyring-backend/--keyring-dir (SPEC §3.1) would
// otherwise provide. Wiring it up would give akt two key-management trees with
// different resolution rules, so it stays unregistered until there is a reason
// to expose it.
func KeysCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		RunE:  ValidateCmd,
		Short: "Manage your application's keys",
		Long: `Keyring management commands. These keys may be in any format supported by the
CometBFT crypto library and can be used by light-clients, full nodes, or any other application
that needs to sign with a private key.

The keyring supports the following backends:

    os          Uses the operating system's default credentials store.
    file        Uses encrypted file-based keystore within the app's configuration directory.
                This keyring will request a password each time it is accessed, which may occur
                multiple times in a single command resulting in repeated password prompts.
    kwallet     Uses KDE Wallet Manager as a credentials management application.
    pass        Uses the pass command line utility to store and retrieve keys.
    test        Stores keys insecurely to disk. It does not prompt for a password to be unlocked
                and it should be use only for testing purposes.

kwallet and pass backends depend on external tools. Refer to their respective documentation for more
information:
    KWallet     https://github.com/KDE/kwallet
    pass        https://www.passwordstore.org/

The pass backend requires GnuPG: https://gnupg.org/
`,
	}

	cmd.AddCommand(
		MnemonicKeyCommand(),
		AddKeyCommand(),
		ExportKeyCommand(),
		ImportKeyCommand(),
		ImportKeyHexCommand(),
		ListKeysCmd(),
		ListKeyTypesCmd(),
		ShowKeysCmd(),
		DeleteKeyCommand(),
		RenameKeyCommand(),
		ParseKeyStringCommand(),
		MigrateCommand(),
	)

	cmd.PersistentFlags().Var(output.NewEnumFlag("text", "text", "json"), cflags.FlagOutput, "Output format (text|json)")
	cflags.AddKeyringFlags(cmd.PersistentFlags())

	cmd.PersistentFlags().Bool(cflags.FlagOffline, true, "")
	err := cmd.PersistentFlags().MarkHidden(cflags.FlagOffline)
	if err != nil {
		panic(err)
	}

	return cmd
}
