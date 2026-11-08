package cli

import (
	"context"
	"io"
	"testing"
	"time"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

func TestRestoredQueryFlagsMatchPositionalFilters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() *cobra.Command
	}{
		{"deployment", GetQueryDeploymentCmds},
		{"order", GetQueryMarketOrderCmd},
		{"bid", GetQueryMarketBidCmd},
		{"lease", GetQueryMarketLeaseCmd},
		{"cert", GetQueryCertCertificatesCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := "active"
			if tc.name == "cert" {
				state = "valid"
			}
			var requests []any
			for _, args := range [][]string{
				{stateTestOwner, state},
				{"--owner", stateTestOwner, "--state", state},
				{stateTestOwner, "--state", state},
				{state, "--owner", stateTestOwner},
			} {
				dep, market, cert := &capturingDeploymentQuery{}, &capturingMarketQuery{}, &capturingCertQuery{}
				query := &certQueryClient{stubQueryClient: &stubQueryClient{dep: dep, mkt: market}, cert: cert}
				_, err := execQueryCmdFrom(t, tc.build(), query, stateTestProvider, args...)
				require.NoError(t, err)
				switch tc.name {
				case "deployment":
					requests = append(requests, dep.lastList)
				case "order":
					requests = append(requests, market.orders)
				case "bid":
					requests = append(requests, market.bids)
				case "lease":
					requests = append(requests, market.leases)
				case "cert":
					requests = append(requests, cert.last)
				}
			}
			for _, request := range requests[1:] {
				require.Equal(t, requests[0], request)
			}
		})
	}
}

func TestRestoredQueryFlagsFailBeforeClientDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() *cobra.Command
	}{
		{"deployment", GetQueryDeploymentCmds}, {"order", GetQueryMarketOrderCmd},
		{"bid", GetQueryMarketBidCmd}, {"lease", GetQueryMarketLeaseCmd},
		{"cert", GetQueryCertCertificatesCmd},
	} {
		for _, args := range [][]string{
			{"--owner", "invalid-address"}, {"--state", "invalid-state"},
			{stateTestOwner, "--owner", stateTestProvider},
		} {
			t.Run(tc.name+"/"+args[0], func(t *testing.T) {
				cmd := tc.build()
				called := false
				cmd.PersistentPreRunE = func(*cobra.Command, []string) error { called = true; return nil }
				require.Error(t, execTxCmdArgsOnly(t, cmd, args...))
				require.False(t, called)
			})
		}
	}
}

type flagDeploymentQuery struct {
	dvbeta.QueryClient
	deployment *dvbeta.QueryDeploymentRequest
	group      *dvbeta.QueryGroupRequest
}

func (q *flagDeploymentQuery) Deployment(_ context.Context, request *dvbeta.QueryDeploymentRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentResponse, error) {
	q.deployment = request
	return &dvbeta.QueryDeploymentResponse{Deployment: dv1.Deployment{ID: request.ID, State: dv1.DeploymentActive}}, nil
}

func (q *flagDeploymentQuery) Group(_ context.Context, request *dvbeta.QueryGroupRequest, _ ...grpc.CallOption) (*dvbeta.QueryGroupResponse, error) {
	q.group = request
	return &dvbeta.QueryGroupResponse{}, nil
}

func TestDeploymentIdentityFlagsPreserveUnifiedGetAndGroupList(t *testing.T) {
	for _, tc := range []struct {
		build func() *cobra.Command
		flags []string
		group bool
	}{
		{GetQueryDeploymentCmds, []string{"--dseq", "42"}, false},
		{GetQueryDeploymentGroupCmd, []string{"--dseq", "42"}, false},
		{GetQueryDeploymentGroupCmd, []string{"--dseq", "42", "--gseq", "3"}, true},
	} {
		query := &flagDeploymentQuery{}
		_, err := execQueryCmdFrom(t, tc.build(), &stubQueryClient{dep: query}, stateTestOwner, tc.flags...)
		require.NoError(t, err)
		if tc.group {
			require.Equal(t, dv1.GroupID{Owner: stateTestOwner, DSeq: 42, GSeq: 3}, query.group.ID)
			require.Nil(t, query.deployment)
		} else {
			require.Equal(t, dv1.DeploymentID{Owner: stateTestOwner, DSeq: 42}, query.deployment.ID)
			require.Nil(t, query.group)
		}
	}
}

