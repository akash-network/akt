package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/types/query"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

// groupDeploymentQuery answers every deployment-module call the group command
// can make and records what it was asked, so a test can tell which path ran.
type groupDeploymentQuery struct {
	dvbeta.QueryClient

	listRes   *dvbeta.QueryDeploymentsResponse
	listErr   error
	lastList  *dvbeta.QueryDeploymentsRequest
	lastGet   *dvbeta.QueryDeploymentRequest
	lastGroup *dvbeta.QueryGroupRequest
}

func (s *groupDeploymentQuery) Deployments(_ context.Context, req *dvbeta.QueryDeploymentsRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentsResponse, error) {
	s.lastList = req
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.listRes, nil
}

func (s *groupDeploymentQuery) Deployment(_ context.Context, req *dvbeta.QueryDeploymentRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentResponse, error) {
	s.lastGet = req

	return &dvbeta.QueryDeploymentResponse{Groups: dvbeta.Groups{testGroup(req.ID.Owner, req.ID.DSeq, 1)}}, nil
}

func (s *groupDeploymentQuery) Group(_ context.Context, req *dvbeta.QueryGroupRequest, _ ...grpc.CallOption) (*dvbeta.QueryGroupResponse, error) {
	s.lastGroup = req

	return &dvbeta.QueryGroupResponse{Group: testGroup(req.ID.Owner, req.ID.DSeq, req.ID.GSeq)}, nil
}

func testGroup(owner string, dseq uint64, gseq uint32) dvbeta.Group {
	return dvbeta.Group{ID: dv1.GroupID{Owner: owner, DSeq: dseq, GSeq: gseq}, State: dvbeta.GroupOpen}
}

// printedGroupIDs decodes a JSON group array into owner/dseq/gseq strings, in
// output order.
func printedGroupIDs(t *testing.T, out string) []string {
	t.Helper()

	var groups []struct {
		ID struct {
			Owner string `json:"owner"`
			DSeq  string `json:"dseq"`
			GSeq  uint32 `json:"gseq"`
		} `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &groups), out)

	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, fmt.Sprintf("%s/%s/%d", g.ID.Owner, g.ID.DSeq, g.ID.GSeq))
	}

	return ids
}

// printedGroupPage decodes the JSON object a group listing prints: the groups
// of one page of deployments plus that page's pagination.
func printedGroupPage(t *testing.T, out string) (ids []string, nextKey []byte) {
	t.Helper()

	var page struct {
		Groups     json.RawMessage `json:"groups"`
		Pagination struct {
			NextKey []byte `json:"next_key"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &page), out)

	return printedGroupIDs(t, string(page.Groups)), page.Pagination.NextKey
}

// TestQueryDeploymentGroupListsNetworkWithoutFilter: no chain query lists
// groups, so a bare `query deployment group` lists one network-wide page of
// deployments and prints every group they carry (SPEC §3.8.5).
func TestQueryDeploymentGroupListsNetworkWithoutFilter(t *testing.T) {
	dep := &groupDeploymentQuery{listRes: &dvbeta.QueryDeploymentsResponse{
		Deployments: dvbeta.DeploymentResponses{
			{Groups: dvbeta.Groups{testGroup(stateTestOwner, 7, 1), testGroup(stateTestOwner, 7, 2)}},
			{Groups: dvbeta.Groups{testGroup(stateTestProvider, 9, 1)}},
		},
		Pagination: &query.PageResponse{NextKey: []byte("next")},
	}}

	out, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep})
	require.NoError(t, err)
	require.Empty(t, dep.lastList.Filters.Owner, "no owner filter: the whole network")
	require.Equal(t, uint64(100), dep.lastList.Pagination.Limit, "the default page size applies")

	ids, nextKey := printedGroupPage(t, out)
	require.Equal(t, []string{
		stateTestOwner + "/7/1",
		stateTestOwner + "/7/2",
		stateTestProvider + "/9/1",
	}, ids)
	require.Equal(t, []byte("next"), nextKey, "the deployments page key is what continues the listing")
}

