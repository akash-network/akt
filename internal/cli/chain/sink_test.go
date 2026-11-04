package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	stdmath "math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

// sinkTestQuery captures the public requests and supports boundary failures.
type sinkTestQuery struct {
	types.QueryClient
	calls     []string
	reqs      []interface{}
	version   uint64
	round     uint64
	fail      map[string]error
	empty     map[string]bool
	policies  []types.ReporterPolicy
	transfer  *types.TransferState
	transfers []types.TransferState
	params    *types.Params
}

func (q *sinkTestQuery) record(method string, request interface{}) (bool, error) {
	q.calls = append(q.calls, method)
	q.reqs = append(q.reqs, request)
	return q.fail[method] != nil || q.empty[method], q.fail[method]
}

func (q *sinkTestQuery) State(_ context.Context, request *types.QueryStateRequest, _ ...grpc.CallOption) (*types.QueryStateResponse, error) {
	if stop, err := q.record("state", request); stop {
		return nil, err
	}
	return &types.QueryStateResponse{}, nil
}

func (q *sinkTestQuery) Params(_ context.Context, request *types.QueryParamsRequest, _ ...grpc.CallOption) (*types.QueryParamsResponse, error) {
	if stop, err := q.record("params", request); stop {
		return nil, err
	}
	params := types.DefaultParams()
	if q.params != nil {
		params = *q.params
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (q *sinkTestQuery) Reporters(_ context.Context, request *types.QueryReportersRequest, _ ...grpc.CallOption) (*types.QueryReportersResponse, error) {
	if stop, err := q.record("reporters", request); stop {
		return nil, err
	}
	policies := q.policies
	if policies == nil {
		version := q.version
		if version == 0 {
			version = 1
		}
		policies = []types.ReporterPolicy{{
			Version:   version,
			Members:   []string{sinkTestID().Owner},
			Threshold: math.LegacyOneDec(),
		}}
	}
	return &types.QueryReportersResponse{Reporters: policies}, nil
}

func (q *sinkTestQuery) Transfer(_ context.Context, request *types.QueryTransferRequest, _ ...grpc.CallOption) (*types.QueryTransferResponse, error) {
	if stop, err := q.record("transfer", request); stop {
		return nil, err
	}
	state := sinkTestState(types.HoldNone, request.ID)
	if q.round > 0 {
		state.Settlement.Round = q.round
	}
	if q.transfer != nil {
		state = *q.transfer
	}
	return &types.QueryTransferResponse{Transfer: state}, nil
}

func (q *sinkTestQuery) Account(_ context.Context, request *types.QueryAccountRequest, _ ...grpc.CallOption) (*types.QueryAccountResponse, error) {
	if stop, err := q.record("account", request); stop {
		return nil, err
	}
	return &types.QueryAccountResponse{}, nil
}

func (q *sinkTestQuery) Transfers(_ context.Context, request *types.QueryTransfersRequest, _ ...grpc.CallOption) (*types.QueryTransfersResponse, error) {
	if stop, err := q.record("transfers", request); stop {
		return nil, err
	}
	return &types.QueryTransfersResponse{Transfers: q.transfers}, nil
}

func (q *sinkTestQuery) Disputes(_ context.Context, request *types.QueryDisputesRequest, _ ...grpc.CallOption) (*types.QueryDisputesResponse, error) {
	if stop, err := q.record("disputes", request); stop {
		return nil, err
	}
	return &types.QueryDisputesResponse{Transfers: q.transfers}, nil
}

type sinkTestAggregate struct {
	clientv1beta3.QueryClient
	sink types.QueryClient
}

func (q *sinkTestAggregate) Sink() types.QueryClient { return q.sink }

func sinkTestID() types.ID {
	return types.ID{
		Owner:    sdk.AccAddress(bytes.Repeat([]byte{5}, 20)).String(),
		Height:   42,
		Sequence: 12,
	}
}

func sinkTestIDString() string {
	id := sinkTestID()
	return id.String()
}

func sinkTestState(hold types.Hold, id types.ID) types.TransferState {
	return types.TransferState{
		ID: id,
		Data: types.RecordData{
			Dest:   sinkTestDestination(),
			Amount: sdk.NewInt64Coin("uakt", 1_000_000),
		},
		Status: types.StatusTransferPending,
		Settlement: types.SettlementData{
			Hold:  hold,
			Round: 1,
		},
	}
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
	client := &moduleCommandClient{
		cctx:  cctx,
		tx:    capture,
		query: &sinkTestAggregate{sink: query},
	}
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
	cmd := &cobra.Command{
		Use:  source.Use,
		RunE: source.RunE,
	}
	for _, name := range keep {
		cmd.Flags().AddFlag(source.Flags().Lookup(name))
	}
	return cmd
}

func TestSinkParsers(t *testing.T) {
	t.Run("transfer ID", func(t *testing.T) {
		id, err := parseSinkTransferID(sinkTestIDString())
		require.NoError(t, err)
		require.Equal(t, sinkTestID(), id)

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
			require.Equal(t, sdk.NewCoin(sdkutil.DenomUakt, math.NewInt(want)), coin, in)
		}

		for _, bad := range []string{"1.5uakt", "0.0000001akt", "0uakt", "5uact", "akt", "-1uakt", "18446744073709551616uakt"} {
			_, err := parseSinkAmount(bad)
			require.Errorf(t, err, "accepted %q", bad)
		}
	})

	t.Run("evidence", func(t *testing.T) {
		observation, err := parseSinkObservation(types.OutcomeMinted, "400", sinkTestSignature())
		require.NoError(t, err)
		require.Equal(t, uint64(400), observation.Slot)
		require.Len(t, observation.TransactionSignature, 64)

		zero := base58.Encode(make([]byte, 64))
		short := base58.Encode(bytes.Repeat([]byte{1}, 63))
		for _, args := range [][2]string{{"0", sinkTestSignature()}, {"x", sinkTestSignature()}, {"400", zero}, {"400", short}, {"400", "0OIl"}} {
			_, err := parseSinkObservation(types.OutcomeMinted, args[0], args[1])
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
	t.Run("transfer", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkTransferCmd(), &sinkTestAggregate{sink: query}, nil, nil, sinkTestIDString()))
		require.Equal(t, &types.QueryTransferRequest{ID: sinkTestID()}, query.reqs[0])
	})

	for _, args := range [][]string{nil, {"minted"}, {sinkTestID().Owner}, {sinkTestID().Owner, "refund-pending"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			query := &sinkTestQuery{}
			cmd := GetSinkTransfersCmd()
			key := []byte{0, 255, 9}
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagPageKey, base64.StdEncoding.EncodeToString(key)))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "50"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagReverse, "true"))
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagCountTotal, "true"))
			require.NoError(t, runSemanticQuery(t, cmd, &sinkTestAggregate{sink: query}, nil, nil, args...))
			req := query.reqs[0].(*types.QueryTransfersRequest)
			require.Equal(t, &sdkquery.PageRequest{
				Key:        key,
				Limit:      50,
				Reverse:    true,
				CountTotal: true,
			}, req.Pagination)
			if len(args) > 0 && args[0] == sinkTestID().Owner {
				require.Equal(t, args[0], req.Owner)
			} else {
				require.Empty(t, req.Owner)
			}
			wantStatus := types.StatusInvalid
			if len(args) == 2 {
				wantStatus = types.StatusRefundPending
			}
			if len(args) == 1 && args[0] == "minted" {
				wantStatus = types.StatusMinted
			}
			require.Equal(t, wantStatus, req.Status)
		})
	}

	t.Run("disputes include terminal holds", func(t *testing.T) {
		state := sinkTestState(types.HoldConflict, sinkTestID())
		state.Status = types.StatusMinted
		query := &sinkTestQuery{
			transfers: []types.TransferState{state},
		}
		cmd := GetSinkDisputesCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "3"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagPageKey, "AQI="))
		out, err := runSemanticQueryBuffer(t, cmd, &sinkTestAggregate{sink: query}, nil)
		require.NoError(t, err)
		require.Equal(t, &types.QueryDisputesRequest{
			Pagination: &sdkquery.PageRequest{
				Key:   []byte{1, 2},
				Limit: 3,
			},
		}, query.reqs[0])
		require.Contains(t, out, "STATUS_MINTED")
		require.Contains(t, out, "HOLD_CONFLICT")
		require.Len(t, query.calls, 1)
	})

	for arg, version := range map[string]uint64{"": 0, "0": 0, "7": 7, "all": stdmath.MaxUint64} {
		t.Run("reporters "+arg, func(t *testing.T) {
			query := &sinkTestQuery{}
			var args []string
			if arg != "" {
				args = []string{arg}
			}
			require.NoError(t, runSemanticQuery(t, GetSinkReportersCmd(), &sinkTestAggregate{sink: query}, nil, nil, args...))
			require.Equal(t, &types.QueryReportersRequest{Version: version}, query.reqs[0])
		})
	}

	t.Run("account", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkAccountCmd(), &sinkTestAggregate{sink: query}, nil, nil, sinkTestID().Owner))
		require.Equal(t, &types.QueryAccountRequest{Owner: sinkTestID().Owner}, query.reqs[0])
	})

	t.Run("separate state and params RPCs", func(t *testing.T) {
		query := &sinkTestQuery{}
		require.NoError(t, runSemanticQuery(t, GetSinkStateCmd(), &sinkTestAggregate{sink: query}, nil, nil))
		out, err := runSemanticQueryBuffer(t, GetSinkParamsCmd(), &sinkTestAggregate{sink: query}, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"state", "params"}, query.calls)
		require.Contains(t, out, `"reporter_challenge_period"`)
		require.NotContains(t, out, `"totals"`)
	})
}

