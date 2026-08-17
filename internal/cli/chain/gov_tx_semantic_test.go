package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	wtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type govCaptureTxClient struct {
	response interface{}
	err      error
	messages [][]sdk.Msg
}

func (client *govCaptureTxClient) BroadcastMsgs(
	_ context.Context,
	messages []sdk.Msg,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	client.messages = append(client.messages, append([]sdk.Msg(nil), messages...))
	return client.response, client.err
}

func (client *govCaptureTxClient) BroadcastTx(
	_ context.Context,
	_ sdk.Tx,
	_ ...clientv1beta3.BroadcastOption,
) (interface{}, error) {
	return client.response, client.err
}

type govCommandClient struct {
	cctx sdkclient.Context
	tx   clientv1beta3.TxClient
}

func (*govCommandClient) Query() clientv1beta3.QueryClient { return nil }
func (*govCommandClient) Node() clientv1beta3.NodeClient   { return nil }

func (client *govCommandClient) ClientContext() sdkclient.Context { return client.cctx }
func (*govCommandClient) PrintMessage(interface{}) error          { return nil }
func (*govCommandClient) PrintJSON(interface{}) error             { return nil }
func (client *govCommandClient) Tx() clientv1beta3.TxClient       { return client.tx }

func govTestAddresses() (sdk.AccAddress, sdk.AccAddress) {
	return sdk.AccAddress(bytes.Repeat([]byte{1}, 20)), sdk.AccAddress(bytes.Repeat([]byte{2}, 20))
}

func runGovTxHandler(
	t *testing.T,
	cmd *cobra.Command,
	txClient *govCaptureTxClient,
	args ...string,
) (string, error) {
	t.Helper()

	from, _ := govTestAddresses()
	encoding := aktcodec.MakeEncodingConfig()
	cctx := sdkclient.Context{}.
		WithCodec(encoding.Codec).
		WithLegacyAmino(encoding.Amino).
		WithInterfaceRegistry(encoding.InterfaceRegistry).
		WithTxConfig(encoding.TxConfig).
		WithFromAddress(from)

	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetContext(context.WithValue(
		context.Background(),
		ContextTypeClient,
		&govCommandClient{cctx: cctx, tx: txClient},
	))

	err := cmd.RunE(cmd, args)
	return output.String(), err
}

func writeGovProposalInput(t *testing.T, value interface{}) string {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "proposal.json")
	require.NoError(t, os.WriteFile(path, payload, 0o600))
	return path
}

func validGovProposalInput(t *testing.T) (string, sdk.AccAddress, sdk.AccAddress) {
	t.Helper()

	from, recipient := govTestAddresses()
	message := json.RawMessage(`{
		"@type":"/cosmos.bank.v1beta1.MsgSend",
		"from_address":"` + from.String() + `",
		"to_address":"` + recipient.String() + `",
		"amount":[{"denom":"uakt","amount":"17"}]
	}`)
	proposal := ProposalMsg{
		Messages:  []json.RawMessage{message},
		Metadata:  "ipfs://proposal-metadata",
		Deposit:   "5uakt,2uact",
		Title:     "Fund a deterministic recipient",
		Summary:   "Transfer a fixed amount if governance approves",
		Expedited: true,
	}

	return writeGovProposalInput(t, proposal), from, recipient
}

func requireSingleGovMessage[T sdk.Msg](t *testing.T, client *govCaptureTxClient) T {
	t.Helper()
	require.Len(t, client.messages, 1)
	require.Len(t, client.messages[0], 1)

	message, ok := client.messages[0][0].(T)
	require.Truef(t, ok, "broadcast message has type %T", client.messages[0][0])
	return message
}

type govGeneratedProposalFixture struct {
	cctx      sdkclient.Context
	proposer  sdk.AccAddress
	authority sdk.AccAddress
	contract  sdk.AccAddress
	admin     sdk.AccAddress
}

