package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/cosmos/gogoproto/proto"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	types "pkg.akt.dev/go/node/sink/v1"
)

// sinkTestQuery records every sink query request and answers from fixed
// state. pages serves OpenTransfers by start_after cursor.
type sinkTestQuery struct {
	types.QueryClient
	calls []string
	reqs  []interface{}
	epoch uint64
	round uint64
	pages map[uint64]*types.QueryOpenTransfersResponse
	// fail and empty make one method return an error or a nil response.
	fail  map[string]error
	empty map[string]bool
	// pageErr answers an OpenTransfers cursor that has no page in pages.
	pageErr error
}

// record logs a call and reports whether the method should answer with
// nil, either with an error or as a malformed empty reply.
func (q *sinkTestQuery) record(method string, request interface{}) (bool, error) {
	q.calls = append(q.calls, method)
	q.reqs = append(q.reqs, request)
	return q.fail[method] != nil || q.empty[method], q.fail[method]
}

func (q *sinkTestQuery) Sink(_ context.Context, request *types.QuerySink, _ ...grpc.CallOption) (*types.QuerySinkResponse, error) {
	if stop, err := q.record("sink", request); stop {
		return nil, err
	}
	return &types.QuerySinkResponse{Status: types.QuerySinkResponse_transfer_pending, Transfer: sinkTestState(nil, types.Hold_HOLD_NONE, 12)}, nil
}

func (q *sinkTestQuery) State(_ context.Context, request *types.QueryStateRequest, _ ...grpc.CallOption) (*types.QueryStateResponse, error) {
	if stop, err := q.record("state", request); stop {
		return nil, err
	}
	return &types.QueryStateResponse{Params: types.DefaultParams(), Totals: types.Totals{ReporterEpoch: q.epoch}}, nil
}

func (q *sinkTestQuery) Transfer(_ context.Context, request *types.QueryTransferRequest, _ ...grpc.CallOption) (*types.QueryTransferResponse, error) {
	if stop, err := q.record("transfer", request); stop {
		return nil, err
	}
	state := sinkTestState(nil, types.Hold_HOLD_NONE, request.Id)
	state.Settlement.Round = q.round
	return &types.QueryTransferResponse{Transfer: state}, nil
}

func (q *sinkTestQuery) Account(_ context.Context, request *types.QueryAccountRequest, _ ...grpc.CallOption) (*types.QueryAccountResponse, error) {
	if stop, err := q.record("account", request); stop {
		return nil, err
	}
	return &types.QueryAccountResponse{}, nil
}

func (q *sinkTestQuery) OpenTransfers(_ context.Context, request *types.QueryOpenTransfersRequest, _ ...grpc.CallOption) (*types.QueryOpenTransfersResponse, error) {
	if stop, err := q.record("open-transfers", request); stop {
		return nil, err
	}
	if page, ok := q.pages[request.StartAfter]; ok {
		return page, nil
	}
	if q.pageErr != nil {
		return nil, q.pageErr
	}
	return &types.QueryOpenTransfersResponse{}, nil
}

type sinkTestAggregate struct {
	clientv1beta3.QueryClient
	sink types.QueryClient
}

func (q *sinkTestAggregate) Sink() types.QueryClient { return q.sink }

// sinkTestState builds an open transfer with a valid 77-byte record.
func sinkTestState(t *testing.T, hold types.Hold, id uint64) types.TransferState {
	transfer := types.Transfer{ID: id, Amount: 1_000_000, Height: 7}
	copy(transfer.Sender[:], bytes.Repeat([]byte{5}, 20))
	copy(transfer.Recipient[:], bytes.Repeat([]byte{9}, 32))
	record, err := transfer.Marshal()
	if err != nil && t != nil {
		t.Fatal(err)
	}
	return types.TransferState{Record: record, Settlement: types.Settlement{Hold: hold}}
}

func sinkTestDestination() string {
	return base58.Encode(bytes.Repeat([]byte{9}, 32))
}

func sinkTestSignature() string {
	return base58.Encode(bytes.Repeat([]byte{1}, 64))
}

