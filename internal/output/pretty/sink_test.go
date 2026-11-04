package pretty

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/ansi"
	abci "github.com/cometbft/cometbft/abci/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/gogoproto/jsonpb"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func sinkTestTransfer() types.TransferState {
	owner := sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String()
	return types.TransferState{
		ID: types.ID{
			Owner:    owner,
			Height:   42,
			Sequence: 12,
		},
		Data: types.RecordData{
			Dest:   base58.Encode(bytes.Repeat([]byte{9}, 32)),
			Amount: sdk.NewInt64Coin("uakt", 1_500_000),
		},
		Status: types.StatusRefundPending,
		Settlement: types.SettlementData{
			Round: 2,
			Policy: types.ReporterPolicy{
				Version:   5,
				Members:   []string{owner},
				Threshold: math.LegacyMustNewDecFromStr("0.67"),
			},
			ChallengeEnd: 1_792_000_000,
		},
	}
}

func sinkTestSignature(fill byte) []byte { return bytes.Repeat([]byte{fill}, 64) }

func TestRenderSinkTransferShowsFullIdentitiesAndSettlement(t *testing.T) {
	state := sinkTestTransfer()
	state.Settlement.Hold = types.HoldVeto
	state.Settlement.VoidAuthorization = &types.VoidAuthorization{
		RequestedHeight: 44,
		RequestedRound:  2,
	}
	state.Settlement.VetoBy = state.ID.Owner
	state.Settlement.VetoReason = "mint found"
	state.Settlement.VetoPolicyVersion = 5
	state.Settlement.GovernanceObservation = &types.Observation{
		Outcome:              types.OutcomeMinted,
		Slot:                 500,
		TransactionSignature: sinkTestSignature(3),
	}
	state.Settlement.Votes = []types.Vote{
		{
			Reporter:      state.ID.Owner,
			PolicyVersion: 5,
			Observation: types.Observation{
				Outcome:              types.OutcomeVoided,
				Slot:                 400,
				TransactionSignature: sinkTestSignature(1),
			},
		},
		{
			Reporter:      state.ID.Owner,
			PolicyVersion: 4,
			Observation: types.Observation{
				Outcome:              types.OutcomeMinted,
				Slot:                 401,
				TransactionSignature: sinkTestSignature(2),
			},
		},
	}
	out, err := RenderSinkTransfer(state)
	require.NoError(t, err)
	plain := ansi.Strip(out)
	for _, want := range []string{
		state.ID.String(), state.ID.Owner, state.Data.Dest, "1.5 AKT",
		"Refund pending", "Veto", "Policy Version:", "0.67 (1 of 1)",
		"0 minted, 1 voided", "2026-10-14T17:46:40Z", "mint found",
		"Void Ordered Height:", "Void Ordered Round:", "Veto Policy Version:",
		base58.Encode(sinkTestSignature(1)), base58.Encode(sinkTestSignature(2)),
		"Governance Evidence:", "slot 500", base58.Encode(sinkTestSignature(3)),
	} {
		require.Contains(t, plain, want)
	}
	var buf bytes.Buffer
	require.NoError(t, formatSinkTransfer(&buf, nil, sdkclient.Context{}, &types.QueryTransferResponse{Transfer: state}))
	require.Equal(t, out, buf.String())
}

func TestRenderSinkTransfersPreservesTerminalHoldsAndCursor(t *testing.T) {
	state := sinkTestTransfer()
	state.Status = types.StatusMinted
	state.Settlement.Hold = types.HoldConflict
	cursor := []byte{0, 255, 5, 0}
	out, err := RenderSinkTransfers([]types.TransferState{state}, &sdkquery.PageResponse{NextKey: cursor})
	require.NoError(t, err)
	for _, want := range []string{state.ID.String(), state.Data.Dest, "Minted", "Conflict", base64.StdEncoding.EncodeToString(cursor), "--height", "--page-key"} {
		require.Contains(t, ansi.Strip(out), want)
	}
	empty, err := RenderSinkTransfers(nil, nil)
	require.NoError(t, err)
	require.Equal(t, "(no transfers)", strings.TrimSpace(ansi.Strip(empty)))

	var buf bytes.Buffer
	require.NoError(t, formatSinkDisputes(&buf, nil, sdkclient.Context{}, &types.QueryDisputesResponse{
		Transfers: []types.TransferState{state},
	}))
	require.Contains(t, ansi.Strip(buf.String()), "Conflict")
}

