package cli

import (
	"context"
	"fmt"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	node "github.com/cosmos/cosmos-sdk/client/grpc/node"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

const (
	feePolicyAnnotationKey = "akt.fee-policy"
	feePolicyPrebuiltTx    = "prebuilt"
)

// reconcileGasPricesWithNode applies the selected RPC node's live CheckTx
// minimum before a command derives fees from gas prices.
func reconcileGasPricesWithNode(
	ctx context.Context,
	cctx sdkclient.Context,
	flagSet *pflag.FlagSet,
) error {
	pricesFlag := flagSet.Lookup(flagdefs.FlagGasPrices)
	if pricesFlag == nil {
		return nil
	}

	fees, _ := flagSet.GetString(flagdefs.FlagFees)
	offline, _ := flagSet.GetBool(flagdefs.FlagOffline)
	if fees != "" || cctx.Offline || offline {
		return nil
	}

	configuredRaw, _ := flagSet.GetString(flagdefs.FlagGasPrices)
	configured, err := sdk.ParseDecCoins(configuredRaw)
	if err != nil {
		return fmt.Errorf("--%s: %w", flagdefs.FlagGasPrices, err)
	}

	// A separately configured gRPC endpoint can belong to another operator.
	// Clear it on this copy so the generated node client routes Config through
	// the same CometBFT RPC client that simulation and broadcast use.
	rpcContext := cctx.WithGRPCClient(nil)
	response, err := node.NewServiceClient(rpcContext).Config(ctx, &node.ConfigRequest{})
	if err != nil {
		return fmt.Errorf("query selected RPC node minimum gas prices: %w", err)
	}

	minimumRaw := strings.TrimSpace(response.GetMinimumGasPrice())
	minimum, err := sdk.ParseDecCoins(minimumRaw)
	if err != nil {
		return fmt.Errorf("selected RPC node returned invalid minimum gas prices %q: %w", minimumRaw, err)
	}
	if minimum.Empty() {
		return nil
	}

	effective, err := reconcileGasPrices(configured, minimum)
	if err != nil {
		return err
	}
	if err := pricesFlag.Value.Set(effective.String()); err != nil {
		return fmt.Errorf("apply selected RPC node minimum gas prices: %w", err)
	}

	return nil
}

func reconcileGasPrices(configured, minimum sdk.DecCoins) (sdk.DecCoins, error) {
	if configured.Empty() {
		return minimum, nil
	}

	effective := append(sdk.DecCoins(nil), configured...)
	matched := false
	for i := range effective {
		for _, floor := range minimum {
			if effective[i].Denom != floor.Denom {
				continue
			}

			matched = true
			if floor.Amount.GT(effective[i].Amount) {
				effective[i].Amount = floor.Amount
			}
			break
		}
	}

	if !matched {
		return nil, fmt.Errorf(
			"configured gas prices %q have no denomination accepted by the selected RPC node (minimum gas prices %q)",
			configured,
			minimum,
		)
	}

	return effective, nil
}

func commandDerivesFees(cmd *cobra.Command) bool {
	return cmd.Annotations == nil || cmd.Annotations[feePolicyAnnotationKey] != feePolicyPrebuiltTx
}
