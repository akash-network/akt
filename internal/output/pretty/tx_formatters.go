package pretty

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	audittypes "pkg.akt.dev/go/node/audit/v1"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

// RegisterAllTxFormatters registers TxPrettyFormatter implementations for all
// known transaction message types. Called from root command setup.
func RegisterAllTxFormatters() {
	// Bank
	RegisterTx((*banktypes.MsgSend)(nil), TxPrettyFormatterFunc{TitleStr: "Send", FormatFn: fmtBankSend})
	RegisterTx((*banktypes.MsgMultiSend)(nil), TxPrettyFormatterFunc{TitleStr: "Multi Send", FormatFn: fmtBankMultiSend})

	// Deployment
	RegisterTx((*dv1beta.MsgCreateDeployment)(nil), TxPrettyFormatterFunc{TitleStr: "Deployment Created", FormatFn: fmtDeploymentCreate})
	RegisterTx((*dv1beta.MsgUpdateDeployment)(nil), TxPrettyFormatterFunc{TitleStr: "Deployment Updated", FormatFn: fmtDeploymentUpdate})
	RegisterTx((*dv1beta.MsgCloseDeployment)(nil), TxPrettyFormatterFunc{TitleStr: "Deployment Closed", FormatFn: fmtDeploymentClose})
	RegisterTx((*dv1beta.MsgCloseGroup)(nil), TxPrettyFormatterFunc{TitleStr: "Group Closed", FormatFn: fmtGroupClose})
	RegisterTx((*dv1beta.MsgPauseGroup)(nil), TxPrettyFormatterFunc{TitleStr: "Group Paused", FormatFn: fmtGroupPause})
	RegisterTx((*dv1beta.MsgStartGroup)(nil), TxPrettyFormatterFunc{TitleStr: "Group Started", FormatFn: fmtGroupStart})

	// Market
	RegisterTx((*mtypes.MsgCreateBid)(nil), TxPrettyFormatterFunc{TitleStr: "Bid Created", FormatFn: fmtMarketCreateBid})
	RegisterTx((*mtypes.MsgCloseBid)(nil), TxPrettyFormatterFunc{TitleStr: "Bid Closed", FormatFn: fmtMarketCloseBid})
	RegisterTx((*mtypes.MsgCreateLease)(nil), TxPrettyFormatterFunc{TitleStr: "Lease Created", FormatFn: fmtMarketCreateLease})
	RegisterTx((*mtypes.MsgWithdrawLease)(nil), TxPrettyFormatterFunc{TitleStr: "Lease Withdrawn", FormatFn: fmtMarketWithdrawLease})
	RegisterTx((*mtypes.MsgCloseLease)(nil), TxPrettyFormatterFunc{TitleStr: "Lease Closed", FormatFn: fmtMarketCloseLease})

	// Provider
	RegisterTx((*ptypes.MsgCreateProvider)(nil), TxPrettyFormatterFunc{TitleStr: "Provider Created", FormatFn: fmtProviderCreate})
	RegisterTx((*ptypes.MsgUpdateProvider)(nil), TxPrettyFormatterFunc{TitleStr: "Provider Updated", FormatFn: fmtProviderUpdate})

	// Cert
	RegisterTx((*certtypes.MsgCreateCertificate)(nil), TxPrettyFormatterFunc{TitleStr: "Certificate Published", FormatFn: fmtCertCreate})
	RegisterTx((*certtypes.MsgRevokeCertificate)(nil), TxPrettyFormatterFunc{TitleStr: "Certificate Revoked", FormatFn: fmtCertRevoke})

	// Audit
	RegisterTx((*audittypes.MsgSignProviderAttributes)(nil), TxPrettyFormatterFunc{TitleStr: "Attributes Signed", FormatFn: fmtAuditSign})
	RegisterTx((*audittypes.MsgDeleteProviderAttributes)(nil), TxPrettyFormatterFunc{TitleStr: "Attributes Deleted", FormatFn: fmtAuditDelete})

	// Staking
	RegisterTx((*stakingtypes.MsgCreateValidator)(nil), TxPrettyFormatterFunc{TitleStr: "Validator Created", FormatFn: fmtStakingCreateValidator})
	RegisterTx((*stakingtypes.MsgEditValidator)(nil), TxPrettyFormatterFunc{TitleStr: "Validator Edited", FormatFn: fmtStakingEditValidator})
	RegisterTx((*stakingtypes.MsgDelegate)(nil), TxPrettyFormatterFunc{TitleStr: "Delegate", FormatFn: fmtStakingDelegate})
	RegisterTx((*stakingtypes.MsgBeginRedelegate)(nil), TxPrettyFormatterFunc{TitleStr: "Redelegate", FormatFn: fmtStakingRedelegate})
	RegisterTx((*stakingtypes.MsgUndelegate)(nil), TxPrettyFormatterFunc{TitleStr: "Undelegate", FormatFn: fmtStakingUndelegate})
	RegisterTx((*stakingtypes.MsgCancelUnbondingDelegation)(nil), TxPrettyFormatterFunc{TitleStr: "Cancel Unbonding", FormatFn: fmtStakingCancelUnbonding})

	// Distribution
	RegisterTx((*distrtypes.MsgWithdrawDelegatorReward)(nil), TxPrettyFormatterFunc{TitleStr: "Withdraw Rewards", FormatFn: fmtDistrWithdrawReward})
	RegisterTx((*distrtypes.MsgWithdrawValidatorCommission)(nil), TxPrettyFormatterFunc{TitleStr: "Withdraw Commission", FormatFn: fmtDistrWithdrawCommission})
	RegisterTx((*distrtypes.MsgSetWithdrawAddress)(nil), TxPrettyFormatterFunc{TitleStr: "Set Withdraw Address", FormatFn: fmtDistrSetWithdrawAddr})
	RegisterTx((*distrtypes.MsgFundCommunityPool)(nil), TxPrettyFormatterFunc{TitleStr: "Fund Community Pool", FormatFn: fmtDistrFundCommunityPool})
	RegisterTx((*distrtypes.MsgDepositValidatorRewardsPool)(nil), TxPrettyFormatterFunc{TitleStr: "Fund Validator Rewards", FormatFn: fmtDistrFundValidatorRewards})

	// Gov
	RegisterTx((*govv1.MsgSubmitProposal)(nil), TxPrettyFormatterFunc{TitleStr: "Proposal Submitted", FormatFn: fmtGovSubmitProposal})
	RegisterTx((*govv1.MsgDeposit)(nil), TxPrettyFormatterFunc{TitleStr: "Proposal Deposit", FormatFn: fmtGovDeposit})
	RegisterTx((*govv1.MsgVote)(nil), TxPrettyFormatterFunc{TitleStr: "Vote", FormatFn: fmtGovVote})
	RegisterTx((*govv1.MsgVoteWeighted)(nil), TxPrettyFormatterFunc{TitleStr: "Weighted Vote", FormatFn: fmtGovVoteWeighted})

	// Authz
	RegisterTx((*authz.MsgGrant)(nil), TxPrettyFormatterFunc{TitleStr: "Authorization Granted", FormatFn: fmtAuthzGrant})
	RegisterTx((*authz.MsgRevoke)(nil), TxPrettyFormatterFunc{TitleStr: "Authorization Revoked", FormatFn: fmtAuthzRevoke})
	RegisterTx((*authz.MsgExec)(nil), TxPrettyFormatterFunc{TitleStr: "Authz Exec", FormatFn: fmtAuthzExec})

	// Feegrant
	RegisterTx((*feegrant.MsgGrantAllowance)(nil), TxPrettyFormatterFunc{TitleStr: "Fee Allowance Granted", FormatFn: fmtFeegrantGrant})
	RegisterTx((*feegrant.MsgRevokeAllowance)(nil), TxPrettyFormatterFunc{TitleStr: "Fee Allowance Revoked", FormatFn: fmtFeegrantRevoke})

	// Escrow
	RegisterTx((*ev1.MsgAccountDeposit)(nil), TxPrettyFormatterFunc{TitleStr: "Escrow Deposit", FormatFn: fmtEscrowDeposit})

	// Slashing
	RegisterTx((*slashingtypes.MsgUnjail)(nil), TxPrettyFormatterFunc{TitleStr: "Unjail", FormatFn: fmtSlashingUnjail})

	// Vesting
	RegisterTx((*vestingtypes.MsgCreateVestingAccount)(nil), TxPrettyFormatterFunc{TitleStr: "Vesting Account Created", FormatFn: fmtVestingCreate})
	RegisterTx((*vestingtypes.MsgCreatePermanentLockedAccount)(nil), TxPrettyFormatterFunc{TitleStr: "Permanent Locked Account Created", FormatFn: fmtVestingPermanentLocked})
	RegisterTx((*vestingtypes.MsgCreatePeriodicVestingAccount)(nil), TxPrettyFormatterFunc{TitleStr: "Periodic Vesting Account Created", FormatFn: fmtVestingPeriodic})

	// Upgrade
	RegisterTx((*upgradetypes.MsgSoftwareUpgrade)(nil), TxPrettyFormatterFunc{TitleStr: "Software Upgrade", FormatFn: fmtUpgradeSoftware})
	RegisterTx((*upgradetypes.MsgCancelUpgrade)(nil), TxPrettyFormatterFunc{TitleStr: "Upgrade Cancelled", FormatFn: fmtUpgradeCancel})

	// Crisis
	RegisterTx((*crisistypes.MsgVerifyInvariant)(nil), TxPrettyFormatterFunc{TitleStr: "Verify Invariant", FormatFn: fmtCrisisVerifyInvariant})

	// WASM
	RegisterTx((*wasmtypes.MsgStoreCode)(nil), TxPrettyFormatterFunc{TitleStr: "Code Stored", FormatFn: fmtWasmStoreCode})
	RegisterTx((*wasmtypes.MsgInstantiateContract)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Instantiated", FormatFn: fmtWasmInstantiate})
	RegisterTx((*wasmtypes.MsgInstantiateContract2)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Instantiated", FormatFn: fmtWasmInstantiate2})
	RegisterTx((*wasmtypes.MsgExecuteContract)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Executed", FormatFn: fmtWasmExecute})
	RegisterTx((*wasmtypes.MsgMigrateContract)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Migrated", FormatFn: fmtWasmMigrate})
	RegisterTx((*wasmtypes.MsgUpdateAdmin)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Admin Updated", FormatFn: fmtWasmUpdateAdmin})
	RegisterTx((*wasmtypes.MsgClearAdmin)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Admin Cleared", FormatFn: fmtWasmClearAdmin})
	RegisterTx((*wasmtypes.MsgUpdateInstantiateConfig)(nil), TxPrettyFormatterFunc{TitleStr: "Instantiate Config Updated", FormatFn: fmtWasmUpdateInstantiateConfig})
	RegisterTx((*wasmtypes.MsgUpdateContractLabel)(nil), TxPrettyFormatterFunc{TitleStr: "Contract Label Set", FormatFn: fmtWasmSetContractLabel})

	// Oracle
	RegisterTx((*oracletypes.MsgAddPriceEntry)(nil), TxPrettyFormatterFunc{TitleStr: "Price Feed", FormatFn: fmtOracleFeed})

	// BME
	RegisterTx((*bmetypes.MsgBurnMint)(nil), TxPrettyFormatterFunc{TitleStr: "Burn Mint", FormatFn: fmtBMEBurnMint})
	RegisterTx((*bmetypes.MsgMintACT)(nil), TxPrettyFormatterFunc{TitleStr: "Mint ACT", FormatFn: fmtBMEMintACT})
	RegisterTx((*bmetypes.MsgBurnACT)(nil), TxPrettyFormatterFunc{TitleStr: "Burn ACT", FormatFn: fmtBMEBurnACT})
}

// ---------------------------------------------------------------------------
// Bank
// ---------------------------------------------------------------------------

func fmtBankSend(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*banktypes.MsgSend)
	KV(w, "From", m.FromAddress)
	KV(w, "To", m.ToAddress)
	KV(w, "Amount", FormatCoins(m.Amount))
	return nil
}

func fmtBankMultiSend(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*banktypes.MsgMultiSend)
	for i, inp := range m.Inputs {
		if i > 0 {
			Newline(w)
		}
		KV(w, fmt.Sprintf("Input %d", i+1), fmt.Sprintf("%s  %s", inp.Address, FormatCoins(inp.Coins)))
	}
	for i, out := range m.Outputs {
		KV(w, fmt.Sprintf("Output %d", i+1), fmt.Sprintf("%s  %s", out.Address, FormatCoins(out.Coins)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Deployment
// ---------------------------------------------------------------------------

func fmtDeploymentCreate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgCreateDeployment)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "Deposit", FormatCoin(m.Deposit.Amount))
	KV(w, "Groups", fmt.Sprintf("%d", len(m.Groups)))
	return nil
}

func fmtDeploymentUpdate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgUpdateDeployment)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	return nil
}

func fmtDeploymentClose(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgCloseDeployment)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	return nil
}

func fmtGroupClose(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgCloseGroup)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	return nil
}

func fmtGroupPause(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgPauseGroup)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	return nil
}

