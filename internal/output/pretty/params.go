package pretty

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

func init() {
	// Cosmos SDK modules
	Register((*stakingtypes.Params)(nil), PrettyFormatterFunc(formatStakingParams))
	Register((*govv1.QueryParamsResponse)(nil), PrettyFormatterFunc(formatGovParams))
	Register((*minttypes.Params)(nil), PrettyFormatterFunc(formatMintParams))
	Register((*slashingtypes.Params)(nil), PrettyFormatterFunc(formatSlashingParams))
	Register((*distrtypes.Params)(nil), PrettyFormatterFunc(formatDistributionParams))
	Register((*authtypes.Params)(nil), PrettyFormatterFunc(formatAuthParams))

	// Akash modules
	Register((*dvbeta.QueryParamsResponse)(nil), PrettyFormatterFunc(formatDeploymentParams))
	Register((*mvbeta.QueryParamsResponse)(nil), PrettyFormatterFunc(formatMarketParams))

	// CosmWasm
	Register((*wasmtypes.Params)(nil), PrettyFormatterFunc(formatWasmParams))

	// Oracle
	Register((*oracletypes.QueryParamsResponse)(nil), PrettyFormatterFunc(formatOracleParams))
}

// ---------------------------------------------------------------------------
// Staking
// ---------------------------------------------------------------------------

// RenderStakingParams renders staking module parameters as a pretty string.
func RenderStakingParams(p *stakingtypes.Params) string {
	const w = 16 // max: "Min Commission:" = 15
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Staking Parameters"))
	KVWidth(&buf, w, "Unbonding Time", FormatDuration(p.UnbondingTime))
	KVWidth(&buf, w, "Max Validators", fmt.Sprintf("%d", p.MaxValidators))
	KVWidth(&buf, w, "Max Entries", fmt.Sprintf("%d", p.MaxEntries))
	KVWidth(&buf, w, "History Depth", FormatHeight(int64(p.HistoricalEntries)))
	KVWidth(&buf, w, "Bond Denom", p.BondDenom)
	KVWidth(&buf, w, "Min Commission", FormatPercentDec(p.MinCommissionRate))
	return buf.String()
}

func formatStakingParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderStakingParams(msg.(*stakingtypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Governance
// ---------------------------------------------------------------------------

// RenderGovParams renders governance module parameters as a pretty string.
func RenderGovParams(res *govv1.QueryParamsResponse) string {
	// The "Expedited" and "Burn" blocks below are SubKV entries, which render
	// at SubKVKeyWidth; this column must stay SubKVIndentDelta wider or the
	// two blocks stop sharing a value column (SPEC §10.12).
	const w = KVKeyWidth
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Governance Parameters"))

	p := res.Params
	if p == nil {
		fmt.Fprintln(&buf, Dim("  (no params)"))
		return buf.String()
	}

	if p.VotingPeriod != nil {
		KVWidth(&buf, w, "Voting Period", FormatDuration(*p.VotingPeriod))
	}
	if len(p.MinDeposit) > 0 {
		KVWidth(&buf, w, "Min Deposit", FormatCoins(p.MinDeposit))
	}
	if p.MaxDepositPeriod != nil {
		KVWidth(&buf, w, "Max Deposit Pd", FormatDuration(*p.MaxDepositPeriod))
	}

	KVWidth(&buf, w, "Quorum", FormatPercent(p.Quorum))
	KVWidth(&buf, w, "Threshold", FormatPercent(p.Threshold))
	KVWidth(&buf, w, "Veto Threshold", FormatPercent(p.VetoThreshold))

	if p.ExpeditedVotingPeriod != nil && *p.ExpeditedVotingPeriod > 0 {
		Newline(&buf)
		KVHeader(&buf, "Expedited")
		SubKV(&buf, "Voting Period", FormatDuration(*p.ExpeditedVotingPeriod))
		SubKV(&buf, "Threshold", FormatPercent(p.ExpeditedThreshold))
		if len(p.ExpeditedMinDeposit) > 0 {
			SubKV(&buf, "Min Deposit", FormatCoins(p.ExpeditedMinDeposit))
		}
	}

	Newline(&buf)
	KVHeader(&buf, "Burn")
	SubKV(&buf, "Vote Quorum", FormatBool(p.BurnVoteQuorum))
	SubKV(&buf, "Deposit Prevote", FormatBool(p.BurnProposalDepositPrevote))
	SubKV(&buf, "Vote Veto", FormatBool(p.BurnVoteVeto))

	return buf.String()
}

func formatGovParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderGovParams(msg.(*govv1.QueryParamsResponse)))
	return err
}

