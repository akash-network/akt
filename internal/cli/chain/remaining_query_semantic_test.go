package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"testing"
	"time"

	"cosmossdk.io/math"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdked25519 "github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/authz"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

type remainingQueryRecorder struct {
	method      string
	request     interface{}
	err         error
	nilResponse bool
}

func (recorder *remainingQueryRecorder) record(method string, request interface{}) bool {
	recorder.method = method
	recorder.request = request
	return recorder.nilResponse
}

type remainingDistributionQuery struct {
	distributiontypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingDistributionQuery) Params(_ context.Context, request *distributiontypes.QueryParamsRequest, _ ...grpc.CallOption) (*distributiontypes.QueryParamsResponse, error) {
	if query.record("distribution-params", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryParamsResponse{Params: distributiontypes.DefaultParams()}, nil
}

func (query *remainingDistributionQuery) ValidatorDistributionInfo(_ context.Context, request *distributiontypes.QueryValidatorDistributionInfoRequest, _ ...grpc.CallOption) (*distributiontypes.QueryValidatorDistributionInfoResponse, error) {
	if query.record("validator-distribution-info", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryValidatorDistributionInfoResponse{OperatorAddress: request.ValidatorAddress}, nil
}

func (query *remainingDistributionQuery) ValidatorOutstandingRewards(_ context.Context, request *distributiontypes.QueryValidatorOutstandingRewardsRequest, _ ...grpc.CallOption) (*distributiontypes.QueryValidatorOutstandingRewardsResponse, error) {
	if query.record("validator-outstanding-rewards", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryValidatorOutstandingRewardsResponse{}, nil
}

func (query *remainingDistributionQuery) ValidatorCommission(_ context.Context, request *distributiontypes.QueryValidatorCommissionRequest, _ ...grpc.CallOption) (*distributiontypes.QueryValidatorCommissionResponse, error) {
	if query.record("validator-commission", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryValidatorCommissionResponse{}, nil
}

func (query *remainingDistributionQuery) ValidatorSlashes(_ context.Context, request *distributiontypes.QueryValidatorSlashesRequest, _ ...grpc.CallOption) (*distributiontypes.QueryValidatorSlashesResponse, error) {
	if query.record("validator-slashes", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryValidatorSlashesResponse{}, nil
}

func (query *remainingDistributionQuery) DelegationRewards(_ context.Context, request *distributiontypes.QueryDelegationRewardsRequest, _ ...grpc.CallOption) (*distributiontypes.QueryDelegationRewardsResponse, error) {
	if query.record("delegation-rewards", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryDelegationRewardsResponse{}, nil
}

func (query *remainingDistributionQuery) DelegationTotalRewards(_ context.Context, request *distributiontypes.QueryDelegationTotalRewardsRequest, _ ...grpc.CallOption) (*distributiontypes.QueryDelegationTotalRewardsResponse, error) {
	if query.record("delegation-total-rewards", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryDelegationTotalRewardsResponse{}, nil
}

func (query *remainingDistributionQuery) CommunityPool(_ context.Context, request *distributiontypes.QueryCommunityPoolRequest, _ ...grpc.CallOption) (*distributiontypes.QueryCommunityPoolResponse, error) {
	if query.record("community-pool", request) || query.err != nil {
		return nil, query.err
	}
	return &distributiontypes.QueryCommunityPoolResponse{}, nil
}

type remainingAuthzQuery struct {
	authz.QueryClient
	remainingQueryRecorder
}

func (query *remainingAuthzQuery) Grants(_ context.Context, request *authz.QueryGrantsRequest, _ ...grpc.CallOption) (*authz.QueryGrantsResponse, error) {
	if query.record("authz-grants", request) || query.err != nil {
		return nil, query.err
	}
	return &authz.QueryGrantsResponse{}, nil
}

func (query *remainingAuthzQuery) GranterGrants(_ context.Context, request *authz.QueryGranterGrantsRequest, _ ...grpc.CallOption) (*authz.QueryGranterGrantsResponse, error) {
	if query.record("authz-granter-grants", request) || query.err != nil {
		return nil, query.err
	}
	return &authz.QueryGranterGrantsResponse{}, nil
}

func (query *remainingAuthzQuery) GranteeGrants(_ context.Context, request *authz.QueryGranteeGrantsRequest, _ ...grpc.CallOption) (*authz.QueryGranteeGrantsResponse, error) {
	if query.record("authz-grantee-grants", request) || query.err != nil {
		return nil, query.err
	}
	return &authz.QueryGranteeGrantsResponse{}, nil
}

type remainingFeegrantQuery struct {
	feegrant.QueryClient
	remainingQueryRecorder
}

func (query *remainingFeegrantQuery) Allowance(_ context.Context, request *feegrant.QueryAllowanceRequest, _ ...grpc.CallOption) (*feegrant.QueryAllowanceResponse, error) {
	if query.record("feegrant-allowance", request) || query.err != nil {
		return nil, query.err
	}
	return &feegrant.QueryAllowanceResponse{Allowance: &feegrant.Grant{Granter: request.Granter, Grantee: request.Grantee}}, nil
}

func (query *remainingFeegrantQuery) Allowances(_ context.Context, request *feegrant.QueryAllowancesRequest, _ ...grpc.CallOption) (*feegrant.QueryAllowancesResponse, error) {
	if query.record("feegrant-allowances", request) || query.err != nil {
		return nil, query.err
	}
	return &feegrant.QueryAllowancesResponse{}, nil
}

func (query *remainingFeegrantQuery) AllowancesByGranter(_ context.Context, request *feegrant.QueryAllowancesByGranterRequest, _ ...grpc.CallOption) (*feegrant.QueryAllowancesByGranterResponse, error) {
	if query.record("feegrant-allowances-by-granter", request) || query.err != nil {
		return nil, query.err
	}
	return &feegrant.QueryAllowancesByGranterResponse{}, nil
}

type remainingBMEQuery struct {
	bmetypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingBMEQuery) Params(_ context.Context, request *bmetypes.QueryParamsRequest, _ ...grpc.CallOption) (*bmetypes.QueryParamsResponse, error) {
	if query.record("bme-params", request) || query.err != nil {
		return nil, query.err
	}
	return &bmetypes.QueryParamsResponse{Params: bmetypes.DefaultParams()}, nil
}

func (query *remainingBMEQuery) VaultState(_ context.Context, request *bmetypes.QueryVaultStateRequest, _ ...grpc.CallOption) (*bmetypes.QueryVaultStateResponse, error) {
	if query.record("bme-vault-state", request) || query.err != nil {
		return nil, query.err
	}
	return &bmetypes.QueryVaultStateResponse{}, nil
}

func (query *remainingBMEQuery) Status(_ context.Context, request *bmetypes.QueryStatusRequest, _ ...grpc.CallOption) (*bmetypes.QueryStatusResponse, error) {
	if query.record("bme-status", request) || query.err != nil {
		return nil, query.err
	}
	return &bmetypes.QueryStatusResponse{}, nil
}

func (query *remainingBMEQuery) LedgerRecords(_ context.Context, request *bmetypes.QueryLedgerRecordsRequest, _ ...grpc.CallOption) (*bmetypes.QueryLedgerRecordsResponse, error) {
	if query.record("bme-ledger", request) || query.err != nil {
		return nil, query.err
	}
	return &bmetypes.QueryLedgerRecordsResponse{}, nil
}

type remainingMintQuery struct {
	minttypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingMintQuery) Params(_ context.Context, request *minttypes.QueryParamsRequest, _ ...grpc.CallOption) (*minttypes.QueryParamsResponse, error) {
	if query.record("mint-params", request) || query.err != nil {
		return nil, query.err
	}
	return &minttypes.QueryParamsResponse{Params: minttypes.DefaultParams()}, nil
}

func (query *remainingMintQuery) Inflation(_ context.Context, request *minttypes.QueryInflationRequest, _ ...grpc.CallOption) (*minttypes.QueryInflationResponse, error) {
	if query.record("mint-inflation", request) || query.err != nil {
		return nil, query.err
	}
	return &minttypes.QueryInflationResponse{Inflation: math.LegacyMustNewDecFromStr("0.125")}, nil
}

func (query *remainingMintQuery) AnnualProvisions(_ context.Context, request *minttypes.QueryAnnualProvisionsRequest, _ ...grpc.CallOption) (*minttypes.QueryAnnualProvisionsResponse, error) {
	if query.record("mint-annual-provisions", request) || query.err != nil {
		return nil, query.err
	}
	return &minttypes.QueryAnnualProvisionsResponse{AnnualProvisions: math.LegacyMustNewDecFromStr("123.5")}, nil
}

type remainingOracleQuery struct {
	oracletypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingOracleQuery) Prices(_ context.Context, request *oracletypes.QueryPricesRequest, _ ...grpc.CallOption) (*oracletypes.QueryPricesResponse, error) {
	if query.record("oracle-prices", request) || query.err != nil {
		return nil, query.err
	}
	return &oracletypes.QueryPricesResponse{}, nil
}

func (query *remainingOracleQuery) AggregatedPrice(_ context.Context, request *oracletypes.QueryAggregatedPriceRequest, _ ...grpc.CallOption) (*oracletypes.QueryAggregatedPriceResponse, error) {
	if query.record("oracle-aggregated-price", request) || query.err != nil {
		return nil, query.err
	}
	return &oracletypes.QueryAggregatedPriceResponse{}, nil
}

func (query *remainingOracleQuery) Params(_ context.Context, request *oracletypes.QueryParamsRequest, _ ...grpc.CallOption) (*oracletypes.QueryParamsResponse, error) {
	if query.record("oracle-params", request) || query.err != nil {
		return nil, query.err
	}
	return &oracletypes.QueryParamsResponse{}, nil
}

type remainingParamsQuery struct {
	paramstypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingParamsQuery) Params(_ context.Context, request *paramstypes.QueryParamsRequest, _ ...grpc.CallOption) (*paramstypes.QueryParamsResponse, error) {
	if query.record("params-subspace", request) || query.err != nil {
		return nil, query.err
	}
	return &paramstypes.QueryParamsResponse{Param: paramstypes.ParamChange{Subspace: request.Subspace, Key: request.Key, Value: `"value"`}}, nil
}

type remainingSlashingQuery struct {
	slashingtypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingSlashingQuery) SigningInfo(_ context.Context, request *slashingtypes.QuerySigningInfoRequest, _ ...grpc.CallOption) (*slashingtypes.QuerySigningInfoResponse, error) {
	if query.record("slashing-signing-info", request) || query.err != nil {
		return nil, query.err
	}
	return &slashingtypes.QuerySigningInfoResponse{}, nil
}

func (query *remainingSlashingQuery) SigningInfos(_ context.Context, request *slashingtypes.QuerySigningInfosRequest, _ ...grpc.CallOption) (*slashingtypes.QuerySigningInfosResponse, error) {
	if query.record("slashing-signing-infos", request) || query.err != nil {
		return nil, query.err
	}
	return &slashingtypes.QuerySigningInfosResponse{}, nil
}

func (query *remainingSlashingQuery) Params(_ context.Context, request *slashingtypes.QueryParamsRequest, _ ...grpc.CallOption) (*slashingtypes.QueryParamsResponse, error) {
	if query.record("slashing-params", request) || query.err != nil {
		return nil, query.err
	}
	return &slashingtypes.QueryParamsResponse{Params: slashingtypes.DefaultParams()}, nil
}

type remainingEvidenceQuery struct {
	evidencetypes.QueryClient
	remainingQueryRecorder
}

func (query *remainingEvidenceQuery) Evidence(_ context.Context, request *evidencetypes.QueryEvidenceRequest, _ ...grpc.CallOption) (*evidencetypes.QueryEvidenceResponse, error) {
	if query.record("evidence-one", request) || query.err != nil {
		return nil, query.err
	}
	packed, err := types.NewAnyWithValue(&evidencetypes.Equivocation{})
	if err != nil {
		return nil, err
	}
	return &evidencetypes.QueryEvidenceResponse{Evidence: packed}, nil
}

func (query *remainingEvidenceQuery) AllEvidence(_ context.Context, request *evidencetypes.QueryAllEvidenceRequest, _ ...grpc.CallOption) (*evidencetypes.QueryAllEvidenceResponse, error) {
	if query.record("evidence-all", request) || query.err != nil {
		return nil, query.err
	}
	return &evidencetypes.QueryAllEvidenceResponse{}, nil
}

type remainingQueryAggregate struct {
	clientv1beta3.QueryClient
	distribution distributiontypes.QueryClient
	authz        authz.QueryClient
	feegrant     feegrant.QueryClient
	bme          bmetypes.QueryClient
	mint         minttypes.QueryClient
	oracle       oracletypes.QueryClient
	params       paramstypes.QueryClient
	slashing     slashingtypes.QueryClient
	evidence     evidencetypes.QueryClient
}

func (query *remainingQueryAggregate) Distribution() distributiontypes.QueryClient {
	return query.distribution
}
func (query *remainingQueryAggregate) Authz() authz.QueryClient            { return query.authz }
func (query *remainingQueryAggregate) Feegrant() feegrant.QueryClient      { return query.feegrant }
func (query *remainingQueryAggregate) BME() bmetypes.QueryClient           { return query.bme }
func (query *remainingQueryAggregate) Mint() minttypes.QueryClient         { return query.mint }
func (query *remainingQueryAggregate) Oracle() oracletypes.QueryClient     { return query.oracle }
func (query *remainingQueryAggregate) Params() paramstypes.QueryClient     { return query.params }
func (query *remainingQueryAggregate) Slashing() slashingtypes.QueryClient { return query.slashing }
func (query *remainingQueryAggregate) Evidence() evidencetypes.QueryClient { return query.evidence }

func remainingPageFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagPage, "2"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "5"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagCountTotal, "true"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagReverse, "true"))
}

func remainingConflictingPageFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagPage, "2"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagOffset, "1"))
}

func TestRemainingChainQueryRequestsPreserveInputs(t *testing.T) {
	validator := semanticValidatorAddress(21)
	grantee := sdk.AccAddress(bytes.Repeat([]byte{22}, 20)).String()
	consPubKeyJSON := `{"@type":"/cosmos.crypto.ed25519.PubKey","key":"OauFcTKbN5Lx3fJL689cikXBqe+hcp6Y+x0rYUdR9Jk="}`
	consPubKeyBytes, err := base64.StdEncoding.DecodeString("OauFcTKbN5Lx3fJL689cikXBqe+hcp6Y+x0rYUdR9Jk=")
	require.NoError(t, err)
	consAddress := sdk.ConsAddress((&sdked25519.PubKey{Key: consPubKeyBytes}).Address()).String()
	page := &sdkquery.PageRequest{Offset: 5, Limit: 5, CountTotal: true, Reverse: true}
	start := "2026-08-10T01:02:03Z"
	end := "2026-08-11T04:05:06Z"
	startTime, err := time.Parse(time.RFC3339, start)
	require.NoError(t, err)
	endTime, err := time.Parse(time.RFC3339, end)
	require.NoError(t, err)

	tests := []struct {
		name          string
		command       *cobra.Command
		recorder      *remainingQueryRecorder
		query         clientv1beta3.QueryClient
		args          []string
		configure     func(*testing.T, *cobra.Command)
		method        string
		assertRequest func(*testing.T, interface{})
	}{
		{name: "distribution params", command: GetQueryDistributionParamsCmd(), recorder: &remainingQueryRecorder{}, method: "distribution-params", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, &distributiontypes.QueryParamsRequest{}, request)
		}},
		{name: "validator distribution info", command: GetQueryDistributionValidatorDistributionInfoCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator}, method: "validator-distribution-info", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, validator, request.(*distributiontypes.QueryValidatorDistributionInfoRequest).ValidatorAddress)
		}},
		{name: "validator outstanding rewards", command: GetQueryDistributionValidatorOutstandingRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator}, method: "validator-outstanding-rewards", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, validator, request.(*distributiontypes.QueryValidatorOutstandingRewardsRequest).ValidatorAddress)
		}},
		{name: "validator commission", command: GetQueryDistributionValidatorCommissionCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator}, method: "validator-commission", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, validator, request.(*distributiontypes.QueryValidatorCommissionRequest).ValidatorAddress)
		}},
		{name: "validator slashes", command: GetQueryDistributionValidatorSlashesCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator, "7", "99"}, configure: remainingPageFlags, method: "validator-slashes", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*distributiontypes.QueryValidatorSlashesRequest)
			require.Equal(t, validator, request.ValidatorAddress)
			require.Equal(t, uint64(7), request.StartingHeight)
			require.Equal(t, uint64(99), request.EndingHeight)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "delegation rewards", command: GetQueryDistributionDelegatorRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, validator}, method: "delegation-rewards", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*distributiontypes.QueryDelegationRewardsRequest)
			require.Equal(t, stateTestOwner, request.DelegatorAddress)
			require.Equal(t, validator, request.ValidatorAddress)
		}},
		{name: "total rewards", command: GetQueryDistributionDelegatorRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, method: "delegation-total-rewards", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, stateTestOwner, request.(*distributiontypes.QueryDelegationTotalRewardsRequest).DelegatorAddress)
		}},
		{name: "community pool", command: GetQueryDistributionCommunityPoolCmd(), recorder: &remainingQueryRecorder{}, method: "community-pool", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, &distributiontypes.QueryCommunityPoolRequest{}, request)
		}},
		{name: "authz pair", command: GetQueryAuthzGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, grantee, "/cosmos.bank.v1beta1.MsgSend"}, configure: remainingPageFlags, method: "authz-grants", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*authz.QueryGrantsRequest)
			require.Equal(t, stateTestOwner, request.Granter)
			require.Equal(t, grantee, request.Grantee)
			require.Equal(t, "/cosmos.bank.v1beta1.MsgSend", request.MsgTypeUrl)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "authz by granter", command: GetQueryAuthzGranterGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingPageFlags, method: "authz-granter-grants", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*authz.QueryGranterGrantsRequest)
			require.Equal(t, stateTestOwner, request.Granter)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "authz by grantee", command: GetQueryAuthzGranteeGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{grantee}, configure: remainingPageFlags, method: "authz-grantee-grants", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*authz.QueryGranteeGrantsRequest)
			require.Equal(t, grantee, request.Grantee)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "feegrant pair", command: GetQueryFeeGrantCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, grantee}, method: "feegrant-allowance", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*feegrant.QueryAllowanceRequest)
			require.Equal(t, stateTestOwner, request.Granter)
			require.Equal(t, grantee, request.Grantee)
		}},
		{name: "feegrant by grantee", command: GetQueryFeeGrantsByGranteeCmd(), recorder: &remainingQueryRecorder{}, args: []string{grantee}, configure: remainingPageFlags, method: "feegrant-allowances", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*feegrant.QueryAllowancesRequest)
			require.Equal(t, grantee, request.Grantee)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "feegrant by granter", command: GetQueryFeeGrantsByGranterCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingPageFlags, method: "feegrant-allowances-by-granter", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*feegrant.QueryAllowancesByGranterRequest)
			require.Equal(t, stateTestOwner, request.Granter)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "bme params", command: GetBMEParamsCmd(), recorder: &remainingQueryRecorder{}, method: "bme-params", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &bmetypes.QueryParamsRequest{}, request) }},
		{name: "bme vault state", command: GetBMEVaultStateCmd(), recorder: &remainingQueryRecorder{}, method: "bme-vault-state", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &bmetypes.QueryVaultStateRequest{}, request) }},
		{name: "bme status", command: GetBMEStatusCmd(), recorder: &remainingQueryRecorder{}, method: "bme-status", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &bmetypes.QueryStatusRequest{}, request) }},
		{name: "bme ledger", command: GetBMELedgerRecordsCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) {
			remainingPageFlags(t, cmd)
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagOwner, stateTestOwner))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagDenom, "uakt"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagToDenom, "uact"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagStatus, "ledger_record_status_pending"))
		}, method: "bme-ledger", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*bmetypes.QueryLedgerRecordsRequest)
			require.Equal(t, bmetypes.LedgerRecordFilters{Source: stateTestOwner, Denom: "uakt", ToDenom: "uact", Status: "ledger_record_status_pending"}, request.Filters)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "mint params", command: GetQueryMintParamsCmd(), recorder: &remainingQueryRecorder{}, method: "mint-params", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &minttypes.QueryParamsRequest{}, request) }},
		{name: "mint inflation", command: GetQueryMintInflationCmd(), recorder: &remainingQueryRecorder{}, method: "mint-inflation", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &minttypes.QueryInflationRequest{}, request) }},
		{name: "mint annual provisions", command: GetQueryMintAnnualProvisionsCmd(), recorder: &remainingQueryRecorder{}, method: "mint-annual-provisions", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, &minttypes.QueryAnnualProvisionsRequest{}, request)
		}},
		{name: "oracle prices", command: GetOraclePricesCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) {
			remainingPageFlags(t, cmd)
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagAssetDenom, "akt"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagBaseDenom, "usd"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagStartTime, start))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagEndTime, end))
		}, method: "oracle-prices", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*oracletypes.QueryPricesRequest)
			require.Equal(t, oracletypes.PricesFilter{AssetDenom: "akt", BaseDenom: "usd", StartTime: startTime, EndTime: endTime}, request.Filters)
			require.Equal(t, page, request.Pagination)
		}},
		{name: "oracle aggregated price", command: GetOracleAggregatedPriceCmd(), recorder: &remainingQueryRecorder{}, args: []string{"uAKT"}, method: "oracle-aggregated-price", assertRequest: func(t *testing.T, value interface{}) {
			require.Equal(t, "akt", value.(*oracletypes.QueryAggregatedPriceRequest).Denom)
		}},
		{name: "oracle params", command: GetQueryOracleParamsCmd(), recorder: &remainingQueryRecorder{}, method: "oracle-params", assertRequest: func(t *testing.T, request interface{}) { require.Equal(t, &oracletypes.QueryParamsRequest{}, request) }},
		{name: "params subspace", command: GetQueryParamsSubspaceCmd(), recorder: &remainingQueryRecorder{}, args: []string{"staking", "MaxValidators"}, method: "params-subspace", assertRequest: func(t *testing.T, value interface{}) {
			request := value.(*paramstypes.QueryParamsRequest)
			require.Equal(t, "staking", request.Subspace)
			require.Equal(t, "MaxValidators", request.Key)
		}},
		{name: "slashing signing info", command: GetQuerySlashingSigningInfoCmd(), recorder: &remainingQueryRecorder{}, args: []string{consPubKeyJSON}, method: "slashing-signing-info", assertRequest: func(t *testing.T, value interface{}) {
			require.Equal(t, consAddress, value.(*slashingtypes.QuerySigningInfoRequest).ConsAddress)
		}},
		{name: "slashing signing infos", command: GetQuerySlashingSigningInfosCmd(), recorder: &remainingQueryRecorder{}, configure: remainingPageFlags, method: "slashing-signing-infos", assertRequest: func(t *testing.T, value interface{}) {
			require.Equal(t, page, value.(*slashingtypes.QuerySigningInfosRequest).Pagination)
		}},
		{name: "slashing params", command: GetQuerySlashingParamsCmd(), recorder: &remainingQueryRecorder{}, method: "slashing-params", assertRequest: func(t *testing.T, request interface{}) {
			require.Equal(t, &slashingtypes.QueryParamsRequest{}, request)
		}},
		{name: "one evidence", command: GetQueryEvidenceCmd(), recorder: &remainingQueryRecorder{}, args: []string{"AABBCCDD"}, method: "evidence-one", assertRequest: func(t *testing.T, value interface{}) {
			require.Equal(t, "AABBCCDD", value.(*evidencetypes.QueryEvidenceRequest).Hash)
		}},
		{name: "all evidence", command: GetQueryEvidenceCmd(), recorder: &remainingQueryRecorder{}, configure: remainingPageFlags, method: "evidence-all", assertRequest: func(t *testing.T, value interface{}) {
			require.Equal(t, page, value.(*evidencetypes.QueryAllEvidenceRequest).Pagination)
		}},
	}

	for index := range tests {
		test := &tests[index]
		switch test.method {
		case "distribution-params", "validator-distribution-info", "validator-outstanding-rewards", "validator-commission", "validator-slashes", "delegation-rewards", "delegation-total-rewards", "community-pool":
			test.query = &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: *test.recorder}}
		case "authz-grants", "authz-granter-grants", "authz-grantee-grants":
			test.query = &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: *test.recorder}}
		case "feegrant-allowance", "feegrant-allowances", "feegrant-allowances-by-granter":
			test.query = &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: *test.recorder}}
		case "bme-params", "bme-vault-state", "bme-status", "bme-ledger":
			test.query = &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: *test.recorder}}
		case "mint-params", "mint-inflation", "mint-annual-provisions":
			test.query = &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: *test.recorder}}
		case "oracle-prices", "oracle-aggregated-price", "oracle-params":
			test.query = &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: *test.recorder}}
		case "params-subspace":
			test.query = &remainingQueryAggregate{params: &remainingParamsQuery{remainingQueryRecorder: *test.recorder}}
		case "slashing-signing-info", "slashing-signing-infos", "slashing-params":
			test.query = &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: *test.recorder}}
		case "evidence-one", "evidence-all":
			test.query = &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: *test.recorder}}
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.configure != nil {
				test.configure(t, test.command)
			}
			require.NoError(t, runSemanticQuery(t, test.command, test.query, nil, io.Discard, test.args...))

			var recorder *remainingQueryRecorder
			aggregate := test.query.(*remainingQueryAggregate)
			switch {
			case aggregate.distribution != nil:
				recorder = &aggregate.distribution.(*remainingDistributionQuery).remainingQueryRecorder
			case aggregate.authz != nil:
				recorder = &aggregate.authz.(*remainingAuthzQuery).remainingQueryRecorder
			case aggregate.feegrant != nil:
				recorder = &aggregate.feegrant.(*remainingFeegrantQuery).remainingQueryRecorder
			case aggregate.bme != nil:
				recorder = &aggregate.bme.(*remainingBMEQuery).remainingQueryRecorder
			case aggregate.mint != nil:
				recorder = &aggregate.mint.(*remainingMintQuery).remainingQueryRecorder
			case aggregate.oracle != nil:
				recorder = &aggregate.oracle.(*remainingOracleQuery).remainingQueryRecorder
			case aggregate.params != nil:
				recorder = &aggregate.params.(*remainingParamsQuery).remainingQueryRecorder
			case aggregate.slashing != nil:
				recorder = &aggregate.slashing.(*remainingSlashingQuery).remainingQueryRecorder
			case aggregate.evidence != nil:
				recorder = &aggregate.evidence.(*remainingEvidenceQuery).remainingQueryRecorder
			}
			require.Equal(t, test.method, recorder.method)
			test.assertRequest(t, recorder.request)
		})
	}
}