func fmtGroupStart(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*dv1beta.MsgStartGroup)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	return nil
}

// ---------------------------------------------------------------------------
// Market
// ---------------------------------------------------------------------------

func fmtMarketCreateBid(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*mtypes.MsgCreateBid)
	KV(w, "Provider", m.ID.Provider)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	KV(w, "OSEQ", fmt.Sprintf("%d", m.ID.OSeq))
	KV(w, "Price", FormatDecCoin(m.Price))
	return nil
}

func fmtMarketCloseBid(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*mtypes.MsgCloseBid)
	KV(w, "Provider", m.ID.Provider)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	KV(w, "OSEQ", fmt.Sprintf("%d", m.ID.OSeq))
	return nil
}

func fmtMarketCreateLease(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*mtypes.MsgCreateLease)
	KV(w, "Owner", m.BidID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.BidID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.BidID.GSeq))
	KV(w, "OSEQ", fmt.Sprintf("%d", m.BidID.OSeq))
	KV(w, "Provider", m.BidID.Provider)
	return nil
}

func fmtMarketWithdrawLease(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*mtypes.MsgWithdrawLease)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	KV(w, "OSEQ", fmt.Sprintf("%d", m.ID.OSeq))
	KV(w, "Provider", m.ID.Provider)
	return nil
}