// TestQueryDeploymentGroupListsOwnerGroups: a bare owner address lists the
// groups of that owner's deployments instead of failing as an incomplete path.
func TestQueryDeploymentGroupListsOwnerGroups(t *testing.T) {
	dep := &groupDeploymentQuery{listRes: &dvbeta.QueryDeploymentsResponse{}}

	out, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner)
	require.NoError(t, err)
	require.Equal(t, stateTestOwner, dep.lastList.Filters.Owner)
	require.Nil(t, dep.lastGet)

	ids, nextKey := printedGroupPage(t, out)
	require.Empty(t, ids, "an owner with no deployments prints an empty list")
	require.Empty(t, nextKey)
}

// TestQueryDeploymentGroupPagination: --limit and --page-key page the
// deployments the groups are read from.
func TestQueryDeploymentGroupPagination(t *testing.T) {
	dep := &groupDeploymentQuery{listRes: &dvbeta.QueryDeploymentsResponse{}}

	_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, "--limit", "5", "--page-key", "bmV4dA==")
	require.NoError(t, err)
	require.Equal(t, &query.PageRequest{Key: []byte("next"), Limit: 5}, dep.lastList.Pagination)
}

func TestQueryDeploymentGroupListErrors(t *testing.T) {
	// The chain takes an offset only with a state filter, which a group
	// listing cannot set, so the offset forms are refused before any request.
	for _, args := range [][]string{{"--page", "2"}, {"--offset", "5"}} {
		t.Run("offset paging "+args[0], func(t *testing.T) {
			dep := &groupDeploymentQuery{}

			_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, args...)
			require.ErrorContains(t, err, "deployment group: "+args[0]+" needs a state filter")
			require.Nil(t, dep.lastList, "no request is sent")
		})
	}

	t.Run("invalid page key", func(t *testing.T) {
		dep := &groupDeploymentQuery{}

		_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, "--page-key", "!!!")
		require.ErrorContains(t, err, "invalid pagination key")
		require.Nil(t, dep.lastList, "no request is sent")
	})

	t.Run("query failure", func(t *testing.T) {
		dep := &groupDeploymentQuery{listErr: errors.New("rpc unavailable")}

		_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep})
		require.ErrorContains(t, err, "rpc unavailable")
	})
}

// TestQueryDeploymentGroupGetPaths: a dseq still selects one deployment, and
// a gseq one group, without touching the list path.
func TestQueryDeploymentGroupGetPaths(t *testing.T) {
	t.Run("owner/dseq lists that deployment's groups", func(t *testing.T) {
		dep := &groupDeploymentQuery{}

		out, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner+"/12345")
		require.NoError(t, err)
		require.Equal(t, dv1.DeploymentID{Owner: stateTestOwner, DSeq: 12345}, dep.lastGet.ID)
		require.Nil(t, dep.lastList)
		require.Equal(t, []string{stateTestOwner + "/12345/1"}, printedGroupIDs(t, out))
	})

	t.Run("owner/dseq/gseq gets one group", func(t *testing.T) {
		dep := &groupDeploymentQuery{}

		_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner+"/12345/2")
		require.NoError(t, err)
		require.Equal(t, dv1.GroupID{Owner: stateTestOwner, DSeq: 12345, GSeq: 2}, dep.lastGroup.ID)
		require.Nil(t, dep.lastList)
	})

	t.Run("invalid path", func(t *testing.T) {
		dep := &groupDeploymentQuery{}

		_, err := execQueryCmd(t, GetQueryDeploymentGroupCmd(), &stubQueryClient{dep: dep}, stateTestOwner+"/notanumber")
		require.ErrorContains(t, err, "group ID: invalid dseq")
		require.Nil(t, dep.lastList)
		require.Nil(t, dep.lastGet)
	})
}
