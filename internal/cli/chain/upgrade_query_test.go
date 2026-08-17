package cli

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"testing"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	cmclient "github.com/cometbft/cometbft/rpc/client"
	cmcore "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type recordingUpgradeQuery struct {
	upgradetypes.QueryClient
	currentRequest *upgradetypes.QueryCurrentPlanRequest
	current        *upgradetypes.QueryCurrentPlanResponse
	currentErr     error
	appliedRequest *upgradetypes.QueryAppliedPlanRequest
	applied        *upgradetypes.QueryAppliedPlanResponse
	appliedErr     error
	moduleRequest  *upgradetypes.QueryModuleVersionsRequest
	modules        *upgradetypes.QueryModuleVersionsResponse
	modulesErr     error
}

func (query *recordingUpgradeQuery) CurrentPlan(
	_ context.Context,
	request *upgradetypes.QueryCurrentPlanRequest,
	_ ...grpc.CallOption,
) (*upgradetypes.QueryCurrentPlanResponse, error) {
	copy := *request
	query.currentRequest = &copy
	return query.current, query.currentErr
}

func (query *recordingUpgradeQuery) AppliedPlan(
	_ context.Context,
	request *upgradetypes.QueryAppliedPlanRequest,
	_ ...grpc.CallOption,
) (*upgradetypes.QueryAppliedPlanResponse, error) {
	copy := *request
	query.appliedRequest = &copy
	return query.applied, query.appliedErr
}

func (query *recordingUpgradeQuery) ModuleVersions(
	_ context.Context,
	request *upgradetypes.QueryModuleVersionsRequest,
	_ ...grpc.CallOption,
) (*upgradetypes.QueryModuleVersionsResponse, error) {
	copy := *request
	query.moduleRequest = &copy
	return query.modules, query.modulesErr
}

type aggregateUpgradeQuery struct {
	clientv1beta3.QueryClient
	upgrade upgradetypes.QueryClient
}

func (query *aggregateUpgradeQuery) Upgrade() upgradetypes.QueryClient { return query.upgrade }
func (*aggregateUpgradeQuery) ClientContext() sdkclient.Context        { return sdkclient.Context{} }

type upgradeCometClient struct {
	cmclient.Client
	response  *cmcore.ResultBlockchainInfo
	err       error
	minHeight int64
	maxHeight int64
}

func (client *upgradeCometClient) BlockchainInfo(
	_ context.Context,
	minHeight int64,
	maxHeight int64,
) (*cmcore.ResultBlockchainInfo, error) {
	client.minHeight = minHeight
	client.maxHeight = maxHeight
	return client.response, client.err
}