func TestSinkQueriesRejectInvalidInputBeforeTransport(t *testing.T) {
	cases := []struct {
		cmd   *cobra.Command
		args  []string
		flags map[string]string
	}{
		{
			cmd:  GetSinkTransferCmd(),
			args: []string{"12"},
		},
		{
			cmd:  GetSinkTransferCmd(),
			args: []string{sinkTestID().Owner + "/0/1"},
		},
		{
			cmd:  GetSinkAccountCmd(),
			args: []string{"not-an-address"},
		},
		{
			cmd:  GetSinkTransfersCmd(),
			args: []string{"not-an-address"},
		},
		{
			cmd:  GetSinkTransfersCmd(),
			args: []string{sinkTestID().Owner, "bad-status"},
		},
		{
			cmd:  GetSinkTransfersCmd(),
			args: []string{"minted", sinkTestID().Owner},
		},
		{
			cmd:   GetSinkTransfersCmd(),
			flags: map[string]string{flagdefs.FlagPageKey: "bad!"},
		},
		{
			cmd:   GetSinkTransfersCmd(),
			flags: map[string]string{flagdefs.FlagPage: "2"},
		},
		{
			cmd:   GetSinkTransfersCmd(),
			args:  []string{"minted"},
			flags: map[string]string{flagdefs.FlagPageKey: "AQ==", flagdefs.FlagOffset: "1"},
		},
		{
			cmd:   GetSinkDisputesCmd(),
			flags: map[string]string{flagdefs.FlagPageKey: "AQ==", flagdefs.FlagOffset: "1"},
		},
		{
			cmd:  GetSinkReportersCmd(),
			args: []string{"no"},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			query := &sinkTestQuery{}
			for k, v := range tc.flags {
				require.NoError(t, tc.cmd.Flags().Set(k, v))
			}
			require.Error(t, runSemanticQuery(t, tc.cmd, &sinkTestAggregate{sink: query}, nil, nil, tc.args...))
			require.Empty(t, query.calls)
		})
	}
}

