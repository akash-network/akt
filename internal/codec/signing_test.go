package codec_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	signingv1beta1 "cosmossdk.io/api/cosmos/tx/signing/v1beta1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/codec"
	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSinkAminoSigningResolvesNestedFieldsWithKubernetesDescriptors(t *testing.T) {
	encoding := codec.MakeEncodingConfig()
	owner := sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String()
	msg := &types.MsgReportOutcome{
		Reporter: owner,
		ID: types.ID{
			Owner:    owner,
			Height:   29188334,
			Sequence: 12,
		},
		PolicyVersion: 4,
		Round:         2,
		Observation: types.Observation{
			Outcome:              types.OutcomeMinted,
			Slot:                 400,
			TransactionSignature: bytes.Repeat([]byte{1}, 64),
		},
	}
	builder := encoding.TxConfig.NewTxBuilder()
	require.NoError(t, builder.SetMsgs(msg))
	signBytes, err := authsigning.GetSignBytesAdapter(context.Background(), encoding.TxConfig.SignModeHandler(),
		signingtypes.SignMode_SIGN_MODE_LEGACY_AMINO_JSON,
		authsigning.SignerData{Address: owner}, builder.GetTx())
	require.NoError(t, err)
	var doc struct {
		Messages []struct {
			Type  string `json:"type"`
			Value struct {
				ID            map[string]string      `json:"id"`
				PolicyVersion string                 `json:"policy_version"`
				Round         string                 `json:"round"`
				Observation   map[string]interface{} `json:"observation"`
			} `json:"value"`
		} `json:"msgs"`
	}
	require.NoError(t, json.Unmarshal(signBytes, &doc))
	require.Len(t, doc.Messages, 1)
	require.Equal(t, map[string]string{"owner": owner, "height": "29188334", "sequence": "12"}, doc.Messages[0].Value.ID)
	require.Equal(t, "4", doc.Messages[0].Value.PolicyVersion)
	require.Equal(t, "2", doc.Messages[0].Value.Round)
	require.Equal(t, "400", doc.Messages[0].Value.Observation["slot"])
	require.NotEmpty(t, doc.Messages[0].Value.Observation["transaction_signature"])

	// Both direct modes remain registered alongside Amino.
	modes := encoding.TxConfig.SignModeHandler().SupportedModes()
	require.ElementsMatch(t, []signingv1beta1.SignMode{
		signingv1beta1.SignMode_SIGN_MODE_DIRECT,
		signingv1beta1.SignMode_SIGN_MODE_DIRECT_AUX,
		signingv1beta1.SignMode_SIGN_MODE_LEGACY_AMINO_JSON,
	}, modes)
}
