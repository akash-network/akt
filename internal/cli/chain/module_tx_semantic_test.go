package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
)

type moduleTxCapture struct {
	response interface{}
	err      error
	messages [][]sdk.Msg
}

func (capture *moduleTxCapture) BroadcastMsgs(
	_ context.Context,
	messages []sdk.Msg,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	capture.messages = append(capture.messages, append([]sdk.Msg(nil), messages...))
	return capture.response, capture.err
}

func (capture *moduleTxCapture) BroadcastTx(
	_ context.Context,
	_ sdk.Tx,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	return capture.response, capture.err
}

type moduleCommandClient struct {
	cctx sdkclient.Context
	tx   clientv1beta3.TxClient
}

func (*moduleCommandClient) Query() clientv1beta3.QueryClient { return nil }
func (*moduleCommandClient) Node() clientv1beta3.NodeClient   { return nil }
func (client *moduleCommandClient) ClientContext() sdkclient.Context {
	return client.cctx
}
func (*moduleCommandClient) PrintMessage(interface{}) error { return nil }
func (*moduleCommandClient) PrintJSON(interface{}) error    { return nil }
func (client *moduleCommandClient) Tx() clientv1beta3.TxClient {
	return client.tx
}

type moduleTxFixture struct {
	cctx     sdkclient.Context
	from     sdk.AccAddress
	provider sdk.AccAddress
	owner    sdk.AccAddress
}

func newModuleTxFixture() moduleTxFixture {
	encoding := aktcodec.MakeEncodingConfig()
	from := sdk.AccAddress(bytes.Repeat([]byte{51}, 20))
	return moduleTxFixture{
		cctx: sdkclient.Context{}.
			WithCodec(encoding.Codec).
			WithLegacyAmino(encoding.Amino).
			WithInterfaceRegistry(encoding.InterfaceRegistry).
			WithTxConfig(encoding.TxConfig).
			WithFrom(from.String()).
			WithFromAddress(from),
		from:     from,
		provider: sdk.AccAddress(bytes.Repeat([]byte{52}, 20)),
		owner:    sdk.AccAddress(bytes.Repeat([]byte{53}, 20)),
	}
}

func runModuleTxHandler(
	t *testing.T,
	fixture moduleTxFixture,
	cmd *cobra.Command,
	capture *moduleTxCapture,
	writer io.Writer,
	args ...string,
) error {
	t.Helper()

	cctx := fixture.cctx.WithOutput(writer)
	clientContext := cctx
	ctx := context.WithValue(context.Background(), ClientContextKey, &clientContext)
	ctx = context.WithValue(ctx, ContextTypeClient, &moduleCommandClient{cctx: cctx, tx: capture})
	cmd.SetContext(ctx)
	cmd.SetOut(writer)
	cmd.SetErr(writer)

	return cmd.RunE(cmd, args)
}

func requireSingleModuleMessage[T sdk.Msg](t *testing.T, capture *moduleTxCapture) T {
	t.Helper()

	require.Len(t, capture.messages, 1)
	require.Len(t, capture.messages[0], 1)
	message, ok := capture.messages[0][0].(T)
	require.Truef(t, ok, "broadcast message type = %T", capture.messages[0][0])

	return message
}

func TestBMECommandsBroadcastExactMessages(t *testing.T) {
	fixture := newModuleTxFixture()

	t.Run("burn mint", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "BURN-MINT-17"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, GetTxBMEBurnMintCmd(), capture, &output, "3000000uakt", "uact")
		require.NoError(t, err)
		require.Contains(t, output.String(), "BURN-MINT-17")

		message := requireSingleModuleMessage[*bmetypes.MsgBurnMint](t, capture)
		require.Equal(t, fixture.from.String(), message.Owner)
		require.Equal(t, fixture.from.String(), message.To)
		require.Equal(t, sdk.NewInt64Coin("uakt", 3_000_000), message.CoinsToBurn)
		require.Equal(t, "uact", message.DenomToMint)
	})

	t.Run("mint act", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "MINT-ACT-18"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, GetTxBMEMintACTCmd(), capture, &output, "1750000uakt")
		require.NoError(t, err)
		require.Contains(t, output.String(), "MINT-ACT-18")

		message := requireSingleModuleMessage[*bmetypes.MsgMintACT](t, capture)
		require.Equal(t, fixture.from.String(), message.Owner)
		require.Equal(t, fixture.from.String(), message.To)
		require.Equal(t, sdk.NewInt64Coin("uakt", 1_750_000), message.CoinsToBurn)
	})

	t.Run("burn act", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "BURN-ACT-19"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, GetTxBMEBurnACTCmd(), capture, &output, "925000uact")
		require.NoError(t, err)
		require.Contains(t, output.String(), "BURN-ACT-19")

		message := requireSingleModuleMessage[*bmetypes.MsgBurnACT](t, capture)
		require.Equal(t, fixture.from.String(), message.Owner)
		require.Equal(t, fixture.from.String(), message.To)
		require.Equal(t, sdk.NewInt64Coin("uact", 925_000), message.CoinsToBurn)
	})
}

