package pretty

import (
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/spf13/cobra"

	"cosmossdk.io/math"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	clioutput "pkg.akt.dev/akt/internal/output"
)

// SimulationResult is the rendered form of a --dry-run simulation (SPEC §10.11.7).
//
// It deliberately has no gas-wanted field. A dry run sends the transaction with
// the placeholder gas limit the CLI substitutes for --gas, and the node echoes
// that placeholder back in GasInfo.GasWanted; surfacing it would show the user
// an internal constant dressed up as a chain reading.
type SimulationResult struct {
	Simulated     bool      `json:"simulated"`
	GasUsed       uint64    `json:"gas_used"`
	GasAdjustment float64   `json:"gas_adjustment"`
	GasEstimate   uint64    `json:"gas_estimate"`
	EstimatedFee  sdk.Coins `json:"estimated_fee,omitempty"`
}

// NewSimulationResult derives the simulation summary from a node simulate
// response plus the fee-related flags of cmd.
//
// The chain client computes the adjusted gas estimate during simulation but
// discards it on the simulate-only path (it returns the raw SimulateResponse
// and drops the adjusted value), so the estimate is recomputed here from the
// same inputs the client used.
func NewSimulationResult(cmd *cobra.Command, sim *txtypes.SimulateResponse) SimulationResult {
	var gasUsed uint64
	if sim != nil && sim.GasInfo != nil {
		gasUsed = sim.GasInfo.GasUsed
	}

	adjustment := gasAdjustmentFromFlags(cmd)
	estimate := uint64(adjustment * float64(gasUsed))

	return SimulationResult{
		Simulated:     true,
		GasUsed:       gasUsed,
		GasAdjustment: adjustment,
		GasEstimate:   estimate,
		EstimatedFee:  estimatedFee(cmd, estimate),
	}
}

// RenderSimulation renders the Simulation section for a --dry-run result.
func RenderSimulation(w io.Writer, result SimulationResult) {
	fmt.Fprintln(w, Section("Simulation"))

	//nolint:gosec // G115: gas readings are orders of magnitude below MaxInt64.
	KV(w, "Gas Used", FormatGas(int64(result.GasUsed)))
	KV(w, "Gas Adjustment", strconv.FormatFloat(result.GasAdjustment, 'f', -1, 64))
	//nolint:gosec // G115: gas readings are orders of magnitude below MaxInt64.
	KV(w, "Gas Estimate", FormatGas(int64(result.GasEstimate)))

	if len(result.EstimatedFee) > 0 {
		KV(w, "Estimated Fee", FormatCoins(result.EstimatedFee))
	} else {
		KV(w, "Estimated Fee", Dim("unknown (no --fees or --gas-prices set)"))
	}

	KV(w, "Status", StyleYellow.Render("simulated")+" "+Dim("(not broadcast)"))
}

// printSimulationResult renders a *txtypes.SimulateResponse in the requested
// output format. Without this the response falls through to the raw proto
// printer, which dumps gas_info verbatim — including the placeholder
// gas_wanted — and never shows the gas estimate the dry run was run for.
func printSimulationResult(cmd *cobra.Command, format string, sim *txtypes.SimulateResponse) error {
	result := NewSimulationResult(cmd, sim)

	switch format {
	case cflags.OutputJSON:
		return clioutput.FprintJSONSemantics(cmd.OutOrStdout(), clioutput.FormatJSON, result)
	case cflags.OutputYAML:
		return clioutput.FprintJSONSemantics(cmd.OutOrStdout(), clioutput.FormatYAML, result)
	}

	RenderSimulation(clioutput.TerminalAwareWriter(cmd.OutOrStdout()), result)

	return nil
}

// gasAdjustmentFromFlags reads --gas-adjustment, falling back to the default
// the flag itself declares when the command does not register it.
func gasAdjustmentFromFlags(cmd *cobra.Command) float64 {
	if cmd == nil {
		return cflags.DefaultGasAdjustment
	}

	adjustment, err := cmd.Flags().GetFloat64(cflags.FlagGasAdjustment)
	if err != nil || adjustment <= 0 {
		return cflags.DefaultGasAdjustment
	}

	return adjustment
}

// estimatedFee mirrors tx.Factory.BuildUnsignedTx: an explicit --fees wins
// outright, otherwise the fee is ceil(gasPrice * gasLimit) for each --gas-prices
// denom. Keeping the two in step is what makes the dry-run fee the fee a real
// broadcast would attach.
func estimatedFee(cmd *cobra.Command, gas uint64) sdk.Coins {
	if cmd == nil {
		return nil
	}

	if raw, err := cmd.Flags().GetString(cflags.FlagFees); err == nil && strings.TrimSpace(raw) != "" {
		fees, err := sdk.ParseCoinsNormalized(raw)
		if err != nil {
			return nil
		}

		return fees
	}

	raw, err := cmd.Flags().GetString(cflags.FlagGasPrices)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}

	prices, err := sdk.ParseDecCoins(raw)
	if err != nil || prices.IsZero() {
		return nil
	}

	gasDec := math.LegacyNewDecFromBigInt(new(big.Int).SetUint64(gas))

	fees := make(sdk.Coins, len(prices))
	for i, price := range prices {
		fees[i] = sdk.NewCoin(price.Denom, price.Amount.Mul(gasDec).Ceil().RoundInt())
	}

	return fees.Sort()
}