// runSinkTx runs a sink transaction command with a capturing broadcaster and
// a sink query client, which report-outcome reads before signing.
func runSinkTx(t *testing.T, cmd *cobra.Command, query *sinkTestQuery, capture *moduleTxCapture, args ...string) error {
	t.Helper()

	return runSinkTxContext(t, cmd, query, capture, nil, args...)
}

// runSinkTxContext is runSinkTx with a hook that adjusts the client context,
// for example to remove the signing account.
func runSinkTxContext(t *testing.T, cmd *cobra.Command, query *sinkTestQuery, capture *moduleTxCapture, adjust func(sdkclient.Context) sdkclient.Context, args ...string) error {
	t.Helper()

	setSinkTxContext(cmd, query, capture, adjust)

	return cmd.RunE(cmd, args)
}

func setSinkTxContext(cmd *cobra.Command, query *sinkTestQuery, capture *moduleTxCapture, adjust func(sdkclient.Context) sdkclient.Context) *moduleCommandClient {
	fixture := newModuleTxFixture()
	cctx := fixture.cctx.WithOutput(io.Discard)
	if adjust != nil {
		cctx = adjust(cctx)
	}
	clientContext := cctx
	ctx := context.WithValue(context.Background(), ClientContextKey, &clientContext)
	client := &moduleCommandClient{cctx: cctx, tx: capture, query: &sinkTestAggregate{sink: query}}
	ctx = context.WithValue(ctx, ContextTypeClient, client)
	signingContext := cctx.TxConfig.SigningContext()
	ctx = context.WithValue(ctx, ContextTypeAddressCodec, signingContext.AddressCodec())
	ctx = context.WithValue(ctx, ContextTypeValidatorCodec, signingContext.ValidatorAddressCodec())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	return client
}

// sinkPartialCmd copies a command with only the named flags, so tests can
// reach the error a missing flag returns.
func sinkPartialCmd(source *cobra.Command, keep ...string) *cobra.Command {
	cmd := &cobra.Command{Use: source.Use, RunE: source.RunE}
	for _, name := range keep {
		cmd.Flags().AddFlag(source.Flags().Lookup(name))
	}
	return cmd
}

func TestSinkParsers(t *testing.T) {
	t.Run("transfer ID", func(t *testing.T) {
		id, err := parseSinkTransferID("12")
		require.NoError(t, err)
		require.Equal(t, uint64(12), id)

		for _, bad := range []string{"0", "012", "+12", "-1", " 12", "1.0", "18446744073709551616", ""} {
			_, err := parseSinkTransferID(bad)
			require.Errorf(t, err, "accepted %q", bad)
		}
	})

	t.Run("amount", func(t *testing.T) {
		for in, want := range map[string]int64{
			"1500000uakt": 1_500_000,
			"1.5akt":      1_500_000,
			"2AKT":        2_000_000,
			"0.000001akt": 1,
		} {
			coin, err := parseSinkAmount(in)
			require.NoErrorf(t, err, "%q", in)
			require.Equal(t, sdk.NewCoin(types.Denom, math.NewInt(want)), coin, in)
		}

		for _, bad := range []string{"1.5uakt", "0.0000001akt", "0uakt", "5uact", "akt", "-1uakt", "18446744073709551616uakt"} {
			_, err := parseSinkAmount(bad)
			require.Errorf(t, err, "accepted %q", bad)
		}
	})

	t.Run("evidence", func(t *testing.T) {
		observation, err := parseSinkObservation(types.Outcome_OUTCOME_MINTED, "400", sinkTestSignature())
		require.NoError(t, err)
		require.Equal(t, uint64(400), observation.Slot)
		require.Len(t, observation.TransactionSignature, 64)

		zero := base58.Encode(make([]byte, 64))
		short := base58.Encode(bytes.Repeat([]byte{1}, 63))
		for _, args := range [][2]string{{"0", sinkTestSignature()}, {"x", sinkTestSignature()}, {"400", zero}, {"400", short}, {"400", "0OIl"}} {
			_, err := parseSinkObservation(types.Outcome_OUTCOME_MINTED, args[0], args[1])
			require.Errorf(t, err, "accepted %v", args)
		}
	})

	t.Run("words", func(t *testing.T) {
		_, err := parseSinkOutcome("refunded")
		require.Error(t, err)
		_, err = parseSinkDecision("approve")
		require.Error(t, err)
		_, _, err = parseSinkPauseTarget("everything")
		require.Error(t, err)
		_, err = parseSinkReason("   ")
		require.Error(t, err)
		_, err = parseSinkReason(string(bytes.Repeat([]byte{'a'}, 257)))
		require.Error(t, err)

		deposits, refunds, err := parseSinkPauseTarget("all")
		require.NoError(t, err)
		require.True(t, deposits && refunds)
	})
}

