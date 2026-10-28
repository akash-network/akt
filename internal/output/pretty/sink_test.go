package pretty

import (
	"bytes"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/ansi"
	abci "github.com/cometbft/cometbft/abci/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func sinkTestTransfer(t *testing.T) (types.Transfer, []byte) {
	t.Helper()

	transfer := types.Transfer{
		ID:     12,
		Amount: 1_500_000,
		Height: 42,
	}
	copy(transfer.Sender[:], bytes.Repeat([]byte{7}, 20))
	copy(transfer.Recipient[:], bytes.Repeat([]byte{9}, 32))

	record, err := transfer.Marshal()
	require.NoError(t, err)

	return transfer, record
}

func sinkTestSignature(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, 64)
}

func TestRenderSinkTransferShowsFullIdentitiesAndSettlement(t *testing.T) {
	transfer, record := sinkTestTransfer(t)
	reporter := sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String()
	other := sdk.AccAddress(bytes.Repeat([]byte{4}, 20)).String()

	out, err := RenderSinkTransfer(types.TransferState{
		Record:      record,
		VoidOrdered: true,
		Settlement: types.Settlement{
			Status:       types.Status_STATUS_VOIDED,
			Hold:         types.Hold_HOLD_VETO,
			Round:        2,
			Policy:       types.ReporterPolicy{Epoch: 5, Members: []string{reporter, other}, Threshold: 2},
			ChallengeEnd: 1_792_000_000,
			Votes: []types.Vote{
				{Reporter: reporter, Observation: types.Observation{Outcome: types.Outcome_OUTCOME_VOIDED, Slot: 400, TransactionSignature: sinkTestSignature(1)}},
				{Reporter: other, Observation: types.Observation{Outcome: types.Outcome_OUTCOME_MINTED, Slot: 401, TransactionSignature: sinkTestSignature(2)}},
			},
			VetoBy:     other,
			VetoReason: "mint found at slot 401",
		},
	})
	require.NoError(t, err)
	plain := ansi.Strip(out)

	for _, want := range []string{
		"ID:", "12",
		sdk.AccAddress(transfer.Sender[:]).String(),
		base58.Encode(transfer.Recipient[:]),
		"1.5 AKT",
		"Void Ordered:", "Yes",
		"Status:", "Voided",
		"Hold:", "Veto",
		"Round:", "2",
		"Reporter Epoch:", "5",
		"Threshold:", "2 of 2",
		"Votes:", "1 minted, 1 voided",
		"Challenge Ends:", "2026-10-14T17:46:40Z",
		"Vetoed By:", other,
		"Veto Reason:", "mint found at slot 401",
		reporter,
		base58.Encode(sinkTestSignature(1)),
		base58.Encode(sinkTestSignature(2)),
	} {
		require.Contains(t, plain, want)
	}
}

func TestRenderSinkTransferRejectsUndecodableRecord(t *testing.T) {
	_, err := RenderSinkTransfer(types.TransferState{Record: []byte{1, 2, 3}})
	require.ErrorContains(t, err, "malformed node response")
}

func TestSinkRecordFormatterShowsPublicStatus(t *testing.T) {
	_, record := sinkTestTransfer(t)
	cases := map[types.QuerySinkResponse_SinkStatus]string{
		types.QuerySinkResponse_transfer_pending: "Transfer pending",
		types.QuerySinkResponse_completed:        "Completed",
		types.QuerySinkResponse_void:             "Void",
		types.QuerySinkResponse_invalid:          "Invalid",
	}

	for status, want := range cases {
		t.Run(status.String(), func(t *testing.T) {
			var buf bytes.Buffer
			res := &types.QuerySinkResponse{Status: status, Transfer: types.TransferState{Record: record}}
			require.NoError(t, formatSinkRecord(&buf, nil, sdkclient.Context{}, res))

			plain := ansi.Strip(buf.String())
			require.Contains(t, plain, "Sink Transfer")
			require.Contains(t, plain, "Status:")
			require.Contains(t, plain, want)
		})
	}
}

