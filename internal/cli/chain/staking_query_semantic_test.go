package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type semanticStakingQuery struct {
	stakingtypes.QueryClient

	method       string
	request      interface{}
	err          error
	nilResponse  bool
	missingField bool
}

func (query *semanticStakingQuery) result(method string, request interface{}) bool {
	query.method = method
	query.request = request
	return query.nilResponse
}

func (query *semanticStakingQuery) Validator(_ context.Context, request *stakingtypes.QueryValidatorRequest, _ ...grpc.CallOption) (*stakingtypes.QueryValidatorResponse, error) {
	if query.result("validator", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryValidatorResponse{}, nil
}

func (query *semanticStakingQuery) Validators(_ context.Context, request *stakingtypes.QueryValidatorsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryValidatorsResponse, error) {
	if query.result("validators", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryValidatorsResponse{}, nil
}

func (query *semanticStakingQuery) ValidatorUnbondingDelegations(_ context.Context, request *stakingtypes.QueryValidatorUnbondingDelegationsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryValidatorUnbondingDelegationsResponse, error) {
	if query.result("validator-unbonding", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryValidatorUnbondingDelegationsResponse{}, nil
}

func (query *semanticStakingQuery) Redelegations(_ context.Context, request *stakingtypes.QueryRedelegationsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryRedelegationsResponse, error) {
	if query.result("redelegations", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryRedelegationsResponse{}, nil
}

func (query *semanticStakingQuery) Delegation(_ context.Context, request *stakingtypes.QueryDelegationRequest, _ ...grpc.CallOption) (*stakingtypes.QueryDelegationResponse, error) {
	if query.result("delegation", request) || query.err != nil {
		return nil, query.err
	}
	response := &stakingtypes.QueryDelegationResponse{DelegationResponse: &stakingtypes.DelegationResponse{}}
	if query.missingField {
		response.DelegationResponse = nil
	}
	return response, nil
}

func (query *semanticStakingQuery) DelegatorDelegations(_ context.Context, request *stakingtypes.QueryDelegatorDelegationsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryDelegatorDelegationsResponse, error) {
	if query.result("delegator-delegations", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryDelegatorDelegationsResponse{}, nil
}

func (query *semanticStakingQuery) ValidatorDelegations(_ context.Context, request *stakingtypes.QueryValidatorDelegationsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryValidatorDelegationsResponse, error) {
	if query.result("validator-delegations", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryValidatorDelegationsResponse{}, nil
}

func (query *semanticStakingQuery) UnbondingDelegation(_ context.Context, request *stakingtypes.QueryUnbondingDelegationRequest, _ ...grpc.CallOption) (*stakingtypes.QueryUnbondingDelegationResponse, error) {
	if query.result("unbonding-delegation", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryUnbondingDelegationResponse{}, nil
}

func (query *semanticStakingQuery) DelegatorUnbondingDelegations(_ context.Context, request *stakingtypes.QueryDelegatorUnbondingDelegationsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryDelegatorUnbondingDelegationsResponse, error) {
	if query.result("delegator-unbonding", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryDelegatorUnbondingDelegationsResponse{}, nil
}

func (query *semanticStakingQuery) HistoricalInfo(_ context.Context, request *stakingtypes.QueryHistoricalInfoRequest, _ ...grpc.CallOption) (*stakingtypes.QueryHistoricalInfoResponse, error) {
	if query.result("historical-info", request) || query.err != nil {
		return nil, query.err
	}
	response := &stakingtypes.QueryHistoricalInfoResponse{Hist: &stakingtypes.HistoricalInfo{}}
	if query.missingField {
		response.Hist = nil
	}
	return response, nil
}

func (query *semanticStakingQuery) Pool(_ context.Context, request *stakingtypes.QueryPoolRequest, _ ...grpc.CallOption) (*stakingtypes.QueryPoolResponse, error) {
	if query.result("pool", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryPoolResponse{}, nil
}

func (query *semanticStakingQuery) Params(_ context.Context, request *stakingtypes.QueryParamsRequest, _ ...grpc.CallOption) (*stakingtypes.QueryParamsResponse, error) {
	if query.result("params", request) || query.err != nil {
		return nil, query.err
	}
	return &stakingtypes.QueryParamsResponse{}, nil
}

type semanticStakingAggregate struct {
	clientv1beta3.QueryClient
	staking stakingtypes.QueryClient
}

func (query *semanticStakingAggregate) Staking() stakingtypes.QueryClient { return query.staking }

func semanticValidatorAddress(seed byte) string {
	return sdk.ValAddress(bytes.Repeat([]byte{seed}, 20)).String()
}

func runStakingSemanticQuery(t *testing.T, commandFactory func() *cobra.Command, query *semanticStakingQuery, output io.Writer, args ...string) error {
	t.Helper()
	return runSemanticQuery(t, commandFactory(), &semanticStakingAggregate{staking: query}, nil, output, args...)
}

func TestStakingQueryRequestsPreserveInputs(t *testing.T) {
	validator := semanticValidatorAddress(2)
	destination := semanticValidatorAddress(3)
	page := &sdkquery.PageRequest{Offset: 5, Limit: 5, CountTotal: true, Reverse: true}
	pageFlags := map[string]string{"page": "2", "limit": "5", "count-total": "true", "reverse": "true"}
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
		method         string
		assertRequest  func(*testing.T, interface{})
	}{
		{name: "validator", commandFactory: GetQueryStakingValidatorCmd, args: []string{validator}, method: "validator", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, validator, got.(*stakingtypes.QueryValidatorRequest).ValidatorAddr)
		}},
		{name: "validators", commandFactory: GetQueryStakingValidatorsCmd, flags: pageFlags, method: "validators", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, page, got.(*stakingtypes.QueryValidatorsRequest).Pagination)
		}},
		{name: "unbonding from", commandFactory: GetQueryStakingValidatorUnbondingDelegationsCmd, args: []string{validator}, flags: pageFlags, method: "validator-unbonding", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, validator, got.(*stakingtypes.QueryValidatorUnbondingDelegationsRequest).ValidatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryValidatorUnbondingDelegationsRequest).Pagination)
		}},
		{name: "redelegations from", commandFactory: GetQueryStakingValidatorRedelegationsCmd, args: []string{validator}, flags: pageFlags, method: "redelegations", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, validator, got.(*stakingtypes.QueryRedelegationsRequest).SrcValidatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryRedelegationsRequest).Pagination)
		}},
		{name: "delegation", commandFactory: GetQueryStakingDelegationCmd, args: []string{stateTestOwner, validator}, method: "delegation", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*stakingtypes.QueryDelegationRequest).DelegatorAddr)
			require.Equal(t, validator, got.(*stakingtypes.QueryDelegationRequest).ValidatorAddr)
		}},
		{name: "delegations", commandFactory: GetQueryStakingDelegationsCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "delegator-delegations", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*stakingtypes.QueryDelegatorDelegationsRequest).DelegatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryDelegatorDelegationsRequest).Pagination)
		}},
		{name: "delegations to", commandFactory: GetQueryStakingValidatorDelegationsCmd, args: []string{validator}, flags: pageFlags, method: "validator-delegations", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, validator, got.(*stakingtypes.QueryValidatorDelegationsRequest).ValidatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryValidatorDelegationsRequest).Pagination)
		}},
		{name: "unbonding delegation", commandFactory: GetQueryStakingUnbondingDelegationCmd, args: []string{stateTestOwner, validator}, method: "unbonding-delegation", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*stakingtypes.QueryUnbondingDelegationRequest).DelegatorAddr)
			require.Equal(t, validator, got.(*stakingtypes.QueryUnbondingDelegationRequest).ValidatorAddr)
		}},
		{name: "unbonding delegations", commandFactory: GetQueryStakingUnbondingDelegationsCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "delegator-unbonding", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*stakingtypes.QueryDelegatorUnbondingDelegationsRequest).DelegatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryDelegatorUnbondingDelegationsRequest).Pagination)
		}},
		{name: "redelegation", commandFactory: GetQueryStakingRedelegationCmd, args: []string{stateTestOwner, validator, destination}, method: "redelegations", assertRequest: func(t *testing.T, got interface{}) {
			request := got.(*stakingtypes.QueryRedelegationsRequest)
			require.Equal(t, stateTestOwner, request.DelegatorAddr)
			require.Equal(t, validator, request.SrcValidatorAddr)
			require.Equal(t, destination, request.DstValidatorAddr)
		}},
		{name: "redelegations", commandFactory: GetQueryStakingRedelegationsCmd, args: []string{stateTestOwner}, flags: pageFlags, method: "redelegations", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, stateTestOwner, got.(*stakingtypes.QueryRedelegationsRequest).DelegatorAddr)
			require.Equal(t, page, got.(*stakingtypes.QueryRedelegationsRequest).Pagination)
		}},
		{name: "historical info", commandFactory: GetQueryStakingHistoricalInfoCmd, args: []string{"47"}, method: "historical-info", assertRequest: func(t *testing.T, got interface{}) {
			require.Equal(t, int64(47), got.(*stakingtypes.QueryHistoricalInfoRequest).Height)
		}},
		{name: "pool", commandFactory: GetQueryStakingPoolCmd, method: "pool", assertRequest: func(t *testing.T, got interface{}) { require.Equal(t, &stakingtypes.QueryPoolRequest{}, got) }},
		{name: "params", commandFactory: GetQueryStakingParamsCmd, method: "params", assertRequest: func(t *testing.T, got interface{}) { require.Equal(t, &stakingtypes.QueryParamsRequest{}, got) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &semanticStakingQuery{}
			cmd := test.commandFactory()
			for name, value := range test.flags {
				require.NoError(t, cmd.Flags().Set(name, value))
			}
			err := runSemanticQuery(t, cmd, &semanticStakingAggregate{staking: query}, nil, io.Discard, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.method, query.method)
			test.assertRequest(t, query.request)
		})
	}
}

