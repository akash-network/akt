package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	chain "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	chaintest "pkg.akt.dev/akt/internal/cli/chain/testutil"
	aktcodec "pkg.akt.dev/akt/internal/codec"
)

type generatedTxFixture struct {
	cctx       sdkclient.Context
	from       sdk.AccAddress
	recipientA sdk.AccAddress
	recipientB sdk.AccAddress
	validatorA sdk.ValAddress
	validatorB sdk.ValAddress
}

func newGeneratedTxFixture(t *testing.T) generatedTxFixture {
	t.Helper()

	enc := aktcodec.MakeEncodingConfig()
	from := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))

	return generatedTxFixture{
		cctx: sdkclient.Context{}.
			WithCodec(enc.Codec).
			WithLegacyAmino(enc.Amino).
			WithInterfaceRegistry(enc.InterfaceRegistry).
			WithTxConfig(enc.TxConfig).
			WithChainID("chain-contract-test").
			WithFromAddress(from).
			WithHomeDir(t.TempDir()),
		from:       from,
		recipientA: sdk.AccAddress(bytes.Repeat([]byte{2}, 20)),
		recipientB: sdk.AccAddress(bytes.Repeat([]byte{3}, 20)),
		validatorA: sdk.ValAddress(bytes.Repeat([]byte{4}, 20)),
		validatorB: sdk.ValAddress(bytes.Repeat([]byte{5}, 20)),
	}
}

func (f generatedTxFixture) executeOffline(
	t *testing.T,
	cmd *cobra.Command,
	args ...string,
) ([]byte, error) {
	t.Helper()

	callArgs := chaintest.TestFlags().
		With(args...).
		WithFrom(f.from.String()).
		WithGenerateOnly().
		WithOffline().
		WithGas(200000).
		WithChainID(f.cctx.ChainID).
		WithOutputJSON()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := chaintest.ExecTestCLICmd(ctx, f.cctx, cmd, callArgs...)
	if out == nil {
		return nil, err
	}

	return append([]byte(nil), out.Bytes()...), err
}

func (f generatedTxFixture) generate(
	t *testing.T,
	cmd *cobra.Command,
	wantSigner []byte,
	args ...string,
) []sdk.Msg {
	t.Helper()

	return f.generateTx(t, cmd, wantSigner, args...).GetMsgs()
}

func (f generatedTxFixture) generateTx(
	t *testing.T,
	cmd *cobra.Command,
	wantSigner []byte,
	args ...string,
) sdk.Tx {
	t.Helper()

	payload, err := f.executeOffline(t, cmd, args...)
	require.NoError(t, err, "command output:\n%s", payload)

	tx, err := f.cctx.TxConfig.TxJSONDecoder()(bytes.TrimSpace(payload))
	require.NoError(t, err, "decode generated transaction:\n%s", payload)

	msgs := tx.GetMsgs()
	msgsV2, err := tx.GetMsgsV2()
	require.NoError(t, err)
	require.Len(t, msgsV2, len(msgs))
	for _, msg := range msgsV2 {
		signers, err := f.cctx.TxConfig.SigningContext().GetSigners(msg)
		require.NoError(t, err)
		require.Equal(t, [][]byte{wantSigner}, signers)
	}

	return tx
}