func TestSinkTransactionsBroadcastExactMessages(t *testing.T) {
	from := newModuleTxFixture().from.String()

	t.Run("deposit", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{TxHash: "SINK-1"},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkDepositCmd(), &sinkTestQuery{}, capture, sinkTestDestination(), "1.5akt"))

		msg := requireSingleModuleMessage[*types.MsgSink](t, capture)
		require.Equal(t, &types.MsgSink{
			DestType: types.DestTypeSolana,
			Dest:     sinkTestDestination(),
			Amount:   sdk.NewCoin(sdkutil.DenomUakt, math.NewInt(1_500_000)),
			Sender:   from,
		}, msg)
	})

	t.Run("guardian pause", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkGuardianPauseCmd(), &sinkTestQuery{}, capture, "refunds"))
		require.Equal(t, &types.MsgGuardianPause{
			Guardian: from,
			Refunds:  true,
		}, requireSingleModuleMessage[*types.MsgGuardianPause](t, capture))
	})

	t.Run("report reads policy and round from the chain", func(t *testing.T) {
		query := &sinkTestQuery{
			version: 4,
			round:   2,
		}
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkReportOutcomeCmd(), query, capture, sinkTestIDString(), "voided", "400", sinkTestSignature()))

		msg := requireSingleModuleMessage[*types.MsgReportOutcome](t, capture)
		require.Equal(t, from, msg.Reporter)
		require.Equal(t, sinkTestID(), msg.ID)
		require.Equal(t, uint64(4), msg.PolicyVersion)
		require.Equal(t, uint64(2), msg.Round)
		require.Equal(t, types.OutcomeVoided, msg.Observation.Outcome)
		require.Equal(t, []string{"reporters", "transfer"}, query.calls)
	})

	t.Run("report overrides skip the reads", func(t *testing.T) {
		query := &sinkTestQuery{
			version: 4,
			round:   2,
		}
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		cmd := GetTxSinkReportOutcomeCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagPolicyVersion, "9"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagRound, "3"))
		require.NoError(t, runSinkTx(t, cmd, query, capture, sinkTestIDString(), "minted", "400", sinkTestSignature()))

		msg := requireSingleModuleMessage[*types.MsgReportOutcome](t, capture)
		require.Equal(t, uint64(9), msg.PolicyVersion)
		require.Equal(t, uint64(3), msg.Round)
		require.Empty(t, query.calls)
	})

	t.Run("veto and refund", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkVetoVoidCmd(), &sinkTestQuery{}, capture, sinkTestIDString(), "mint found"))
		require.Equal(t, &types.MsgVetoVoid{
			Signer: from,
			ID:     sinkTestID(),
			Reason: "mint found",
		}, requireSingleModuleMessage[*types.MsgVetoVoid](t, capture))

		capture = &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkFinalizeRefundCmd(), &sinkTestQuery{}, capture, sinkTestIDString()))
		require.Equal(t, &types.MsgFinalizeRefund{
			Executor: from,
			ID:       sinkTestID(),
		}, requireSingleModuleMessage[*types.MsgFinalizeRefund](t, capture))
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
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeVoidCmd(), &sinkTestQuery{}, capture, sinkTestIDString()))

		proposal, msg := requireSinkProposal[*types.MsgRequestVoid](t, capture)
		require.Equal(t, &types.MsgRequestVoid{
			Authority:     gov,
			ID:            sinkTestID(),
			ExpectedRound: 1,
		}, msg)
		require.Equal(t, "Void sink transfer "+sinkTestIDString(), proposal.Title)
		require.NotEmpty(t, proposal.Summary)
		require.Equal(t, newModuleTxFixture().from.String(), proposal.Proposer)
	})

	t.Run("title, summary and deposit flags", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		cmd := GetTxSinkProposeVetoCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "Hold transfer 12"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagSummary, "Evidence does not match."))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "50000000uakt"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagExpedite, "true"))
		require.NoError(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, sinkTestIDString(), "evidence does not match"))

		proposal, msg := requireSinkProposal[*types.MsgVetoVoid](t, capture)
		require.Equal(t, &types.MsgVetoVoid{
			Signer: gov,
			ID:     sinkTestID(),
			Reason: "evidence does not match",
		}, msg)
		require.Equal(t, "Hold transfer 12", proposal.Title)
		require.Equal(t, "Evidence does not match.", proposal.Summary)
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 50_000_000)), sdk.Coins(proposal.InitialDeposit))
		require.True(t, proposal.Expedited)
	})

	t.Run("resolve restart and both confirmations", func(t *testing.T) {
		for _, decision := range []string{"restart", "confirm-mint", "confirm-void"} {
			cmd := GetTxSinkProposeResolveCmd()
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagExpectedRound, "7"))
			args := []string{sinkTestIDString(), decision, "reviewed evidence"}
			if decision != "restart" {
				args = append(args, "400", sinkTestSignature())
			}
			query := &sinkTestQuery{}
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			require.NoError(t, runSinkTx(t, cmd, query, capture, args...))
			require.Empty(t, query.calls)
			switch decision {
			case "restart":
				_, msg := requireSinkProposal[*types.MsgRestartTransfer](t, capture)
				require.Equal(t, &types.MsgRestartTransfer{
					Authority:     gov,
					ID:            sinkTestID(),
					Reason:        "reviewed evidence",
					ExpectedRound: 7,
				}, msg)
			case "confirm-mint":
				_, msg := requireSinkProposal[*types.MsgConfirmMint](t, capture)
				require.Equal(t, uint64(7), msg.ExpectedRound)
				require.Equal(t, sinkTestID(), msg.ID)
				require.Equal(t, gov, msg.Authority)
				require.Equal(t, uint64(400), msg.Evidence.Slot)
				require.Equal(t, bytes.Repeat([]byte{1}, 64), msg.Evidence.TransactionSignature)
			case "confirm-void":
				_, msg := requireSinkProposal[*types.MsgConfirmVoid](t, capture)
				require.Equal(t, uint64(7), msg.ExpectedRound)
				require.Equal(t, uint64(400), msg.Evidence.Slot)
				require.Equal(t, sinkTestID(), msg.ID)
				require.Equal(t, gov, msg.Authority)
			}
		}
	})

	t.Run("params from a file", func(t *testing.T) {
		params := types.DefaultParams()
		params.MinAmountPerTransfer = sdk.NewInt64Coin("uakt", 1_000_000)
		params.ReporterChallengePeriod += 123 * time.Millisecond
		params.ReportersThreshold = math.LegacyMustNewDecFromStr("0.666666666666666667")
		params.MaxTransferAmountPerEpoch = sdk.NewCoin("uakt", math.NewIntFromUint64(^uint64(0)))
		output, err := runSemanticQueryBuffer(t, GetSinkParamsCmd(), &sinkTestAggregate{
			sink: &sinkTestQuery{params: &params},
		}, nil)
		require.NoError(t, err)
		raw := []byte(output)
		path := filepath.Join(t.TempDir(), "sink-params.json")
		require.NoError(t, os.WriteFile(path, raw, 0o600))

		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		require.NoError(t, runSinkTx(t, GetTxSinkProposeParamsCmd(), &sinkTestQuery{}, capture, path))
		_, msg := requireSinkProposal[*types.MsgUpdateParams](t, capture)
		require.Equal(t, gov, msg.Authority)
		proposed, err := newModuleTxFixture().cctx.Codec.MarshalJSON(&msg.Params)
		require.NoError(t, err)
		require.JSONEq(t, string(raw), string(proposed))
	})
}