func TestRenderSinkParamsAlignsValuesAndListsEveryEntry(t *testing.T) {
	p := types.DefaultParams()
	first := sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String()
	second := sdk.AccAddress(bytes.Repeat([]byte{4}, 20)).String()
	p.Reporters = []string{first, second}
	p.ReportersThreshold = math.LegacyMustNewDecFromStr("0.67")
	p.AllowedDestinationVaults = []string{base58.Encode(bytes.Repeat([]byte{8}, 32))}
	p.Enabled = true
	p.MaxEpochAmountPerAccount = sdk.NewInt64Coin("uakt", 0)
	lines := plainLines(RenderSinkParams(p))
	valueColumn := -1
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var column int
		if key, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) != "" {
			column = strings.Index(line, ": ") + 2
			column += len(line[column:]) - len(strings.TrimLeft(line[column:], " "))
		} else {
			column = len(line) - len(strings.TrimLeft(line, " "))
		}
		if valueColumn < 0 {
			valueColumn = column
		}
		require.Equalf(t, valueColumn, column, "value out of column in %q", line)
	}
	plain := strings.Join(lines, "\n")
	for _, want := range []string{"Enabled", "Disabled", "1 AKT", "Account Cap Per Epoch:", "No limit", first, second, "0.67 (2 of 2)", "3 days", p.AllowedDestinationVaults[0]} {
		require.Contains(t, plain, want)
	}
	for _, removed := range []string{"Per Day", "Open Transfers", "Solana Daily"} {
		require.NotContains(t, plain, removed)
	}
	p.MaxAmountPerTransfer = sdk.NewCoin("uakt", math.NewIntFromUint64(^uint64(0)))
	require.Contains(t, ansi.Strip(RenderSinkParams(p)), "18446744073709.551615 AKT")
	p.PauseGuardian = first
	require.Contains(t, strings.Join(strings.Fields(ansi.Strip(RenderSinkParams(p))), " "), "Pause Guardian: "+first)
}

func TestRenderSinkStateShowsSeparatePopulationsAndLockedTotal(t *testing.T) {
	id := sinkTestTransfer().ID
	res := &types.QueryStateResponse{
		Totals: types.Totals{
			PendingAmount:       sdk.NewInt64Coin("uakt", 2_000_000),
			PendingCount:        1,
			MintedAmount:        sdk.NewInt64Coin("uakt", 79_000_000),
			MintedCount:         2,
			VoidedAmount:        sdk.NewInt64Coin("uakt", 3_000_000),
			VoidedCount:         3,
			RefundPendingAmount: sdk.NewInt64Coin("uakt", 4_000_000),
			RefundPendingCount:  4,
			RefundedAmount:      sdk.NewInt64Coin("uakt", 10_000_000),
			RefundedCount:       5,
		},
		Epoch: 123,
		SendUsage: types.EpochUsage{
			Epoch:  123,
			Amount: sdk.NewInt64Coin("uakt", 5_000_000),
		},
		RefundUsage: types.EpochUsage{
			Epoch:  123,
			Amount: sdk.NewInt64Coin("uakt", 7_000_000),
		},
		RefundHead: &id,
	}
	out, err := RenderSinkState(res)
	require.NoError(t, err)
	plain := ansi.Strip(out)
	for _, want := range []string{"2 AKT in 1 transfers", "79 AKT in 2 transfers", "3 AKT in 3 transfers", "4 AKT in 4 transfers", "10 AKT in 5 transfers", "88 AKT", "123", "Sent This Epoch:", "Refunded This Epoch:", id.String(), "Payment still requires"} {
		require.Contains(t, plain, want)
	}
	require.NotContains(t, plain, "Sink Parameters")
	require.NotContains(t, plain, "Today")
	res.Totals.PendingAmount = sdk.NewInt64Coin("uact", 1)
	_, err = RenderSinkState(res)
	require.ErrorContains(t, err, "malformed node response")
}