func TestCoreTransactionCommandContracts(t *testing.T) {
	t.Parallel()

	root := chain.TxCmd()
	distribution := directChild(root, "distribution")
	require.NotNil(t, distribution)

	tests := []struct {
		name     string
		group    *cobra.Command
		children []string
	}{
		{
			name:     "bank",
			group:    chain.GetTxBankCmd(),
			children: []string{"multi-send", "send"},
		},
		{
			name:  "distribution",
			group: distribution,
			children: []string{
				"fund-community-pool",
				"fund-validator-rewards-pool",
				"set-withdraw-addr",
				"withdraw-all-rewards",
				"withdraw-rewards",
			},
		},
		{
			name:  "staking",
			group: chain.GetTxStakingCmd(),
			children: []string{
				"cancel-unbond",
				"create-validator",
				"delegate",
				"edit-validator",
				"redelegate",
				"unbond",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, cmd := range tc.group.Commands() {
				got = append(got, cmd.Name())
			}
			sort.Strings(got)
			require.Equal(t, tc.children, got)
			require.True(t, tc.group.DisableFlagParsing)
		})
	}

	send := directChild(tests[0].group, "send")
	require.NoError(t, send.Args(send, []string{"to", "1uakt"}))
	require.NoError(t, send.Args(send, []string{"from", "to", "1uakt"}))
	require.Error(t, send.Args(send, []string{"to"}))

	multiSend := directChild(tests[0].group, "multi-send")
	require.NotNil(t, multiSend.Flags().Lookup(cflags.FlagSplit))
	require.Error(t, multiSend.Args(multiSend, []string{"from", "to", "1uakt"}))

	withdrawRewards := directChild(distribution, "withdraw-rewards")
	require.NotNil(t, withdrawRewards.Flags().Lookup(chain.FlagCommission))
	withdrawAll := directChild(distribution, "withdraw-all-rewards")
	require.NotNil(t, withdrawAll.Flags().Lookup(chain.FlagMaxMessagesPerTx))

	cancelUnbond := directChild(tests[2].group, "cancel-unbond")
	require.NoError(t, cancelUnbond.Args(cancelUnbond, []string{"validator", "1uakt", "7"}))
	require.Error(t, cancelUnbond.Args(cancelUnbond, []string{"validator", "1uakt"}))
	require.NotNil(t, directChild(tests[2].group, "edit-validator").Flags().Lookup(cflags.FlagCommissionRate))
	createValidator := directChild(tests[2].group, "create-validator")
	require.Equal(t, "26656", createValidator.Flags().Lookup(cflags.FlagP2PPort).DefValue)
}

func TestBankSendGeneratesValidatedMessages(t *testing.T) {
	f := newGeneratedTxFixture(t)

	t.Run("from flag", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxBankSendTxCmd(),
			f.from,
			f.recipientA.String(),
			"7uakt",
		)
		require.Len(t, msgs, 1)

		msg, ok := msgs[0].(*banktypes.MsgSend)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.FromAddress)
		require.Equal(t, f.recipientA.String(), msg.ToAddress)
		require.Equal(t, "7uakt", msg.Amount.String())
	})

	t.Run("positional sender overrides from flag", func(t *testing.T) {
		positionalSender := sdk.AccAddress(bytes.Repeat([]byte{6}, 20))
		msgs := f.generate(
			t,
			chain.GetTxBankSendTxCmd(),
			positionalSender,
			positionalSender.String(),
			f.recipientA.String(),
			"9uakt",
		)

		msg, ok := msgs[0].(*banktypes.MsgSend)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, positionalSender.String(), msg.FromAddress)
		require.Equal(t, f.recipientA.String(), msg.ToAddress)
		require.Equal(t, "9uakt", msg.Amount.String())
	})
}

func TestBankMultiSendPreservesOrSplitsAmounts(t *testing.T) {
	f := newGeneratedTxFixture(t)

	for _, tc := range []struct {
		name       string
		extraArgs  []string
		wantInput  string
		wantOutput string
	}{
		{
			name:       "amount per recipient",
			wantInput:  "20uakt",
			wantOutput: "10uakt",
		},
		{
			name:       "split total amount",
			extraArgs:  []string{"--split=true"},
			wantInput:  "10uakt",
			wantOutput: "5uakt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				f.from.String(),
				f.recipientA.String(),
				f.recipientB.String(),
				"10uakt",
			}
			args = append(args, tc.extraArgs...)
			msgs := f.generate(t, chain.GetTxBankMultiSendTxCmd(), f.from, args...)

			msg, ok := msgs[0].(*banktypes.MsgMultiSend)
			require.True(t, ok, "message type = %T", msgs[0])
			require.Len(t, msg.Inputs, 1)
			require.Len(t, msg.Outputs, 2)
			require.Equal(t, f.from.String(), msg.Inputs[0].Address)
			require.Equal(t, tc.wantInput, msg.Inputs[0].Coins.String())
			require.Equal(t, f.recipientA.String(), msg.Outputs[0].Address)
			require.Equal(t, f.recipientB.String(), msg.Outputs[1].Address)
			for _, output := range msg.Outputs {
				require.Equal(t, tc.wantOutput, output.Coins.String())
			}
			require.NoError(t, banktypes.ValidateInputOutputs(msg.Inputs[0], msg.Outputs))
		})
	}
}