func TestSinkQueriesSendExactRequests(t *testing.T) {
	t.Run("record", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkRecordCmd(), &sinkTestAggregate{sink: query}, nil, nil, "12"))
		require.Equal(t, []string{"sink"}, query.calls)
		require.Equal(t, &types.QuerySink{Id: "12"}, query.reqs[0])
	})

	t.Run("transfer", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkTransferCmd(), &sinkTestAggregate{sink: query}, nil, nil, "12"))
		require.Equal(t, &types.QueryTransferRequest{Id: 12}, query.reqs[0])
	})

	t.Run("open transfers page", func(t *testing.T) {
		query := &sinkTestQuery{}
		cmd := GetSinkOpenTransfersCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagStartAfter, "5"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "50"))
		require.NoError(t, runSemanticQuery(t, cmd, &sinkTestAggregate{sink: query}, nil, nil))
		require.Equal(t, &types.QueryOpenTransfersRequest{StartAfter: 5, Limit: 50}, query.reqs[0])
	})

	t.Run("held only reads every page", func(t *testing.T) {
		query := &sinkTestQuery{pages: map[uint64]*types.QueryOpenTransfersResponse{
			0: {Transfers: []types.TransferState{sinkTestState(t, types.Hold_HOLD_NONE, 1), sinkTestState(t, types.Hold_HOLD_VETO, 2)}, NextStartAfter: 2},
			2: {Transfers: []types.TransferState{sinkTestState(t, types.Hold_HOLD_CONFLICT, 3)}},
		}}
		cmd := GetSinkOpenTransfersCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagHeldOnly, "true"))

		out, err := runSemanticQueryBuffer(t, cmd, &sinkTestAggregate{sink: query}, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"open-transfers", "open-transfers"}, query.calls)
		require.Contains(t, out, `"HOLD_VETO"`)
		require.Contains(t, out, `"HOLD_CONFLICT"`)
		require.NotContains(t, out, `"HOLD_NONE"`)
	})

	t.Run("held only rejects a cursor that does not advance", func(t *testing.T) {
		query := &sinkTestQuery{pages: map[uint64]*types.QueryOpenTransfersResponse{
			0: {NextStartAfter: 4},
			4: {NextStartAfter: 4},
		}}
		cmd := GetSinkOpenTransfersCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagHeldOnly, "true"))
		err := runSemanticQuery(t, cmd, &sinkTestAggregate{sink: query}, nil, nil)
		require.ErrorContains(t, err, "malformed node response")
		require.Len(t, query.calls, 2)
	})

	t.Run("account", func(t *testing.T) {
		query := &sinkTestQuery{}
		address := sdk.AccAddress(bytes.Repeat([]byte{5}, 20)).String()
		require.NoError(t, runSemanticQuery(t, GetSinkAccountCmd(), &sinkTestAggregate{sink: query}, nil, nil, address))
		require.Equal(t, &types.QueryAccountRequest{Sender: address}, query.reqs[0])
	})

	t.Run("state and params", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkStateCmd(), &sinkTestAggregate{sink: query}, nil, nil))
		out, err := runSemanticQueryBuffer(t, GetSinkParamsCmd(), &sinkTestAggregate{sink: query}, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"state", "state"}, query.calls)
		require.Contains(t, out, `"challenge_period_seconds"`)
		require.NotContains(t, out, `"totals"`)
	})
}

func TestSinkQueriesRejectInvalidInputBeforeTransport(t *testing.T) {
	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{name: "record leading zero", cmd: GetSinkRecordCmd(), args: []string{"012"}},
		{name: "record zero", cmd: GetSinkRecordCmd(), args: []string{"0"}},
		{name: "transfer not a number", cmd: GetSinkTransferCmd(), args: []string{"twelve"}},
		{name: "account bad address", cmd: GetSinkAccountCmd(), args: []string{"not-an-address"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := &sinkTestQuery{}
			require.Error(t, runSemanticQuery(t, tc.cmd, &sinkTestAggregate{sink: query}, nil, nil, tc.args...))
			require.Empty(t, query.calls)
		})
	}
}