func TestRemainingChainQueriesRejectInvalidInputBeforeTransport(t *testing.T) {
	validator := semanticValidatorAddress(21)
	tests := []struct {
		name      string
		command   *cobra.Command
		recorder  *remainingQueryRecorder
		query     clientv1beta3.QueryClient
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{name: "distribution validator", command: GetQueryDistributionValidatorDistributionInfoCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "outstanding rewards validator", command: GetQueryDistributionValidatorOutstandingRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "commission validator", command: GetQueryDistributionValidatorCommissionCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "slashes validator", command: GetQueryDistributionValidatorSlashesCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad", "1", "2"}},
		{name: "slash start height", command: GetQueryDistributionValidatorSlashesCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator, "bad", "9"}},
		{name: "slash end height", command: GetQueryDistributionValidatorSlashesCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator, "7", "bad"}},
		{name: "slash pagination", command: GetQueryDistributionValidatorSlashesCmd(), recorder: &remainingQueryRecorder{}, args: []string{validator, "7", "9"}, configure: remainingConflictingPageFlags},
		{name: "delegator", command: GetQueryDistributionDelegatorRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "reward validator", command: GetQueryDistributionDelegatorRewardsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, "bad"}},
		{name: "authz granter", command: GetQueryAuthzGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad", stateTestOwner}},
		{name: "authz grantee", command: GetQueryAuthzGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, "bad"}},
		{name: "authz pair pagination", command: GetQueryAuthzGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, stateTestOwner}, configure: remainingConflictingPageFlags},
		{name: "authz granter command address", command: GetQueryAuthzGranterGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "authz granter pagination", command: GetQueryAuthzGranterGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingConflictingPageFlags},
		{name: "authz grantee command address", command: GetQueryAuthzGranteeGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "authz grantee pagination", command: GetQueryAuthzGranteeGrantsCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingConflictingPageFlags},
		{name: "feegrant granter", command: GetQueryFeeGrantCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad", stateTestOwner}},
		{name: "feegrant grantee", command: GetQueryFeeGrantCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner, "bad"}},
		{name: "feegrant grantee command address", command: GetQueryFeeGrantsByGranteeCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "feegrant grantee pagination", command: GetQueryFeeGrantsByGranteeCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingConflictingPageFlags},
		{name: "feegrant granter command address", command: GetQueryFeeGrantsByGranterCmd(), recorder: &remainingQueryRecorder{}, args: []string{"bad"}},
		{name: "feegrant granter pagination", command: GetQueryFeeGrantsByGranterCmd(), recorder: &remainingQueryRecorder{}, args: []string{stateTestOwner}, configure: remainingConflictingPageFlags},
		{name: "bme owner", command: GetBMELedgerRecordsCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) { require.NoError(t, cmd.Flags().Set(flagdefs.FlagOwner, "bad")) }},
		{name: "bme pagination", command: GetBMELedgerRecordsCmd(), recorder: &remainingQueryRecorder{}, configure: remainingConflictingPageFlags},
		{name: "oracle start time", command: GetOraclePricesCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) {
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagStartTime, "not-time"))
		}},
		{name: "oracle end time", command: GetOraclePricesCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) {
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagEndTime, "not-time"))
		}},
		{name: "oracle inverted range", command: GetOraclePricesCmd(), recorder: &remainingQueryRecorder{}, configure: func(t *testing.T, cmd *cobra.Command) {
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagStartTime, "2026-08-12T00:00:00Z"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagEndTime, "2026-08-11T00:00:00Z"))
		}},
		{name: "oracle pagination", command: GetOraclePricesCmd(), recorder: &remainingQueryRecorder{}, configure: remainingConflictingPageFlags},
		{name: "slashing consensus pubkey", command: GetQuerySlashingSigningInfoCmd(), recorder: &remainingQueryRecorder{}, args: []string{"not-json"}},
		{name: "slashing pagination", command: GetQuerySlashingSigningInfosCmd(), recorder: &remainingQueryRecorder{}, configure: remainingConflictingPageFlags},
		{name: "evidence pagination", command: GetQueryEvidenceCmd(), recorder: &remainingQueryRecorder{}, configure: remainingConflictingPageFlags},
	}

	for index := range tests {
		test := &tests[index]
		switch test.name {
		case "distribution validator", "outstanding rewards validator", "commission validator", "slashes validator", "slash start height", "slash end height", "slash pagination", "delegator", "reward validator":
			test.query = &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: *test.recorder}}
		case "authz granter", "authz grantee", "authz pair pagination", "authz granter command address", "authz granter pagination", "authz grantee command address", "authz grantee pagination":
			test.query = &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: *test.recorder}}
		case "feegrant granter", "feegrant grantee", "feegrant grantee command address", "feegrant grantee pagination", "feegrant granter command address", "feegrant granter pagination":
			test.query = &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: *test.recorder}}
		case "bme owner", "bme pagination":
			test.query = &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: *test.recorder}}
		case "oracle start time", "oracle end time", "oracle inverted range", "oracle pagination":
			test.query = &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: *test.recorder}}
		case "slashing consensus pubkey", "slashing pagination":
			test.query = &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: *test.recorder}}
		case "evidence pagination":
			test.query = &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: *test.recorder}}
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.configure != nil {
				test.configure(t, test.command)
			}
			require.Error(t, runSemanticQuery(t, test.command, test.query, nil, io.Discard, test.args...))
			aggregate := test.query.(*remainingQueryAggregate)
			called := false
			for _, recorder := range []*remainingQueryRecorder{
				test.recorder,
				func() *remainingQueryRecorder {
					switch {
					case aggregate.distribution != nil:
						return &aggregate.distribution.(*remainingDistributionQuery).remainingQueryRecorder
					case aggregate.authz != nil:
						return &aggregate.authz.(*remainingAuthzQuery).remainingQueryRecorder
					case aggregate.feegrant != nil:
						return &aggregate.feegrant.(*remainingFeegrantQuery).remainingQueryRecorder
					case aggregate.bme != nil:
						return &aggregate.bme.(*remainingBMEQuery).remainingQueryRecorder
					case aggregate.oracle != nil:
						return &aggregate.oracle.(*remainingOracleQuery).remainingQueryRecorder
					case aggregate.evidence != nil:
						return &aggregate.evidence.(*remainingEvidenceQuery).remainingQueryRecorder
					default:
						return nil
					}
				}(),
			} {
				if recorder != nil && recorder.method != "" {
					called = true
				}
			}
			require.False(t, called, "transport was called for invalid input")
		})
	}
}