func TestSinkTransactionsRejectInvalidInputBeforeBroadcast(t *testing.T) {
	invalidParams := types.DefaultParams()
	invalidParams.MinAmountPerTransfer = invalidParams.MaxAmountPerTransfer.Add(sdk.NewInt64Coin("uakt", 1))
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
		{
			name: "deposit fractional uakt",
			cmd:  GetTxSinkDepositCmd,
			args: []string{sinkTestDestination(), "1.5uakt"},
		},
		{
			name: "deposit wrong denom",
			cmd:  GetTxSinkDepositCmd,
			args: []string{sinkTestDestination(), "5uact"},
		},
		{
			name: "deposit bad destination",
			cmd:  GetTxSinkDepositCmd,
			args: []string{"not-base58-0OIl", "1akt"},
		},
		{
			name: "deposit zero destination",
			cmd:  GetTxSinkDepositCmd,
			args: []string{base58.Encode(make([]byte, 32)), "1akt"},
		},
		{
			name: "pause unknown target",
			cmd:  GetTxSinkGuardianPauseCmd,
			args: []string{"everything"},
		},
		{
			name: "report bad outcome",
			cmd:  GetTxSinkReportOutcomeCmd,
			args: []string{sinkTestIDString(), "refunded", "400", sinkTestSignature()},
		},
		{
			name: "report zero slot",
			cmd:  GetTxSinkReportOutcomeCmd,
			args: []string{sinkTestIDString(), "minted", "0", sinkTestSignature()},
		},
		{
			name: "report short signature",
			cmd:  GetTxSinkReportOutcomeCmd,
			args: []string{sinkTestIDString(), "minted", "400", base58.Encode([]byte{1, 2, 3})},
		},
		{
			name: "veto blank reason",
			cmd:  GetTxSinkVetoVoidCmd,
			args: []string{sinkTestIDString(), "  "},
		},
		{
			name: "refund bad id",
			cmd:  GetTxSinkFinalizeRefundCmd,
			args: []string{"012"},
		},
		{
			name: "resolve restart with evidence",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "restart", "why", "400", sinkTestSignature()},
		},
		{
			name: "resolve confirm without evidence",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "confirm-void", "why"},
		},
		{
			name: "resolve unknown decision",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "approve", "why"},
		},
		{
			name: "params invalid",
			cmd:  GetTxSinkProposeParamsCmd,
			args: []string{invalidPath},
		},
		{
			name: "params unknown field",
			cmd:  GetTxSinkProposeParamsCmd,
			args: []string{unknownPath},
		},
		{
			name: "params missing file",
			cmd:  GetTxSinkProposeParamsCmd,
			args: []string{filepath.Join(t.TempDir(), "missing.json")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := &sinkTestQuery{}
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			require.Error(t, runSinkTx(t, tc.cmd(), query, capture, tc.args...))
			require.Empty(t, capture.messages)
			require.Empty(t, query.calls)
		})
	}

	t.Run("bad authority", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		cmd := GetTxSinkProposeVoidCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagAuthority, "not-an-address"))
		require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, sinkTestIDString()))
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
		{
			method: "transfer",
			cmd:    GetSinkTransferCmd,
			args:   []string{sinkTestIDString()},
		},
		{
			method: "transfers",
			cmd:    GetSinkTransfersCmd,
		},
		{
			method: "disputes",
			cmd:    GetSinkDisputesCmd,
		},
		{
			method: "reporters",
			cmd:    GetSinkReportersCmd,
		},
		{
			method: "account",
			cmd:    GetSinkAccountCmd,
			args:   []string{address},
		},
		{
			method: "state",
			cmd:    GetSinkStateCmd,
		},
		{
			method: "params",
			cmd:    GetSinkParamsCmd,
		},
	}

	for _, tc := range cases {
		t.Run(tc.cmd().Name()+" transport", func(t *testing.T) {
			query := &sinkTestQuery{
				fail: map[string]error{tc.method: errors.New("node unavailable")},
			}
			require.ErrorContains(t, runSemanticQuery(t, tc.cmd(), &sinkTestAggregate{sink: query}, nil, nil, tc.args...), "node unavailable")
		})
		t.Run(tc.cmd().Name()+" malformed", func(t *testing.T) {
			query := &sinkTestQuery{
				empty: map[string]bool{tc.method: true},
			}
			require.ErrorContains(t, runSemanticQuery(t, tc.cmd(), &sinkTestAggregate{sink: query}, nil, nil, tc.args...), "malformed node response")
		})
	}

}