func TestSinkTransactionsBroadcastExactMessages(t *testing.T) {
	from := newModuleTxFixture().from.String()

	t.Run("deposit", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "SINK-1"}}
		require.NoError(t, runSinkTx(t, GetTxSinkDepositCmd(), &sinkTestQuery{}, capture, sinkTestDestination(), "1.5akt"))

		msg := requireSingleModuleMessage[*types.MsgSink](t, capture)
		require.Equal(t, &types.MsgSink{
			DestType: types.DestTypeSolana,
			Dest:     sinkTestDestination(),
			Amount:   sdk.NewCoin(types.Denom, math.NewInt(1_500_000)),
			Sender:   from,
		}, msg)
	})

	t.Run("guardian pause", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkGuardianPauseCmd(), &sinkTestQuery{}, capture, "refunds"))
		require.Equal(t, &types.MsgGuardianPause{Guardian: from, Refunds: true}, requireSingleModuleMessage[*types.MsgGuardianPause](t, capture))
	})

	t.Run("report reads epoch and round from the chain", func(t *testing.T) {
		query := &sinkTestQuery{epoch: 4, round: 2}
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkReportOutcomeCmd(), query, capture, "12", "voided", "400", sinkTestSignature()))

		msg := requireSingleModuleMessage[*types.MsgReportOutcome](t, capture)
		require.Equal(t, from, msg.Reporter)
		require.Equal(t, uint64(12), msg.Id)
		require.Equal(t, uint64(4), msg.ReporterEpoch)
		require.Equal(t, uint64(2), msg.Round)
		require.Equal(t, types.Outcome_OUTCOME_VOIDED, msg.Observation.Outcome)
		require.Equal(t, []string{"state", "transfer"}, query.calls)
	})

	t.Run("report overrides skip the reads", func(t *testing.T) {
		query := &sinkTestQuery{epoch: 4, round: 2}
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		cmd := GetTxSinkReportOutcomeCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagReporterEpoch, "9"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagRound, "0"))
		require.NoError(t, runSinkTx(t, cmd, query, capture, "12", "minted", "400", sinkTestSignature()))

		msg := requireSingleModuleMessage[*types.MsgReportOutcome](t, capture)
		require.Equal(t, uint64(9), msg.ReporterEpoch)
		require.Equal(t, uint64(0), msg.Round)
		require.Empty(t, query.calls)
	})

	t.Run("veto and refund", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkVetoVoidCmd(), &sinkTestQuery{}, capture, "12", "mint found"))
		require.Equal(t, &types.MsgVetoVoid{Signer: from, Id: 12, Reason: "mint found"}, requireSingleModuleMessage[*types.MsgVetoVoid](t, capture))

		capture = &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkFinalizeRefundCmd(), &sinkTestQuery{}, capture, "12"))
		require.Equal(t, &types.MsgFinalizeRefund{Executor: from, Id: 12}, requireSingleModuleMessage[*types.MsgFinalizeRefund](t, capture))
	})
}

func requireSinkProposal[T sdk.Msg](t *testing.T, capture *moduleTxCapture) (*govv1.MsgSubmitProposal, T) {
	t.Helper()

	proposal := requireSingleModuleMessage[*govv1.MsgSubmitProposal](t, capture)
	require.Len(t, proposal.Messages, 1)
	inner, ok := proposal.Messages[0].GetCachedValue().(T)
	require.Truef(t, ok, "proposal message type = %T", proposal.Messages[0].GetCachedValue())

	return proposal, inner
}

