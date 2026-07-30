package console

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func item(dseq string, leaseDSeqs ...string) DeploymentListItem {
	it := DeploymentListItem{
		Deployment: Deployment{ID: DeploymentID{Owner: "akash1owner", DSeq: FlexString(dseq)}},
	}

	for _, ld := range leaseDSeqs {
		it.Leases = append(it.Leases, Lease{
			ID: LeaseID{Owner: "akash1owner", DSeq: FlexString(ld), Provider: "p" + ld},
		})
	}

	return it
}

func leaseDSeqs(it DeploymentListItem) []string {
	out := make([]string, 0, len(it.Leases))
	for _, l := range it.Leases {
		out = append(out, l.ID.DSeq.String())
	}

	return out
}

// TestRegroupLeasesRepairsUpstreamPermutation pins the fix for GET
// /v1/deployments handing back leases filed under the wrong deployments. Every
// lease carries its own dseq, so the correct pairing is recoverable from the
// response the caller already has.
func TestRegroupLeasesRepairsUpstreamPermutation(t *testing.T) {
	// A rotation, which is what the API actually returns.
	items := []DeploymentListItem{
		item("100", "200"),
		item("200", "300"),
		item("300", "100"),
	}

	regroupLeases(items)

	require.Equal(t, []string{"100"}, leaseDSeqs(items[0]))
	require.Equal(t, []string{"200"}, leaseDSeqs(items[1]))
	require.Equal(t, []string{"300"}, leaseDSeqs(items[2]))
}

// A correct response must survive untouched.
func TestRegroupLeasesLeavesCorrectPairingAlone(t *testing.T) {
	items := []DeploymentListItem{
		item("100", "100"),
		item("200", "200"),
	}

	regroupLeases(items)

	require.Equal(t, []string{"100"}, leaseDSeqs(items[0]))
	require.Equal(t, []string{"200"}, leaseDSeqs(items[1]))
}

// Several leases on one deployment all land together, and nothing is lost.
func TestRegroupLeasesKeepsEveryLeaseItCanPlace(t *testing.T) {
	items := []DeploymentListItem{
		item("100", "200", "200"),
		item("200", "100"),
	}

	regroupLeases(items)

	require.Equal(t, []string{"100"}, leaseDSeqs(items[0]))
	require.Equal(t, []string{"200", "200"}, leaseDSeqs(items[1]))
}

// A lease belonging to a deployment outside this page is dropped rather than
// guessed at -- inventing a home for it is the bug this repairs.
func TestRegroupLeasesDropsUnplaceableLease(t *testing.T) {
	items := []DeploymentListItem{item("100", "999")}

	regroupLeases(items)

	require.Empty(t, items[0].Leases)
}

func TestRegroupLeasesHandlesEmptyPage(t *testing.T) {
	require.NotPanics(t, func() { regroupLeases(nil) })
}
