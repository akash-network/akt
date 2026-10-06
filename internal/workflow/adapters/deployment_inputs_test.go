package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
	manifest "pkg.akt.dev/go/manifest/v2beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func TestChainRejectsConsoleSecretsBeforeBroadcast(t *testing.T) {
	referenced := strings.Replace(validConsoleSDL, "image: nginx:1.27-alpine", "image: nginx:1.27-alpine\n    env:\n      - TOKEN=ac-secret://TOKEN", 1)
	for _, test := range []struct {
		name    string
		sdl     string
		secrets string
	}{
		{name: "secret input", sdl: validConsoleSDL, secrets: "-"},
		{name: "Console reference", sdl: referenced},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := &fakeTxClient{}
			client := newFakeChainClient(tx, testOwner())
			_, err := client.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
				"sdl": test.sdl, "dseq": "72", "deposit": "5000000uact", "secrets-file": test.secrets,
			})
			if err == nil || !strings.Contains(err.Error(), "Console context") || len(tx.msgs) != 0 {
				t.Fatalf("Console-only input reached chain: messages=%d err=%v", len(tx.msgs), err)
			}
		})
	}
}

func TestChainRedeployUsesRecordedSDLWithoutClosingSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := writeTestSDL(t, validConsoleSDL)
	s, err := bbolt.OpenContext(ctx, root, "chain")
	if err != nil {
		t.Fatal(err)
	}
	err = s.PutDeployment(ctx, &store.DeploymentRecord{Owner: testOwner().String(), DSeq: 41, SDLPath: path, State: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "REDEPLOY"}}
	inputs := NewDeploymentInputs(root, "chain", nil)
	client := NewChainClient(&fakeChainSDKClient{tx: tx, cctx: testClientContext()}, inputs)
	result, err := client.BroadcastTx(ctx, msgCreateDeployment, map[string]string{
		"source-dseq": "41", "dseq": "42", "deposit": "5000000uact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.msgs) != 1 {
		t.Fatalf("messages=%d", len(tx.msgs))
	}
	created, ok := tx.msgs[0].(*dv1beta.MsgCreateDeployment)
	if !ok || created.ID.DSeq != 42 {
		t.Fatalf("redeploy message=%T", tx.msgs[0])
	}
	var data map[string]string
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["sdl_path"] != path || strings.Contains(string(result.Data), "services:") {
		t.Fatalf("result must record only source path: %s", result.Data)
	}
	if _, _, err := inputs.prepareChain(ctx, testOwner().String(), map[string]string{"source-dseq": "99"}); err == nil || !strings.Contains(err.Error(), "--sdl-file") {
		t.Fatalf("missing source should name override: %v", err)
	}
	if err := inputs.ValidateChain(ctx, "", map[string]string{"source-dseq": "41", "deposit": "5000000uact"}); err != nil {
		t.Fatalf("preflight source lookup without signer: %v", err)
	}
	if err := inputs.ValidateChain(ctx, "", map[string]string{"source-dseq": "41", "deposit": "5000000uakt"}); err == nil || !strings.Contains(err.Error(), "denomination") {
		t.Fatalf("source deposit mismatch passed preflight: %v", err)
	}
}

func TestChainPatchSharesExactManifestWithProvider(t *testing.T) {
	ctx := context.Background()
	base := writeTestSDL(t, validConsoleSDL)
	patchPath := filepath.Join(t.TempDir(), "patch.yaml")
	if err := os.WriteFile(patchPath, []byte("services:\n  web:\n    image: nginx:1.28-alpine\n    env:\n      MODE: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, groups, err := buildUpdateDeploymentMsg(testOwner(), map[string]string{"sdl": base, "dseq": "42"})
	if err != nil {
		t.Fatal(err)
	}
	query := &fakeWorkflowDeploymentQuery{deploymentResponse: &dv1beta.QueryDeploymentResponse{
		Groups: dv1beta.Groups{{ID: dv1.GroupID{Owner: testOwner().String(), DSeq: 42, GSeq: 1}, GroupSpec: groups[0]}},
	}}
	tx := &fakeTxClient{resp: &sdk.TxResponse{TxHash: "PATCH"}}
	inputs := NewDeploymentInputs("", "", nil)
	client := NewChainClient(&fakeChainSDKClient{tx: tx, query: &fakeWorkflowQueryClient{deployment: query}, cctx: testClientContext()}, inputs)
	result, err := client.BroadcastTx(ctx, msgUpdateDeployment, map[string]string{"sdl": patchPath, "dseq": "42", "patch": "true", "base-sdl": base})
	if err != nil {
		t.Fatal(err)
	}
	message := tx.msgs[0].(*dv1beta.MsgUpdateDeployment)
	if strings.Contains(string(result.Data), "production") {
		t.Fatal("expanded SDL leaked into workflow result")
	}
	if strings.Contains(string(result.Data), "sdl_path") {
		t.Fatal("patch result retained a stale base SDL path")
	}
	// Removing both files proves delivery uses the prepared SDL, not an input
	// path whose contents can change between signing and manifest submission.
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(patchPath); err != nil {
		t.Fatal(err)
	}
	var sent manifest.Manifest
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()
	connection := newAdapterQueryConnection(t, &adapterMarketQueryServer{}, &adapterProviderQueryServer{hostURI: gateway.URL})
	keyring, owner := newAdapterTestIdentity(t)
	provider := NewProviderClient(sdkclient.Context{}.WithFromAddress(owner).WithKeyring(keyring).WithGRPCClient(connection), "jwt", inputs)
	if err := provider.SendManifest(ctx, testProviderAddr().String(), 42, []byte(patchPath)); err != nil {
		t.Fatal(err)
	}
	version, err := sent.Version()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(version, message.Hash) {
		t.Fatal("provider received a different manifest than the signed update hash")
	}
	if sent[0].Services[0].Image != "nginx:1.28-alpine" {
		t.Fatalf("provider image=%q", sent[0].Services[0].Image)
	}
}