func TestRenderSinkAccountAndReporters(t *testing.T) {
	plain := ansi.Strip(RenderSinkAccount(&types.QueryAccountResponse{
		Epoch: 20745,
		Usage: types.AccountUsage{
			TotalSent: sdk.NewInt64Coin("uakt", 9_000_000),
			EpochSent: sdk.NewInt64Coin("uakt", 3_000_000),
			LastEpoch: 20745,
		},
	}))
	for _, want := range []string{"20745", "9 AKT", "3 AKT", "Sent This Epoch:"} {
		require.Contains(t, plain, want)
	}
	require.NotContains(t, plain, "2026-")
	policy := sinkTestTransfer().Settlement.Policy
	prior := policy
	prior.Version = policy.Version - 1
	plain = ansi.Strip(RenderSinkReporters(&types.QueryReportersResponse{
		Reporters: []types.ReporterPolicy{prior, policy},
	}))
	require.Contains(t, plain, policy.Members[0])
	require.Contains(t, plain, "0.67 (1 of 1)")
	require.Equal(t, 2, strings.Count(plain, "Reporter Policy"))
	require.Contains(t, RenderSinkReporters(&types.QueryReportersResponse{}), "no reporter policy")
}

func TestSinkTxFormattersCoverEveryMessage(t *testing.T) {
	RegisterAllTxFormatters()
	state := sinkTestTransfer()
	id := state.ID
	signer := id.Owner
	evidence := types.SolanaEvidence{
		Slot:                 400,
		TransactionSignature: sinkTestSignature(1),
	}
	observation := types.Observation{
		Outcome:              types.OutcomeVoided,
		Slot:                 400,
		TransactionSignature: sinkTestSignature(1),
	}
	cases := []struct {
		msg   sdk.Msg
		title string
		want  []string
	}{
		{
			msg: &types.MsgSink{
				DestType: types.DestTypeSolana,
				Dest:     state.Data.Dest,
				Amount:   state.Data.Amount,
				Sender:   signer,
			},
			title: "Sink Deposit",
			want:  []string{id.String(), "akt q sink transfer " + id.String()},
		},
		{
			msg: &types.MsgGuardianPause{
				Guardian: signer,
				Deposits: true,
			},
			title: "Guardian Pause",
			want:  []string{"Disabled", "Unchanged"},
		},
		{
			msg: &types.MsgReportOutcome{
				Reporter:      signer,
				ID:            id,
				PolicyVersion: 3,
				Round:         1,
				Observation:   observation,
			},
			title: "Report Outcome",
			want:  []string{"Policy Version:", "Voided", "400", base58.Encode(sinkTestSignature(1))},
		},
		{
			msg: &types.MsgVetoVoid{
				Signer: signer,
				ID:     id,
				Reason: "mint found",
			},
			title: "Veto Void",
			want:  []string{"mint found", id.String()},
		},
		{
			msg: &types.MsgFinalizeRefund{
				Executor: signer,
				ID:       id,
			},
			title: "Finalize Refund",
			want:  []string{signer, id.String()},
		},
		{
			msg: &types.MsgRequestVoid{
				Authority:     signer,
				ID:            id,
				ExpectedRound: 2,
			},
			title: "Request Void",
			want:  []string{"Expected Round:", id.String()},
		},
		{
			msg: &types.MsgRestartTransfer{
				Authority:     signer,
				ID:            id,
				ExpectedRound: 2,
				Reason:        "restart",
			},
			title: "Restart Transfer",
			want:  []string{"Expected Round:", "restart"},
		},
		{
			msg: &types.MsgConfirmMint{
				Authority:     signer,
				ID:            id,
				ExpectedRound: 2,
				Evidence:      evidence,
				Reason:        "mint verified",
			},
			title: "Confirm Mint",
			want:  []string{"Expected Round:", "Minted at slot 400", "mint verified"},
		},
		{
			msg: &types.MsgConfirmVoid{
				Authority:     signer,
				ID:            id,
				ExpectedRound: 2,
				Evidence:      evidence,
				Reason:        "void verified",
			},
			title: "Confirm Void",
			want:  []string{"Expected Round:", "Voided at slot 400", "void verified"},
		},
		{
			msg: &types.MsgUpdateParams{
				Authority: signer,
				Params:    types.DefaultParams(),
			},
			title: "Update Sink Parameters",
			want:  []string{"Sink Parameters", "3 days"},
		},
	}
	event, err := sdk.TypedEventToEvent(&types.EventTransferCreated{
		ID:        id,
		Sender:    signer,
		Recipient: state.Data.Dest,
		Amount:    state.Data.Amount,
		Height:    id.Height,
	})
	require.NoError(t, err)
	event.Attributes = append(event.Attributes, abci.EventAttribute{
		Key:   "msg_index",
		Value: "0",
	})
	receipt := &sdk.TxResponse{
		Events: []abci.Event{abci.Event(event)},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			f, ok := LookupTx(tc.msg)
			require.True(t, ok)
			require.Equal(t, tc.title, f.Title())
			var buf bytes.Buffer
			require.NoError(t, f.FormatTx(&buf, nil, sdkclient.Context{}, tc.msg, receipt, 0))
			for _, want := range tc.want {
				require.Contains(t, ansi.Strip(buf.String()), want)
			}
		})
	}
}

