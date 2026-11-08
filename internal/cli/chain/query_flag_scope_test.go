package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func TestMarketDSeqFlagsMatchPositionalOwnerScope(t *testing.T) {
	for _, build := range []func() *cobra.Command{
		GetQueryMarketOrderCmd,
		GetQueryMarketBidCmd,
		GetQueryMarketLeaseCmd,
	} {
		t.Run(build().Name(), func(t *testing.T) {
			var requests []*capturingMarketQuery
			for _, args := range [][]string{
				{"42", "active"},
				{"--dseq", "42", "--state", "active"},
				{"active", "--dseq", "42"},
			} {
				market := &capturingMarketQuery{}
				_, err := execQueryCmdFrom(t, build(), &stubQueryClient{mkt: market}, stateTestOwner, args...)
				require.NoError(t, err)
				require.Equal(t, 1, market.listCalls)
				requests = append(requests, market)
			}
			for _, request := range requests[1:] {
				require.Equal(t, requests[0], request)
			}
		})
	}
}

func TestPositiveDSeqFlagsRequireAnOwnerBeforeQuery(t *testing.T) {
	for _, build := range []func() *cobra.Command{
		GetQueryDeploymentCmds,
		GetQueryDeploymentGroupCmd,
		GetQueryMarketOrderCmd,
		GetQueryMarketBidCmd,
		GetQueryMarketLeaseCmd,
	} {
		t.Run(build().Name(), func(t *testing.T) {
			_, err := execQueryCmdFrom(t, build(), &stubQueryClient{}, "", "--dseq", "42")
			require.ErrorContains(t, err, "no default account set")
		})
	}
}

func TestStateAndZeroSequenceFlagListsRemainNetworkWide(t *testing.T) {
	for _, kind := range []string{"deployment", "order", "bid", "lease", "certificate"} {
		t.Run(kind, func(t *testing.T) {
			state := "active"
			if kind == "certificate" {
				state = "valid"
			}
			args := []string{"--state", state}
			if kind != "certificate" {
				args = append(args, "--dseq", "0")
			}
			got := runBareList(t, kind, stateTestOwner, args...)
			require.Empty(t, got.owner)
			require.Empty(t, got.provider)
		})
	}
}

func TestProviderDSeqFlagsDoNotAddDefaultOwner(t *testing.T) {
	for _, kind := range []string{"bid", "lease"} {
		t.Run(kind, func(t *testing.T) {
			got := runBareList(t, kind, stateTestOwner,
				"--by", "provider", "--provider", stateTestProvider, "--dseq", "42")
			require.Empty(t, got.owner)
			require.Equal(t, stateTestProvider, got.provider)
		})
	}
}

func TestDeploymentGroupFlagsPreservePagedLists(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		owner string
	}{
		{
			name: "network",
			args: []string{"--dseq", "0", "--gseq", "0"},
		},
		{
			name:  "explicit owner",
			args:  []string{"--owner", stateTestProvider},
			owner: stateTestProvider,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dep := &groupDeploymentQuery{listRes: &dvbeta.QueryDeploymentsResponse{}}
			args := append(tc.args, "--limit", "5", "--page-key", "bmV4dA==")
			_, err := execQueryCmdFrom(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner, args...)
			require.NoError(t, err)
			require.Equal(t, tc.owner, dep.lastList.Filters.Owner)
			require.Equal(t, uint64(5), dep.lastList.Pagination.Limit)
			require.Equal(t, []byte("next"), dep.lastList.Pagination.Key)
			require.Nil(t, dep.lastGet)
			require.Nil(t, dep.lastGroup)
		})
	}
}

func TestDeploymentGroupSequenceRequiresDeployment(t *testing.T) {
	dep := &groupDeploymentQuery{}
	_, err := execQueryCmdFrom(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner, "--gseq", "1")
	require.ErrorContains(t, err, "--gseq requires --dseq")
	require.Nil(t, dep.lastList)
	require.Nil(t, dep.lastGet)
	require.Nil(t, dep.lastGroup)
}
