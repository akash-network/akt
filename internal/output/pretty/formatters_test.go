package pretty

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	feegrant "cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	atypes "pkg.akt.dev/go/node/audit/v1"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oraclev2 "pkg.akt.dev/go/node/oracle/v2"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/codec"
)

// dec is a shorthand for a populated LegacyDec fixture. Several chain params
// carry LegacyDec fields whose zero value has a nil big.Int, which the
// renderers cannot operate on; the wire never produces those.
func dec(s string) math.LegacyDec { return math.LegacyMustNewDecFromStr(s) }

// TestRegisteredFormattersDispatch walks the pretty-print registry the way the
// CLI does: look the response type up by its proto name and render it. This is
// the test that fails if a formatter is registered under the wrong message
// type or an init() is dropped in a refactor — both of which are silent in
// production, because an unregistered response falls back to raw JSON.
func TestRegisteredFormattersDispatch(t *testing.T) {
	enc := codec.MakeEncodingConfig()
	cctx := sdkclient.Context{}.
		WithCodec(enc.Codec).
		WithInterfaceRegistry(enc.InterfaceRegistry)

	cmd := &cobra.Command{}
	cmd.Flags().String("output", "pretty", "")

	// Empty (but non-nil) responses: every list formatter must render its
	// header without a live chain, and must not panic on an empty result.
	messages := []proto.Message{
		&dvbeta.QueryDeploymentsResponse{},
		&dvbeta.QueryDeploymentResponse{},
		&dvbeta.QueryGroupResponse{},
		&dvbeta.QueryParamsResponse{},
		&mvbeta.QueryBidsResponse{},
		&mvbeta.QueryBidResponse{},
		&mvbeta.QueryLeasesResponse{},
		&mvbeta.QueryLeaseResponse{},
		&mvbeta.QueryOrdersResponse{},
		&mvbeta.QueryOrderResponse{},
		&mvbeta.QueryParamsResponse{},
		&ptypes.QueryProvidersResponse{},
		&ptypes.QueryProviderResponse{},
		&ctypes.QueryCertificatesResponse{},
		&atypes.QueryProvidersResponse{},
		&etypes.QueryAccountsResponse{},
		&etypes.QueryPaymentsResponse{},
		&banktypes.QueryAllBalancesResponse{},
		&banktypes.QuerySpendableBalancesResponse{},
		&banktypes.QueryBalanceResponse{},
		&banktypes.QueryTotalSupplyResponse{},
		&feegrant.QueryAllowancesResponse{},
		&feegrant.QueryAllowancesByGranterResponse{},
		&govv1.QueryProposalsResponse{},
		&distrtypes.QueryDelegationTotalRewardsResponse{},
		&distrtypes.ValidatorAccumulatedCommission{},
		&slashingtypes.QuerySigningInfosResponse{},
		&slashingtypes.ValidatorSigningInfo{},
		&upgradetypes.QueryModuleVersionsResponse{},
		&upgradetypes.Plan{},

		// Staking, gov, and module params.
		&stakingtypes.QueryValidatorsResponse{},
		&stakingtypes.QueryDelegatorDelegationsResponse{},
		// Pool carries math.Int fields; a zero value leaves them nil, which
		// the renderer cannot divide. Populate them as the chain would.
		&stakingtypes.Pool{
			BondedTokens:    math.NewInt(1_000_000),
			NotBondedTokens: math.NewInt(250_000),
		},
		&stakingtypes.Params{MinCommissionRate: dec("0.05")},
		&govv1.QueryParamsResponse{},
		&minttypes.Params{
			InflationRateChange: dec("0.13"),
			InflationMax:        dec("0.2"),
			InflationMin:        dec("0.07"),
			GoalBonded:          dec("0.67"),
		},
		&slashingtypes.Params{
			MinSignedPerWindow:      dec("0.5"),
			SlashFractionDoubleSign: dec("0.05"),
			SlashFractionDowntime:   dec("0.0001"),
		},
		&distrtypes.Params{
			CommunityTax:        dec("0.02"),
			BaseProposerReward:  dec("0.01"),
			BonusProposerReward: dec("0.04"),
			WithdrawAddrEnabled: true,
		},
		&authtypes.Params{},
		&wasmtypes.Params{},

		// Oracle / BME.
		&oraclev2.QueryParamsResponse{},
		&oraclev2.QueryPricesResponse{},
		&oraclev2.QueryAggregatedPriceResponse{},
		&bmetypes.QueryStatusResponse{},
		&bmetypes.QueryVaultStateResponse{},
		&bmetypes.QueryLedgerRecordsResponse{},

		// CosmWasm.
		&wasmtypes.QueryCodesResponse{},
		&wasmtypes.QueryContractsByCodeResponse{},
		&wasmtypes.QueryContractHistoryResponse{},
		&wasmtypes.QueryPinnedCodesResponse{},
		&wasmtypes.QueryContractsByCreatorResponse{},

		// Auth.
		&authtypes.QueryAccountsResponse{},
		&authtypes.QueryModuleAccountsResponse{},
	}

	for _, msg := range messages {
		name := proto.MessageName(msg)

		t.Run(name, func(t *testing.T) {
			f, ok := Lookup(msg)
			if !ok {
				t.Fatalf("no pretty formatter registered for %s; output would fall back to raw JSON", name)
			}

			var buf bytes.Buffer
			if err := f.Format(&buf, cmd, cctx, msg); err != nil {
				t.Fatalf("format %s: %v", name, err)
			}
		})
	}
}