func TestStakingQueryRejectsInvalidInputsBeforeTransport(t *testing.T) {
	validator := semanticValidatorAddress(2)
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		flags          map[string]string
	}{
		{name: "validator", commandFactory: GetQueryStakingValidatorCmd, args: []string{"bad"}},
		{name: "delegator", commandFactory: GetQueryStakingDelegationCmd, args: []string{"bad", validator}},
		{name: "delegation validator", commandFactory: GetQueryStakingDelegationCmd, args: []string{stateTestOwner, "bad"}},
		{name: "redelegation source", commandFactory: GetQueryStakingRedelegationCmd, args: []string{stateTestOwner, "bad", validator}},
		{name: "redelegation destination", commandFactory: GetQueryStakingRedelegationCmd, args: []string{stateTestOwner, validator, "bad"}},
		{name: "negative height", commandFactory: GetQueryStakingHistoricalInfoCmd, args: []string{"-1"}},
		{name: "non-numeric height", commandFactory: GetQueryStakingHistoricalInfoCmd, args: []string{"x"}},
		{name: "page and offset", commandFactory: GetQueryStakingValidatorsCmd, flags: map[string]string{"page": "2", "offset": "1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &semanticStakingQuery{}
			cmd := test.commandFactory()
			for name, value := range test.flags {
				require.NoError(t, cmd.Flags().Set(name, value))
			}
			err := runSemanticQuery(t, cmd, &semanticStakingAggregate{staking: query}, nil, io.Discard, test.args...)
			require.Error(t, err)
			require.Empty(t, query.method)
		})
	}
}