func TestSinkDepositReceiptPreservesCompositeIDAndMessageScope(t *testing.T) {
	state := sinkTestTransfer()
	msg := &types.MsgSink{
		Sender: state.ID.Owner,
		Dest:   state.Data.Dest,
		Amount: state.Data.Amount,
	}
	firstID := state.ID
	secondID := firstID
	secondID.Sequence++
	marshaler := jsonpb.Marshaler{}
	first, err := marshaler.MarshalToString(&firstID)
	require.NoError(t, err)
	second, err := marshaler.MarshalToString(&secondID)
	require.NoError(t, err)
	res := &sdk.TxResponse{
		Events: []abci.Event{
			{
				Type: "akash.sink.v1.EventTransferCreated",
				Attributes: []abci.EventAttribute{{
					Key:   "msg_index",
					Value: "0",
				}, {
					Key:   "id",
					Value: first,
				}},
			},
			{
				Type: "akash.sink.v1.EventTransferCreated",
				Attributes: []abci.EventAttribute{{
					Key:   "msg_index",
					Value: "1",
				}, {
					Key:   "id",
					Value: second,
				}},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, fmtSinkDeposit(&buf, nil, sdkclient.Context{}, msg, res, 1))
	require.Contains(t, buf.String(), secondID.String())
	require.NotContains(t, buf.String(), firstID.String())
	buf.Reset()
	require.NoError(t, fmtSinkDeposit(&buf, nil, sdkclient.Context{}, msg, nil, 0))
	require.Contains(t, buf.String(), "<owner/height/sequence>")
	require.NotContains(t, buf.String(), "Transfer ID:")
	res.Events[0].Attributes[1].Value = `{"height":"0"}`
	require.Error(t, fmtSinkDeposit(&buf, nil, sdkclient.Context{}, msg, res, 0))
	buf.Reset()
	res.Events[0].Attributes[1].Value = `{broken`
	require.ErrorContains(t, fmtSinkDeposit(&buf, nil, sdkclient.Context{}, msg, res, 0), "decode sink transfer ID from receipt")
	require.Empty(t, buf.String())
}

func TestSinkBroadcastReceiptWithoutBody(t *testing.T) {
	RegisterAllTxFormatters()
	state := sinkTestTransfer()
	msg := &types.MsgSink{
		Sender: state.ID.Owner,
		Dest:   state.Data.Dest,
		Amount: state.Data.Amount,
	}
	event, err := sdk.TypedEventToEvent(&types.EventTransferCreated{
		ID:        state.ID,
		Sender:    msg.Sender,
		Recipient: msg.Dest,
		Amount:    msg.Amount,
		Height:    state.ID.Height,
	})
	require.NoError(t, err)
	for _, status := range []string{TxStatusConfirmed, TxStatusPending, TxStatusFailed} {
		t.Run(status, func(t *testing.T) {
			resp := &sdk.TxResponse{TxHash: strings.Repeat("A", 64)}
			switch status {
			case TxStatusConfirmed:
				resp.Height = state.ID.Height
				resp.Events = []abci.Event{abci.Event(event)}
			case TxStatusFailed:
				resp.Code = 5
			}
			cmd := &cobra.Command{}
			cmd.Flags().String("output", "pretty", "")
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, resp, msg))
			plain := ansi.Strip(buf.String())
			if status == TxStatusConfirmed {
				require.Contains(t, plain, "Sink Deposit")
				require.Contains(t, plain, "1.5 AKT")
				require.Contains(t, plain, "akt q sink transfer "+state.ID.String())
				require.Contains(t, plain, "unknown (transaction body not returned)")
			} else {
				require.NotContains(t, plain, "AKT is locked")
				require.NotContains(t, plain, "Transfer ID:")
			}
			require.Nil(t, resp.Tx)

			buf.Reset()
			require.NoError(t, cmd.Flags().Set("output", "json"))
			require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, resp, msg))
			require.NotContains(t, buf.String(), `"body"`)
			require.NotContains(t, buf.String(), `"fee"`)
		})
	}
}