func TestEscrowDepositBroadcastsExactMessage(t *testing.T) {
	fixture := newModuleTxFixture()
	cmd := GetTxEscrowDeposit()
	require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
	require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "71"))
	require.NoError(t, cmd.Flags().Set(cflags.FlagDepositSources, "balance,grant"))

	capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "ESCROW-71"}}
	var output bytes.Buffer
	err := runModuleTxHandler(t, fixture, cmd, capture, &output, "deployment", "2500000uakt")
	require.NoError(t, err)
	require.Contains(t, output.String(), "ESCROW-71")

	message := requireSingleModuleMessage[*ev1.MsgAccountDeposit](t, capture)
	require.Equal(t, dv1.DeploymentID{Owner: fixture.owner.String(), DSeq: 71}.ToEscrowAccountID(), message.ID)
	require.Equal(t, fixture.from.String(), message.Signer)
	require.Equal(t, sdk.NewInt64Coin("uakt", 2_500_000), message.Deposit.Amount)
	require.Equal(t, deposit.Sources{deposit.SourceBalance, deposit.SourceGrant}, message.Deposit.Sources)
}

func TestMarketCommandsBroadcastExactMessages(t *testing.T) {
	fixture := newModuleTxFixture()

	t.Run("provider creates bid", func(t *testing.T) {
		cmd := GetTxMarketBidCreateCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "0.025uakt"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
		require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "80"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "6"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOSeq, "8"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagDeposit, "500000uakt"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagDepositSources, "grant,balance"))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "BID-CREATE-80"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output)
		require.NoError(t, err)
		require.Contains(t, output.String(), "BID-CREATE-80")

		price, err := sdk.ParseDecCoin("0.025uakt")
		require.NoError(t, err)
		message := requireSingleModuleMessage[*mtypes.MsgCreateBid](t, capture)
		require.Equal(t, mv1.BidID{
			Owner: fixture.owner.String(), DSeq: 80, GSeq: 6, OSeq: 8,
			Provider: fixture.from.String(),
		}, message.ID)
		require.Equal(t, price, message.Price)
		require.Equal(t, sdk.NewInt64Coin("uakt", 500_000), message.Deposit.Amount)
		require.Equal(t, deposit.Sources{deposit.SourceGrant, deposit.SourceBalance}, message.Deposit.Sources)
	})

	t.Run("provider closes bid", func(t *testing.T) {
		cmd := GetTxMarketBidCloseCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
		require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "81"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "7"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOSeq, "9"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagClosedReason, "10001"))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "BID-CLOSE-81"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output)
		require.NoError(t, err)
		require.Contains(t, output.String(), "BID-CLOSE-81")

		message := requireSingleModuleMessage[*mtypes.MsgCloseBid](t, capture)
		require.Equal(t, mv1.BidID{
			Owner: fixture.owner.String(), DSeq: 81, GSeq: 7, OSeq: 9,
			Provider: fixture.from.String(),
		}, message.ID)
		require.Equal(t, mv1.LeaseClosedReasonDecommissioned, message.Reason)
	})

	t.Run("owner creates lease", func(t *testing.T) {
		cmd := GetTxMarketLeaseCreateCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
		require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "11"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOSeq, "13"))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "LEASE-CREATE-91"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output, "91", fixture.provider.String())
		require.NoError(t, err)
		require.Contains(t, output.String(), "LEASE-CREATE-91")

		message := requireSingleModuleMessage[*mtypes.MsgCreateLease](t, capture)
		require.Equal(t, mv1.BidID{
			Owner: fixture.owner.String(), DSeq: 91, GSeq: 11, OSeq: 13,
			Provider: fixture.provider.String(),
		}, message.BidID)
	})

	t.Run("owner withdraws lease", func(t *testing.T) {
		cmd := GetTxMarketLeaseWithdrawCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "5"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOSeq, "6"))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "LEASE-WITHDRAW-92"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output, "92", fixture.provider.String())
		require.NoError(t, err)
		require.Contains(t, output.String(), "LEASE-WITHDRAW-92")

		message := requireSingleModuleMessage[*mtypes.MsgWithdrawLease](t, capture)
		require.Equal(t, mv1.LeaseID{
			Owner: fixture.from.String(), DSeq: 92, GSeq: 5, OSeq: 6,
			Provider: fixture.provider.String(),
		}, message.ID)
	})

	t.Run("owner closes lease", func(t *testing.T) {
		cmd := GetTxMarketLeaseCloseCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
		require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "3"))
		require.NoError(t, cmd.Flags().Set(cflags.FlagOSeq, "4"))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "LEASE-CLOSE-93"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output, "93", fixture.provider.String())
		require.NoError(t, err)
		require.Contains(t, output.String(), "LEASE-CLOSE-93")

		message := requireSingleModuleMessage[*mtypes.MsgCloseLease](t, capture)
		require.Equal(t, mv1.LeaseID{
			Owner: fixture.owner.String(), DSeq: 93, GSeq: 3, OSeq: 4,
			Provider: fixture.provider.String(),
		}, message.ID)
		require.Equal(t, mv1.LeaseClosedReasonOwner, message.Reason)
	})
}