func newGovGeneratedProposalFixture(t *testing.T) govGeneratedProposalFixture {
	t.Helper()

	encoding := aktcodec.MakeEncodingConfig()
	proposer := sdk.AccAddress(bytes.Repeat([]byte{41}, 20))
	return govGeneratedProposalFixture{
		cctx: sdkclient.Context{}.
			WithCodec(encoding.Codec).
			WithLegacyAmino(encoding.Amino).
			WithInterfaceRegistry(encoding.InterfaceRegistry).
			WithTxConfig(encoding.TxConfig).
			WithFromAddress(proposer).
			WithHomeDir(t.TempDir()),
		proposer:  proposer,
		authority: sdk.AccAddress(bytes.Repeat([]byte{42}, 20)),
		contract:  sdk.AccAddress(bytes.Repeat([]byte{43}, 20)),
		admin:     sdk.AccAddress(bytes.Repeat([]byte{44}, 20)),
	}
}

func (fixture govGeneratedProposalFixture) generateProposal(
	t *testing.T,
	cmd *cobra.Command,
	args ...string,
) (*govv1.MsgSubmitProposal, sdk.Msg) {
	t.Helper()

	callArgs := append([]string{}, args...)
	callArgs = append(callArgs,
		fmt.Sprintf("--%s=%s", flagdefs.FlagFrom, fixture.proposer.String()),
		fmt.Sprintf("--%s=%s", flagdefs.FlagAuthority, fixture.authority.String()),
		fmt.Sprintf("--%s=%s", flagdefs.FlagTitle, "Deterministic Wasm proposal"),
		fmt.Sprintf("--%s=%s", flagdefs.FlagSummary, "Exercise the exact nested governance message"),
		fmt.Sprintf("--%s=%s", flagdefs.FlagDeposit, "13uakt,2uact"),
		fmt.Sprintf("--%s=true", flagdefs.FlagExpedite),
		fmt.Sprintf("--%s=true", flagdefs.FlagGenerateOnly),
		fmt.Sprintf("--%s=true", flagdefs.FlagOffline),
		fmt.Sprintf("--%s=7", flagdefs.FlagAccountNumber),
		fmt.Sprintf("--%s=11", flagdefs.FlagSequence),
		fmt.Sprintf("--%s=200000", flagdefs.FlagGas),
		fmt.Sprintf("--%s=%s", flagdefs.FlagOutput, cflags.OutputJSON),
	)

	var output bytes.Buffer
	cctx := fixture.cctx.WithOutput(&output)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(callArgs)
	ctx := context.WithValue(context.Background(), ClientContextKey, &sdkclient.Context{})
	cmd.SetContext(ctx)
	cctx.CmdContext = ctx
	require.NoError(t, SetCmdClientContextHandler(cctx, cmd))

	err := cmd.Execute()
	require.NoError(t, err, "command output:\n%s", output.String())
	tx, err := fixture.cctx.TxConfig.TxJSONDecoder()(bytes.TrimSpace(output.Bytes()))
	require.NoError(t, err, "decode generated transaction:\n%s", output.String())
	require.Len(t, tx.GetMsgs(), 1)

	proposal, ok := tx.GetMsgs()[0].(*govv1.MsgSubmitProposal)
	require.Truef(t, ok, "message type = %T", tx.GetMsgs()[0])
	require.Equal(t, fixture.proposer.String(), proposal.Proposer)
	require.Equal(t, "Deterministic Wasm proposal", proposal.Title)
	require.Equal(t, "Exercise the exact nested governance message", proposal.Summary)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uact", 2), sdk.NewInt64Coin("uakt", 13)), proposal.InitialDeposit)
	require.True(t, proposal.Expedited)
	require.NotEmpty(t, proposal.Messages)

	var nested sdk.Msg
	require.NoError(t, fixture.cctx.InterfaceRegistry.UnpackAny(proposal.Messages[0], &nested))
	return proposal, nested
}