func TestRemainingChainQueryTransportErrorsPreserveCause(t *testing.T) {
	sentinel := errors.New("query transport failed")
	validator := semanticValidatorAddress(21)
	consPubKeyJSON := `{"@type":"/cosmos.crypto.ed25519.PubKey","key":"OauFcTKbN5Lx3fJL689cikXBqe+hcp6Y+x0rYUdR9Jk="}`
	tests := []struct {
		name    string
		command *cobra.Command
		query   clientv1beta3.QueryClient
		args    []string
	}{
		{name: "distribution params", command: GetQueryDistributionParamsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "validator distribution info", command: GetQueryDistributionValidatorDistributionInfoCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{validator}},
		{name: "validator outstanding rewards", command: GetQueryDistributionValidatorOutstandingRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{validator}},
		{name: "validator commission", command: GetQueryDistributionValidatorCommissionCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{validator}},
		{name: "validator slashes", command: GetQueryDistributionValidatorSlashesCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{validator, "1", "2"}},
		{name: "delegation rewards", command: GetQueryDistributionDelegatorRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner, validator}},
		{name: "delegation total rewards", command: GetQueryDistributionDelegatorRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner}},
		{name: "community pool", command: GetQueryDistributionCommunityPoolCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "authz grants", command: GetQueryAuthzGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner, stateTestOwner}},
		{name: "authz granter grants", command: GetQueryAuthzGranterGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner}},
		{name: "authz grantee grants", command: GetQueryAuthzGranteeGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner}},
		{name: "feegrant allowance", command: GetQueryFeeGrantCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner, stateTestOwner}},
		{name: "feegrant allowances", command: GetQueryFeeGrantsByGranteeCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner}},
		{name: "feegrant allowances by granter", command: GetQueryFeeGrantsByGranterCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{stateTestOwner}},
		{name: "bme params", command: GetBMEParamsCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "bme vault state", command: GetBMEVaultStateCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "bme status", command: GetBMEStatusCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "bme ledger", command: GetBMELedgerRecordsCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "mint params", command: GetQueryMintParamsCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "mint inflation", command: GetQueryMintInflationCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "mint annual provisions", command: GetQueryMintAnnualProvisionsCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "oracle prices", command: GetOraclePricesCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "oracle wrapped", command: GetOracleAggregatedPriceCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{"akt"}},
		{name: "oracle params", command: GetQueryOracleParamsCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "params", command: GetQueryParamsSubspaceCmd(), query: &remainingQueryAggregate{params: &remainingParamsQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{"staking", "MaxValidators"}},
		{name: "slashing signing info", command: GetQuerySlashingSigningInfoCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{consPubKeyJSON}},
		{name: "slashing signing infos", command: GetQuerySlashingSigningInfosCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "slashing params", command: GetQuerySlashingParamsCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
		{name: "one evidence", command: GetQueryEvidenceCmd(), query: &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}, args: []string{"AABBCCDD"}},
		{name: "all evidence", command: GetQueryEvidenceCmd(), query: &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: remainingQueryRecorder{err: sentinel}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runSemanticQuery(t, test.command, test.query, nil, io.Discard, test.args...)
			require.ErrorIs(t, err, sentinel)
		})
	}
}