func TestDeploymentGroupCommandsBroadcastExactMessages(t *testing.T) {
	fixture := newModuleTxFixture()

	t.Run("close", func(t *testing.T) {
		cmd := GetTxDeploymentGroupCloseCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "GROUP-CLOSE-101"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output, "101", "7")
		require.NoError(t, err)
		require.Contains(t, output.String(), "GROUP-CLOSE-101")

		message := requireSingleModuleMessage[*dv1beta.MsgCloseGroup](t, capture)
		require.Equal(t, dv1.GroupID{Owner: fixture.owner.String(), DSeq: 101, GSeq: 7}, message.ID)
	})

	t.Run("pause", func(t *testing.T) {
		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "GROUP-PAUSE-102"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, GetDeploymentGroupPauseCmd(), capture, &output, "102", "8")
		require.NoError(t, err)
		require.Contains(t, output.String(), "GROUP-PAUSE-102")

		message := requireSingleModuleMessage[*dv1beta.MsgPauseGroup](t, capture)
		require.Equal(t, dv1.GroupID{Owner: fixture.from.String(), DSeq: 102, GSeq: 8}, message.ID)
	})

	t.Run("start", func(t *testing.T) {
		cmd := GetDeploymentGroupStartCmd()
		require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))

		capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "GROUP-START-103"}}
		var output bytes.Buffer
		err := runModuleTxHandler(t, fixture, cmd, capture, &output, "103", "9")
		require.NoError(t, err)
		require.Contains(t, output.String(), "GROUP-START-103")

		message := requireSingleModuleMessage[*dv1beta.MsgStartGroup](t, capture)
		require.Equal(t, dv1.GroupID{Owner: fixture.owner.String(), DSeq: 103, GSeq: 9}, message.ID)
	})
}

type validModuleTxCase struct {
	name    string
	command *cobra.Command
	args    []string
}