func TestBankTransactionsRejectInvalidInputsBeforeGeneration(t *testing.T) {
	f := newGeneratedTxFixture(t)

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want string
	}{
		{
			name: "send address",
			cmd:  chain.GetTxBankSendTxCmd,
			args: []string{"not-an-address", "1uakt"},
			want: "bech32",
		},
		{
			name: "send coin",
			cmd:  chain.GetTxBankSendTxCmd,
			args: []string{f.recipientA.String(), "not-a-coin"},
			want: "coin",
		},
		{
			name: "send zero",
			cmd:  chain.GetTxBankSendTxCmd,
			args: []string{f.recipientA.String(), "0uakt"},
			want: "invalid coins",
		},
		{
			name: "multi-send zero",
			cmd:  chain.GetTxBankMultiSendTxCmd,
			args: []string{f.from.String(), f.recipientA.String(), f.recipientB.String(), "0uakt"},
			want: "positive amount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := f.executeOffline(t, tc.cmd(), tc.args...)
			require.Error(t, err, "command output:\n%s", output)
			require.Contains(t, strings.ToLower(err.Error()), tc.want)
			require.False(t, bytes.Contains(output, []byte(`"body"`)), "invalid input generated a transaction")
		})
	}
}

func TestStakingGeneratesValidatedMessages(t *testing.T) {
	f := newGeneratedTxFixture(t)

	t.Run("delegate", func(t *testing.T) {
		msgs := f.generate(t, chain.GetTxStakingDelegateCmd(), f.from, f.validatorA.String(), "123uakt")
		msg, ok := msgs[0].(*stakingtypes.MsgDelegate)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.DelegatorAddress)
		require.Equal(t, f.validatorA.String(), msg.ValidatorAddress)
		require.Equal(t, "123uakt", msg.Amount.String())
	})

	t.Run("redelegate", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxStakingRedelegateCmd(),
			f.from,
			f.validatorA.String(),
			f.validatorB.String(),
			"77uakt",
		)
		msg, ok := msgs[0].(*stakingtypes.MsgBeginRedelegate)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.DelegatorAddress)
		require.Equal(t, f.validatorA.String(), msg.ValidatorSrcAddress)
		require.Equal(t, f.validatorB.String(), msg.ValidatorDstAddress)
		require.Equal(t, "77uakt", msg.Amount.String())
	})

	t.Run("unbond", func(t *testing.T) {
		msgs := f.generate(t, chain.GetTxStakingUnbondCmd(), f.from, f.validatorA.String(), "44uakt")
		msg, ok := msgs[0].(*stakingtypes.MsgUndelegate)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.DelegatorAddress)
		require.Equal(t, f.validatorA.String(), msg.ValidatorAddress)
		require.Equal(t, "44uakt", msg.Amount.String())
	})

	t.Run("cancel unbond", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxStakingCancelUnbondingDelegationCmd(),
			f.from,
			f.validatorA.String(),
			"33uakt",
			"42",
		)
		msg, ok := msgs[0].(*stakingtypes.MsgCancelUnbondingDelegation)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.DelegatorAddress)
		require.Equal(t, f.validatorA.String(), msg.ValidatorAddress)
		require.Equal(t, "33uakt", msg.Amount.String())
		require.EqualValues(t, 42, msg.CreationHeight)
	})

	t.Run("edit validator", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxStakingEditValidatorCmd(),
			f.from,
			fmt.Sprintf("--%s=contract-validator", cflags.FlagEditMoniker),
			fmt.Sprintf("--%s=0.12", cflags.FlagCommissionRate),
			fmt.Sprintf("--%s=5", cflags.FlagMinSelfDelegation),
		)
		msg, ok := msgs[0].(*stakingtypes.MsgEditValidator)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, sdk.ValAddress(f.from).String(), msg.ValidatorAddress)
		require.Equal(t, "contract-validator", msg.Description.Moniker)
		require.NotNil(t, msg.CommissionRate)
		require.Equal(t, "0.120000000000000000", msg.CommissionRate.String())
		require.NotNil(t, msg.MinSelfDelegation)
		require.Equal(t, "5", msg.MinSelfDelegation.String())
	})
}