func fmtMarketCloseLease(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*mtypes.MsgCloseLease)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", m.ID.DSeq)))
	KV(w, "GSEQ", fmt.Sprintf("%d", m.ID.GSeq))
	KV(w, "OSEQ", fmt.Sprintf("%d", m.ID.OSeq))
	KV(w, "Provider", m.ID.Provider)
	return nil
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

func fmtProviderCreate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*ptypes.MsgCreateProvider)
	KV(w, "Owner", m.Owner)
	KV(w, "Host URI", m.HostURI)
	if len(m.Attributes) > 0 {
		KV(w, "Attributes", formatAttributes(m.Attributes))
	}
	return nil
}

func fmtProviderUpdate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*ptypes.MsgUpdateProvider)
	KV(w, "Owner", m.Owner)
	KV(w, "Host URI", m.HostURI)
	if len(m.Attributes) > 0 {
		KV(w, "Attributes", formatAttributes(m.Attributes))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Cert
// ---------------------------------------------------------------------------

func fmtCertCreate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*certtypes.MsgCreateCertificate)
	KV(w, "Owner", m.Owner)
	return nil
}

func fmtCertRevoke(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*certtypes.MsgRevokeCertificate)
	KV(w, "Owner", m.ID.Owner)
	KV(w, "Serial", m.ID.Serial)
	return nil
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