func TestRemainingChainQueriesRejectNilSuccessfulResponses(t *testing.T) {
	validator := semanticValidatorAddress(21)
	consPubKeyJSON := `{"@type":"/cosmos.crypto.ed25519.PubKey","key":"OauFcTKbN5Lx3fJL689cikXBqe+hcp6Y+x0rYUdR9Jk="}`
	tests := []struct {
		name    string
		command *cobra.Command
		query   clientv1beta3.QueryClient
		args    []string
	}{
		{name: "distribution params", command: GetQueryDistributionParamsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "validator distribution info", command: GetQueryDistributionValidatorDistributionInfoCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{validator}},
		{name: "validator outstanding rewards", command: GetQueryDistributionValidatorOutstandingRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{validator}},
		{name: "validator commission", command: GetQueryDistributionValidatorCommissionCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{validator}},
		{name: "validator slashes", command: GetQueryDistributionValidatorSlashesCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{validator, "1", "2"}},
		{name: "delegation rewards", command: GetQueryDistributionDelegatorRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner, validator}},
		{name: "delegation total rewards", command: GetQueryDistributionDelegatorRewardsCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner}},
		{name: "community pool", command: GetQueryDistributionCommunityPoolCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "authz grants", command: GetQueryAuthzGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner, stateTestOwner}},
		{name: "authz granter grants", command: GetQueryAuthzGranterGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner}},
		{name: "authz grantee grants", command: GetQueryAuthzGranteeGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner}},
		{name: "feegrant allowance", command: GetQueryFeeGrantCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner, stateTestOwner}},
		{name: "feegrant allowances", command: GetQueryFeeGrantsByGranteeCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner}},
		{name: "feegrant allowances by granter", command: GetQueryFeeGrantsByGranterCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{stateTestOwner}},
		{name: "bme params", command: GetBMEParamsCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "bme vault state", command: GetBMEVaultStateCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "bme status", command: GetBMEStatusCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "bme ledger", command: GetBMELedgerRecordsCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "mint params", command: GetQueryMintParamsCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "mint inflation", command: GetQueryMintInflationCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "mint annual provisions", command: GetQueryMintAnnualProvisionsCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "oracle prices", command: GetOraclePricesCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "oracle aggregated price", command: GetOracleAggregatedPriceCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{"akt"}},
		{name: "oracle params", command: GetQueryOracleParamsCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "params", command: GetQueryParamsSubspaceCmd(), query: &remainingQueryAggregate{params: &remainingParamsQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{"staking", "MaxValidators"}},
		{name: "slashing signing info", command: GetQuerySlashingSigningInfoCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{consPubKeyJSON}},
		{name: "slashing signing infos", command: GetQuerySlashingSigningInfosCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "slashing params", command: GetQuerySlashingParamsCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "evidence list", command: GetQueryEvidenceCmd(), query: &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}},
		{name: "evidence item", command: GetQueryEvidenceCmd(), query: &remainingQueryAggregate{evidence: &remainingEvidenceQuery{remainingQueryRecorder: remainingQueryRecorder{nilResponse: true}}}, args: []string{"AABBCCDD"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				err = runSemanticQuery(t, test.command, test.query, nil, io.Discard, test.args...)
			})
			require.ErrorContains(t, err, "malformed node response")
		})
	}
}