func TestGovWasmProposalCommandsGenerateExactNestedMessages(t *testing.T) {
	fixture := newGovGeneratedProposalFixture(t)
	wasmPath := filepath.Join(t.TempDir(), "canonical-flags.wasm")
	require.NoError(t, os.WriteFile(wasmPath, []byte("\x00asm\x01\x00\x00\x00"), 0o600))

	t.Run("store code", func(t *testing.T) {
		_, nested := fixture.generateProposal(t, GetTxGovWasmProposalStoreCodeCmd(), wasmPath)
		message, ok := nested.(*wtypes.MsgStoreCode)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Sender)
	})

	t.Run("instantiate", func(t *testing.T) {
		cmd := GetTxGovWasmProposalInstantiateContractCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagLabel, "canonical-instantiate"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagNoAdmin, "true"))
		_, nested := fixture.generateProposal(t, cmd, "17", `{"count":1}`)
		_, ok := nested.(*wtypes.MsgInstantiateContract)
		require.Truef(t, ok, "nested message type = %T", nested)
	})

	t.Run("instantiate2", func(t *testing.T) {
		cmd := GetTxGovWasmProposalInstantiateContract2Cmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagLabel, "canonical-instantiate2"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagNoAdmin, "true"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagFixMsg, "true"))
		_, nested := fixture.generateProposal(t, cmd, "19", `{"count":2}`, "74657374")
		message, ok := nested.(*wtypes.MsgInstantiateContract2)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.True(t, message.FixMsg)
	})

	t.Run("store and instantiate", func(t *testing.T) {
		cmd := GetTxGovWasmProposalStoreAndInstantiateContractCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagLabel, "canonical-store-instantiate"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagNoAdmin, "true"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagUnpinCode, "true"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagAmount, "31uakt"))
		_, nested := fixture.generateProposal(t, cmd, wasmPath, `{"count":3}`)
		message, ok := nested.(*wtypes.MsgStoreAndInstantiateContract)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.True(t, message.UnpinCode)
	})

	t.Run("store and migrate", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalStoreAndMigrateContractCmd(),
			wasmPath,
			fixture.contract.String(),
			`{"revision":4}`,
		)
		_, ok := nested.(*wtypes.MsgStoreAndMigrateContract)
		require.Truef(t, ok, "nested message type = %T", nested)
	})

	t.Run("execute", func(t *testing.T) {
		cmd := GetTxGovWasmProposalExecuteContractCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagAmount, "19uakt,3uact"))
		_, nested := fixture.generateProposal(
			t,
			cmd,
			fixture.contract.String(),
			`{"increment":{"by":7}}`,
		)

		message, ok := nested.(*wtypes.MsgExecuteContract)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Sender)
		require.Equal(t, fixture.contract.String(), message.Contract)
		require.JSONEq(t, `{"increment":{"by":7}}`, string(message.Msg))
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uact", 3), sdk.NewInt64Coin("uakt", 19)), sdk.Coins(message.Funds))
	})

	t.Run("sudo", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalSudoContractCmd(),
			fixture.contract.String(),
			`{"promote":{"address":"`+fixture.admin.String()+`"}}`,
		)

		message, ok := nested.(*wtypes.MsgSudoContract)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Authority)
		require.Equal(t, fixture.contract.String(), message.Contract)
		require.JSONEq(t, `{"promote":{"address":"`+fixture.admin.String()+`"}}`, string(message.Msg))
	})

	t.Run("migrate", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalMigrateContractCmd(),
			fixture.contract.String(),
			"77",
			`{"migrate":{"revision":2}}`,
		)

		message, ok := nested.(*wtypes.MsgMigrateContract)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Sender)
		require.Equal(t, fixture.contract.String(), message.Contract)
		require.Equal(t, uint64(77), message.CodeID)
		require.JSONEq(t, `{"migrate":{"revision":2}}`, string(message.Msg))
	})

	t.Run("set admin", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalUpdateContractAdminCmd(),
			fixture.contract.String(),
			fixture.admin.String(),
		)

		message, ok := nested.(*wtypes.MsgUpdateAdmin)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Sender)
		require.Equal(t, fixture.contract.String(), message.Contract)
		require.Equal(t, fixture.admin.String(), message.NewAdmin)
	})

	t.Run("clear admin", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalClearContractAdminCmd(),
			fixture.contract.String(),
		)

		message, ok := nested.(*wtypes.MsgClearAdmin)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Sender)
		require.Equal(t, fixture.contract.String(), message.Contract)
	})

	t.Run("pin codes", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalPinCodesCmd(),
			"3",
			"89",
		)

		message, ok := nested.(*wtypes.MsgPinCodes)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Authority)
		require.Equal(t, []uint64{3, 89}, message.CodeIDs)
	})

	t.Run("unpin codes", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalUnpinCodesCmd(),
			"5",
			"144",
		)

		message, ok := nested.(*wtypes.MsgUnpinCodes)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Authority)
		require.Equal(t, []uint64{5, 144}, message.CodeIDs)
	})

	t.Run("update instantiate config", func(t *testing.T) {
		proposal, _ := fixture.generateProposal(
			t,
			GetTxGovWasmProposalUpdateInstantiateConfigCmd(),
			"8:nobody",
			"13:everybody",
			"21:"+fixture.admin.String(),
		)
		require.Len(t, proposal.Messages, 3)

		var messages []*wtypes.MsgUpdateInstantiateConfig
		for _, packed := range proposal.Messages {
			var nested sdk.Msg
			require.NoError(t, fixture.cctx.InterfaceRegistry.UnpackAny(packed, &nested))
			message, ok := nested.(*wtypes.MsgUpdateInstantiateConfig)
			require.Truef(t, ok, "nested message type = %T", nested)
			messages = append(messages, message)
		}
		require.Equal(t, fixture.authority.String(), messages[0].Sender)
		require.Equal(t, uint64(8), messages[0].CodeID)
		require.Equal(t, wtypes.AccessTypeNobody, messages[0].NewInstantiatePermission.Permission)
		require.Equal(t, uint64(13), messages[1].CodeID)
		require.Equal(t, wtypes.AccessTypeEverybody, messages[1].NewInstantiatePermission.Permission)
		require.Equal(t, uint64(21), messages[2].CodeID)
		require.Equal(t, []string{fixture.admin.String()}, messages[2].NewInstantiatePermission.Addresses)
	})

	t.Run("add upload addresses", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalAddCodeUploadParamsAddresses(),
			fixture.proposer.String(),
			fixture.admin.String(),
		)

		message, ok := nested.(*wtypes.MsgAddCodeUploadParamsAddresses)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Authority)
		require.Equal(t, []string{fixture.proposer.String(), fixture.admin.String()}, message.Addresses)
	})

	t.Run("remove upload addresses", func(t *testing.T) {
		_, nested := fixture.generateProposal(
			t,
			GetTxGovWasmProposalRemoveCodeUploadParamsAddresses(),
			fixture.admin.String(),
		)

		message, ok := nested.(*wtypes.MsgRemoveCodeUploadParamsAddresses)
		require.Truef(t, ok, "nested message type = %T", nested)
		require.Equal(t, fixture.authority.String(), message.Authority)
		require.Equal(t, []string{fixture.admin.String()}, message.Addresses)
	})
}