// runSinkAccountQuery runs the account query with a chosen default account.
func runSinkAccountQuery(t *testing.T, query *sinkTestQuery, adjust func(sdkclient.Context) sdkclient.Context) error {
	t.Helper()

	encoding := newModuleTxFixture().cctx
	cctx := adjust(sdkclient.Context{}.WithCodec(encoding.Codec).WithInterfaceRegistry(encoding.InterfaceRegistry).WithOutput(io.Discard))
	cmd := GetSinkAccountCmd()
	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	ctx = context.WithValue(ctx, ContextTypeQueryClient, &semanticQueryLightClient{
		query: &sinkTestAggregate{sink: query},
		cctx:  cctx,
	})
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
		require.Equal(t, &types.QueryAccountRequest{Owner: from.String()}, query.reqs[0])
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
		{
			name: "deposit",
			cmd:  GetTxSinkDepositCmd,
			args: []string{sinkTestDestination(), "1akt"},
		},
		{
			name: "guardian pause",
			cmd:  GetTxSinkGuardianPauseCmd,
			args: []string{"all"},
		},
		{
			name: "report",
			cmd:  GetTxSinkReportOutcomeCmd,
			args: []string{sinkTestIDString(), "minted", "400", sinkTestSignature()},
		},
		{
			name: "veto",
			cmd:  GetTxSinkVetoVoidCmd,
			args: []string{sinkTestIDString(), "mint found"},
		},
		{
			name: "refund",
			cmd:  GetTxSinkFinalizeRefundCmd,
			args: []string{sinkTestIDString()},
		},
		{
			name: "proposal",
			cmd:  GetTxSinkProposeVoidCmd,
			args: []string{sinkTestIDString()},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &moduleTxCapture{err: errors.New("broadcast refused")}
			require.ErrorContains(t, runSinkTx(t, tc.cmd(), &sinkTestQuery{}, capture, tc.args...), "broadcast refused")
		})
		t.Run(tc.name+" bad fee payer", func(t *testing.T) {
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			cmd := tc.cmd()
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagFeePayer, "not-a-fee-payer"))
			require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, tc.args...))
			require.Empty(t, capture.messages)
		})
	}
}