func TestSinkProposalsWrapGovernanceMessages(t *testing.T) {
	gov := DefaultGovAuthority.String()

	t.Run("void", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeVoidCmd(), &sinkTestQuery{}, capture, "12"))

		proposal, msg := requireSinkProposal[*types.MsgRequestVoid](t, capture)
		require.Equal(t, &types.MsgRequestVoid{Authority: gov, Id: 12}, msg)
		require.Equal(t, "Void sink transfer 12", proposal.Title)
		require.NotEmpty(t, proposal.Summary)
		require.Equal(t, newModuleTxFixture().from.String(), proposal.Proposer)
	})

	t.Run("title, summary and deposit flags", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		cmd := GetTxSinkProposeVetoCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "Hold transfer 12"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagSummary, "Evidence does not match."))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "50000000uakt"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagExpedite, "true"))
		require.NoError(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, "12", "evidence does not match"))

		proposal, msg := requireSinkProposal[*types.MsgVetoVoid](t, capture)
		require.Equal(t, &types.MsgVetoVoid{Signer: gov, Id: 12, Reason: "evidence does not match"}, msg)
		require.Equal(t, "Hold transfer 12", proposal.Title)
		require.Equal(t, "Evidence does not match.", proposal.Summary)
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 50_000_000)), sdk.Coins(proposal.InitialDeposit))
		require.True(t, proposal.Expedited)
	})

	t.Run("resolve restart and confirm", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeResolveCmd(), &sinkTestQuery{}, capture, "12", "restart", "reporters disagreed"))
		_, msg := requireSinkProposal[*types.MsgResolveTransfer](t, capture)
		require.Equal(t, types.Decision_DECISION_RESTART, msg.Decision)
		require.Nil(t, msg.Observation)

		capture = &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeResolveCmd(), &sinkTestQuery{}, capture, "12", "confirm-mint", "mint is final", "400", sinkTestSignature()))
		_, msg = requireSinkProposal[*types.MsgResolveTransfer](t, capture)
		require.Equal(t, types.Decision_DECISION_CONFIRM_MINT, msg.Decision)
		require.Equal(t, types.Outcome_OUTCOME_MINTED, msg.Observation.Outcome)
		require.Equal(t, uint64(400), msg.Observation.Slot)
	})

	t.Run("params from a file", func(t *testing.T) {
		params := types.DefaultParams()
		params.MinTransfer = 1_000_000
		raw, err := newModuleTxFixture().cctx.Codec.MarshalJSON(&params)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "sink-params.json")
		require.NoError(t, os.WriteFile(path, raw, 0o600))

		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeParamsCmd(), &sinkTestQuery{}, capture, path))
		_, msg := requireSinkProposal[*types.MsgUpdateParams](t, capture)
		require.Equal(t, gov, msg.Authority)
		require.Truef(t, proto.Equal(&params, &msg.Params), "proposed params = %v, want %v", msg.Params, params)
	})
}

func TestSinkTransactionsRejectInvalidInputBeforeBroadcast(t *testing.T) {
	invalidParams := types.DefaultParams()
	invalidParams.MinTransfer = invalidParams.MaxTransfer + 1
	raw, err := newModuleTxFixture().cctx.Codec.MarshalJSON(&invalidParams)
	require.NoError(t, err)
	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	require.NoError(t, os.WriteFile(invalidPath, raw, 0o600))
	unknownPath := filepath.Join(t.TempDir(), "unknown.json")
	require.NoError(t, os.WriteFile(unknownPath, []byte(`{"enabled":true,"no_such_field":1}`), 0o600))

	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{name: "deposit fractional uakt", cmd: GetTxSinkDepositCmd, args: []string{sinkTestDestination(), "1.5uakt"}},
		{name: "deposit wrong denom", cmd: GetTxSinkDepositCmd, args: []string{sinkTestDestination(), "5uact"}},
		{name: "deposit bad destination", cmd: GetTxSinkDepositCmd, args: []string{"not-base58-0OIl", "1akt"}},
		{name: "deposit zero destination", cmd: GetTxSinkDepositCmd, args: []string{base58.Encode(make([]byte, 32)), "1akt"}},
		{name: "pause unknown target", cmd: GetTxSinkGuardianPauseCmd, args: []string{"everything"}},
		{name: "report bad outcome", cmd: GetTxSinkReportOutcomeCmd, args: []string{"12", "refunded", "400", sinkTestSignature()}},
		{name: "report zero slot", cmd: GetTxSinkReportOutcomeCmd, args: []string{"12", "minted", "0", sinkTestSignature()}},
		{name: "report short signature", cmd: GetTxSinkReportOutcomeCmd, args: []string{"12", "minted", "400", base58.Encode([]byte{1, 2, 3})}},
		{name: "veto blank reason", cmd: GetTxSinkVetoVoidCmd, args: []string{"12", "  "}},
		{name: "refund bad id", cmd: GetTxSinkFinalizeRefundCmd, args: []string{"012"}},
		{name: "resolve restart with evidence", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "restart", "why", "400", sinkTestSignature()}},
		{name: "resolve confirm without evidence", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "confirm-void", "why"}},
		{name: "resolve unknown decision", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "approve", "why"}},
		{name: "params invalid", cmd: GetTxSinkProposeParamsCmd, args: []string{invalidPath}},
		{name: "params unknown field", cmd: GetTxSinkProposeParamsCmd, args: []string{unknownPath}},
		{name: "params missing file", cmd: GetTxSinkProposeParamsCmd, args: []string{filepath.Join(t.TempDir(), "missing.json")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := &sinkTestQuery{}
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			require.Error(t, runSinkTx(t, tc.cmd(), query, capture, tc.args...))
			require.Empty(t, capture.messages)
			require.Empty(t, query.calls)
		})
	}

	t.Run("bad authority", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		cmd := GetTxSinkProposeVoidCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagAuthority, "not-an-address"))
		require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, "12"))
		require.Empty(t, capture.messages)
	})
}