func TestGovSubmitProposalParsesAndBroadcastsExactMessage(t *testing.T) {
	path, from, recipient := validGovProposalInput(t)
	encoding := aktcodec.MakeEncodingConfig()

	proposal, messages, deposit, err := parseSubmitProposal(encoding.Codec, path)
	require.NoError(t, err)
	require.Equal(t, "ipfs://proposal-metadata", proposal.Metadata)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uact", 2), sdk.NewInt64Coin("uakt", 5)), deposit)
	require.Len(t, messages, 1)
	send, ok := messages[0].(*banktypes.MsgSend)
	require.Truef(t, ok, "parsed proposal message has type %T", messages[0])
	require.Equal(t, from.String(), send.FromAddress)
	require.Equal(t, recipient.String(), send.ToAddress)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 17)), send.Amount)

	txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "SUBMIT-PROPOSAL"}}
	output, err := runGovTxHandler(t, GetTxGovSubmitProposalCmd(), txClient, path)
	require.NoError(t, err)
	require.Contains(t, output, "SUBMIT-PROPOSAL")

	submitted := requireSingleGovMessage[*govv1.MsgSubmitProposal](t, txClient)
	require.Equal(t, from.String(), submitted.Proposer)
	require.Equal(t, proposal.Metadata, submitted.Metadata)
	require.Equal(t, proposal.Title, submitted.Title)
	require.Equal(t, proposal.Summary, submitted.Summary)
	require.True(t, submitted.Expedited)
	require.Equal(t, deposit, submitted.InitialDeposit)
	require.Len(t, submitted.Messages, 1)

	var unpacked sdk.Msg
	require.NoError(t, encoding.InterfaceRegistry.UnpackAny(submitted.Messages[0], &unpacked))
	require.Equal(t, send, unpacked)
}