func TestFeegrantQueryRejectsMissingAllowance(t *testing.T) {
	query := &remainingFeegrantQuery{}
	query.nilResponse = false
	query.QueryClient = feegrant.QueryClient(nil)

	custom := &remainingFeegrantMissingAllowance{remainingFeegrantQuery: *query}
	err := runSemanticQuery(t, GetQueryFeeGrantCmd(), &remainingQueryAggregate{feegrant: custom}, nil, io.Discard, stateTestOwner, stateTestOwner)
	require.ErrorContains(t, err, "missing allowance")
}

func TestEvidenceQueryRejectsMissingEvidence(t *testing.T) {
	err := runSemanticQuery(t, GetQueryEvidenceCmd(), &remainingQueryAggregate{evidence: remainingEvidenceMissingEvidence{}}, nil, io.Discard, "AABBCCDD")
	require.ErrorContains(t, err, "missing evidence")
}

type remainingFeegrantMissingAllowance struct {
	remainingFeegrantQuery
}

func (query *remainingFeegrantMissingAllowance) Allowance(_ context.Context, request *feegrant.QueryAllowanceRequest, _ ...grpc.CallOption) (*feegrant.QueryAllowanceResponse, error) {
	query.record("feegrant-allowance", request)
	return &feegrant.QueryAllowanceResponse{}, nil
}