func executeUpgradeQuery(
	t *testing.T,
	query *recordingUpgradeQuery,
	node cmclient.Client,
	args ...string,
) (string, error) {
	t.Helper()

	encoding := aktcodec.MakeEncodingConfig()
	var output bytes.Buffer
	cctx := sdkclient.Context{}.
		WithCodec(encoding.Codec).
		WithLegacyAmino(encoding.Amino).
		WithInterfaceRegistry(encoding.InterfaceRegistry).
		WithOutput(&output)
	if node != nil {
		cctx = cctx.WithClient(node)
	}

	lightClient := &stubLightClient{
		q:    &aggregateUpgradeQuery{upgrade: query},
		cctx: cctx,
	}
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	ctx = context.WithValue(ctx, ContextTypeQueryClient, lightClient)

	cmd := GetQueryUpgradeCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(append(args, "--output", "json"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	err := cmd.Execute()
	return output.String(), err
}

func TestUpgradeQueryGroupIsRegisteredWithContextAwareLeaves(t *testing.T) {
	root := QueryCmd()
	upgrade := directChild(root, upgradetypes.ModuleName)
	require.NotNil(t, upgrade)
	require.NotNil(t, upgrade.PersistentPreRunE)
	names := make([]string, 0, len(upgrade.Commands()))
	for _, command := range upgrade.Commands() {
		names = append(names, command.Name())
	}
	sort.Strings(names)
	require.Equal(t, []string{"applied", "module_versions", "plan"}, names)
}

func TestUpgradeCurrentPlanPrintsExactScheduledPlan(t *testing.T) {
	query := &recordingUpgradeQuery{current: &upgradetypes.QueryCurrentPlanResponse{
		Plan: &upgradetypes.Plan{
			Name:   "v2.0.0",
			Height: 12345,
			Info:   `{"commit":"abc123"}`,
		},
	}}

	output, err := executeUpgradeQuery(t, query, nil, "plan")
	require.NoError(t, err)
	require.NotNil(t, query.currentRequest)
	require.Contains(t, output, "v2.0.0")
	require.Contains(t, output, "12345")
	require.Contains(t, output, "abc123")
}

func TestUpgradeCurrentPlanRejectsAbsentPlanAndTransportFailure(t *testing.T) {
	query := &recordingUpgradeQuery{current: &upgradetypes.QueryCurrentPlanResponse{}}
	output, err := executeUpgradeQuery(t, query, nil, "plan")
	require.ErrorContains(t, err, "no upgrade scheduled")
	require.Empty(t, output)

	wantErr := errors.New("upgrade query unavailable")
	query = &recordingUpgradeQuery{currentErr: wantErr}
	output, err = executeUpgradeQuery(t, query, nil, "plan")
	require.ErrorIs(t, err, wantErr)
	require.Empty(t, output)
}

func TestUpgradeAppliedPlanBindsNameHeightAndHeader(t *testing.T) {
	query := &recordingUpgradeQuery{applied: &upgradetypes.QueryAppliedPlanResponse{Height: 77}}
	node := &upgradeCometClient{response: &cmcore.ResultBlockchainInfo{
		LastHeight: 77,
		BlockMetas: []*cmtypes.BlockMeta{{
			Header: cmtypes.Header{Height: 77, ChainID: "upgrade-chain"},
		}},
	}}

	output, err := executeUpgradeQuery(t, query, node, "applied", "v1.9.0")
	require.NoError(t, err)
	require.Equal(t, "v1.9.0", query.appliedRequest.Name)
	require.Equal(t, int64(77), node.minHeight)
	require.Equal(t, int64(77), node.maxHeight)
	require.Contains(t, output, "upgrade-chain")
	require.Contains(t, output, "77")
}

func TestUpgradeAppliedPlanFailsClosedBeforeOrAtHeaderLookup(t *testing.T) {
	for name, test := range map[string]struct {
		query     *recordingUpgradeQuery
		node      *upgradeCometClient
		wantError string
	}{
		"query error": {
			query:     &recordingUpgradeQuery{appliedErr: errors.New("applied query failed")},
			wantError: "applied query failed",
		},
		"zero height": {
			query:     &recordingUpgradeQuery{applied: &upgradetypes.QueryAppliedPlanResponse{}},
			wantError: "no upgrade found",
		},
		"header transport": {
			query:     &recordingUpgradeQuery{applied: &upgradetypes.QueryAppliedPlanResponse{Height: 8}},
			node:      &upgradeCometClient{err: errors.New("header query failed")},
			wantError: "header query failed",
		},
		"header absent": {
			query:     &recordingUpgradeQuery{applied: &upgradetypes.QueryAppliedPlanResponse{Height: 9}},
			node:      &upgradeCometClient{response: &cmcore.ResultBlockchainInfo{}},
			wantError: "no headers returned for height 9",
		},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := executeUpgradeQuery(t, test.query, test.node, "applied", "upgrade-name")
			require.ErrorContains(t, err, test.wantError)
			require.Empty(t, output)
		})
	}
}

func TestUpgradeModuleVersionsPreserveOptionalFilter(t *testing.T) {
	for name, test := range map[string]struct {
		args       []string
		wantFilter string
	}{
		"all modules": {args: []string{"module_versions"}},
		"one module":  {args: []string{"module_versions", "deployment"}, wantFilter: "deployment"},
	} {
		t.Run(name, func(t *testing.T) {
			query := &recordingUpgradeQuery{modules: &upgradetypes.QueryModuleVersionsResponse{
				ModuleVersions: []*upgradetypes.ModuleVersion{{Name: "deployment", Version: 7}},
			}}
			output, err := executeUpgradeQuery(t, query, nil, test.args...)
			require.NoError(t, err)
			require.Equal(t, test.wantFilter, query.moduleRequest.ModuleName)
			require.Contains(t, output, "deployment")
			require.Contains(t, output, "7")
		})
	}
}

func TestUpgradeModuleVersionsRejectAbsentResultAndTransportFailure(t *testing.T) {
	query := &recordingUpgradeQuery{modules: &upgradetypes.QueryModuleVersionsResponse{}}
	output, err := executeUpgradeQuery(t, query, nil, "module_versions")
	require.Error(t, err)
	require.Empty(t, output)

	wantErr := errors.New("module versions unavailable")
	query = &recordingUpgradeQuery{modulesErr: wantErr}
	output, err = executeUpgradeQuery(t, query, nil, "module_versions")
	require.ErrorIs(t, err, wantErr)
	require.Empty(t, output)
}