func TestSinkFormattersRejectMalformedTransferData(t *testing.T) {
	for _, mutate := range []func(*types.TransferState){
		func(s *types.TransferState) { s.ID.Height = 0 },
		func(s *types.TransferState) { s.Data.Dest = "bad" },
		func(s *types.TransferState) { s.Status = types.StatusInvalid },
	} {
		state := sinkTestTransfer()
		mutate(&state)
		var buf bytes.Buffer
		require.ErrorContains(t, formatSinkTransfer(&buf, nil, sdkclient.Context{}, &types.QueryTransferResponse{Transfer: state}), "malformed node response")
		require.ErrorContains(t, formatSinkTransfers(&buf, nil, sdkclient.Context{}, &types.QueryTransfersResponse{
			Transfers: []types.TransferState{state},
		}), "malformed node response")
		require.ErrorContains(t, formatSinkDisputes(&buf, nil, sdkclient.Context{}, &types.QueryDisputesResponse{
			Transfers: []types.TransferState{state},
		}), "malformed node response")
		require.Empty(t, buf.String())
	}
	var buf bytes.Buffer
	require.ErrorContains(t, formatSinkState(&buf, nil, sdkclient.Context{}, &types.QueryStateResponse{
		Totals: types.Totals{
			PendingAmount: sdk.NewInt64Coin("uact", 1),
		},
	}), "malformed node response")
	require.Empty(t, buf.String())
}

func TestSinkLabelsCoverFinancialStates(t *testing.T) {
	for status, want := range map[types.Status]string{
		types.StatusTransferPending: "Transfer pending",
		types.StatusMinted:          "Minted",
		types.StatusVoided:          "Voided",
		types.StatusRefundPending:   "Refund pending",
		types.StatusRefunded:        "Refunded",
		types.StatusInvalid:         "STATUS_INVALID",
	} {
		require.Equal(t, want, ansi.Strip(sinkStatusLabel(status)))
	}
	require.Equal(t, "None", sinkHoldLabel(types.HoldNone))
	require.Equal(t, types.Hold(99).String(), ansi.Strip(sinkHoldLabel(types.Hold(99))))
	require.Equal(t, "Unspecified", ansi.Strip(sinkOutcomeLabel(types.OutcomeUnspecified)))
	require.Equal(t, "-", sinkTime(0))
	require.Equal(t, "-", sinkSignature(nil))
	require.Equal(t, "0.666666666666666667 (3 of 3)", sinkThreshold(math.LegacyMustNewDecFromStr("0.666666666666666667"), 3))
	p := types.DefaultParams()
	p.ReporterChallengePeriod = 72*time.Hour + time.Second
	require.Contains(t, RenderSinkParams(p), "Challenge Period:")
}