type remainingEvidenceMissingEvidence struct {
	evidencetypes.QueryClient
}

func (remainingEvidenceMissingEvidence) Evidence(context.Context, *evidencetypes.QueryEvidenceRequest, ...grpc.CallOption) (*evidencetypes.QueryEvidenceResponse, error) {
	return &evidencetypes.QueryEvidenceResponse{}, nil
}

type remainingQueryFailWriter struct{ err error }

func (writer remainingQueryFailWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestRemainingChainQueryOutputFailuresPropagate(t *testing.T) {
	sentinel := errors.New("output failed")
	tests := []struct {
		name    string
		command *cobra.Command
		query   clientv1beta3.QueryClient
		args    []string
	}{
		{name: "distribution", command: GetQueryDistributionCommunityPoolCmd(), query: &remainingQueryAggregate{distribution: &remainingDistributionQuery{}}},
		{name: "authz", command: GetQueryAuthzGrantsCmd(), query: &remainingQueryAggregate{authz: &remainingAuthzQuery{}}, args: []string{stateTestOwner, stateTestOwner}},
		{name: "feegrant", command: GetQueryFeeGrantsByGranterCmd(), query: &remainingQueryAggregate{feegrant: &remainingFeegrantQuery{}}, args: []string{stateTestOwner}},
		{name: "bme", command: GetBMEParamsCmd(), query: &remainingQueryAggregate{bme: &remainingBMEQuery{}}},
		{name: "mint", command: GetQueryMintInflationCmd(), query: &remainingQueryAggregate{mint: &remainingMintQuery{}}},
		{name: "oracle", command: GetOraclePricesCmd(), query: &remainingQueryAggregate{oracle: &remainingOracleQuery{}}},
		{name: "params", command: GetQueryParamsSubspaceCmd(), query: &remainingQueryAggregate{params: &remainingParamsQuery{}}, args: []string{"staking", "MaxValidators"}},
		{name: "slashing", command: GetQuerySlashingSigningInfosCmd(), query: &remainingQueryAggregate{slashing: &remainingSlashingQuery{}}},
		{name: "evidence", command: GetQueryEvidenceCmd(), query: &remainingQueryAggregate{evidence: &remainingEvidenceQuery{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runSemanticQuery(t, test.command, test.query, nil, remainingQueryFailWriter{err: sentinel}, test.args...)
			require.ErrorIs(t, err, sentinel)
		})
	}
}