func fmtAuditSign(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*audittypes.MsgSignProviderAttributes)
	KV(w, "Auditor", m.Auditor)
	KV(w, "Provider", m.Owner)
	if len(m.Attributes) > 0 {
		KV(w, "Attributes", formatAttributes(m.Attributes))
	}
	return nil
}

func fmtAuditDelete(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*audittypes.MsgDeleteProviderAttributes)
	KV(w, "Auditor", m.Auditor)
	KV(w, "Provider", m.Owner)
	if len(m.Keys) > 0 {
		KV(w, "Keys", strings.Join(m.Keys, ", "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Staking
// ---------------------------------------------------------------------------

func fmtStakingCreateValidator(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgCreateValidator)
	KV(w, "Operator", m.ValidatorAddress)
	KV(w, "Moniker", m.Description.Moniker)
	KV(w, "Self-Delegation", FormatCoin(m.Value))
	KV(w, "Commission", m.Commission.Rate.String())
	return nil
}

func fmtStakingEditValidator(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgEditValidator)
	KV(w, "Operator", m.ValidatorAddress)
	if m.Description.Moniker != stakingtypes.DoNotModifyDesc {
		KV(w, "Moniker", m.Description.Moniker)
	}
	if m.CommissionRate != nil {
		KV(w, "Commission", m.CommissionRate.String())
	}
	return nil
}

func fmtStakingDelegate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgDelegate)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "Validator", m.ValidatorAddress)
	KV(w, "Amount", FormatCoin(m.Amount))
	return nil
}

