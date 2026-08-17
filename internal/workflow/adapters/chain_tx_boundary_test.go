package adapters

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func TestBroadcastCreateDeploymentCarriesResolvedIdentitySDLAndDeposit(t *testing.T) {
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "CREATE1", Height: 73}}
	client := newFakeChainClient(tx, testOwner())

	result, err := client.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     testSDLPath,
		"dseq":    "72",
		"deposit": "5000000uakt",
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if len(tx.msgs) != 1 {
		t.Fatalf("broadcast messages = %d, want one", len(tx.msgs))
	}
	message, ok := tx.msgs[0].(*dv1beta.MsgCreateDeployment)
	if !ok {
		t.Fatalf("broadcast message type = %T", tx.msgs[0])
	}
	if message.ID != (dv1.DeploymentID{Owner: testOwner().String(), DSeq: 72}) {
		t.Errorf("deployment identity = %+v", message.ID)
	}
	if len(message.Groups) == 0 || len(message.Hash) == 0 {
		t.Errorf("SDL was not compiled into groups/hash: groups=%d hash=%x", len(message.Groups), message.Hash)
	}
	if message.Deposit.Amount.String() != "5000000uakt" {
		t.Errorf("deposit = %s, want 5000000uakt", message.Deposit.Amount)
	}

	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode result data: %v", err)
	}
	if result.TxHash != "CREATE1" || result.Height != 73 || data["dseq"] != "72" ||
		data["owner"] != testOwner().String() {
		t.Fatalf("tx result = %#v data=%v", result, data)
	}
}

func TestBroadcastUpdateVerifiesLiveGroupsBeforeSendingNewHash(t *testing.T) {
	params := map[string]string{"sdl": testSDLPath, "dseq": "82"}
	_, expectedGroups, err := buildUpdateDeploymentMsg(testOwner(), params)
	if err != nil {
		t.Fatalf("build expected update: %v", err)
	}
	query := &fakeWorkflowDeploymentQuery{deploymentResponse: &dv1beta.QueryDeploymentResponse{
		Groups: dv1beta.Groups{{
			ID:        dv1.GroupID{Owner: testOwner().String(), DSeq: 82, GSeq: 1},
			GroupSpec: expectedGroups[0],
		}},
	}}
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "UPDATE1", Height: 83}}
	client := &chainClient{cl: &fakeChainSDKClient{
		tx:    tx,
		query: &fakeWorkflowQueryClient{deployment: query},
		cctx:  testClientContext(),
	}}

	result, err := client.BroadcastTx(context.Background(), msgUpdateDeployment, params)
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if query.lastDeploymentID != (dv1.DeploymentID{Owner: testOwner().String(), DSeq: 82}) {
		t.Errorf("verified deployment = %+v", query.lastDeploymentID)
	}
	if len(tx.msgs) != 1 {
		t.Fatalf("broadcast messages = %d, want one", len(tx.msgs))
	}
	message, ok := tx.msgs[0].(*dv1beta.MsgUpdateDeployment)
	if !ok {
		t.Fatalf("broadcast message type = %T", tx.msgs[0])
	}
	if message.ID != query.lastDeploymentID || len(message.Hash) == 0 {
		t.Errorf("update message = %+v", message)
	}
	if result.TxHash != "UPDATE1" || result.Height != 83 {
		t.Errorf("tx result = %+v", result)
	}
}