func TestRenderSinkOpenTransfersPagesAndEmpty(t *testing.T) {
	_, record := sinkTestTransfer(t)

	out, err := RenderSinkOpenTransfers(&types.QueryOpenTransfersResponse{
		Transfers:      []types.TransferState{{Record: record, Settlement: types.Settlement{Status: types.Status_STATUS_PENDING}}},
		NextStartAfter: 12,
	})
	require.NoError(t, err)
	plain := ansi.Strip(out)
	require.Contains(t, plain, "CHALLENGE ENDS")
	require.Contains(t, plain, "Pending")
	require.Contains(t, plain, "akt q sink open-transfers --start-after 12")

	empty, err := RenderSinkOpenTransfers(&types.QueryOpenTransfersResponse{})
	require.NoError(t, err)
	require.Equal(t, "(no open transfers)", strings.TrimSpace(ansi.Strip(empty)))
}

func TestRenderSinkParamsAlignsValuesAndListsEveryEntry(t *testing.T) {
	first := sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String()
	second := sdk.AccAddress(bytes.Repeat([]byte{4}, 20)).String()
	vault := bytes.Repeat([]byte{8}, 32)

	out := RenderSinkParams(types.Params{
		Enabled:                true,
		MaxTransfer:            1_000_000_000,
		SolanaDailyMintCap:     10_000_000_000,
		MaxDailyPerAccount:     20,
		MaxPendingPerAccount:   3,
		Reporters:              []string{first, second},
		ReporterThreshold:      2,
		ChallengePeriodSeconds: 259_200,
		DailyRefundCap:         1_000_000_000,
		VaultAllowlist:         [][]byte{vault},
	})
	lines := plainLines(out)

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
	for _, want := range []string{
		"Deposits:", "Enabled",
		"Refunds:", "Disabled",
		"Min Transfer:", "None",
		"1000 AKT",
		"Max Amount Per Account Per Day:", "No limit",
		first, second,
		"Reporter Threshold:", "2 of 2",
		"Challenge Period:", "3 days",
		"Pause Guardian:", "None",
		base58.Encode(vault),
	} {
		require.Contains(t, plain, want)
	}
}

func TestRenderSinkStateShowsLockedTotal(t *testing.T) {
	out, err := RenderSinkState(&types.QueryStateResponse{Totals: types.Totals{
		LastTransferId: 12,
		PendingAmount:  2_000_000,
		PendingCount:   1,
		MintedAmount:   79_000_000,
		VoidedAmount:   3_000_000,
		RefundedAmount: 10_000_000,
		RefundDay:      20_745,
		RefundedToday:  7_000_000,
		ReporterEpoch:  1,
	}})
	require.NoError(t, err)
	plain := ansi.Strip(out)

	require.Contains(t, plain, "Pending:             2 AKT")
	require.Contains(t, plain, "Open Transfers:      1")
	require.Contains(t, plain, "7 AKT on 2026-10-19")
	require.Contains(t, plain, "84 AKT")
	require.Contains(t, plain, "Sink Parameters")
}

func TestRenderSinkAccount(t *testing.T) {
	sender := bytes.Repeat([]byte{7}, 20)

	plain := ansi.Strip(RenderSinkAccount(&types.QueryAccountResponse{Usage: types.AccountUsage{
		Sender:       sender,
		Day:          20_745,
		DailyCount:   2,
		PendingCount: 1,
		DailyAmount:  3_000_000,
	}}))

	require.Contains(t, plain, sdk.AccAddress(sender).String())
	require.Contains(t, plain, "2026-10-19")
	require.Contains(t, plain, "3 AKT")
	require.Contains(t, plain, "Open Transfers:      1")
}

func TestRenderSinkDepositNamesTheFollowUp(t *testing.T) {
	msg := &types.MsgSink{
		DestType: types.DestTypeSolana,
		Dest:     base58.Encode(bytes.Repeat([]byte{9}, 32)),
		Amount:   sdk.NewCoin(types.Denom, math.NewInt(2_500_000)),
		Sender:   sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String(),
	}

	withID := ansi.Strip(RenderSinkDeposit(msg, "12"))
	require.Contains(t, withID, "Record ID:")
	require.Contains(t, withID, "2.5 AKT")
	require.Contains(t, withID, msg.Dest)
	require.Contains(t, withID, "akt q sink record 12")

	pending := ansi.Strip(RenderSinkDeposit(msg, ""))
	require.NotContains(t, pending, "Record ID:")
	require.Contains(t, pending, "akt q sink record <id>")
}