func TestGovLegacyProposalFlagsBuildTextProposal(t *testing.T) {
	from, _ := govTestAddresses()
	cmd := GetTxGovSubmitLegacyProposalCmd()
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "Legacy title"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagDescription, "Legacy description"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagType, "text"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "11uakt"))

	txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "LEGACY-PROPOSAL"}}
	_, err := runGovTxHandler(t, cmd, txClient)
	require.NoError(t, err)

	submitted := requireSingleGovMessage[*govv1beta1.MsgSubmitProposal](t, txClient)
	require.Equal(t, from.String(), submitted.Proposer)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 11)), submitted.InitialDeposit)

	encoding := aktcodec.MakeEncodingConfig()
	var content govv1beta1.Content
	require.NoError(t, encoding.InterfaceRegistry.UnpackAny(submitted.Content, &content))
	text, ok := content.(*govv1beta1.TextProposal)
	require.Truef(t, ok, "legacy proposal content has type %T", content)
	require.Equal(t, "Legacy title", text.Title)
	require.Equal(t, "Legacy description", text.Description)
}

func TestGovDepositVoteWeightedVoteAndCancelMessages(t *testing.T) {
	from, _ := govTestAddresses()

	t.Run("deposit", func(t *testing.T) {
		txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "DEPOSIT"}}
		_, err := runGovTxHandler(t, GetTxGovDepositCmd(), txClient, "42", "7uakt,3uact")
		require.NoError(t, err)

		message := requireSingleGovMessage[*govv1.MsgDeposit](t, txClient)
		require.Equal(t, uint64(42), message.ProposalId)
		require.Equal(t, from.String(), message.Depositor)
		require.Equal(
			t,
			sdk.NewCoins(sdk.NewInt64Coin("uact", 3), sdk.NewInt64Coin("uakt", 7)),
			sdk.Coins(message.Amount),
		)
	})

	t.Run("vote", func(t *testing.T) {
		cmd := GetTxGovVoteCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagMetadata, "ipfs://vote-reason"))
		txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "VOTE"}}
		_, err := runGovTxHandler(t, cmd, txClient, "43", "no_with_veto")
		require.NoError(t, err)

		message := requireSingleGovMessage[*govv1.MsgVote](t, txClient)
		require.Equal(t, uint64(43), message.ProposalId)
		require.Equal(t, from.String(), message.Voter)
		require.Equal(t, govv1.OptionNoWithVeto, message.Option)
		require.Equal(t, "ipfs://vote-reason", message.Metadata)
	})

	t.Run("weighted vote", func(t *testing.T) {
		cmd := GetTxGovWeightedVoteCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagMetadata, "weighted rationale"))
		txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "WEIGHTED-VOTE"}}
		_, err := runGovTxHandler(t, cmd, txClient, "44", "yes=0.60,no=0.25,abstain=0.10,no_with_veto=0.05")
		require.NoError(t, err)

		message := requireSingleGovMessage[*govv1.MsgVoteWeighted](t, txClient)
		require.Equal(t, uint64(44), message.ProposalId)
		require.Equal(t, from.String(), message.Voter)
		require.Equal(t, "weighted rationale", message.Metadata)
		require.Equal(t, []*govv1.WeightedVoteOption{
			{Option: govv1.OptionYes, Weight: "0.600000000000000000"},
			{Option: govv1.OptionNo, Weight: "0.250000000000000000"},
			{Option: govv1.OptionAbstain, Weight: "0.100000000000000000"},
			{Option: govv1.OptionNoWithVeto, Weight: "0.050000000000000000"},
		}, message.Options)
	})

	t.Run("cancel", func(t *testing.T) {
		txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "CANCEL"}}
		_, err := runGovTxHandler(t, GetTxGovCancelProposalCmd(), txClient, "45")
		require.NoError(t, err)

		message := requireSingleGovMessage[*govv1.MsgCancelProposal](t, txClient)
		require.Equal(t, uint64(45), message.ProposalId)
		require.Equal(t, from.String(), message.Proposer)
	})
}