func TestSinkQueriesReportTransportAndMalformedResponses(t *testing.T) {
	address := sdk.AccAddress(bytes.Repeat([]byte{5}, 20)).String()
	cases := []struct {
		method string
		cmd    func() *cobra.Command
		args   []string
	}{
		{method: "sink", cmd: GetSinkRecordCmd, args: []string{"12"}},
		{method: "transfer", cmd: GetSinkTransferCmd, args: []string{"12"}},
		{method: "open-transfers", cmd: GetSinkOpenTransfersCmd},
		{method: "account", cmd: GetSinkAccountCmd, args: []string{address}},
		{method: "state", cmd: GetSinkStateCmd},
		{method: "state", cmd: GetSinkParamsCmd},
	}

	for _, tc := range cases {
		t.Run(tc.cmd().Name()+" transport", func(t *testing.T) {
			query := &sinkTestQuery{fail: map[string]error{tc.method: errors.New("node unavailable")}}
			require.ErrorContains(t, runSemanticQuery(t, tc.cmd(), &sinkTestAggregate{sink: query}, nil, nil, tc.args...), "node unavailable")
		})
		t.Run(tc.cmd().Name()+" malformed", func(t *testing.T) {
			query := &sinkTestQuery{empty: map[string]bool{tc.method: true}}
			require.ErrorContains(t, runSemanticQuery(t, tc.cmd(), &sinkTestAggregate{sink: query}, nil, nil, tc.args...), "malformed node response")
		})
	}

	t.Run("held only later page", func(t *testing.T) {
		query := &sinkTestQuery{
			pages:   map[uint64]*types.QueryOpenTransfersResponse{0: {NextStartAfter: 3}},
			pageErr: errors.New("page 2 unavailable"),
		}
		cmd := GetSinkOpenTransfersCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagHeldOnly, "true"))
		require.ErrorContains(t, runSemanticQuery(t, cmd, &sinkTestAggregate{sink: query}, nil, nil), "page 2 unavailable")
	})
}

func TestSinkOpenTransfersRequireItsFlags(t *testing.T) {
	source := GetSinkOpenTransfersCmd()
	for _, keep := range [][]string{
		nil,
		{flagdefs.FlagStartAfter},
		{flagdefs.FlagStartAfter, flagdefs.FlagLimit},
	} {
		query := &sinkTestQuery{}
		require.Error(t, runSemanticQuery(t, sinkPartialCmd(source, keep...), &sinkTestAggregate{sink: query}, nil, nil))
		require.Empty(t, query.calls)
	}
}