func TestStakingQueryRejectsNilSuccessfulResponses(t *testing.T) {
	validator := semanticValidatorAddress(2)
	destination := semanticValidatorAddress(3)
	tests := []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
	}{
		{name: "validator", commandFactory: GetQueryStakingValidatorCmd, args: []string{validator}},
		{name: "validators", commandFactory: GetQueryStakingValidatorsCmd},
		{name: "unbonding from", commandFactory: GetQueryStakingValidatorUnbondingDelegationsCmd, args: []string{validator}},
		{name: "redelegations from", commandFactory: GetQueryStakingValidatorRedelegationsCmd, args: []string{validator}},
		{name: "delegation", commandFactory: GetQueryStakingDelegationCmd, args: []string{stateTestOwner, validator}},
		{name: "delegations", commandFactory: GetQueryStakingDelegationsCmd, args: []string{stateTestOwner}},
		{name: "delegations to", commandFactory: GetQueryStakingValidatorDelegationsCmd, args: []string{validator}},
		{name: "unbonding delegation", commandFactory: GetQueryStakingUnbondingDelegationCmd, args: []string{stateTestOwner, validator}},
		{name: "unbonding delegations", commandFactory: GetQueryStakingUnbondingDelegationsCmd, args: []string{stateTestOwner}},
		{name: "redelegation", commandFactory: GetQueryStakingRedelegationCmd, args: []string{stateTestOwner, validator, destination}},
		{name: "redelegations", commandFactory: GetQueryStakingRedelegationsCmd, args: []string{stateTestOwner}},
		{name: "historical info", commandFactory: GetQueryStakingHistoricalInfoCmd, args: []string{"7"}},
		{name: "pool", commandFactory: GetQueryStakingPoolCmd},
		{name: "params", commandFactory: GetQueryStakingParamsCmd},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &semanticStakingQuery{nilResponse: true}
			var err error
			require.NotPanics(t, func() {
				err = runStakingSemanticQuery(t, test.commandFactory, query, io.Discard, test.args...)
			})
			require.ErrorContains(t, err, "malformed node response")
		})
	}
}