func fmtStakingRedelegate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgBeginRedelegate)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "From", m.ValidatorSrcAddress)
	KV(w, "To", m.ValidatorDstAddress)
	KV(w, "Amount", FormatCoin(m.Amount))
	return nil
}

func fmtStakingUndelegate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgUndelegate)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "Validator", m.ValidatorAddress)
	KV(w, "Amount", FormatCoin(m.Amount))
	return nil
}

func fmtStakingCancelUnbonding(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*stakingtypes.MsgCancelUnbondingDelegation)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "Validator", m.ValidatorAddress)
	KV(w, "Amount", FormatCoin(m.Amount))
	return nil
}

// ---------------------------------------------------------------------------
// Distribution
// ---------------------------------------------------------------------------

func fmtDistrWithdrawReward(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*distrtypes.MsgWithdrawDelegatorReward)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "Validator", m.ValidatorAddress)
	return nil
}

func fmtDistrWithdrawCommission(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*distrtypes.MsgWithdrawValidatorCommission)
	KV(w, "Validator", m.ValidatorAddress)
	return nil
}

func fmtDistrSetWithdrawAddr(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*distrtypes.MsgSetWithdrawAddress)
	KV(w, "Delegator", m.DelegatorAddress)
	KV(w, "Withdraw Address", m.WithdrawAddress)
	return nil
}

func fmtDistrFundCommunityPool(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*distrtypes.MsgFundCommunityPool)
	KV(w, "Depositor", m.Depositor)
	KV(w, "Amount", FormatCoins(m.Amount))
	return nil
}

func fmtDistrFundValidatorRewards(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*distrtypes.MsgDepositValidatorRewardsPool)
	KV(w, "Depositor", m.Depositor)
	KV(w, "Validator", m.ValidatorAddress)
	KV(w, "Amount", FormatCoins(m.Amount))
	return nil
}

// ---------------------------------------------------------------------------
// Gov
// ---------------------------------------------------------------------------

func fmtGovSubmitProposal(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*govv1.MsgSubmitProposal)
	KV(w, "Proposer", m.Proposer)
	if m.Title != "" {
		KV(w, "Title", m.Title)
	}
	if len(m.InitialDeposit) > 0 {
		KV(w, "Deposit", FormatCoins(m.InitialDeposit))
	}
	return nil
}

func fmtGovDeposit(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*govv1.MsgDeposit)
	KV(w, "Depositor", m.Depositor)
	KV(w, "Proposal ID", Bold(fmt.Sprintf("%d", m.ProposalId)))
	KV(w, "Amount", FormatCoins(m.Amount))
	return nil
}

func fmtGovVote(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*govv1.MsgVote)
	KV(w, "Voter", m.Voter)
	KV(w, "Proposal ID", Bold(fmt.Sprintf("%d", m.ProposalId)))
	KV(w, "Option", govVoteOptionString(m.Option))
	return nil
}

func fmtGovVoteWeighted(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*govv1.MsgVoteWeighted)
	KV(w, "Voter", m.Voter)
	KV(w, "Proposal ID", Bold(fmt.Sprintf("%d", m.ProposalId)))
	var opts []string
	for _, o := range m.Options {
		opts = append(opts, fmt.Sprintf("%s=%s", govVoteOptionString(o.Option), o.Weight))
	}
	KV(w, "Options", strings.Join(opts, ", "))
	return nil
}

