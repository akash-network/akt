package provider

import (
	"context"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	mv1 "pkg.akt.dev/go/node/market/v1"
)

func TestProviderNodeOverrideReplacesContextEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "http://127.0.0.1:26659"} {
		root := Commands()
		cmd, _, err := root.Find([]string{"lease-status"})
		require.NoError(t, err)
		args := []string{"--dseq", "42"}
		if endpoint != "" {
			args = append(args, "--node", endpoint)
		}
		require.NoError(t, cmd.ParseFlags(args))
		ctx := context.WithValue(walletlessCommandContext(), chaincli.ContextTypeRPCURI, "http://127.0.0.1:26658")
		cmd.SetContext(ctx)
		cctx, err := providerQueryContext(cmd)
		require.NoError(t, err)
		if endpoint == "" {
			endpoint = "http://127.0.0.1:26658"
		}
		require.Equal(t, endpoint, cctx.NodeURI)
		require.Equal(t, endpoint, sdkclient.GetClientContextFromCmd(cmd).NodeURI)
	}
}

func TestManifestSequenceFlagsFilterTargetProviders(t *testing.T) {
	cmd := newProviderCmd(t, "--dseq", "42", "--gseq", "2", "--oseq", "3")
	cmd.SetContext(providerOutputTestContext(t))
	scope, err := leaseScopeFromCmd(cmd, nil)
	require.NoError(t, err)
	providers, err := activeLeaseProviders(context.Background(), scope, func(context.Context, string, uint64) ([]mv1.Lease, error) {
		return []mv1.Lease{
			{ID: mv1.LeaseID{DSeq: 42, GSeq: 1, OSeq: 3, Provider: "wrong-group"}, State: mv1.LeaseActive},
			{ID: mv1.LeaseID{DSeq: 42, GSeq: 2, OSeq: 1, Provider: "wrong-order"}, State: mv1.LeaseActive},
			{ID: mv1.LeaseID{DSeq: 42, GSeq: 2, OSeq: 3, Provider: testProviderAddr}, State: mv1.LeaseActive},
		}, nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{testProviderAddr}, providers)
}

func TestRestoredProviderDSeqConflictsFailBeforeAuthentication(t *testing.T) {
	for _, name := range []string{"lease-status", "lease-attestation", "lease-logs", "lease-events", "get-manifest", "service-status"} {
		cmd, _, err := Commands().Find([]string{name})
		require.NoError(t, err)
		require.NoError(t, cmd.ParseFlags([]string{"42", "--dseq", "99"}))
		require.ErrorContains(t, cmd.ValidateArgs(cmd.Flags().Args()), "both positionally")
	}
}

func TestProviderServiceAndReplicaFlags(t *testing.T) {
	cmd := leaseShellCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--replica-index", "3", "--service", "web", "--", "echo", "hello"}))
	require.Equal(t, []string{"echo", "hello"}, cmd.Flags().Args())
	index, err := cmd.Flags().GetUint(flagdefs.FlagReplicaIndex)
	require.NoError(t, err)
	require.EqualValues(t, 3, index)
	require.NoError(t, leaseEventsCmd().ParseFlags([]string{"--service", "web,api"}))
}