func TestGovTransactionBoundariesDoNotBroadcast(t *testing.T) {
	unknownMessagePath := writeGovProposalInput(t, ProposalMsg{
		Messages: []json.RawMessage{json.RawMessage(`{"@type":"/unknown.Msg"}`)},
		Deposit:  "1uakt",
		Title:    "Unknown message",
		Summary:  "Must be rejected",
	})
	badDepositPath := writeGovProposalInput(t, ProposalMsg{
		Deposit: "not-a-coin",
		Title:   "Bad deposit",
		Summary: "Must be rejected",
	})

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
		wantError string
	}{
		{name: "deposit proposal id", command: GetTxGovDepositCmd, args: []string{"-1", "1uakt"}, wantError: "not a valid uint"},
		{name: "vote proposal id", command: GetTxGovVoteCmd, args: []string{"nope", "yes"}, wantError: "not a valid int"},
		{name: "vote option", command: GetTxGovVoteCmd, args: []string{"1", "maybe"}, wantError: "not a valid vote option"},
		{name: "cancel proposal id", command: GetTxGovCancelProposalCmd, args: []string{"1.5"}, wantError: "not a valid uint"},
		{name: "submit unknown message", command: GetTxGovSubmitProposalCmd, args: []string{unknownMessagePath}, wantError: "unable to resolve type URL"},
		{name: "submit invalid deposit", command: GetTxGovSubmitProposalCmd, args: []string{badDepositPath}, wantError: "invalid decimal coin expression"},
		{
			name:    "legacy missing title",
			command: GetTxGovSubmitLegacyProposalCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagDescription, "description"))
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagType, "text"))
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "1uakt"))
			},
			wantError: "proposal title is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			txClient := &govCaptureTxClient{response: &sdk.TxResponse{TxHash: "MUST-NOT-BROADCAST"}}
			_, err := runGovTxHandler(t, cmd, txClient, test.args...)
			require.ErrorContains(t, err, test.wantError)
			require.Empty(t, txClient.messages)
		})
	}
}