func TestStakingCreateValidatorGeneratesValidMessage(t *testing.T) {
	f := newGeneratedTxFixture(t)
	pubKey := ed25519.GenPrivKey().PubKey()
	pubKeyJSON, err := f.cctx.Codec.MarshalInterfaceJSON(pubKey)
	require.NoError(t, err)

	definition := fmt.Sprintf(`{
  "pubkey": %s,
  "amount": "1000000uakt",
  "moniker": "contract-validator",
  "identity": "identity",
  "website": "https://validator.example",
  "security": "security@example.com",
  "details": "generated offline",
  "commission-rate": "0.1",
  "commission-max-rate": "0.2",
  "commission-max-change-rate": "0.01",
  "min-self-delegation": "1"
}`, pubKeyJSON)
	path := filepath.Join(t.TempDir(), "validator.json")
	require.NoError(t, os.WriteFile(path, []byte(definition), 0o600))

	tx := f.generateTx(
		t,
		chain.GetTxStakingCreateValidatorCmd(),
		f.from,
		path,
		fmt.Sprintf("--%s=node-contract-id", cflags.FlagNodeID),
		fmt.Sprintf("--%s=203.0.113.8", cflags.FlagIP),
		fmt.Sprintf("--%s=27656", cflags.FlagP2PPort),
	)
	msgs := tx.GetMsgs()
	require.Len(t, msgs, 1)
	msg, ok := msgs[0].(*stakingtypes.MsgCreateValidator)
	require.True(t, ok, "message type = %T", msgs[0])
	require.Equal(t, sdk.ValAddress(f.from).String(), msg.ValidatorAddress)
	require.Equal(t, "1000000uakt", msg.Value.String())
	require.Equal(t, "contract-validator", msg.Description.Moniker)
	require.Equal(t, "0.100000000000000000", msg.Commission.Rate.String())
	require.Equal(t, "1", msg.MinSelfDelegation.String())
	require.NoError(t, msg.Validate(f.cctx.TxConfig.SigningContext().ValidatorAddressCodec()))
	txWithMemo, ok := tx.(sdk.TxWithMemo)
	require.True(t, ok, "generated transaction type = %T", tx)
	require.Equal(t, "node-contract-id@203.0.113.8:27656", txWithMemo.GetMemo())

	for _, port := range []string{"0", "65536"} {
		t.Run("reject port "+port, func(t *testing.T) {
			output, err := f.executeOffline(
				t,
				chain.GetTxStakingCreateValidatorCmd(),
				path,
				fmt.Sprintf("--%s=%s", cflags.FlagP2PPort, port),
			)
			require.Error(t, err, "command output:\n%s", output)
			require.Contains(t, err.Error(), "must be between 1 and 65535")
			require.False(t, bytes.Contains(output, []byte(`"body"`)), "invalid port generated a transaction")
		})
	}
}

func TestStakingTransactionsRejectInvalidInputsBeforeGeneration(t *testing.T) {
	f := newGeneratedTxFixture(t)

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want string
	}{
		{
			name: "delegate validator",
			cmd:  chain.GetTxStakingDelegateCmd,
			args: []string{"not-a-validator", "1uakt"},
			want: "bech32",
		},
		{
			name: "delegate amount",
			cmd:  chain.GetTxStakingDelegateCmd,
			args: []string{f.validatorA.String(), "not-a-coin"},
			want: "coin",
		},
		{
			name: "redelegate destination",
			cmd:  chain.GetTxStakingRedelegateCmd,
			args: []string{f.validatorA.String(), "not-a-validator", "1uakt"},
			want: "bech32",
		},
		{
			name: "cancel height",
			cmd:  chain.GetTxStakingCancelUnbondingDelegationCmd,
			args: []string{f.validatorA.String(), "1uakt", "not-a-height"},
			want: "invalid height",
		},
		{
			name: "edit commission",
			cmd:  chain.GetTxStakingEditValidatorCmd,
			args: []string{fmt.Sprintf("--%s=not-a-rate", cflags.FlagCommissionRate)},
			want: "invalid new commission rate",
		},
		{
			name: "edit minimum self delegation",
			cmd:  chain.GetTxStakingEditValidatorCmd,
			args: []string{fmt.Sprintf("--%s=not-an-integer", cflags.FlagMinSelfDelegation)},
			want: "positive integer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := f.executeOffline(t, tc.cmd(), tc.args...)
			require.Error(t, err, "command output:\n%s", output)
			require.Contains(t, strings.ToLower(err.Error()), tc.want)
			require.False(t, bytes.Contains(output, []byte(`"body"`)), "invalid input generated a transaction")
		})
	}
}

