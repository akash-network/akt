package cli

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

func GetQueryModuleNameToAddressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "module-name-to-address [module-name]",
		Short: "module name to address",
		Long: `Derive a module account's address from its name.

The address is computed locally from the name, so any string produces one.
It is not checked against the chain -- run "akt query auth module-accounts"
to see the modules that actually exist.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx := client.GetClientContextFromCmd(cmd)

			// The derivation hashes whatever it is given, so "" and "   " each
			// produced a syntactically valid, wholly unowned address at exit 0.
			// A name that is merely misspelled cannot be caught here without a
			// node, but an empty one is unambiguously a mistake.
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("module name is required; see \"akt query auth module-accounts\" for the modules on this chain")
			}

			if name != args[0] {
				return fmt.Errorf("module name %q has leading or trailing whitespace; the address is derived from the exact string", args[0])
			}

			address := authtypes.NewModuleAddress(name)

			return cctx.PrintString(address.String())
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