func TestGovTransactionBroadcastErrorsPreserveCause(t *testing.T) {
	path, _, _ := validGovProposalInput(t)
	broadcastErr := errors.New("governance broadcast unavailable")

	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		configure func(*testing.T, *cobra.Command)
	}{
		{name: "submit", command: GetTxGovSubmitProposalCmd, args: []string{path}},
		{
			name:    "legacy submit",
			command: GetTxGovSubmitLegacyProposalCmd,
			configure: func(t *testing.T, cmd *cobra.Command) {
				t.Helper()
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "title"))
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagDescription, "description"))
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagType, "text"))
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "1uakt"))
			},
		},
		{name: "deposit", command: GetTxGovDepositCmd, args: []string{"1", "1uakt"}},
		{name: "vote", command: GetTxGovVoteCmd, args: []string{"1", "yes"}},
		{name: "weighted vote", command: GetTxGovWeightedVoteCmd, args: []string{"1", "yes=1"}},
		{name: "cancel", command: GetTxGovCancelProposalCmd, args: []string{"1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := test.command()
			if test.configure != nil {
				test.configure(t, cmd)
			}
			txClient := &govCaptureTxClient{err: broadcastErr}
			output, err := runGovTxHandler(t, cmd, txClient, test.args...)
			require.ErrorIs(t, err, broadcastErr)
			require.Len(t, txClient.messages, 1, "a valid message must reach the failed transport exactly once")
			require.Empty(t, output, "a failed broadcast must not print a success receipt")
		})
	}
}

func TestGovProposalParsingRejectsMalformedFilesAndConflictingLegacyInputs(t *testing.T) {
	encoding := aktcodec.MakeEncodingConfig()

	t.Run("malformed JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "proposal.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"messages":[`), 0o600))
		_, _, _, err := parseSubmitProposal(encoding.Codec, path)
		require.Error(t, err)
	})

	t.Run("missing file", func(t *testing.T) {
		_, _, _, err := parseSubmitProposal(encoding.Codec, filepath.Join(t.TempDir(), "missing.json"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("legacy file conflicts with direct flags", func(t *testing.T) {
		path := writeGovProposalInput(t, legacyProposal{
			Title:       "title",
			Description: "description",
			Type:        govv1beta1.ProposalTypeText,
			Deposit:     "1uakt",
		})
		cmd := GetTxGovSubmitLegacyProposalCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagProposal, path))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "ignored title"))

		proposal, err := parseSubmitLegacyProposal(cmd.Flags())
		require.Nil(t, proposal)
		require.EqualError(t, err, "--title flag provided alongside --proposal, which is a noop")
	})
}

func TestGovProposalHelpersPreserveProposalInputs(t *testing.T) {
	t.Run("proposal file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "draft.json")
		value := ProposalMsg{Title: "title", Summary: "summary", Deposit: "3uakt"}
		require.NoError(t, writeFile(path, value))

		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Zero(t, info.Mode().Perm()&0o077, "draft proposal must not be group/world readable")

		var decoded ProposalMsg
		payload, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.Equal(t, value, decoded)
	})

	t.Run("pin code ids", func(t *testing.T) {
		ids, err := parsePinCodesArgs([]string{"1", "18446744073709551615"})
		require.NoError(t, err)
		require.Equal(t, []uint64{1, ^uint64(0)}, ids)

		ids, err = parsePinCodesArgs([]string{"7", "not-an-id", "9"})
		require.ErrorContains(t, err, "code IDs")
		require.Equal(t, []uint64{7, 0, 0}, ids)
	})

	t.Run("common proposal flags", func(t *testing.T) {
		cmd := GetTxGovWasmProposalPinCodesCmd()
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagTitle, "Pin code"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagSummary, "Keep audited code available"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagDeposit, "9uakt"))
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagExpedite, "true"))

		from, _ := govTestAddresses()
		encoding := aktcodec.MakeEncodingConfig()
		cctx := sdkclient.Context{}.
			WithCodec(encoding.Codec).
			WithLegacyAmino(encoding.Amino).
			WithInterfaceRegistry(encoding.InterfaceRegistry).
			WithTxConfig(encoding.TxConfig).
			WithFrom(from.String()).
			WithFromAddress(from)
		cmd.SetContext(context.WithValue(context.Background(), ClientContextKey, &cctx))

		gotContext, title, summary, deposit, expedited, err := getProposalInfo(cmd)
		require.NoError(t, err)
		require.Equal(t, from.String(), gotContext.GetFromAddress().String())
		require.Equal(t, "Pin code", title)
		require.Equal(t, "Keep audited code available", summary)
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 9)), deposit)
		require.True(t, expedited)
	})
}