func TestDistributionGeneratesValidatedMessages(t *testing.T) {
	f := newGeneratedTxFixture(t)
	ownValidator := sdk.ValAddress(f.from)

	t.Run("withdraw reward and commission", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxDistributionWithdrawRewardsCmd(),
			f.from,
			ownValidator.String(),
			fmt.Sprintf("--%s=true", chain.FlagCommission),
		)
		require.Len(t, msgs, 2)
		reward, ok := msgs[0].(*distrtypes.MsgWithdrawDelegatorReward)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), reward.DelegatorAddress)
		require.Equal(t, ownValidator.String(), reward.ValidatorAddress)
		commission, ok := msgs[1].(*distrtypes.MsgWithdrawValidatorCommission)
		require.True(t, ok, "message type = %T", msgs[1])
		require.Equal(t, ownValidator.String(), commission.ValidatorAddress)
	})

	t.Run("set withdraw address", func(t *testing.T) {
		msgs := f.generate(t, chain.GetTxDistributionSetWithdrawAddrCmd(), f.from, f.recipientA.String())
		msg, ok := msgs[0].(*distrtypes.MsgSetWithdrawAddress)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.DelegatorAddress)
		require.Equal(t, f.recipientA.String(), msg.WithdrawAddress)
	})

	t.Run("fund community pool", func(t *testing.T) {
		msgs := f.generate(t, chain.GetTxDistributionFundCommunityPoolCmd(), f.from, "25uakt")
		msg, ok := msgs[0].(*distrtypes.MsgFundCommunityPool)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.Depositor)
		require.Equal(t, "25uakt", msg.Amount.String())
	})

	t.Run("fund validator rewards pool", func(t *testing.T) {
		msgs := f.generate(
			t,
			chain.GetTxDistributionDepositValidatorRewardsPoolCmd(),
			f.from,
			f.validatorA.String(),
			"31uakt",
		)
		msg, ok := msgs[0].(*distrtypes.MsgDepositValidatorRewardsPool)
		require.True(t, ok, "message type = %T", msgs[0])
		require.Equal(t, f.from.String(), msg.Depositor)
		require.Equal(t, f.validatorA.String(), msg.ValidatorAddress)
		require.Equal(t, "31uakt", msg.Amount.String())
	})
}

func TestDistributionTransactionsRejectInvalidAndOfflineInputs(t *testing.T) {
	f := newGeneratedTxFixture(t)

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want string
	}{
		{
			name: "withdraw validator",
			cmd:  chain.GetTxDistributionWithdrawRewardsCmd,
			args: []string{"not-a-validator"},
			want: "bech32",
		},
		{
			name: "withdraw address",
			cmd:  chain.GetTxDistributionSetWithdrawAddrCmd,
			args: []string{"not-an-address"},
			want: "bech32",
		},
		{
			name: "community amount",
			cmd:  chain.GetTxDistributionFundCommunityPoolCmd,
			args: []string{"not-a-coin"},
			want: "coin",
		},
		{
			name: "validator pool validator",
			cmd:  chain.GetTxDistributionDepositValidatorRewardsPoolCmd,
			args: []string{"not-a-validator", "1uakt"},
			want: "bech32",
		},
		{
			name: "validator pool amount",
			cmd:  chain.GetTxDistributionDepositValidatorRewardsPoolCmd,
			args: []string{f.validatorA.String(), "not-a-coin"},
			want: "coin",
		},
		{
			name: "withdraw all offline",
			cmd:  chain.GetTxDistributionWithdrawAllRewardsCmd,
			want: "cannot generate tx in offline mode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, err := f.executeOffline(t, tc.cmd(), tc.args...)
			require.Error(t, err, "command output:\n%s", output)
			require.Contains(t, strings.ToLower(err.Error()), tc.want)
			require.False(t, bytes.Contains(output, []byte(`"body"`)), "invalid input generated a transaction")
		})
	}
}

func directChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}

	return nil
}