// ---------------------------------------------------------------------------
// Minting
// ---------------------------------------------------------------------------

// RenderMintParams renders minting module parameters as a pretty string.
func RenderMintParams(p *minttypes.Params) string {
	const w = 17 // max: "Blocks Per Year:" = 16
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Minting Parameters"))
	KVWidth(&buf, w, "Denom", p.MintDenom)
	KVWidth(&buf, w, "Rate Change", FormatPercentDec(p.InflationRateChange))
	KVWidth(&buf, w, "Max Inflation", FormatPercentDec(p.InflationMax))
	KVWidth(&buf, w, "Min Inflation", FormatPercentDec(p.InflationMin))
	KVWidth(&buf, w, "Goal Bonded", FormatPercentDec(p.GoalBonded))
	KVWidth(&buf, w, "Blocks Per Year", FormatHeight(int64(p.BlocksPerYear)))
	return buf.String()
}

func formatMintParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderMintParams(msg.(*minttypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Slashing
// ---------------------------------------------------------------------------

// RenderSlashingParams renders slashing module parameters as a pretty string.
func RenderSlashingParams(p *slashingtypes.Params) string {
	const w = 18 // max: "Slash Double Sign:" = 18
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Slashing Parameters"))
	KVWidth(&buf, w, "Signed Window", FormatHeight(p.SignedBlocksWindow))
	KVWidth(&buf, w, "Min Signed/Win", FormatPercentDec(p.MinSignedPerWindow))
	KVWidth(&buf, w, "Downtime Jail", FormatDuration(p.DowntimeJailDuration))
	KVWidth(&buf, w, "Slash Dbl Sign", FormatPercentDec(p.SlashFractionDoubleSign))
	KVWidth(&buf, w, "Slash Downtime", FormatPercentDec(p.SlashFractionDowntime))
	return buf.String()
}

func formatSlashingParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderSlashingParams(msg.(*slashingtypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Distribution
// ---------------------------------------------------------------------------

// RenderDistributionParams renders distribution module parameters as a pretty string.
func RenderDistributionParams(p *distrtypes.Params) string {
	const w = 16 // max: "Withdraw Addr:" = 15
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Distribution Parameters"))
	KVWidth(&buf, w, "Community Tax", FormatPercentDec(p.CommunityTax))
	KVWidth(&buf, w, "Withdraw Addr", FormatBool(p.WithdrawAddrEnabled))
	return buf.String()
}

func formatDistributionParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDistributionParams(msg.(*distrtypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// RenderAuthParams renders auth module parameters as a pretty string.
func RenderAuthParams(p *authtypes.Params) string {
	const w = 16 // max: "Max Memo Chars:" = 15
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Auth Parameters"))
	KVWidth(&buf, w, "Max Memo Chars", FormatHeight(int64(p.MaxMemoCharacters)))
	KVWidth(&buf, w, "Tx Sig Limit", fmt.Sprintf("%d", p.TxSigLimit))
	KVWidth(&buf, w, "Tx Size/Byte", fmt.Sprintf("%d", p.TxSizeCostPerByte))
	KVWidth(&buf, w, "Verify ED25519", fmt.Sprintf("%d", p.SigVerifyCostED25519))
	KVWidth(&buf, w, "Verify Secp256k", fmt.Sprintf("%d", p.SigVerifyCostSecp256k1))
	return buf.String()
}

func formatAuthParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderAuthParams(msg.(*authtypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Deployment
// ---------------------------------------------------------------------------

// RenderDeploymentParams renders deployment module parameters as a pretty string.
func RenderDeploymentParams(res *dvbeta.QueryParamsResponse) string {
	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Deployment Parameters"))
	KVWidth(&buf, w, "Min Deposits", FormatCoins(res.Params.MinDeposits))
	return buf.String()
}

func formatDeploymentParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDeploymentParams(msg.(*dvbeta.QueryParamsResponse)))
	return err
}

// ---------------------------------------------------------------------------
// Market
// ---------------------------------------------------------------------------

// RenderMarketParams renders market module parameters as a pretty string.
func RenderMarketParams(res *mvbeta.QueryParamsResponse) string {
	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Market Parameters"))
	KVWidth(&buf, w, "Order Max Bids", fmt.Sprintf("%d", res.Params.OrderMaxBids))
	if len(res.Params.BidMinDeposits) > 0 {
		KVWidth(&buf, w, "Bid Min Deposits", FormatCoins(res.Params.BidMinDeposits))
	}
	return buf.String()
}

func formatMarketParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderMarketParams(msg.(*mvbeta.QueryParamsResponse)))
	return err
}

// ---------------------------------------------------------------------------
// WASM
// ---------------------------------------------------------------------------

// wasmAccessTypeLabel returns a human-readable label for a wasm AccessType.
func wasmAccessTypeLabel(at wasmtypes.AccessType) string {
	switch at {
	case wasmtypes.AccessTypeNobody:
		return "Nobody"
	case wasmtypes.AccessTypeEverybody:
		return "Everybody"
	case wasmtypes.AccessTypeAnyOfAddresses:
		return "Any of Addresses"
	default:
		return at.String()
	}
}

// RenderWasmParams renders wasm module parameters as a pretty string.
func RenderWasmParams(p *wasmtypes.Params) string {
	const w = 21 // max: "Instantiate Default:" = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Wasm Parameters"))
	KVWidth(&buf, w, "Code Upload Access", wasmAccessTypeLabel(p.CodeUploadAccess.Permission))
	if len(p.CodeUploadAccess.Addresses) > 0 {
		KVWidth(&buf, w, "Upload Addresses", strings.Join(p.CodeUploadAccess.Addresses, ", "))
	}
	KVWidth(&buf, w, "Instantiate Default", wasmAccessTypeLabel(p.InstantiateDefaultPermission))
	return buf.String()
}

func formatWasmParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmParams(msg.(*wasmtypes.Params)))
	return err
}

// ---------------------------------------------------------------------------
// Oracle
// ---------------------------------------------------------------------------

// RenderOracleParams renders oracle module parameters as a pretty string.
func RenderOracleParams(res *oracletypes.QueryParamsResponse) string {
	const w = 20 // max: "Min Price Sources:" = 18
	var buf strings.Builder
	p := res.Params

	fmt.Fprintln(&buf, Section("Oracle Parameters"))
	if len(p.Sources) > 0 {
		KVWidth(&buf, w, "Sources", strings.Join(p.Sources, ", "))
	}
	KVWidth(&buf, w, "Min Price Sources", fmt.Sprintf("%d", p.MinPriceSources))
	KVWidth(&buf, w, "Max Staleness", FormatDuration(p.MaxPriceStalenessPeriod))
	KVWidth(&buf, w, "TWAP Window", FormatDuration(p.TwapWindow))
	KVWidth(&buf, w, "Max Deviation", fmt.Sprintf("%d bps", p.MaxPriceDeviationBps))
	KVWidth(&buf, w, "Price Retention", FormatDuration(p.PriceRetention))
	if p.PruneEpoch != "" {
		KVWidth(&buf, w, "Prune Epoch", p.PruneEpoch)
	}
	KVWidth(&buf, w, "Max Prune/Epoch", FormatHeight(p.MaxPrunePerEpoch))
	return buf.String()
}

func formatOracleParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderOracleParams(msg.(*oracletypes.QueryParamsResponse)))
	return err
}