func TestStakingQueryRejectsMissingRequiredFields(t *testing.T) {
	validator := semanticValidatorAddress(2)
	for _, test := range []struct {
		name           string
		commandFactory func() *cobra.Command
		args           []string
		field          string
	}{
		{name: "delegation", commandFactory: GetQueryStakingDelegationCmd, args: []string{stateTestOwner, validator}, field: "delegation response"},
		{name: "historical info", commandFactory: GetQueryStakingHistoricalInfoCmd, args: []string{"7"}, field: "historical info"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := runStakingSemanticQuery(t, test.commandFactory, &semanticStakingQuery{missingField: true}, io.Discard, test.args...)
			require.ErrorContains(t, err, "malformed node response")
			require.ErrorContains(t, err, test.field)
		})
	}
}

func TestStakingQueryPreservesTransportAndOutputErrors(t *testing.T) {
	t.Run("transport cause", func(t *testing.T) {
		cause := errors.New("staking transport failed")
		err := runStakingSemanticQuery(t, GetQueryStakingParamsCmd, &semanticStakingQuery{err: cause}, io.Discard)
		require.ErrorIs(t, err, cause)
	})

	t.Run("short output", func(t *testing.T) {
		err := runStakingSemanticQuery(t, GetQueryStakingParamsCmd, &semanticStakingQuery{}, outputBoundaryShortWriter{})
		require.ErrorIs(t, err, io.ErrShortWrite)
	})
}

func TestStakingQueryCommandRegistersLeaves(t *testing.T) {
	cmd := GetQueryStakingCmd()
	for _, name := range []string{"validator", "validators", "unbonding-delegations-from", "redelegations-from", "delegation", "delegations", "delegations-to", "unbonding-delegation", "unbonding-delegations", "redelegation", "redelegations", "historical-info", "pool", "params"} {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
	}
}
