package pretty

import (
	"bytes"
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	eidv1 "pkg.akt.dev/go/node/escrow/id/v1"
	etypesv1 "pkg.akt.dev/go/node/escrow/types/v1"
	rtypes "pkg.akt.dev/go/node/types/resources/v1beta4"
)

func makeResourceUnits() dvbeta.ResourceUnits {
	return dvbeta.ResourceUnits{
		{
			Resources: rtypes.Resources{
				CPU: &rtypes.CPU{
					Units: rtypes.ResourceValue{Val: math.NewInt(4000)},
				},
				Memory: &rtypes.Memory{
					Quantity: rtypes.ResourceValue{Val: math.NewInt(8589934592)},
				},
				Storage: rtypes.Volumes{
					{
						Name:     "default",
						Quantity: rtypes.ResourceValue{Val: math.NewInt(53687091200)},
					},
				},
				GPU: &rtypes.GPU{
					Units: rtypes.ResourceValue{Val: math.NewInt(0)},
				},
			},
			Count: 1,
			Price: sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("12.5")),
		},
	}
}

func makeGroup(owner string, dseq uint64, gseq uint32, name string, state dvbeta.Group_State) dvbeta.Group {
	return dvbeta.Group{
		ID: dv1.GroupID{
			Owner: owner,
			DSeq:  dseq,
			GSeq:  gseq,
		},
		State:     state,
		CreatedAt: 18234567,
		GroupSpec: dvbeta.GroupSpec{
			Name:      name,
			Resources: makeResourceUnits(),
		},
	}
}

func makeEscrowAccount() etypesv1.Account {
	return etypesv1.Account{
		ID: eidv1.Account{
			Scope: eidv1.ScopeDeployment,
			XID:   "akash1qwerty/100",
		},
		State: etypesv1.AccountState{
			State: etypesv1.StateOpen,
			Owner: "akash1qwerty",
			Funds: []etypesv1.Balance{
				{Denom: "uakt", Amount: math.LegacyMustNewDecFromStr("500.25")},
			},
			Transferred: sdk.NewDecCoins(
				sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("100.5")),
			),
		},
	}
}

func TestRenderDeploymentList(t *testing.T) {
	tests := map[string]struct {
		resp *dvbeta.QueryDeploymentsResponse
	}{
		"Empty": {
			resp: &dvbeta.QueryDeploymentsResponse{},
		},
		"WithDeployments": {
			resp: &dvbeta.QueryDeploymentsResponse{
				Deployments: dvbeta.DeploymentResponses{
					{
						Deployment: dv1.Deployment{
							ID: dv1.DeploymentID{
								Owner: "akash1qwerty",
								DSeq:  100,
							},
							State:     dv1.DeploymentActive,
							CreatedAt: 18234567,
						},
						Groups: dvbeta.Groups{
							makeGroup("akash1qwerty", 100, 1, "web", dvbeta.GroupOpen),
						},
					},
					{
						Deployment: dv1.Deployment{
							ID: dv1.DeploymentID{
								Owner: "akash1zxcvbn",
								DSeq:  200,
							},
							State:     dv1.DeploymentClosed,
							CreatedAt: 17000000,
						},
						Groups: dvbeta.Groups{
							makeGroup("akash1zxcvbn", 200, 1, "api", dvbeta.GroupClosed),
							makeGroup("akash1zxcvbn", 200, 2, "worker", dvbeta.GroupClosed),
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDeploymentList(tc.resp))
		})
	}
}

func TestRenderDeploymentDetail(t *testing.T) {
	tests := map[string]struct {
		resp *dvbeta.QueryDeploymentResponse
	}{
		"SingleGroup": {
			resp: &dvbeta.QueryDeploymentResponse{
				Deployment: dv1.Deployment{
					ID: dv1.DeploymentID{
						Owner: "akash1qwerty",
						DSeq:  100,
					},
					State:     dv1.DeploymentActive,
					Hash:      []byte{0xab, 0xcd, 0xef, 0x01, 0x23},
					CreatedAt: 18234567,
				},
				Groups: dvbeta.Groups{
					makeGroup("akash1qwerty", 100, 1, "web", dvbeta.GroupOpen),
				},
				EscrowAccount: makeEscrowAccount(),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDeploymentDetail(tc.resp))
		})
	}
}

func TestRenderGroupDetail(t *testing.T) {
	tests := map[string]struct {
		resp *dvbeta.QueryGroupResponse
	}{
		"Normal": {
			resp: &dvbeta.QueryGroupResponse{
				Group: makeGroup("akash1qwerty", 100, 1, "web", dvbeta.GroupOpen),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderGroupDetail(tc.resp))
		})
	}
}

func TestRenderGroupsList(t *testing.T) {
	tests := map[string]struct {
		groups dvbeta.Groups
	}{
		"TwoGroups": {
			groups: dvbeta.Groups{
				makeGroup("akash1qwerty", 100, 1, "web", dvbeta.GroupOpen),
				makeGroup("akash1qwerty", 100, 2, "worker", dvbeta.GroupClosed),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderGroupsList(tc.groups))
		})
	}
}

func TestPrintGroupsListUsesPlainCommandWriterOutsideTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	cmd := &cobra.Command{Use: "group"}
	cflags.AddQueryFlagsToCmd(cmd)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	groups := dvbeta.Groups{makeGroup("akash1qwerty", 100, 1, "web", dvbeta.GroupOpen)}
	require.NoError(t, PrintGroupsList(cmd, sdkclient.Context{}, groups))
	require.Contains(t, stdout.String(), "Group 1")
	require.NotContains(t, stdout.String(), "\x1b[")
}