func TestSinkReportOutcomeReadsBeforeSigning(t *testing.T) {
	args := []string{sinkTestIDString(), "minted", "400", sinkTestSignature()}

	for name, query := range map[string]*sinkTestQuery{
		"reporters fails": {
			fail: map[string]error{"reporters": errors.New("unavailable")},
		},
		"reporters empty": {
			empty: map[string]bool{"reporters": true},
		},
		"transfer fails": {
			fail: map[string]error{"transfer": errors.New("transfer unavailable")},
		},
		"transfer empty": {
			empty: map[string]bool{"transfer": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			require.Error(t, runSinkTx(t, GetTxSinkReportOutcomeCmd(), query, capture, args...))
			require.Empty(t, capture.messages)
		})
	}

	t.Run("missing flags", func(t *testing.T) {
		source := GetTxSinkReportOutcomeCmd()
		require.Error(t, runSinkTx(t, sinkPartialCmd(source), &sinkTestQuery{}, &moduleTxCapture{}, args...))

		partial := sinkPartialCmd(source, flagdefs.FlagPolicyVersion)
		require.NoError(t, partial.Flags().Set(flagdefs.FlagPolicyVersion, "1"))
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
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
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
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			require.Error(t, runSinkTx(t, sinkPartialCmd(source, keep...), &sinkTestQuery{}, capture, sinkTestIDString()))
			require.Empty(t, capture.messages)
		}
	})

	t.Run("proposal deposit", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		cmd := GetTxSinkProposeVoidCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "fifty"))
		require.ErrorContains(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, sinkTestIDString()), "invalid proposal deposit")
		require.Empty(t, capture.messages)
	})

	t.Run("proposal without a message", func(t *testing.T) {
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
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
		{
			name: "params bad authority",
			cmd:  GetTxSinkProposeParamsCmd,
			args: []string{"params.json"},
		},
		{
			name: "void bad id",
			cmd:  GetTxSinkProposeVoidCmd,
			args: []string{"0"},
		},
		{
			name: "veto bad id",
			cmd:  GetTxSinkProposeVetoCmd,
			args: []string{"x", "reason"},
		},
		{
			name: "veto blank reason",
			cmd:  GetTxSinkProposeVetoCmd,
			args: []string{sinkTestIDString(), " "},
		},
		{
			name: "veto bad authority",
			cmd:  GetTxSinkProposeVetoCmd,
			args: []string{sinkTestIDString(), "reason"},
		},
		{
			name: "resolve bad id",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{"x", "restart", "reason"},
		},
		{
			name: "resolve blank reason",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "restart", ""},
		},
		{
			name: "resolve bad authority",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "restart", "reason"},
		},
		{
			name: "resolve bad evidence",
			cmd:  GetTxSinkProposeResolveCmd,
			args: []string{sinkTestIDString(), "confirm-void", "reason", "0", sinkTestSignature()},
		},
		{
			name: "report bad id",
			cmd:  GetTxSinkReportOutcomeCmd,
			args: []string{"0", "minted", "400", sinkTestSignature()},
		},
		{
			name: "veto void bad id",
			cmd:  GetTxSinkVetoVoidCmd,
			args: []string{"0", "reason"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			if flag := cmd.Flags().Lookup(flagdefs.FlagAuthority); flag != nil && (tc.name == "params bad authority" || tc.name == "veto bad authority" || tc.name == "resolve bad authority") {
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagAuthority, "not-an-address"))
			}
			capture := &moduleTxCapture{
				response: &sdk.TxResponse{},
			}
			require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, tc.args...))
			require.Empty(t, capture.messages)
		})
	}
}