// runSinkAccountQuery runs the account query with a chosen default account.
func runSinkAccountQuery(t *testing.T, query *sinkTestQuery, adjust func(sdkclient.Context) sdkclient.Context) error {
	t.Helper()

	encoding := newModuleTxFixture().cctx
	cctx := adjust(sdkclient.Context{}.WithCodec(encoding.Codec).WithInterfaceRegistry(encoding.InterfaceRegistry).WithOutput(io.Discard))
	cmd := GetSinkAccountCmd()
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	ctx = context.WithValue(ctx, ContextTypeQueryClient, &semanticQueryLightClient{query: &sinkTestAggregate{sink: query}, cctx: cctx})
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagOutput, "json"))

	return cmd.RunE(cmd, nil)
}

func TestSinkAccountUsesTheDefaultAccount(t *testing.T) {
	from := sdk.AccAddress(bytes.Repeat([]byte{5}, 20))

	t.Run("resolved", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSinkAccountQuery(t, query, func(c sdkclient.Context) sdkclient.Context { return c.WithFromAddress(from) }))
		require.Equal(t, &types.QueryAccountRequest{Sender: from.String()}, query.reqs[0])
	})

	t.Run("none configured", func(t *testing.T) {
		query := &sinkTestQuery{}
		err := runSinkAccountQuery(t, query, func(c sdkclient.Context) sdkclient.Context { return c })
		require.ErrorContains(t, err, "no default account set")
		require.Empty(t, query.calls)
	})

	t.Run("unresolvable", func(t *testing.T) {
		query := &sinkTestQuery{}
		err := runSinkAccountQuery(t, query, func(c sdkclient.Context) sdkclient.Context { return c.WithFrom("mykey") })
		require.ErrorContains(t, err, "keyring is unavailable")
		require.Empty(t, query.calls)
	})
}

func TestSinkTransactionsPreserveBroadcastErrors(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{name: "deposit", cmd: GetTxSinkDepositCmd, args: []string{sinkTestDestination(), "1akt"}},
		{name: "guardian pause", cmd: GetTxSinkGuardianPauseCmd, args: []string{"all"}},
		{name: "report", cmd: GetTxSinkReportOutcomeCmd, args: []string{"12", "minted", "400", sinkTestSignature()}},
		{name: "veto", cmd: GetTxSinkVetoVoidCmd, args: []string{"12", "mint found"}},
		{name: "refund", cmd: GetTxSinkFinalizeRefundCmd, args: []string{"12"}},
		{name: "proposal", cmd: GetTxSinkProposeVoidCmd, args: []string{"12"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &moduleTxCapture{err: errors.New("broadcast refused")}
			require.ErrorContains(t, runSinkTx(t, tc.cmd(), &sinkTestQuery{}, capture, tc.args...), "broadcast refused")
		})
		t.Run(tc.name+" bad fee payer", func(t *testing.T) {
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			cmd := tc.cmd()
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagFeePayer, "not-a-fee-payer"))
			require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, tc.args...))
			require.Empty(t, capture.messages)
		})
	}
}

func TestSinkReportOutcomeReadsBeforeSigning(t *testing.T) {
	args := []string{"12", "minted", "400", sinkTestSignature()}

	for name, query := range map[string]*sinkTestQuery{
		"state fails":    {fail: map[string]error{"state": errors.New("state unavailable")}},
		"state empty":    {empty: map[string]bool{"state": true}},
		"transfer fails": {fail: map[string]error{"transfer": errors.New("transfer unavailable")}},
		"transfer empty": {empty: map[string]bool{"transfer": true}},
	} {
		t.Run(name, func(t *testing.T) {
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			require.Error(t, runSinkTx(t, GetTxSinkReportOutcomeCmd(), query, capture, args...))
			require.Empty(t, capture.messages)
		})
	}

	t.Run("missing flags", func(t *testing.T) {
		source := GetTxSinkReportOutcomeCmd()
		require.Error(t, runSinkTx(t, sinkPartialCmd(source), &sinkTestQuery{}, &moduleTxCapture{}, args...))

		partial := sinkPartialCmd(source, flagdefs.FlagReporterEpoch)
		require.NoError(t, partial.Flags().Set(flagdefs.FlagReporterEpoch, "1"))
		require.Error(t, runSinkTx(t, partial, &sinkTestQuery{}, &moduleTxCapture{}, args...))
	})
}