func validModuleTxCases(t *testing.T, fixture moduleTxFixture) []validModuleTxCase {
	t.Helper()

	escrow := GetTxEscrowDeposit()
	require.NoError(t, escrow.Flags().Set(cflags.FlagDSeq, "201"))

	bidClose := GetTxMarketBidCloseCmd()
	require.NoError(t, bidClose.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
	require.NoError(t, bidClose.Flags().Set(cflags.FlagDSeq, "202"))
	require.NoError(t, bidClose.Flags().Set(cflags.FlagGSeq, "2"))
	require.NoError(t, bidClose.Flags().Set(cflags.FlagOSeq, "3"))

	bidCreate := GetTxMarketBidCreateCmd()
	require.NoError(t, bidCreate.Flags().Set(cflags.FlagPrice, "0.03uakt"))
	require.NoError(t, bidCreate.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
	require.NoError(t, bidCreate.Flags().Set(cflags.FlagDSeq, "209"))
	require.NoError(t, bidCreate.Flags().Set(cflags.FlagDeposit, "5uakt"))

	return []validModuleTxCase{
		{name: "bme burn mint", command: GetTxBMEBurnMintCmd(), args: []string{"1uakt", "uact"}},
		{name: "bme mint act", command: GetTxBMEMintACTCmd(), args: []string{"2uakt"}},
		{name: "bme burn act", command: GetTxBMEBurnACTCmd(), args: []string{"3uact"}},
		{name: "escrow deposit", command: escrow, args: []string{"deployment", "4uakt"}},
		{name: "market bid create", command: bidCreate},
		{name: "market bid close", command: bidClose},
		{name: "market lease create", command: GetTxMarketLeaseCreateCmd(), args: []string{"203", fixture.provider.String()}},
		{name: "market lease withdraw", command: GetTxMarketLeaseWithdrawCmd(), args: []string{"204", fixture.provider.String()}},
		{name: "market lease close", command: GetTxMarketLeaseCloseCmd(), args: []string{"205", fixture.provider.String()}},
		{name: "deployment group close", command: GetTxDeploymentGroupCloseCmd(), args: []string{"206", "4"}},
		{name: "deployment group pause", command: GetDeploymentGroupPauseCmd(), args: []string{"207", "5"}},
		{name: "deployment group start", command: GetDeploymentGroupStartCmd(), args: []string{"208", "6"}},
	}
}

func TestModuleTransactionsPreserveBroadcastErrors(t *testing.T) {
	fixture := newModuleTxFixture()
	broadcastErr := errors.New("semantic transaction broadcast failed")

	for _, test := range validModuleTxCases(t, fixture) {
		t.Run(test.name, func(t *testing.T) {
			capture := &moduleTxCapture{err: broadcastErr}
			var output bytes.Buffer
			err := runModuleTxHandler(t, fixture, test.command, capture, &output, test.args...)
			require.ErrorIs(t, err, broadcastErr)
			require.Len(t, capture.messages, 1)
			require.Len(t, capture.messages[0], 1)
			require.Empty(t, output.String())
		})
	}
}

func TestModuleTransactionsPreserveOutputErrors(t *testing.T) {
	fixture := newModuleTxFixture()
	outputErr := errors.New("semantic transaction output failed")

	for _, test := range validModuleTxCases(t, fixture) {
		t.Run(test.name, func(t *testing.T) {
			capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "BROADCAST-SUCCEEDED"}}
			err := runModuleTxHandler(t, fixture, test.command, capture, chainTestErrorWriter{err: outputErr}, test.args...)
			require.ErrorIs(t, err, outputErr)
			require.Len(t, capture.messages, 1)
			require.Len(t, capture.messages[0], 1)
		})
	}
}

func TestMarketTransactionsRejectInvalidInputBeforeBroadcast(t *testing.T) {
	fixture := newModuleTxFixture()

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{
			name:    "bid create malformed price",
			command: GetTxMarketBidCreateCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "not-a-price"))
			},
		},
		{
			name:    "bid create zero deployment",
			command: GetTxMarketBidCreateCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "0.02uakt"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDeposit, "1uakt"))
			},
		},
		{
			name:    "bid create malformed deposit",
			command: GetTxMarketBidCreateCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "0.02uakt"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "300"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDeposit, "not-a-deposit"))
			},
		},
		{
			name:    "bid create duplicate source",
			command: GetTxMarketBidCreateCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "0.02uakt"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "300"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDeposit, "1uakt"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDepositSources, "grant,grant"))
			},
		},
		{
			name:    "bid create zero price",
			command: GetTxMarketBidCreateCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagPrice, "0uakt"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "300"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDeposit, "1uakt"))
			},
		},
		{name: "lease create missing identity", command: GetTxMarketLeaseCreateCmd},
		{name: "lease withdraw missing provider", command: GetTxMarketLeaseWithdrawCmd, args: []string{"301"}},
		{name: "lease close malformed dseq", command: GetTxMarketLeaseCloseCmd, args: []string{"not-a-dseq", fixture.provider.String()}},
		{name: "lease create malformed provider", command: GetTxMarketLeaseCreateCmd, args: []string{"302", "not-a-provider"}},
		{name: "lease create self bid", command: GetTxMarketLeaseCreateCmd, args: []string{"302", fixture.from.String()}},
		{name: "lease withdraw self bid", command: GetTxMarketLeaseWithdrawCmd, args: []string{"302", fixture.from.String()}},
		{
			name:    "lease withdraw malformed owner",
			command: GetTxMarketLeaseWithdrawCmd,
			args:    []string{"303", fixture.provider.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, "not-an-owner"))
			},
		},
		{
			name:    "lease close zero group sequence",
			command: GetTxMarketLeaseCloseCmd,
			args:    []string{"304", fixture.provider.String()},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagGSeq, "0"))
			},
		},
		{
			name:    "bid close zero deployment",
			command: GetTxMarketBidCloseCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
			},
		},
		{
			name:    "bid close owner",
			command: GetTxMarketBidCloseCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, "not-an-owner"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "305"))
			},
		},
		{
			name:    "bid close reason",
			command: GetTxMarketBidCloseCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, fixture.owner.String()))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "306"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagClosedReason, "9999"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "MUST-NOT-BROADCAST"}}
			var output bytes.Buffer
			err := runModuleTxHandler(t, fixture, cmd, capture, &output, test.args...)
			require.Error(t, err)
			require.Empty(t, capture.messages)
			require.Empty(t, output.String())
		})
	}
}