func TestRestoredTransactionFlagsMatchPositionalMessages(t *testing.T) {
	fixture := newModuleTxFixture()
	for _, tc := range []struct {
		name        string
		build       func() *cobra.Command
		args, flags []string
	}{
		{"deployment close", GetTxDeploymentCloseCmd, []string{"42"}, []string{"--dseq", "42"}},
		{"group close", GetTxDeploymentGroupCloseCmd, []string{"42", "3"}, []string{"--dseq", "42", "--gseq", "3"}},
		{"group pause", GetDeploymentGroupPauseCmd, []string{"42", "3"}, []string{"--dseq", "42", "--gseq", "3"}},
		{"group start", GetDeploymentGroupStartCmd, []string{"42", "3"}, []string{"--dseq", "42", "--gseq", "3"}},
		{"lease create", GetTxMarketLeaseCreateCmd, []string{"42", fixture.provider.String()}, []string{"--dseq", "42", "--provider", fixture.provider.String()}},
		{"lease close", GetTxMarketLeaseCloseCmd, []string{"42", fixture.provider.String()}, []string{"--dseq", "42", "--provider", fixture.provider.String()}},
		{"lease withdraw", GetTxMarketLeaseWithdrawCmd, []string{"42", fixture.provider.String()}, []string{"--dseq", "42", "--provider", fixture.provider.String()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var messages [][]sdk.Msg
			for _, input := range [][]string{tc.args, tc.flags} {
				cmd := tc.build()
				require.NoError(t, cmd.ParseFlags(input))
				args := cmd.Flags().Args()
				require.NoError(t, cmd.ValidateArgs(args))
				capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "PARITY"}}
				require.NoError(t, runModuleTxHandler(t, fixture, cmd, capture, io.Discard, args...))
				require.Len(t, capture.messages, 1)
				messages = append(messages, capture.messages[0])
			}
			require.Equal(t, messages[0], messages[1])
			err := execTxCmdArgsOnly(t, tc.build(), append(tc.args, tc.flags...)...)
			require.ErrorContains(t, err, "both positionally")
		})
	}
}

func TestBidReclamationWindowReachesMessage(t *testing.T) {
	fixture := newModuleTxFixture()
	for _, value := range []string{"", "24h", "nonsense", "-1h"} {
		t.Run(value, func(t *testing.T) {
			cmd := GetTxMarketBidCreateCmd()
			for name, value := range map[string]string{
				flagdefs.FlagOwner: fixture.owner.String(), flagdefs.FlagDSeq: "42",
				flagdefs.FlagDeposit: "500000uakt", flagdefs.FlagPrice: "0.025uakt",
				flagdefs.FlagReclamationWindow: value,
			} {
				require.NoError(t, cmd.Flags().Set(name, value))
			}
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			err := runModuleTxHandler(t, fixture, cmd, capture, io.Discard)
			if value == "nonsense" || value == "-1h" {
				require.Error(t, err)
				require.Empty(t, capture.messages)
				return
			}
			require.NoError(t, err)
			message := requireSingleModuleMessage[*mvbeta.MsgCreateBid](t, capture)
			if value == "" {
				require.Nil(t, message.ReclamationWindow)
			} else {
				require.Equal(t, 24*time.Hour, *message.ReclamationWindow)
			}
		})
	}
}

func TestBlockQueryFlagReachesRPC(t *testing.T) {
	rpc := &blockRPCProbe{searchResponse: &coretypes.ResultBlockSearch{}}
	result := executeBlockCommand(t, QueryBlocksCmd(), rpc, nil, "--query", "block.height > 17", "--output", "json")
	require.NoError(t, result.err)
	require.Equal(t, "block.height > 17", rpc.searchQuery)
}

func TestStatusCommandUsesPublicRPCAndCommandOutput(t *testing.T) {
	rpc := &blockRPCProbe{statusResponse: &coretypes.ResultStatus{}}
	cmd := StatusCmd()
	cmd.Flags().String(flagdefs.FlagOutput, "json", "")
	result := executeBlockCommand(t, cmd, rpc, nil)
	require.NoError(t, result.err)
	require.Contains(t, result.stdout, "node_info")
	require.Equal(t, 1, rpc.statusCalls)
}

func TestZeroQuerySequenceFlagsRemainNetworkWide(t *testing.T) {
	market := &capturingMarketQuery{}
	_, err := execQueryCmdFrom(t, GetQueryMarketLeaseCmd(), &stubQueryClient{mkt: market}, stateTestOwner,
		"--owner", "", "--dseq", "0", "--gseq", "0", "--oseq", "0")
	require.NoError(t, err)
	require.Empty(t, market.leases.Filters.Owner)
	require.Zero(t, market.leases.Filters.DSeq)
	require.Zero(t, market.leases.Filters.GSeq)
	require.Zero(t, market.leases.Filters.OSeq)
}