func TestSinkTransactionsRejectMissingFlagsAndSigner(t *testing.T) {
	t.Run("deposit destination type", func(t *testing.T) {
		capture := &moduleTxCapture{}
		require.Error(t, runSinkTx(t, sinkPartialCmd(GetTxSinkDepositCmd()), &sinkTestQuery{}, capture, sinkTestDestination(), "1akt"))
		require.Empty(t, capture.messages)
	})

	t.Run("guardian without a signer", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		noSigner := func(c sdkclient.Context) sdkclient.Context { return c.WithFrom("").WithFromAddress(nil) }
		require.Error(t, runSinkTxContext(t, GetTxSinkGuardianPauseCmd(), &sinkTestQuery{}, capture, noSigner, "all"))
		require.Empty(t, capture.messages)
	})

	t.Run("proposal flags", func(t *testing.T) {
		source := GetTxSinkProposeVoidCmd()
		for _, keep := range [][]string{
			nil,
			{flagdefs.FlagAuthority},
			{flagdefs.FlagAuthority, flagdefs.FlagTitle},
			{flagdefs.FlagAuthority, flagdefs.FlagTitle, flagdefs.FlagSummary},
			{flagdefs.FlagAuthority, flagdefs.FlagTitle, flagdefs.FlagSummary, flagdefs.FlagDeposit},
		} {
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			require.Error(t, runSinkTx(t, sinkPartialCmd(source, keep...), &sinkTestQuery{}, capture, "12"))
			require.Empty(t, capture.messages)
		}
	})

	t.Run("proposal deposit", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		cmd := GetTxSinkProposeVoidCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "fifty"))
		require.ErrorContains(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, "12"), "invalid proposal deposit")
		require.Empty(t, capture.messages)
	})

	t.Run("proposal without a message", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{}}
		cmd := GetTxSinkProposeVoidCmd()
		client := setSinkTxContext(cmd, &sinkTestQuery{}, capture, nil)
		require.ErrorContains(t, submitSinkProposal(cmd, client, nil, "title", "summary"), "build sink proposal")
		require.Empty(t, capture.messages)
	})
}

func TestSinkProposalsRejectInvalidArguments(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{name: "params bad authority", cmd: GetTxSinkProposeParamsCmd, args: []string{"params.json"}},
		{name: "void bad id", cmd: GetTxSinkProposeVoidCmd, args: []string{"0"}},
		{name: "veto bad id", cmd: GetTxSinkProposeVetoCmd, args: []string{"x", "reason"}},
		{name: "veto blank reason", cmd: GetTxSinkProposeVetoCmd, args: []string{"12", " "}},
		{name: "veto bad authority", cmd: GetTxSinkProposeVetoCmd, args: []string{"12", "reason"}},
		{name: "resolve bad id", cmd: GetTxSinkProposeResolveCmd, args: []string{"x", "restart", "reason"}},
		{name: "resolve blank reason", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "restart", ""}},
		{name: "resolve bad authority", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "restart", "reason"}},
		{name: "resolve bad evidence", cmd: GetTxSinkProposeResolveCmd, args: []string{"12", "confirm-void", "reason", "0", sinkTestSignature()}},
		{name: "report bad id", cmd: GetTxSinkReportOutcomeCmd, args: []string{"0", "minted", "400", sinkTestSignature()}},
		{name: "veto void bad id", cmd: GetTxSinkVetoVoidCmd, args: []string{"0", "reason"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			if flag := cmd.Flags().Lookup(flagdefs.FlagAuthority); flag != nil && (tc.name == "params bad authority" || tc.name == "veto bad authority" || tc.name == "resolve bad authority") {
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagAuthority, "not-an-address"))
			}
			capture := &moduleTxCapture{response: &sdk.TxResponse{}}
			require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, tc.args...))
			require.Empty(t, capture.messages)
		})
	}
}

func TestSinkParserEdgeCases(t *testing.T) {
	_, err := parseSinkObservation(types.Outcome_OUTCOME_UNSPECIFIED, "400", sinkTestSignature())
	require.Error(t, err)

	deposits, refunds, err := parseSinkPauseTarget("deposits")
	require.NoError(t, err)
	require.True(t, deposits)
	require.False(t, refunds)
}
