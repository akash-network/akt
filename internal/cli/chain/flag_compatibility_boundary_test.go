package cli

import (
	"context"
	"errors"
	"io"
	"testing"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func TestSearchCompatibilityRejectsAmbiguousInput(t *testing.T) {
	require.ErrorContains(t, GetQueryAuthTxsByEventsCmd().ValidateArgs(nil), "events are required")
	for _, tc := range []struct {
		build func() *cobra.Command
		flag  string
	}{
		{QueryBlocksCmd, flagdefs.FlagQuery},
		{GetQueryAuthTxsByEventsCmd, flagdefs.FlagEvents},
	} {
		for _, args := range [][]string{{"one", "two"}, {"one", "--" + tc.flag, "two"}} {
			cmd := tc.build()
			require.NoError(t, cmd.ParseFlags(args))
			require.Error(t, cmd.ValidateArgs(cmd.Flags().Args()))
		}
		cmd := tc.build()
		require.NoError(t, cmd.Flags().Set(tc.flag, "one"))
		cmd.SetContext(context.WithValue(context.Background(), ContextTypeQueryClient,
			&stubLightClient{cctx: sdkclient.Context{}}))
		require.ErrorContains(t, cmd.RunE(cmd, []string{"two"}), "both positionally")
	}
}

func TestFlagOnlyQueriesPropagateOwnerResolutionFailures(t *testing.T) {
	for _, build := range []func() *cobra.Command{GetQueryDeploymentGroupCmd, GetQueryEscrowBlocksRemainingCmd} {
		for _, tc := range []struct {
			name string
			args []string
			from string
			want string
		}{
			{"invalid owner", []string{"--owner", "bad", "--dseq", "42"}, "", "decoding"},
			{"missing keyring", []string{"--dseq", "42"}, "missing-account", "keyring is unavailable"},
		} {
			cmd := build()
			t.Run(cmd.Name()+"/"+tc.name, func(t *testing.T) {
				require.NoError(t, cmd.ParseFlags(tc.args))
				cmd.SetContext(context.WithValue(context.Background(), ContextTypeQueryClient,
					&stubLightClient{cctx: sdkclient.Context{}.WithFrom(tc.from)}))
				require.ErrorContains(t, cmd.RunE(cmd, cmd.Flags().Args()), tc.want)
			})
		}
	}
	for _, args := range [][]string{{"bad"}, {"--dseq", "42"}} {
		cmd := GetQueryEscrowBlocksRemainingCmd()
		require.NoError(t, cmd.ParseFlags(args))
		cmd.SetContext(context.WithValue(context.Background(), ContextTypeQueryClient,
			&stubLightClient{cctx: sdkclient.Context{}}))
		require.Error(t, cmd.RunE(cmd, cmd.Flags().Args()))
	}
}

func TestTransactionHandlersRejectDuplicateIdentityBeforeBroadcast(t *testing.T) {
	fixture := newModuleTxFixture()
	err := GetTxDeploymentCloseCmd().ValidateArgs(nil)
	require.ErrorIs(t, err, errDSeqRequired)
	require.ErrorContains(t, err, "provide [dseq] or --dseq")
	for _, tc := range []struct {
		build func() *cobra.Command
		args  []string
	}{
		{GetTxDeploymentCloseCmd, []string{"42"}},
		{GetTxDeploymentUpdateCmd, []string{"../../workflow/adapters/testdata/deployment.yaml", "42"}},
	} {
		cmd := tc.build()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDSeq, "99"))
		capture := &moduleTxCapture{}
		err := runModuleTxHandler(t, fixture, cmd, capture, io.Discard, tc.args...)
		require.ErrorContains(t, err, "both positionally")
		require.Empty(t, capture.messages)
	}
	group := GetTxDeploymentGroupCloseCmd()
	require.NoError(t, group.Flags().Set(flagdefs.FlagDSeq, "99"))
	_, err = groupIDFromFlagsAndArgs(group, []string{"42", "1"}, fixture.owner)
	require.ErrorContains(t, err, "both positionally")
	lease := GetTxMarketLeaseCloseCmd()
	require.NoError(t, lease.Flags().Set(flagdefs.FlagOwner, "bad"))
	_, err = leaseIDFromFlagsAndArgs(lease, nil, fixture.owner)
	require.Error(t, err)
	lease = GetTxMarketLeaseCloseCmd()
	require.NoError(t, lease.Flags().Set(flagdefs.FlagDSeq, "99"))
	_, err = leaseIDFromFlagsAndArgs(lease, []string{"42"}, fixture.owner)
	require.ErrorContains(t, err, "both positionally")
	lease = &cobra.Command{}
	cflags.AddOrderIDFlags(lease.Flags())
	_, err = leaseIDFromFlagsAndArgs(lease, nil, fixture.owner)
	require.ErrorContains(t, err, "provider")
}

func TestStatusHandlesRPCFailuresAndYAML(t *testing.T) {
	wantErr := errors.New("status unavailable")
	for _, tc := range []struct {
		name string
		node sdkclient.CometRPC
		want string
	}{
		{"missing node", nil, "no RPC client is defined"},
		{"RPC failure", &blockRPCProbe{statusErr: wantErr}, wantErr.Error()},
		{"missing response", &blockRPCProbe{}, "missing node status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := StatusCmd()
			if tc.node == nil {
				require.NoError(t, cmd.Flags().Lookup(flagdefs.FlagNode).Value.Set(""))
			}
			result := executeBlockCommand(t, cmd, tc.node, nil)
			require.ErrorContains(t, result.err, tc.want)
		})
	}
	cmd := StatusCmd()
	cmd.Flags().String(flagdefs.FlagOutput, "yaml", "")
	result := executeBlockCommand(t, cmd, &blockRPCProbe{statusResponse: &coretypes.ResultStatus{}}, nil)
	require.NoError(t, result.err)
	require.Contains(t, result.stdout, "node_info:")
}

func TestRPCCommandsRejectInvalidNodeOverrides(t *testing.T) {
	for _, cmd := range []*cobra.Command{StatusCmd(), EventsCmd()} {
		result := executeBlockCommand(t, cmd, nil, nil, "--node", "http://[invalid")
		require.Error(t, result.err)
	}
	cmd := EventsCmd()
	require.NoError(t, cmd.Flags().Lookup(flagdefs.FlagNode).Value.Set(""))
	cmd.SetContext(context.WithValue(context.Background(), ClientContextKey, &sdkclient.Context{}))
	require.Error(t, getEvents(cmd.Context(), cmd, nil))
}