// ---------------------------------------------------------------------------
// RenderModuleParamsFromJSON — TUI bridge
// ---------------------------------------------------------------------------

// RenderModuleParamsFromJSON renders governance parameters for a given module
// from raw REST JSON. It produces the same visual output as the CLI Render*Params
// functions, maintaining Pretty/TUI visual parity (SPEC §10.8).
//
// For standard modules (gov, mint, staking, etc.) the JSON is the full REST
// response containing a "params" wrapper. For generic modules (deployment,
// market, transfer, ibc, crisis) the JSON is a flat key-value object
// reconstructed from the params subspace.
//
// Falls back to syntax-highlighted JSON on parse failure or unknown modules.
func RenderModuleParamsFromJSON(module string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return Dim("(no data)")
	}

	var result string
	var err error

	switch module {
	case "staking":
		result, err = renderStakingParamsJSON(raw)
	case "gov":
		result, err = renderGovParamsJSON(raw)
	case "mint":
		result, err = renderMintParamsJSON(raw)
	case "slashing":
		result, err = renderSlashingParamsJSON(raw)
	case "distribution":
		result, err = renderDistributionParamsJSON(raw)
	case "auth":
		result, err = renderAuthParamsJSON(raw)
	case "bank":
		result, err = renderBankParamsJSON(raw)
	case "deployment":
		result, err = renderDeploymentParamsJSON(raw)
	case "market":
		result, err = renderMarketParamsJSON(raw)
	case "transfer":
		result, err = renderTransferParamsJSON(raw)
	case "ibc":
		result, err = renderIBCParamsJSON(raw)
	case "crisis":
		result, err = renderCrisisParamsJSON(raw)
	default:
		err = fmt.Errorf("unknown module")
	}

	if err != nil {
		return fallbackHighlightedJSON(raw)
	}
	return result
}