func TestSinkTxFormattersCoverEveryMessage(t *testing.T) {
	RegisterAllTxFormatters()

	signer := sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String()
	evidence := types.Observation{Outcome: types.Outcome_OUTCOME_VOIDED, Slot: 400, TransactionSignature: sinkTestSignature(1)}

	cases := []struct {
		msg   sdk.Msg
		title string
		want  []string
	}{
		{
			msg:   &types.MsgSink{DestType: types.DestTypeSolana, Dest: base58.Encode(bytes.Repeat([]byte{9}, 32)), Amount: sdk.NewCoin(types.Denom, math.NewInt(1_000_000)), Sender: signer},
			title: "Sink Deposit",
			want:  []string{"Record ID:", "12", "akt q sink record 12"},
		},
		{
			msg:   &types.MsgGuardianPause{Guardian: signer, Deposits: true},
			title: "Guardian Pause",
			want:  []string{"Deposits:", "Disabled", "Refunds:", "Unchanged"},
		},
		{
			msg:   &types.MsgReportOutcome{Reporter: signer, Id: 12, ReporterEpoch: 3, Round: 1, Observation: evidence},
			title: "Report Outcome",
			want:  []string{"Outcome:", "Voided", "Slot:", "400", base58.Encode(sinkTestSignature(1)), "Reporter Epoch:", "3"},
		},
		{
			msg:   &types.MsgVetoVoid{Signer: signer, Id: 12, Reason: "mint found"},
			title: "Veto Void",
			want:  []string{"Reason:", "mint found"},
		},
		{
			msg:   &types.MsgFinalizeRefund{Executor: signer, Id: 12},
			title: "Finalize Refund",
			want:  []string{"Executor:", signer, "Transfer ID:", "12"},
		},
		{
			msg:   &types.MsgRequestVoid{Authority: signer, Id: 12},
			title: "Request Void",
			want:  []string{"Authority:", signer},
		},
		{
			msg:   &types.MsgResolveTransfer{Authority: signer, Id: 12, Decision: types.Decision_DECISION_CONFIRM_VOID, Observation: &evidence, Reason: "token account recipient"},
			title: "Resolve Transfer",
			want:  []string{"Decision:", "Confirm void", "Evidence:", "Voided at slot 400", "token account recipient"},
		},
		{
			msg:   &types.MsgUpdateParams{Authority: signer, Params: types.Params{ChallengePeriodSeconds: 259_200}},
			title: "Update Sink Parameters",
			want:  []string{"Authority:", signer, "Sink Parameters", "3 days"},
		},
	}

	receipt := &sdk.TxResponse{Events: []abci.Event{{
		Type:       "sink_deposit",
		Attributes: []abci.EventAttribute{{Key: "transfer_id", Value: "12"}, {Key: "msg_index", Value: "0"}},
	}}}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			f, ok := LookupTx(tc.msg)
			require.Truef(t, ok, "no tx formatter registered for %T", tc.msg)
			require.Equal(t, tc.title, f.Title())

			var buf bytes.Buffer
			require.NoError(t, f.FormatTx(&buf, nil, sdkclient.Context{}, tc.msg, receipt, 0))

			plain := ansi.Strip(buf.String())
			for _, want := range tc.want {
				require.Contains(t, plain, want)
			}
		})
	}
}

func TestSinkLabelsCoverEveryValue(t *testing.T) {
	for status, want := range map[types.QuerySinkResponse_SinkStatus]string{
		types.QuerySinkResponse_validation_pending: "Validation pending",
		types.QuerySinkResponse_validated:          "Validated",
	} {
		require.Equal(t, want, ansi.Strip(sinkPublicStatusLabel(status)))
	}
	for status, want := range map[types.Status]string{
		types.Status_STATUS_PENDING:  "Pending",
		types.Status_STATUS_MINTED:   "Minted",
		types.Status_STATUS_VOIDED:   "Voided",
		types.Status_STATUS_REFUNDED: "Refunded",
		types.Status(99):             "99",
	} {
		require.Equal(t, want, ansi.Strip(sinkStatusLabel(status)))
	}
	for hold, want := range map[types.Hold]string{
		types.Hold_HOLD_CONFLICT: "Conflict",
		types.Hold(99):           "99",
	} {
		require.Equal(t, want, ansi.Strip(sinkHoldLabel(hold)))
	}
	require.Equal(t, "Unspecified", ansi.Strip(sinkOutcomeLabel(types.Outcome_OUTCOME_UNSPECIFIED)))
	for decision, want := range map[types.Decision]string{
		types.Decision_DECISION_RESTART:      "Restart",
		types.Decision_DECISION_CONFIRM_MINT: "Confirm mint",
		types.Decision_DECISION_CONFIRM_VOID: "Confirm void",
		types.Decision(99):                   "Unspecified",
	} {
		require.Equal(t, want, ansi.Strip(sinkDecisionLabel(decision)))
	}
	require.Equal(t, "-", sinkSignature(nil))
	require.Equal(t, "-", sinkDay(0))
	require.Equal(t, "-", sinkDay(stdmathMaxUint64))
}