func TestSinkParserEdgeCases(t *testing.T) {
	_, err := parseSinkObservation(types.OutcomeUnspecified, "400", sinkTestSignature())
	require.Error(t, err)

	deposits, refunds, err := parseSinkPauseTarget("deposits")
	require.NoError(t, err)
	require.True(t, deposits)
	require.False(t, refunds)
}

func TestSinkRoundsAndPoliciesFailBeforeBroadcast(t *testing.T) {
	id := sinkTestID()
	badID := sinkTestState(types.HoldNone, id)
	badID.ID.Sequence++
	zeroRound := sinkTestState(types.HoldNone, id)
	zeroRound.Settlement.Round = 0
	cases := []struct {
		name  string
		query *sinkTestQuery
	}{
		{
			name: "no current policy",
			query: &sinkTestQuery{
				policies: []types.ReporterPolicy{},
			},
		},
		{
			name: "ambiguous current policy",
			query: &sinkTestQuery{
				policies: []types.ReporterPolicy{{Version: 1}, {Version: 2}},
			},
		},
		{
			name: "zero policy version",
			query: &sinkTestQuery{
				policies: []types.ReporterPolicy{{Threshold: math.LegacyOneDec()}},
			},
		},
		{
			name:  "wrong transfer",
			query: &sinkTestQuery{transfer: &badID},
		},
		{
			name:  "zero round",
			query: &sinkTestQuery{transfer: &zeroRound},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &moduleTxCapture{}
			require.Error(t, runSinkTx(t, GetTxSinkReportOutcomeCmd(), tc.query, capture, sinkTestIDString(), "minted", "400", sinkTestSignature()))
			require.Empty(t, capture.messages)
		})
	}
	for _, flag := range []string{flagdefs.FlagPolicyVersion, flagdefs.FlagRound} {
		t.Run("explicit zero "+flag, func(t *testing.T) {
			cmd := GetTxSinkReportOutcomeCmd()
			require.NoError(t, cmd.Flags().Set(flag, "0"))
			capture := &moduleTxCapture{}
			require.Error(t, runSinkTx(t, cmd, &sinkTestQuery{}, capture, sinkTestIDString(), "minted", "400", sinkTestSignature()))
			require.Empty(t, capture.messages)
		})
	}
	for _, constructor := range []func() *cobra.Command{GetTxSinkProposeVoidCmd, GetTxSinkProposeResolveCmd} {
		cmd := constructor()
		args := []string{sinkTestIDString()}
		if cmd.Name() == "propose-resolve" {
			args = append(args, "restart", "new round")
		}
		capture := &moduleTxCapture{
			response: &sdk.TxResponse{},
		}
		query := &sinkTestQuery{round: 9}
		require.NoError(t, runSinkTx(t, cmd, query, capture, args...))
		require.Equal(t, []string{"transfer"}, query.calls)
		if cmd.Name() == "propose-void" {
			_, msg := requireSinkProposal[*types.MsgRequestVoid](t, capture)
			require.Equal(t, uint64(9), msg.ExpectedRound)
		} else {
			_, msg := requireSinkProposal[*types.MsgRestartTransfer](t, capture)
			require.Equal(t, uint64(9), msg.ExpectedRound)
		}
		for _, value := range []string{"0", "18446744073709551615"} {
			if value != "0" && cmd.Name() == "propose-void" {
				continue
			}
			cmd = constructor()
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagExpectedRound, value))
			capture = &moduleTxCapture{}
			query = &sinkTestQuery{}
			require.Error(t, runSinkTx(t, cmd, query, capture, args...))
			require.Empty(t, capture.messages)
			require.Empty(t, query.calls)
		}
		cmd = constructor()
		capture = &moduleTxCapture{}
		require.ErrorContains(t, runSinkTx(t, cmd, &sinkTestQuery{
			fail: map[string]error{"transfer": errors.New("round unavailable")},
		}, capture, args...), "round unavailable")
		require.Empty(t, capture.messages)
	}
}

func TestSinkCompositeIDUsesSDKParsing(t *testing.T) {
	owner := sinkTestID().Owner
	for _, input := range []string{owner + "/42/12", owner + "/042/012"} {
		id, err := parseSinkTransferID(input)
		require.NoError(t, err)
		require.Equal(t, sinkTestID(), id)
	}
	for _, input := range []string{owner + "/42/0", owner + "/-1/12", owner + "/9223372036854775808/12", "bad/42/12", "12"} {
		_, err := parseSinkTransferID(input)
		require.Error(t, err)
	}
}