func TestDeploymentGroupTransactionsRejectInvalidInputBeforeBroadcast(t *testing.T) {
	fixture := newModuleTxFixture()

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{name: "close missing deployment", command: GetTxDeploymentGroupCloseCmd},
		{name: "pause malformed deployment", command: GetDeploymentGroupPauseCmd, args: []string{"not-a-dseq", "1"}},
		{name: "start malformed group", command: GetDeploymentGroupStartCmd, args: []string{"401", "not-a-gseq"}},
		{name: "close zero group", command: GetTxDeploymentGroupCloseCmd, args: []string{"402", "0"}},
		{name: "pause zero group", command: GetDeploymentGroupPauseCmd, args: []string{"402", "0"}},
		{name: "start zero group", command: GetDeploymentGroupStartCmd, args: []string{"402", "0"}},
		{
			name:    "pause malformed owner",
			command: GetDeploymentGroupPauseCmd,
			args:    []string{"403", "1"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, "not-an-owner"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "MUST-NOT-BROADCAST"}}
			var output bytes.Buffer
			err := runModuleTxHandler(t, fixture, cmd, capture, &output, test.args...)
			require.Error(t, err)
			require.Empty(t, capture.messages)
			require.Empty(t, output.String())
		})
	}
}

func TestBMEAndEscrowRejectInvalidMessagesBeforeBroadcast(t *testing.T) {
	fixture := newModuleTxFixture()

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{name: "burn mint zero amount", command: GetTxBMEBurnMintCmd, args: []string{"0uakt", "uact"}},
		{name: "burn mint malformed coin", command: GetTxBMEBurnMintCmd, args: []string{"not-a-coin", "uact"}},
		{name: "burn mint malformed destination denom", command: GetTxBMEBurnMintCmd, args: []string{"1uakt", "not a denom"}},
		{
			name:    "burn mint malformed fee payer",
			command: GetTxBMEBurnMintCmd,
			args:    []string{"1uakt", "uact"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagFeePayer, "not-a-fee-payer"))
			},
		},
		{name: "mint act zero amount", command: GetTxBMEMintACTCmd, args: []string{"0uakt"}},
		{name: "mint act malformed coin", command: GetTxBMEMintACTCmd, args: []string{"not-a-coin"}},
		{
			name:    "mint act malformed fee payer",
			command: GetTxBMEMintACTCmd,
			args:    []string{"1uakt"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagFeePayer, "not-a-fee-payer"))
			},
		},
		{name: "burn act zero amount", command: GetTxBMEBurnACTCmd, args: []string{"0uact"}},
		{name: "burn act malformed coin", command: GetTxBMEBurnACTCmd, args: []string{"not-a-coin"}},
		{
			name:    "burn act malformed fee payer",
			command: GetTxBMEBurnACTCmd,
			args:    []string{"1uact"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagFeePayer, "not-a-fee-payer"))
			},
		},
		{
			name:    "escrow invalid scope",
			command: GetTxEscrowDeposit,
			args:    []string{"lease", "1uakt"},
		},
		{
			name:    "escrow zero deployment",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "1uakt"},
		},
		{
			name:    "escrow zero amount",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "0uakt"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set("dseq", "71"))
			},
		},
		{
			name:    "escrow malformed amount",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "not-a-coin"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "72"))
			},
		},
		{
			name:    "escrow malformed owner",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "1uakt"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagOwner, "not-an-owner"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "73"))
			},
		},
		{
			name:    "escrow invalid deposit source",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "1uakt"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "74"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDepositSources, "wallet"))
			},
		},
		{
			name:    "escrow duplicate deposit source",
			command: GetTxEscrowDeposit,
			args:    []string{"deployment", "1uakt"},
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(cflags.FlagDSeq, "75"))
				require.NoError(t, cmd.Flags().Set(cflags.FlagDepositSources, "grant,grant"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			capture := &moduleTxCapture{response: &sdk.TxResponse{TxHash: "MUST-NOT-BROADCAST"}}
			var output bytes.Buffer
			err := runModuleTxHandler(t, fixture, cmd, capture, &output, test.args...)
			require.Error(t, err)
			require.Empty(t, capture.messages)
			require.Empty(t, output.String())
		})
	}
}