// fallbackHighlightedJSON renders raw JSON with syntax highlighting.
func fallbackHighlightedJSON(raw json.RawMessage) string {
	var buf strings.Builder
	if err := WriteHighlightedJSON(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// JSON → pretty renderers for TUI (one per module)
//
// Each function unmarshals the REST JSON into simple Go structs (with json
// tags matching the REST response format) and renders using the same Section,
// KV, and Format* helpers as the typed Render* functions above.
// ---------------------------------------------------------------------------

func renderStakingParamsJSON(raw json.RawMessage) (string, error) {
	const w = 16
	var resp struct {
		Params struct {
			UnbondingTime     string `json:"unbonding_time"`
			MaxValidators     int    `json:"max_validators"`
			MaxEntries        int    `json:"max_entries"`
			HistoricalEntries int    `json:"historical_entries"`
			BondDenom         string `json:"bond_denom"`
			MinCommissionRate string `json:"min_commission_rate"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Staking Parameters"))
	KVWidth(&buf, w, "Unbonding Time", FormatDurationString(p.UnbondingTime))
	KVWidth(&buf, w, "Max Validators", fmt.Sprintf("%d", p.MaxValidators))
	KVWidth(&buf, w, "Max Entries", fmt.Sprintf("%d", p.MaxEntries))
	KVWidth(&buf, w, "History Depth", FormatHeight(int64(p.HistoricalEntries)))
	KVWidth(&buf, w, "Bond Denom", p.BondDenom)
	KVWidth(&buf, w, "Min Commission", FormatPercent(p.MinCommissionRate))
	return buf.String(), nil
}

func renderGovParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			VotingPeriod               string          `json:"voting_period"`
			MinDeposit                 json.RawMessage `json:"min_deposit"`
			MaxDepositPeriod           string          `json:"max_deposit_period"`
			Quorum                     string          `json:"quorum"`
			Threshold                  string          `json:"threshold"`
			VetoThreshold              string          `json:"veto_threshold"`
			ExpeditedVotingPeriod      string          `json:"expedited_voting_period"`
			ExpeditedThreshold         string          `json:"expedited_threshold"`
			ExpeditedMinDeposit        json.RawMessage `json:"expedited_min_deposit"`
			BurnVoteQuorum             bool            `json:"burn_vote_quorum"`
			BurnProposalDepositPrevote bool            `json:"burn_proposal_deposit_prevote"`
			BurnVoteVeto               bool            `json:"burn_vote_veto"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	// The "Expedited" and "Burn" blocks below are SubKV entries, which render
	// at SubKVKeyWidth; this column must stay SubKVIndentDelta wider or the
	// two blocks stop sharing a value column (SPEC §10.12).
	const w = KVKeyWidth
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Governance Parameters"))
	KVWidth(&buf, w, "Voting Period", FormatDurationString(p.VotingPeriod))
	KVWidth(&buf, w, "Min Deposit", formatCoinsJSON(p.MinDeposit))
	KVWidth(&buf, w, "Max Deposit Pd", FormatDurationString(p.MaxDepositPeriod))
	KVWidth(&buf, w, "Quorum", FormatPercent(p.Quorum))
	KVWidth(&buf, w, "Threshold", FormatPercent(p.Threshold))
	KVWidth(&buf, w, "Veto Threshold", FormatPercent(p.VetoThreshold))

	if p.ExpeditedVotingPeriod != "" && p.ExpeditedVotingPeriod != "0s" {
		Newline(&buf)
		KVHeader(&buf, "Expedited")
		SubKV(&buf, "Voting Period", FormatDurationString(p.ExpeditedVotingPeriod))
		SubKV(&buf, "Threshold", FormatPercent(p.ExpeditedThreshold))
		SubKV(&buf, "Min Deposit", formatCoinsJSON(p.ExpeditedMinDeposit))
	}

	Newline(&buf)
	KVHeader(&buf, "Burn")
	SubKV(&buf, "Vote Quorum", FormatBool(p.BurnVoteQuorum))
	SubKV(&buf, "Deposit Prevote", FormatBool(p.BurnProposalDepositPrevote))
	SubKV(&buf, "Vote Veto", FormatBool(p.BurnVoteVeto))
	return buf.String(), nil
}

func renderMintParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			MintDenom           string `json:"mint_denom"`
			InflationRateChange string `json:"inflation_rate_change"`
			InflationMax        string `json:"inflation_max"`
			InflationMin        string `json:"inflation_min"`
			GoalBonded          string `json:"goal_bonded"`
			BlocksPerYear       int64  `json:"blocks_per_year,string"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	const w = 17
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Minting Parameters"))
	KVWidth(&buf, w, "Denom", p.MintDenom)
	KVWidth(&buf, w, "Rate Change", FormatPercent(p.InflationRateChange))
	KVWidth(&buf, w, "Max Inflation", FormatPercent(p.InflationMax))
	KVWidth(&buf, w, "Min Inflation", FormatPercent(p.InflationMin))
	KVWidth(&buf, w, "Goal Bonded", FormatPercent(p.GoalBonded))
	KVWidth(&buf, w, "Blocks Per Year", FormatHeight(p.BlocksPerYear))
	return buf.String(), nil
}

func renderSlashingParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			SignedBlocksWindow      int64  `json:"signed_blocks_window,string"`
			MinSignedPerWindow      string `json:"min_signed_per_window"`
			DowntimeJailDuration    string `json:"downtime_jail_duration"`
			SlashFractionDoubleSign string `json:"slash_fraction_double_sign"`
			SlashFractionDowntime   string `json:"slash_fraction_downtime"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	const w = 18
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Slashing Parameters"))
	KVWidth(&buf, w, "Signed Window", FormatHeight(p.SignedBlocksWindow))
	KVWidth(&buf, w, "Min Signed/Win", FormatPercent(p.MinSignedPerWindow))
	KVWidth(&buf, w, "Downtime Jail", FormatDurationString(p.DowntimeJailDuration))
	KVWidth(&buf, w, "Slash Dbl Sign", FormatPercent(p.SlashFractionDoubleSign))
	KVWidth(&buf, w, "Slash Downtime", FormatPercent(p.SlashFractionDowntime))
	return buf.String(), nil
}

func renderDistributionParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			CommunityTax        string `json:"community_tax"`
			WithdrawAddrEnabled bool   `json:"withdraw_addr_enabled"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	const w = 16
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Distribution Parameters"))
	KVWidth(&buf, w, "Community Tax", FormatPercent(p.CommunityTax))
	KVWidth(&buf, w, "Withdraw Addr", FormatBool(p.WithdrawAddrEnabled))
	return buf.String(), nil
}

func renderAuthParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			MaxMemoCharacters      int64 `json:"max_memo_characters,string"`
			TxSigLimit             int64 `json:"tx_sig_limit,string"`
			TxSizeCostPerByte      int64 `json:"tx_size_cost_per_byte,string"`
			SigVerifyCostED25519   int64 `json:"sig_verify_cost_ed25519,string"`
			SigVerifyCostSecp256k1 int64 `json:"sig_verify_cost_secp256k1,string"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	const w = 16
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Auth Parameters"))
	KVWidth(&buf, w, "Max Memo Chars", FormatHeight(p.MaxMemoCharacters))
	KVWidth(&buf, w, "Tx Sig Limit", fmt.Sprintf("%d", p.TxSigLimit))
	KVWidth(&buf, w, "Tx Size/Byte", fmt.Sprintf("%d", p.TxSizeCostPerByte))
	KVWidth(&buf, w, "Verify ED25519", fmt.Sprintf("%d", p.SigVerifyCostED25519))
	KVWidth(&buf, w, "Verify Secp256k", fmt.Sprintf("%d", p.SigVerifyCostSecp256k1))
	return buf.String(), nil
}

func renderBankParamsJSON(raw json.RawMessage) (string, error) {
	var resp struct {
		Params struct {
			SendEnabled        json.RawMessage `json:"send_enabled"`
			DefaultSendEnabled bool            `json:"default_send_enabled"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	p := resp.Params

	const w = 16
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Bank Parameters"))
	KVWidth(&buf, w, "Default Send", FormatBool(p.DefaultSendEnabled))
	return buf.String(), nil
}

// Generic module JSON renderers (deployment, market, transfer, ibc, crisis).
// These modules use the x/params subspace and the REST JSON is a flat
// key-value object like {"MinDeposits": "[...]", "OrderMaxBids": "20"}.

func renderDeploymentParamsJSON(raw json.RawMessage) (string, error) {
	kv, err := parseGenericParamsJSON(raw)
	if err != nil {
		return "", err
	}

	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Deployment Parameters"))
	if v, ok := kv["MinDeposits"]; ok {
		KVWidth(&buf, w, "Min Deposits", formatGenericCoins(v))
	}
	return buf.String(), nil
}

func renderMarketParamsJSON(raw json.RawMessage) (string, error) {
	kv, err := parseGenericParamsJSON(raw)
	if err != nil {
		return "", err
	}

	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Market Parameters"))
	if v, ok := kv["OrderMaxBids"]; ok {
		KVWidth(&buf, w, "Order Max Bids", strings.Trim(v, "\""))
	}
	if v, ok := kv["BidMinDeposits"]; ok {
		KVWidth(&buf, w, "Bid Min Deposits", formatGenericCoins(v))
	}
	return buf.String(), nil
}

func renderTransferParamsJSON(raw json.RawMessage) (string, error) {
	kv, err := parseGenericParamsJSON(raw)
	if err != nil {
		return "", err
	}

	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Transfer Parameters"))
	if v, ok := kv["SendEnabled"]; ok {
		KVWidth(&buf, w, "Send Enabled", formatGenericBool(v))
	}
	if v, ok := kv["ReceiveEnabled"]; ok {
		KVWidth(&buf, w, "Receive Enabled", formatGenericBool(v))
	}
	return buf.String(), nil
}

func renderIBCParamsJSON(raw json.RawMessage) (string, error) {
	kv, err := parseGenericParamsJSON(raw)
	if err != nil {
		return "", err
	}

	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("IBC Parameters"))
	if v, ok := kv["AllowedClients"]; ok {
		KVWidth(&buf, w, "Allowed Clients", formatGenericStringList(v))
	}
	return buf.String(), nil
}

func renderCrisisParamsJSON(raw json.RawMessage) (string, error) {
	kv, err := parseGenericParamsJSON(raw)
	if err != nil {
		return "", err
	}

	const w = 20
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Crisis Parameters"))
	if v, ok := kv["ConstantFee"]; ok {
		KVWidth(&buf, w, "Constant Fee", formatGenericCoin(v))
	}
	return buf.String(), nil
}

// ---------------------------------------------------------------------------
// JSON helpers for TUI bridge
// ---------------------------------------------------------------------------

// parseGenericParamsJSON parses a flat key-value JSON object (from the x/params
// subspace) where values are JSON-encoded strings.
func parseGenericParamsJSON(raw json.RawMessage) (map[string]string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = string(v)
	}
	return result, nil
}

// formatCoinsJSON parses a JSON array of coins and formats them via FormatCoins.
func formatCoinsJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return "0"
	}

	var coins sdk.Coins
	if err := json.Unmarshal(raw, &coins); err != nil {
		return string(raw)
	}
	return FormatCoins(coins)
}

// formatGenericCoins parses a JSON-encoded string value containing a coin array
// (from x/params subspace, values are stored as JSON strings).
func formatGenericCoins(s string) string {
	// The value might be a JSON string containing a JSON array, or
	// it might be a direct JSON array. Try both.
	unquoted := strings.Trim(s, "\"")
	// Try unquoting escaped JSON.
	var inner string
	if err := json.Unmarshal([]byte(s), &inner); err == nil {
		unquoted = inner
	}

	var coins sdk.Coins
	if err := json.Unmarshal([]byte(unquoted), &coins); err != nil {
		return strings.Trim(s, "\"")
	}
	return FormatCoins(coins)
}

// formatGenericCoin parses a JSON-encoded string value containing a single coin.
func formatGenericCoin(s string) string {
	unquoted := strings.Trim(s, "\"")
	var inner string
	if err := json.Unmarshal([]byte(s), &inner); err == nil {
		unquoted = inner
	}

	var coin sdk.Coin
	if err := json.Unmarshal([]byte(unquoted), &coin); err != nil {
		return strings.Trim(s, "\"")
	}
	return FormatCoin(coin)
}

// formatGenericBool formats a JSON-encoded boolean string from x/params.
func formatGenericBool(s string) string {
	clean := strings.Trim(s, "\"")
	switch clean {
	case "true":
		return FormatBool(true)
	case "false":
		return FormatBool(false)
	default:
		return clean
	}
}

// formatGenericStringList formats a JSON-encoded string array from x/params.
func formatGenericStringList(s string) string {
	unquoted := strings.Trim(s, "\"")
	var inner string
	if err := json.Unmarshal([]byte(s), &inner); err == nil {
		unquoted = inner
	}

	var items []string
	if err := json.Unmarshal([]byte(unquoted), &items); err != nil {
		return strings.Trim(s, "\"")
	}
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}
