package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

func GetQueryModuleNameToAddressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "module-name-to-address [module-name]",
		Short: "module name to address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx := client.GetClientContextFromCmd(cmd)
			address := authtypes.NewModuleAddress(args[0])
			return cctx.PrintString(address.String())
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