func govVoteOptionString(o govv1.VoteOption) string {
	switch o {
	case govv1.OptionYes:
		return "Yes"
	case govv1.OptionNo:
		return "No"
	case govv1.OptionAbstain:
		return "Abstain"
	case govv1.OptionNoWithVeto:
		return "NoWithVeto"
	default:
		return o.String()
	}
}

// ---------------------------------------------------------------------------
// Authz
// ---------------------------------------------------------------------------

func fmtAuthzGrant(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*authz.MsgGrant)
	KV(w, "Granter", m.Granter)
	KV(w, "Grantee", m.Grantee)
	if m.Grant.Authorization != nil {
		KV(w, "Type", shortTypeName(m.Grant.Authorization.TypeUrl))
	}
	if m.Grant.Expiration != nil {
		KV(w, "Expiration", m.Grant.Expiration.Format(time.RFC3339))
	}
	return nil
}

func fmtAuthzRevoke(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*authz.MsgRevoke)
	KV(w, "Granter", m.Granter)
	KV(w, "Grantee", m.Grantee)
	KV(w, "Msg Type", shortTypeName(m.MsgTypeUrl))
	return nil
}

func fmtAuthzExec(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*authz.MsgExec)
	KV(w, "Grantee", m.Grantee)
	KV(w, "Messages", fmt.Sprintf("%d", len(m.Msgs)))
	return nil
}

// ---------------------------------------------------------------------------
// Feegrant
// ---------------------------------------------------------------------------

func fmtFeegrantGrant(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*feegrant.MsgGrantAllowance)
	KV(w, "Granter", m.Granter)
	KV(w, "Grantee", m.Grantee)
	if m.Allowance != nil {
		KV(w, "Type", shortTypeName(m.Allowance.TypeUrl))
	}
	return nil
}

func fmtFeegrantRevoke(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*feegrant.MsgRevokeAllowance)
	KV(w, "Granter", m.Granter)
	KV(w, "Grantee", m.Grantee)
	return nil
}

// ---------------------------------------------------------------------------
// Escrow
// ---------------------------------------------------------------------------

func fmtEscrowDeposit(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*ev1.MsgAccountDeposit)
	KV(w, "Signer", m.Signer)
	KV(w, "Account", m.ID.XID)
	KV(w, "Amount", FormatCoin(m.Deposit.Amount))
	return nil
}

// ---------------------------------------------------------------------------
// Slashing
// ---------------------------------------------------------------------------

func fmtSlashingUnjail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*slashingtypes.MsgUnjail)
	KV(w, "Validator", m.ValidatorAddr)
	return nil
}

// ---------------------------------------------------------------------------
// Vesting
// ---------------------------------------------------------------------------

func fmtVestingCreate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*vestingtypes.MsgCreateVestingAccount)
	KV(w, "From", m.FromAddress)
	KV(w, "To", m.ToAddress)
	KV(w, "Amount", FormatCoins(m.Amount))
	KV(w, "End Time", time.Unix(m.EndTime, 0).UTC().Format(time.RFC3339))
	return nil
}

func fmtVestingPermanentLocked(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*vestingtypes.MsgCreatePermanentLockedAccount)
	KV(w, "From", m.FromAddress)
	KV(w, "To", m.ToAddress)
	KV(w, "Amount", FormatCoins(m.Amount))
	return nil
}

func fmtVestingPeriodic(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*vestingtypes.MsgCreatePeriodicVestingAccount)
	KV(w, "From", m.FromAddress)
	KV(w, "To", m.ToAddress)
	KV(w, "Periods", fmt.Sprintf("%d", len(m.VestingPeriods)))
	return nil
}

// ---------------------------------------------------------------------------
// Upgrade
// ---------------------------------------------------------------------------

func fmtUpgradeSoftware(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*upgradetypes.MsgSoftwareUpgrade)
	KV(w, "Authority", m.Authority)
	KV(w, "Name", m.Plan.Name)
	KV(w, "Height", FormatHeight(m.Plan.Height))
	return nil
}

func fmtUpgradeCancel(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*upgradetypes.MsgCancelUpgrade)
	KV(w, "Authority", m.Authority)
	return nil
}

// ---------------------------------------------------------------------------
// Crisis
// ---------------------------------------------------------------------------

