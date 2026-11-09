package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestGenerateOnlyDoesNotContactNode(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "generate-only must not contact the node", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	m := rootTestManager(t)
	require.NoError(t, m.CreateContext(aktctx.Context{
		Name:    "offline-generation",
		Network: aktctx.Network{Name: "mainnet"},
		Keyring: aktctx.Keyring{Name: "default"},
	}))
	cfg := m.Config()
	cfg.CurrentContext = "offline-generation"
	cfg.Networks[0].Endpoints.RPC = []string{server.URL}
	require.NoError(t, aktctx.SaveConfig(m.Root(), &cfg))

	enc := aktcodec.MakeEncodingConfig()
	keyrings := aktkeyring.NewManager(m.Root(), cfg.Keyrings, enc.Codec)
	kr, err := keyrings.Get("default")
	require.NoError(t, err)
	pub := secp256k1.GenPrivKey().PubKey()
	_, err = kr.SaveOfflineKey("cheng", pub)
	require.NoError(t, err)
	address := sdk.AccAddress(pub.Address()).String()
	constituents := make([]cryptotypes.PubKey, 5)
	for i := range constituents {
		constituents[i] = secp256k1.GenPrivKey().PubKey()
	}
	guardian := multisig.NewLegacyAminoPubKey(3, constituents)
	_, err = kr.SaveMultisig("guardian", guardian)
	require.NoError(t, err)
	_, err = kr.SaveOfflineKey("imported-guardian", guardian)
	require.NoError(t, err)
	guardianAddress := sdk.AccAddress(guardian.Address()).String()

	for _, tc := range []struct {
		name        string
		args        []string
		signer      string
		gas         string
		wantGas     string
		wantType    string
		wantError   string
		contextNode bool
		wantSigner  string
		defaultFees bool
	}{
		{
			name:        "five-key multisig guardian with default fees",
			args:        []string{"sink", "guardian-pause", "all"},
			signer:      "guardian",
			wantSigner:  guardianAddress,
			wantGas:     "200000",
			wantType:    "/akash.sink.v1.MsgGuardianPause",
			contextNode: true,
			defaultFees: true,
		},
		{
			name:        "imported multisig guardian with default fees",
			args:        []string{"sink", "guardian-pause", "all"},
			signer:      "imported-guardian",
			wantSigner:  guardianAddress,
			wantGas:     "200000",
			wantType:    "/akash.sink.v1.MsgGuardianPause",
			contextNode: true,
			defaultFees: true,
		},
		{
			name:        "multisig address with default fees",
			args:        []string{"sink", "guardian-pause", "all"},
			signer:      guardianAddress,
			wantSigner:  guardianAddress,
			wantGas:     "200000",
			wantType:    "/akash.sink.v1.MsgGuardianPause",
			contextNode: true,
			defaultFees: true,
		},
		{
			name:        "address absent from keyring with default fees",
			args:        []string{"sink", "guardian-pause", "all"},
			signer:      "akash1aagclq778tq4ga0lke3wrd3mgw0rt3cc6670dp",
			wantSigner:  "akash1aagclq778tq4ga0lke3wrd3mgw0rt3cc6670dp",
			wantGas:     "200000",
			wantType:    "/akash.sink.v1.MsgGuardianPause",
			contextNode: true,
			defaultFees: true,
		},
		{
			name:        "guardian pause with public key and default gas",
			args:        []string{"sink", "guardian-pause", "all"},
			signer:      "cheng",
			wantGas:     "200000",
			wantType:    "/akash.sink.v1.MsgGuardianPause",
			contextNode: true,
		},
		{
			name:      "generation rejects simulation",
			args:      []string{"sink", "guardian-pause", "all", "--dry-run"},
			signer:    "cheng",
			wantError: "--generate-only and --dry-run cannot be used together",
		},
		{
			name:     "guardian pause with address and explicit auto gas",
			args:     []string{"sink", "guardian-pause", "all"},
			signer:   address,
			gas:      "auto",
			wantGas:  "200000",
			wantType: "/akash.sink.v1.MsgGuardianPause",
		},
		{
			name:     "guardian pause preserves numeric gas",
			args:     []string{"sink", "guardian-pause", "all"},
			signer:   "cheng",
			gas:      "345678",
			wantGas:  "345678",
			wantType: "/akash.sink.v1.MsgGuardianPause",
		},
		{
			name:     "bank send",
			args:     []string{"bank", "send", address, "1uakt"},
			signer:   "cheng",
			wantGas:  "200000",
			wantType: "/cosmos.bank.v1beta1.MsgSend",
		},
		{
			name:     "deployment close",
			args:     []string{"deployment", "close", "12345"},
			signer:   address,
			wantGas:  "200000",
			wantType: "/akash.deployment.v1beta4.MsgCloseDeployment",
		},
		{
			name:     "SDK transaction factory",
			args:     []string{"upgrade", "cancel-software-upgrade", "--title", "cancel", "--summary", "cancel upgrade", "--deposit", "1uakt", "--gas-prices", ""},
			signer:   "cheng",
			wantGas:  "200000",
			wantType: "/cosmos.gov.v1.MsgSubmitProposal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			root := NewRootCmd(BuildInfo{Version: "test"})
			root.SetContext(ctx)
			var out, diagnostics bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&diagnostics)
			initialize := root.PersistentPreRunE
			root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
				if err := initialize(cmd, args); err != nil {
					return err
				}
				// SDK-owned handlers print through client.Context.Output.
				cctx := chaincli.GetClientContextFromCmd(cmd).WithOutput(&out)
				return chaincli.SetCmdClientContext(cmd, cctx)
			}
			args := append([]string{"--home", m.Root(), "tx"}, tc.args...)
			args = append(args, "--generate-only", "--from="+tc.signer, "--note", "offline payload")
			if !tc.defaultFees {
				args = append(args, "--fees", "77uakt")
			}
			if !tc.contextNode {
				args = append(args, "--node", server.URL)
			}
			if tc.gas != "" {
				args = append(args, "--gas", tc.gas)
			}
			root.SetArgs(args)
			err := Execute(root)
			require.Zero(t, requests.Load(), "generation made an HTTP or WebSocket request")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, out.String())
				return
			}
			require.NoError(t, err, diagnostics.String())

			var tx struct {
				Body struct {
					Messages []map[string]any `json:"messages"`
					Memo     string           `json:"memo"`
				} `json:"body"`
				AuthInfo struct {
					Fee struct {
						GasLimit string    `json:"gas_limit"`
						Amount   sdk.Coins `json:"amount"`
					} `json:"fee"`
				} `json:"auth_info"`
				Signatures []string `json:"signatures"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &tx), out.String())
			require.Len(t, tx.Body.Messages, 1)
			require.Equal(t, tc.wantType, tx.Body.Messages[0]["@type"])
			require.Equal(t, "offline payload", tx.Body.Memo)
			require.Equal(t, tc.wantGas, tx.AuthInfo.Fee.GasLimit)
			if tc.defaultFees {
				require.Equal(t, "5000uakt", tx.AuthInfo.Fee.Amount.String())
			} else {
				require.Equal(t, "77uakt", tx.AuthInfo.Fee.Amount.String())
			}
			require.Empty(t, tx.Signatures)
			if tc.args[0] == "sink" {
				wantSigner := tc.wantSigner
				if wantSigner == "" {
					wantSigner = address
				}
				require.Equal(t, wantSigner, tx.Body.Messages[0]["guardian"])
				require.Equal(t, true, tx.Body.Messages[0]["deposits"])
				require.Equal(t, true, tx.Body.Messages[0]["refunds"])
			}
		})
	}
}