// TestLookupMissesUnregisteredTypes is the negative control for the dispatch
// test above: Lookup must report a miss rather than returning some other
// module's formatter (which would render nonsense for the wrong response).
func TestLookupMissesUnregisteredTypes(t *testing.T) {
	// A well-known proto that pretty deliberately does not format.
	if _, ok := Lookup(&banktypes.QueryDenomMetadataResponse{}); ok {
		t.Error("an unregistered response type must miss, not resolve to another formatter")
	}
}

// TestFormatDecAsAKTScalesMicroDenoms is a money-display test. Chain amounts
// arrive in uakt; the AKT/mAKT/uAKT selection decides which decimal point the
// user reads. A wrong bucket boundary misreports a balance by 1000x.
func TestFormatDecAsAKTScalesMicroDenoms(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0", "0 AKT"},
		{"1", "1 uAKT"},
		{"999", "999 uAKT"},
		{"1000", "1 mAKT"},
		{"999999", "999.999 mAKT"},
		{"1000000", "1 AKT"},
		{"1500000", "1.5 AKT"},
		{"-1500000", "-1.5 AKT"},
		{"-999", "-999 uAKT"},
	}

	for _, c := range cases {
		got := FormatDecAsAKT(math.LegacyMustNewDecFromStr(c.in))
		if got != c.want {
			t.Errorf("FormatDecAsAKT(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatDecAmountHonorsDenom covers both arms of FormatDecAmount: a
// micro-denom is scaled and re-symbolized, while a non-micro denom (an IBC
// hash, a bare "akt") must be shown verbatim — scaling it would invent value
// that is not there.
func TestFormatDecAmountHonorsDenom(t *testing.T) {
	amount := math.LegacyMustNewDecFromStr("2500000")

	if got := FormatDecAmount(amount, "uatom"); got != "2.5 ATOM" {
		t.Errorf("uatom = %q, want 2.5 ATOM", got)
	}

	for _, denom := range []string{"akt", "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2", "u"} {
		got := FormatDecAmount(amount, denom)
		if !strings.HasSuffix(got, denom) {
			t.Errorf("denom %q rendered as %q; non-micro denoms must be shown as-is", denom, got)
		}
		if strings.Contains(got, "2.5 ") {
			t.Errorf("denom %q was scaled to %q; only micro-denoms may be scaled", denom, got)
		}
	}
}

// TestIsMicroDenomBoundaries pins the classifier that decides whether an
// amount is divided by a million. "u" alone, "u/..." and uppercase forms must
// not be treated as micro-denoms.
func TestIsMicroDenomBoundaries(t *testing.T) {
	cases := map[string]bool{
		"uakt":  true,
		"uatom": true,
		"uosmo": true,
		"u":     false, // no base symbol
		"":      false,
		"akt":   false,
		"u/abc": false, // not a denom symbol
		"UAKT":  false, // uppercase is not the convention
		"u1":    false, // digit after the prefix
		"ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2": false,
	}

	for denom, want := range cases {
		if got := isMicroDenom(denom); got != want {
			t.Errorf("isMicroDenom(%q) = %v, want %v", denom, got, want)
		}
	}
}

// TestFormatBytesBinaryUnits pins the byte formatter used for provider memory
// and storage. It uses binary units, so the boundaries are powers of 1024, not
// 1000; an off-by-one at a boundary shows "1024Mi" instead of "1Gi".
func TestFormatBytesBinaryUnits(t *testing.T) {
	const (
		ki = uint64(1024)
		mi = 1024 * ki
		gi = 1024 * mi
		ti = 1024 * gi
	)

	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{mi - 1, "1048575"},
		{mi, "1Mi"},
		{512 * mi, "512Mi"},
		{gi, "1Gi"},
		{ti, "1Ti"},
	}

	for _, c := range cases {
		if got := FormatBytes(c.in); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResourceRatiosHandleZeroTotal covers the divide-by-nothing guard on both
// ratio helpers: a provider reporting no capacity must render "-" rather than
// "0/0", which reads as a real (and full) machine.
func TestResourceRatiosHandleZeroTotal(t *testing.T) {
	if got := FormatMemoryRatio(0, 0); got != "-" {
		t.Errorf("FormatMemoryRatio(0,0) = %q, want -", got)
	}
	if got := FormatResourceRatio(0, 0); got != "-" {
		t.Errorf("FormatResourceRatio(0,0) = %q, want -", got)
	}

	if got := FormatMemoryRatio(512*1024*1024, 1024*1024*1024); got != "512Mi/1Gi" {
		t.Errorf("FormatMemoryRatio = %q, want 512Mi/1Gi", got)
	}
	if got := FormatResourceRatio(3, 8); got != "3/8" {
		t.Errorf("FormatResourceRatio = %q, want 3/8", got)
	}
}

// TestFormatNumberAndPower covers the two numeric display helpers shared with
// the TUI: thousands separators for exact counts, and compact suffixes for
// voting power.
func TestFormatNumberAndPower(t *testing.T) {
	numbers := map[int64]string{
		0:        "0",
		999:      "999",
		1000:     "1,000",
		18234567: "18,234,567",
		-1234:    "-1,234",
	}
	for in, want := range numbers {
		if got := FormatNumber(in); got != want {
			t.Errorf("FormatNumber(%d) = %q, want %q", in, got, want)
		}
	}

	powers := map[int64]string{
		0:             "0",
		999:           "999",
		2500:          "2.5K",
		1_500_000:     "1.5M",
		2_500_000_000: "2.5B",
	}
	for in, want := range powers {
		if got := FormatPower(in); got != want {
			t.Errorf("FormatPower(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestFormatShortDuration covers the block-time formatter's three ranges. It
// is read live during upgrades (`akt monitor`), where a wrong unit makes a
// stalled chain look healthy.
func TestFormatShortDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{350 * time.Millisecond, "350ms"},
		{999 * time.Millisecond, "999ms"},
		{time.Second, "1.0s"},
		{3500 * time.Millisecond, "3.5s"},
		{59 * time.Second, "59.0s"},
		{90 * time.Second, "1m30s"},
	}

	for _, c := range cases {
		if got := FormatShortDuration(c.in); got != c.want {
			t.Errorf("FormatShortDuration(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