func fmtCrisisVerifyInvariant(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*crisistypes.MsgVerifyInvariant)
	KV(w, "Sender", m.Sender)
	KV(w, "Module", m.InvariantModuleName)
	KV(w, "Route", m.InvariantRoute)
	return nil
}

// ---------------------------------------------------------------------------
// WASM
// ---------------------------------------------------------------------------

func fmtWasmStoreCode(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgStoreCode)
	KV(w, "Sender", m.Sender)
	return nil
}

func fmtWasmInstantiate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgInstantiateContract)
	KV(w, "Sender", m.Sender)
	KV(w, "Code ID", fmt.Sprintf("%d", m.CodeID))
	KV(w, "Label", m.Label)
	admin := m.Admin
	if admin == "" {
		admin = "none"
	}
	KV(w, "Admin", admin)
	return nil
}

func fmtWasmInstantiate2(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgInstantiateContract2)
	KV(w, "Sender", m.Sender)
	KV(w, "Code ID", fmt.Sprintf("%d", m.CodeID))
	KV(w, "Label", m.Label)
	admin := m.Admin
	if admin == "" {
		admin = "none"
	}
	KV(w, "Admin", admin)
	KV(w, "Salt", fmt.Sprintf("%x", m.Salt))
	return nil
}

func fmtWasmExecute(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgExecuteContract)
	KV(w, "Sender", m.Sender)
	KV(w, "Contract", m.Contract)
	if len(m.Funds) > 0 {
		KV(w, "Funds", FormatCoins(m.Funds))
	}
	return nil
}

func fmtWasmMigrate(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgMigrateContract)
	KV(w, "Sender", m.Sender)
	KV(w, "Contract", m.Contract)
	KV(w, "New Code ID", fmt.Sprintf("%d", m.CodeID))
	return nil
}

func fmtWasmUpdateAdmin(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgUpdateAdmin)
	KV(w, "Sender", m.Sender)
	KV(w, "Contract", m.Contract)
	KV(w, "New Admin", m.NewAdmin)
	return nil
}

func fmtWasmClearAdmin(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgClearAdmin)
	KV(w, "Sender", m.Sender)
	KV(w, "Contract", m.Contract)
	return nil
}

func fmtWasmUpdateInstantiateConfig(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgUpdateInstantiateConfig)
	KV(w, "Sender", m.Sender)
	KV(w, "Code ID", fmt.Sprintf("%d", m.CodeID))
	return nil
}

func fmtWasmSetContractLabel(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*wasmtypes.MsgUpdateContractLabel)
	KV(w, "Sender", m.Sender)
	KV(w, "Contract", m.Contract)
	KV(w, "Label", m.NewLabel)
	return nil
}

// ---------------------------------------------------------------------------
// Oracle
// ---------------------------------------------------------------------------

func fmtOracleFeed(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*oracletypes.MsgAddPriceEntry)
	KV(w, "Sender", m.Signer)
	KV(w, "Asset", m.ID.Denom)
	KV(w, "Base", m.ID.BaseDenom)
	KV(w, "Price", m.Price.String())
	return nil
}

// ---------------------------------------------------------------------------
// BME
// ---------------------------------------------------------------------------

func fmtBMEBurnMint(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*bmetypes.MsgBurnMint)
	KV(w, "Sender", m.Owner)
	KV(w, "Burned", FormatCoin(m.CoinsToBurn))
	KV(w, "Minted Denom", m.DenomToMint)
	return nil
}

func fmtBMEMintACT(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*bmetypes.MsgMintACT)
	KV(w, "Sender", m.Owner)
	KV(w, "Burned", FormatCoin(m.CoinsToBurn))
	return nil
}

func fmtBMEBurnACT(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg sdk.Msg, _ *sdk.TxResponse, _ int) error {
	m := msg.(*bmetypes.MsgBurnACT)
	KV(w, "Sender", m.Owner)
	KV(w, "Burned", FormatCoin(m.CoinsToBurn))
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatAttributes formats a list of Akash provider attributes as "key=value, ..."
func formatAttributes(attrs interface{}) string {
	// Attributes implement the Stringer interface in most cases.
	// Use fmt.Sprint as a safe fallback.
	return fmt.Sprint(attrs)
}