const stdmathMaxUint64 = ^uint64(0)

func TestRenderSinkTransferShowsGovernanceEvidence(t *testing.T) {
	_, record := sinkTestTransfer(t)
	evidence := types.Observation{Outcome: types.Outcome_OUTCOME_MINTED, Slot: 900, TransactionSignature: sinkTestSignature(3)}

	out, err := RenderSinkTransfer(types.TransferState{Record: record, Settlement: types.Settlement{GovernanceObservation: &evidence}})
	require.NoError(t, err)
	require.Contains(t, ansi.Strip(out), "Governance Evidence: Minted at slot 900, signature "+base58.Encode(sinkTestSignature(3)))
}

func TestSinkFormattersRejectMalformedRecords(t *testing.T) {
	bad := types.TransferState{Record: []byte{1}}
	var buf bytes.Buffer

	require.ErrorContains(t, formatSinkRecord(&buf, nil, sdkclient.Context{}, &types.QuerySinkResponse{Transfer: bad}), "malformed node response")
	require.ErrorContains(t, formatSinkTransfer(&buf, nil, sdkclient.Context{}, &types.QueryTransferResponse{Transfer: bad}), "malformed node response")
	require.ErrorContains(t, formatSinkOpenTransfers(&buf, nil, sdkclient.Context{}, &types.QueryOpenTransfersResponse{Transfers: []types.TransferState{bad}}), "malformed node response")
	require.Empty(t, buf.String())

	_, record := sinkTestTransfer(t)
	require.NoError(t, formatSinkTransfer(&buf, nil, sdkclient.Context{}, &types.QueryTransferResponse{Transfer: types.TransferState{Record: record}}))
	require.Contains(t, ansi.Strip(buf.String()), "Settlement")
}

func TestRenderSinkParamsOptionalValues(t *testing.T) {
	guardian := sdk.AccAddress(bytes.Repeat([]byte{6}, 20)).String()

	plain := ansi.Strip(RenderSinkParams(types.Params{
		MinTransfer:              1_000_000,
		MaxDailyAmountPerAccount: 50_000_000,
		ChallengePeriodSeconds:   stdmathMaxUint64,
		PauseGuardian:            guardian,
	}))

	require.Contains(t, plain, "Min Transfer:                    1 AKT")
	require.Contains(t, plain, "Max Amount Per Account Per Day:  50 AKT")
	require.Contains(t, plain, "18446744073709551615s")
	require.Contains(t, plain, "Pause Guardian:                  "+guardian)
}

func TestRenderSinkStateRejectsOverflowingTotals(t *testing.T) {
	overflow := &types.QueryStateResponse{Totals: types.Totals{PendingAmount: stdmathMaxUint64, MintedAmount: 1}}

	_, err := RenderSinkState(overflow)
	require.ErrorContains(t, err, "malformed node response")

	var buf bytes.Buffer
	require.Error(t, formatSinkState(&buf, nil, sdkclient.Context{}, overflow))
	require.Empty(t, buf.String())

	require.NoError(t, formatSinkState(&buf, nil, sdkclient.Context{}, &types.QueryStateResponse{}))
	require.Contains(t, ansi.Strip(buf.String()), "Refunded Today:      -")
}

// Uncapped testnets set the caps to the uint64 maximum, which is above the
// int64 range FormatCoin once assumed.
func TestRenderSinkParamsWithUint64Caps(t *testing.T) {
	plain := ansi.Strip(RenderSinkParams(types.Params{
		MaxTransfer:        stdmathMaxUint64,
		SolanaDailyMintCap: stdmathMaxUint64,
		DailyRefundCap:     stdmathMaxUint64,
	}))
	require.Contains(t, plain, "Max Transfer:                    18446744073709.551615 AKT")
}
