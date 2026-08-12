package pretty

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/charmbracelet/x/ansi"
	abci "github.com/cometbft/cometbft/abci/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	audittypes "pkg.akt.dev/go/node/audit/v1"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/id/v1"
	escrowtypes "pkg.akt.dev/go/node/escrow/v1"
	marketid "pkg.akt.dev/go/node/market/v1"
	markettypes "pkg.akt.dev/go/node/market/v1beta5"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
	dtypes "pkg.akt.dev/go/node/types/deposit/v1"
)

type txFormatterSemanticCase struct {
	msg      sdk.Msg
	title    string
	response *sdk.TxResponse
	context  sdkclient.Context
	want     string
}

type txFormatterErrorWriter struct {
	err error
}

func (w txFormatterErrorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestRegisteredTxFormattersMatchSemanticContract(t *testing.T) {
	ensureFormattersRegistered()

	const (
		owner     = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"
		other     = "akash1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxqy6045"
		provider  = "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"
		validator = "akashvaloper1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5axudam"
		contract  = "akash14hj2tavq8fpesdwxxcu44rty3hh90vhujrvcmstl4zr3txmfvw9s4se2wp"
	)

	expires := time.Date(2026, time.August, 11, 17, 30, 0, 0, time.UTC)
	inner, err := codectypes.NewAnyWithValue(&banktypes.MsgSend{
		FromAddress: owner,
		ToAddress:   other,
		Amount:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_250_000)),
	})
	require.NoError(t, err)
	allowance, err := codectypes.NewAnyWithValue(&feegrant.BasicAllowance{
		SpendLimit: sdk.NewCoins(sdk.NewInt64Coin("uakt", 9_500_000)),
		Expiration: &expires,
	})
	require.NoError(t, err)

	rate := math.LegacyMustNewDecFromStr("0.075")
	groupID := dv1.GroupID{Owner: owner, DSeq: 91_234_567, GSeq: 4}
	bidID := marketid.BidID{Owner: owner, DSeq: 91_234_567, GSeq: 4, OSeq: 2, Provider: provider}
	leaseID := marketid.LeaseID{Owner: owner, DSeq: 91_234_567, GSeq: 4, OSeq: 2, Provider: provider}
	cases := map[string]txFormatterSemanticCase{
		"BankMultiSend": {
			msg: &banktypes.MsgMultiSend{
				Inputs: []banktypes.Input{
					{Address: owner, Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 2_500_000))},
					{Address: other, Coins: sdk.NewCoins(sdk.NewInt64Coin("uatom", 750))},
				},
				Outputs: []banktypes.Output{
					{Address: provider, Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 2_000_000))},
					{Address: other, Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 500_000), sdk.NewInt64Coin("uatom", 750))},
				},
			},
			title: "Multi Send",
			want: txFormatterRows("Input 1", owner+"  2.5 AKT") +
				"\n" +
				txFormatterRows(
					"Input 2", other+"  750 uATOM",
					"Output 1", provider+"  2 AKT",
					"Output 2", other+"  500 mAKT, 750 uATOM",
				),
		},
		"DeploymentClosed": {
			msg:   &dv1beta.MsgCloseDeployment{ID: dv1.DeploymentID{Owner: owner, DSeq: 91_234_567}},
			title: "Deployment Closed",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
			),
		},
		"GroupClosed": {
			msg:   &dv1beta.MsgCloseGroup{ID: groupID},
			title: "Group Closed",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
			),
		},
		"GroupPaused": {
			msg:   &dv1beta.MsgPauseGroup{ID: groupID},
			title: "Group Paused",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
			),
		},
		"GroupStarted": {
			msg:   &dv1beta.MsgStartGroup{ID: groupID},
			title: "Group Started",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
			),
		},
		"BidCreated": {
			msg: &markettypes.MsgCreateBid{
				ID:    bidID,
				Price: sdk.NewDecCoinFromDec("uakt", math.LegacyNewDec(2_750_000)),
			},
			title: "Bid Created",
			want: txFormatterRows(
				"Provider", provider,
				"DSEQ", "91234567",
				"GSEQ", "4",
				"OSEQ", "2",
				"Price", "2.75 AKT",
			),
		},
		"BidClosed": {
			msg:   &markettypes.MsgCloseBid{ID: bidID},
			title: "Bid Closed",
			want: txFormatterRows(
				"Provider", provider,
				"DSEQ", "91234567",
				"GSEQ", "4",
				"OSEQ", "2",
			),
		},
		"LeaseCreated": {
			msg:   &markettypes.MsgCreateLease{BidID: bidID},
			title: "Lease Created",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
				"OSEQ", "2",
				"Provider", provider,
			),
		},
		"LeaseWithdrawn": {
			msg:   &markettypes.MsgWithdrawLease{ID: leaseID},
			title: "Lease Withdrawn",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
				"OSEQ", "2",
				"Provider", provider,
			),
		},
		"LeaseClosed": {
			msg:   &markettypes.MsgCloseLease{ID: leaseID},
			title: "Lease Closed",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"GSEQ", "4",
				"OSEQ", "2",
				"Provider", provider,
			),
		},
		"ProviderAttributes": {
			msg: &ptypes.MsgCreateProvider{
				Owner:   provider,
				HostURI: "https://provider.example.test:8443",
				Attributes: attrtypes.Attributes{
					{Key: "region", Value: "us-west"},
					{Key: "cpu.arch", Value: "amd64"},
				},
			},
			title: "Provider Created",
			want: txFormatterRows(
				"Owner", provider,
				"Host URI", "https://provider.example.test:8443",
				"Attributes", "region=us-west, cpu.arch=amd64",
			),
		},
		"ProviderUpdatedWithoutAttributes": {
			msg: &ptypes.MsgUpdateProvider{
				Owner:   provider,
				HostURI: "https://new-provider.example.test:8443",
			},
			title: "Provider Updated",
			want: txFormatterRows(
				"Owner", provider,
				"Host URI", "https://new-provider.example.test:8443",
			),
		},
		"CertificatePublishedEvent": {
			msg:      &certtypes.MsgCreateCertificate{Owner: owner},
			title:    "Certificate Published",
			response: txFormatterEventResponse(0, "akash.cert.v1.EventCertificateCreated", "serial", `"1844674407370955161"`),
			want: txFormatterRows(
				"Owner", owner,
				"Serial", "1844674407370955161",
			),
		},
		"CertificateRevoked": {
			msg: &certtypes.MsgRevokeCertificate{ID: certtypes.ID{
				Owner:  owner,
				Serial: "1844674407370955161",
			}},
			title: "Certificate Revoked",
			want: txFormatterRows(
				"Owner", owner,
				"Serial", "1844674407370955161",
			),
		},
		"AuditAttributesSigned": {
			msg: &audittypes.MsgSignProviderAttributes{
				Auditor: other,
				Owner:   provider,
				Attributes: attrtypes.Attributes{
					{Key: "host", Value: "verified"},
					{Key: "tier", Value: "community"},
				},
			},
			title: "Attributes Signed",
			want: txFormatterRows(
				"Auditor", other,
				"Provider", provider,
				"Attributes", "host=verified, tier=community",
			),
		},
		"AuditAttributesDeleted": {
			msg: &audittypes.MsgDeleteProviderAttributes{
				Auditor: other,
				Owner:   provider,
				Keys:    []string{"host", "tier"},
			},
			title: "Attributes Deleted",
			want: txFormatterRows(
				"Auditor", other,
				"Provider", provider,
				"Keys", "host, tier",
			),
		},
		"ValidatorCreatedPercentage": {
			msg: &stakingtypes.MsgCreateValidator{
				ValidatorAddress: validator,
				Description:      stakingtypes.Description{Moniker: "phoenix-one"},
				Value:            sdk.NewInt64Coin("uakt", 10_500_000),
				Commission: stakingtypes.CommissionRates{
					Rate: rate,
				},
			},
			title: "Validator Created",
			want: txFormatterRows(
				"Operator", validator,
				"Moniker", "phoenix-one",
				"Self-Delegation", "10.5 AKT",
				"Commission", "7.5%",
			),
		},
		"ValidatorEdited": {
			msg: &stakingtypes.MsgEditValidator{
				ValidatorAddress: validator,
				Description:      stakingtypes.Description{Moniker: "phoenix-two"},
				CommissionRate:   &rate,
			},
			title: "Validator Edited",
			want: txFormatterRows(
				"Operator", validator,
				"Moniker", "phoenix-two",
				"Commission", "7.5%",
			),
		},
		"RedelegateCompletionEvent": {
			msg: &stakingtypes.MsgBeginRedelegate{
				DelegatorAddress:    owner,
				ValidatorSrcAddress: validator,
				ValidatorDstAddress: "akashvaloper1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxuj24g4",
				Amount:              sdk.NewInt64Coin("uakt", 750_000),
			},
			title:    "Redelegate",
			response: txFormatterEventResponse(0, stakingtypes.EventTypeRedelegate, stakingtypes.AttributeKeyCompletionTime, "2026-08-25T17:30:00Z"),
			want: txFormatterRows(
				"Delegator", owner,
				"From", validator,
				"To", "akashvaloper1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxuj24g4",
				"Amount", "750 mAKT",
				"Completion", "2026-08-25T17:30:00Z",
			),
		},
		"UndelegateCompletionEvent": {
			msg: &stakingtypes.MsgUndelegate{
				DelegatorAddress: owner,
				ValidatorAddress: validator,
				Amount:           sdk.NewInt64Coin("uakt", 1_500_000),
			},
			title:    "Undelegate",
			response: txFormatterEventResponse(0, stakingtypes.EventTypeUnbond, stakingtypes.AttributeKeyCompletionTime, "2026-08-25T18:45:00Z"),
			want: txFormatterRows(
				"Delegator", owner,
				"Validator", validator,
				"Amount", "1.5 AKT",
				"Completion", "2026-08-25T18:45:00Z",
			),
		},
		"CancelUnbonding": {
			msg: &stakingtypes.MsgCancelUnbondingDelegation{
				DelegatorAddress: owner,
				ValidatorAddress: validator,
				Amount:           sdk.NewInt64Coin("uakt", 250_000),
			},
			title: "Cancel Unbonding",
			want: txFormatterRows(
				"Delegator", owner,
				"Validator", validator,
				"Amount", "250 mAKT",
			),
		},
		"WithdrawRewardsEvent": {
			msg: &distrtypes.MsgWithdrawDelegatorReward{
				DelegatorAddress: owner,
				ValidatorAddress: validator,
			},
			title:    "Withdraw Rewards",
			response: txFormatterEventResponse(0, distrtypes.EventTypeWithdrawRewards, sdk.AttributeKeyAmount, "1250000uakt,750uatom"),
			want: txFormatterRows(
				"Delegator", owner,
				"Validator", validator,
				"Rewards", "1.25 AKT, 750 uATOM",
			),
		},
		"WithdrawCommissionEvent": {
			msg:      &distrtypes.MsgWithdrawValidatorCommission{ValidatorAddress: validator},
			title:    "Withdraw Commission",
			response: txFormatterEventResponse(0, distrtypes.EventTypeWithdrawCommission, sdk.AttributeKeyAmount, "3250000uakt"),
			want: txFormatterRows(
				"Validator", validator,
				"Commission", "3.25 AKT",
			),
		},
		"SetWithdrawAddress": {
			msg: &distrtypes.MsgSetWithdrawAddress{
				DelegatorAddress: owner,
				WithdrawAddress:  other,
			},
			title: "Set Withdraw Address",
			want: txFormatterRows(
				"Delegator", owner,
				"Withdraw Address", other,
			),
		},
		"FundCommunityPool": {
			msg: &distrtypes.MsgFundCommunityPool{
				Depositor: owner,
				Amount:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 8_250_000)),
			},
			title: "Fund Community Pool",
			want: txFormatterRows(
				"Depositor", owner,
				"Amount", "8.25 AKT",
			),
		},
		"FundValidatorRewards": {
			msg: &distrtypes.MsgDepositValidatorRewardsPool{
				Depositor:        owner,
				ValidatorAddress: validator,
				Amount:           sdk.NewCoins(sdk.NewInt64Coin("uakt", 4_500_000)),
			},
			title: "Fund Validator Rewards",
			want: txFormatterRows(
				"Depositor", owner,
				"Validator", validator,
				"Amount", "4.5 AKT",
			),
		},
		"ProposalSubmittedEvent": {
			msg: &govv1.MsgSubmitProposal{
				Proposer:       owner,
				Title:          "Raise community-pool transparency",
				InitialDeposit: sdk.NewCoins(sdk.NewInt64Coin("uakt", 5_500_000)),
			},
			title:    "Proposal Submitted",
			response: txFormatterEventResponse(0, govtypes.EventTypeSubmitProposal, govtypes.AttributeKeyProposalID, "412"),
			want: txFormatterRows(
				"Proposer", owner,
				"Proposal ID", "412",
				"Title", "Raise community-pool transparency",
				"Deposit", "5.5 AKT",
			),
		},
		"ProposalDeposit": {
			msg: &govv1.MsgDeposit{
				ProposalId: 412,
				Depositor:  other,
				Amount:     sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_750_000)),
			},
			title: "Proposal Deposit",
			want: txFormatterRows(
				"Depositor", other,
				"Proposal ID", "412",
				"Amount", "1.75 AKT",
			),
		},
		"WeightedVoteAllOptions": {
			msg: &govv1.MsgVoteWeighted{
				ProposalId: 412,
				Voter:      owner,
				Options: []*govv1.WeightedVoteOption{
					{Option: govv1.OptionYes, Weight: "0.4"},
					{Option: govv1.OptionNo, Weight: "0.2"},
					{Option: govv1.OptionAbstain, Weight: "0.1"},
					{Option: govv1.OptionNoWithVeto, Weight: "0.2"},
					{Option: govv1.VoteOption(99), Weight: "0.1"},
				},
			},
			title: "Weighted Vote",
			want: txFormatterRows(
				"Voter", owner,
				"Proposal ID", "412",
				"Options", "Yes=0.4, No=0.2, Abstain=0.1, NoWithVeto=0.2, 99=0.1",
			),
		},
		"AuthzExecRecurses": {
			msg:   &authz.MsgExec{Grantee: other, Msgs: []*codectypes.Any{inner}},
			title: "Authz Exec",
			want: txFormatterRows("Grantee", other) +
				"\n  Message 1: Send\n" +
				txFormatterRows(
					"From", owner,
					"To", other,
					"Amount", "1.25 AKT",
				),
		},
		"AuthorizationGranted": {
			msg: &authz.MsgGrant{
				Granter: owner,
				Grantee: other,
				Grant: authz.Grant{
					Authorization: inner,
					Expiration:    &expires,
				},
			},
			title: "Authorization Granted",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
				"Type", "MsgSend",
				"Expiration", "2026-08-11T17:30:00Z",
			),
		},
		"AuthorizationRevoked": {
			msg: &authz.MsgRevoke{
				Granter:    owner,
				Grantee:    other,
				MsgTypeUrl: "/cosmos.bank.v1beta1.MsgSend",
			},
			title: "Authorization Revoked",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
				"Msg Type", "MsgSend",
			),
		},
		"FeeAllowanceExpiration": {
			msg: &feegrant.MsgGrantAllowance{
				Granter:   owner,
				Grantee:   other,
				Allowance: allowance,
			},
			title: "Fee Allowance Granted",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
				"Type", "BasicAllowance",
				"Expiration", "2026-08-11T17:30:00Z",
			),
		},
		"FeeAllowanceRevoked": {
			msg: &feegrant.MsgRevokeAllowance{
				Granter: owner,
				Grantee: other,
			},
			title: "Fee Allowance Revoked",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
			),
		},
		"PeriodicVestingTotal": {
			msg: &vestingtypes.MsgCreatePeriodicVestingAccount{
				FromAddress: owner,
				ToAddress:   other,
				VestingPeriods: []vestingtypes.Period{
					{Length: 3600, Amount: sdk.NewCoins(sdk.NewInt64Coin("uakt", 500_000))},
					{Length: 7200, Amount: sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000_000))},
				},
			},
			title: "Periodic Vesting Account Created",
			want: txFormatterRows(
				"From", owner,
				"To", other,
				"Periods", "2 (1.5 AKT total)",
			),
		},
		"EscrowDeploymentIdentity": {
			msg: &escrowtypes.MsgAccountDeposit{
				Signer: other,
				ID: ev1.Account{
					Scope: ev1.ScopeDeployment,
					XID:   dv1.DeploymentID{Owner: owner, DSeq: 91_234_567}.String(),
				},
				Deposit: dtypes.Deposit{Amount: sdk.NewInt64Coin("uakt", 2_750_000)},
			},
			title: "Escrow Deposit",
			want: txFormatterRows(
				"Owner", owner,
				"DSEQ", "91234567",
				"Amount", "2.75 AKT",
			),
		},
		"ValidatorUnjailed": {
			msg:   &slashingtypes.MsgUnjail{ValidatorAddr: validator},
			title: "Unjail",
			want:  txFormatterRows("Validator", validator),
		},
		"VestingAccountCreated": {
			msg: &vestingtypes.MsgCreateVestingAccount{
				FromAddress: owner,
				ToAddress:   other,
				Amount:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 12_500_000)),
				EndTime:     time.Date(2027, time.August, 11, 17, 30, 0, 0, time.UTC).Unix(),
			},
			title: "Vesting Account Created",
			want: txFormatterRows(
				"From", owner,
				"To", other,
				"Amount", "12.5 AKT",
				"End Time", "2027-08-11T17:30:00Z",
			),
		},
		"PermanentLockedAccountCreated": {
			msg: &vestingtypes.MsgCreatePermanentLockedAccount{
				FromAddress: owner,
				ToAddress:   other,
				Amount:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 6_750_000)),
			},
			title: "Permanent Locked Account Created",
			want: txFormatterRows(
				"From", owner,
				"To", other,
				"Amount", "6.75 AKT",
			),
		},
		"SoftwareUpgrade": {
			msg: &upgradetypes.MsgSoftwareUpgrade{
				Authority: owner,
				Plan:      upgradetypes.Plan{Name: "phoenix", Height: 23_456_789},
			},
			title: "Software Upgrade",
			want: txFormatterRows(
				"Authority", owner,
				"Name", "phoenix",
				"Height", "23,456,789",
			),
		},
		"UpgradeCancelled": {
			msg:   &upgradetypes.MsgCancelUpgrade{Authority: owner},
			title: "Upgrade Cancelled",
			want:  txFormatterRows("Authority", owner),
		},
		"InvariantVerified": {
			msg: &crisistypes.MsgVerifyInvariant{
				Sender:              owner,
				InvariantModuleName: "distribution",
				InvariantRoute:      "nonnegative-outstanding",
			},
			title: "Verify Invariant",
			want: txFormatterRows(
				"Sender", owner,
				"Module", "distribution",
				"Route", "nonnegative-outstanding",
			),
		},
		"WasmStoredCodeEvent": {
			msg:      &wasmtypes.MsgStoreCode{Sender: owner},
			title:    "Code Stored",
			response: txFormatterEventResponse(0, wasmtypes.EventTypeStoreCode, wasmtypes.AttributeKeyCodeID, "108"),
			want: txFormatterRows(
				"Sender", owner,
				"Code ID", "108",
			),
		},
		"WasmInstantiationEvent": {
			msg: &wasmtypes.MsgInstantiateContract{
				Sender: owner,
				CodeID: 108,
				Label:  "settlement-router",
			},
			title:    "Contract Instantiated",
			response: txFormatterEventResponse(0, wasmtypes.EventTypeInstantiate, wasmtypes.AttributeKeyContractAddr, contract),
			want: txFormatterRows(
				"Sender", owner,
				"Code ID", "108",
				"Contract", contract,
				"Label", "settlement-router",
				"Admin", "none",
			),
		},
		"WasmInstantiation2": {
			msg: &wasmtypes.MsgInstantiateContract2{
				Sender: owner,
				Admin:  other,
				CodeID: 109,
				Label:  "predictable-router",
				Salt:   []byte{0x00, 0x7f, 0xa5},
			},
			title:    "Contract Instantiated",
			response: txFormatterEventResponse(0, wasmtypes.EventTypeInstantiate, wasmtypes.AttributeKeyContractAddr, contract),
			want: txFormatterRows(
				"Sender", owner,
				"Code ID", "109",
				"Contract", contract,
				"Label", "predictable-router",
				"Admin", other,
				"Salt", "007fa5",
			),
		},
		"WasmContractMigrated": {
			msg: &wasmtypes.MsgMigrateContract{
				Sender:   owner,
				Contract: contract,
				CodeID:   110,
			},
			title: "Contract Migrated",
			want: txFormatterRows(
				"Sender", owner,
				"Contract", contract,
				"New Code ID", "110",
			),
		},
		"WasmAdminUpdated": {
			msg: &wasmtypes.MsgUpdateAdmin{
				Sender:   owner,
				Contract: contract,
				NewAdmin: other,
			},
			title: "Contract Admin Updated",
			want: txFormatterRows(
				"Sender", owner,
				"Contract", contract,
				"New Admin", other,
			),
		},
		"WasmAdminCleared": {
			msg: &wasmtypes.MsgClearAdmin{
				Sender:   owner,
				Contract: contract,
			},
			title: "Contract Admin Cleared",
			want: txFormatterRows(
				"Sender", owner,
				"Contract", contract,
			),
		},
		"WasmInstantiateConfigUpdated": {
			msg: &wasmtypes.MsgUpdateInstantiateConfig{
				Sender: owner,
				CodeID: 110,
			},
			title: "Instantiate Config Updated",
			want: txFormatterRows(
				"Sender", owner,
				"Code ID", "110",
			),
		},
		"WasmContractLabelSet": {
			msg: &wasmtypes.MsgUpdateContractLabel{
				Sender:   owner,
				Contract: contract,
				NewLabel: "settlement-router-v2",
			},
			title: "Contract Label Set",
			want: txFormatterRows(
				"Sender", owner,
				"Contract", contract,
				"Label", "settlement-router-v2",
			),
		},
		"OraclePriceFeed": {
			msg: &oracletypes.MsgAddPriceEntry{
				Signer: owner,
				ID: oracletypes.DataID{
					Denom:     "uakt",
					BaseDenom: "usd",
				},
				Price: math.LegacyMustNewDecFromStr("0.53600423"),
			},
			title: "Price Feed",
			want: txFormatterRows(
				"Sender", owner,
				"Asset", "uakt",
				"Base", "usd",
				"Price", "0.536004230000000000",
			),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			formatter, ok := LookupTx(tc.msg)
			require.True(t, ok, "formatter registered for %T", tc.msg)
			require.Equal(t, tc.title, formatter.Title())

			response := tc.response
			if response == nil {
				response = &sdk.TxResponse{}
			}

			var output bytes.Buffer
			require.NoError(t, formatter.FormatTx(&output, nil, tc.context, tc.msg, response, 0))
			require.Equal(t, tc.want, ansi.Strip(output.String()))
		})
	}
}

func TestRegisteredTxFormattersHandleSparseBoundaries(t *testing.T) {
	ensureFormattersRegistered()

	const (
		owner    = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"
		other    = "akash1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxqy6045"
		provider = "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"
	)

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	banktypes.RegisterInterfaces(interfaceRegistry)
	feegrant.RegisterInterfaces(interfaceRegistry)
	protoCodec := codec.NewProtoCodec(interfaceRegistry)
	codecContext := sdkclient.Context{Codec: protoCodec}

	inner, err := codectypes.NewAnyWithValue(&banktypes.MsgSend{
		FromAddress: owner,
		ToAddress:   other,
		Amount:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 500)),
	})
	require.NoError(t, err)
	rawInner := &codectypes.Any{TypeUrl: inner.TypeUrl, Value: append([]byte(nil), inner.Value...)}

	expires := time.Date(2026, time.September, 1, 9, 15, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	allowance, err := codectypes.NewAnyWithValue(&feegrant.BasicAllowance{Expiration: &expires})
	require.NoError(t, err)
	rawAllowance := &codectypes.Any{TypeUrl: allowance.TypeUrl, Value: append([]byte(nil), allowance.Value...)}

	unregistered, err := codectypes.NewAnyWithValue(&banktypes.MsgUpdateParams{})
	require.NoError(t, err)

	cases := map[string]txFormatterSemanticCase{
		"ProviderUpdateAttributes": {
			msg: &ptypes.MsgUpdateProvider{
				Owner:      provider,
				HostURI:    "https://provider.example.test:8443",
				Attributes: attrtypes.Attributes{{Key: "region", Value: "us-south"}},
			},
			title: "Provider Updated",
			want: txFormatterRows(
				"Owner", provider,
				"Host URI", "https://provider.example.test:8443",
				"Attributes", "region=us-south",
			),
		},
		"ValidatorEditOmitsUnchangedFields": {
			msg: &stakingtypes.MsgEditValidator{
				ValidatorAddress: "akashvaloper1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5axudam",
				Description:      stakingtypes.Description{Moniker: stakingtypes.DoNotModifyDesc},
			},
			title: "Validator Edited",
			want:  txFormatterRows("Operator", "akashvaloper1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5axudam"),
		},
		"AuthzGrantOmitsUnsetOptionalFields": {
			msg:   &authz.MsgGrant{Granter: owner, Grantee: other},
			title: "Authorization Granted",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
			),
		},
		"AuthzExecDecodesWireAny": {
			msg:     &authz.MsgExec{Grantee: other, Msgs: []*codectypes.Any{rawInner}},
			title:   "Authz Exec",
			context: codecContext,
			want: txFormatterRows("Grantee", other) +
				"\n  Message 1: Send\n" +
				txFormatterRows(
					"From", owner,
					"To", other,
					"Amount", "500 uAKT",
				),
		},
		"AuthzExecReportsMalformedAny": {
			msg: &authz.MsgExec{Grantee: other, Msgs: []*codectypes.Any{
				nil,
				{TypeUrl: "/cosmos.bank.v1beta1.MsgSend", Value: []byte{0xff}},
			}},
			title:   "Authz Exec",
			context: codecContext,
			want: txFormatterRows(
				"Grantee", other,
				"Message 1", "unknown (cannot decode)",
				"Message 2", "MsgSend (cannot decode)",
			),
		},
		"AuthzExecNamesUnregisteredAny": {
			msg:   &authz.MsgExec{Grantee: other, Msgs: []*codectypes.Any{unregistered}},
			title: "Authz Exec",
			want: txFormatterRows(
				"Grantee", other,
				"Message 1", "MsgUpdateParams",
			),
		},
		"FeegrantDecodesWireExpiration": {
			msg: &feegrant.MsgGrantAllowance{
				Granter:   owner,
				Grantee:   other,
				Allowance: rawAllowance,
			},
			title:   "Fee Allowance Granted",
			context: codecContext,
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
				"Type", "BasicAllowance",
				"Expiration", "2026-09-01T07:15:00Z",
			),
		},
		"FeegrantOmitsNilAllowance": {
			msg:   &feegrant.MsgGrantAllowance{Granter: owner, Grantee: other},
			title: "Fee Allowance Granted",
			want: txFormatterRows(
				"Granter", owner,
				"Grantee", other,
			),
		},
		"EscrowPreservesMalformedAccountIdentity": {
			msg: &escrowtypes.MsgAccountDeposit{
				Signer:  other,
				ID:      ev1.Account{Scope: ev1.ScopeDeployment, XID: "malformed"},
				Deposit: dtypes.Deposit{Amount: sdk.NewInt64Coin("uakt", 500)},
			},
			title: "Escrow Deposit",
			want: txFormatterRows(
				"Signer", other,
				"Account", "malformed",
				"Amount", "500 uAKT",
			),
		},
		"Instantiate2UsesNoneForEmptyAdmin": {
			msg: &wasmtypes.MsgInstantiateContract2{
				Sender: owner,
				CodeID: 12,
				Label:  "no-admin",
				Salt:   []byte{0x01},
			},
			title: "Contract Instantiated",
			want: txFormatterRows(
				"Sender", owner,
				"Code ID", "12",
				"Label", "no-admin",
				"Admin", "none",
				"Salt", "01",
			),
		},
		"OracleUnsetPriceIsZero": {
			msg: &oracletypes.MsgAddPriceEntry{
				Signer: owner,
				ID:     oracletypes.DataID{Denom: "uakt", BaseDenom: "usd"},
			},
			title: "Price Feed",
			want: txFormatterRows(
				"Sender", owner,
				"Asset", "uakt",
				"Base", "usd",
				"Price", "0.000000000000000000",
			),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			formatter, ok := LookupTx(tc.msg)
			require.True(t, ok, "formatter registered for %T", tc.msg)
			require.Equal(t, tc.title, formatter.Title())

			var output bytes.Buffer
			require.NoError(t, formatter.FormatTx(&output, nil, tc.context, tc.msg, &sdk.TxResponse{}, 0))
			require.Equal(t, tc.want, ansi.Strip(output.String()))
		})
	}
}

func TestTxFormatterEventAndIdentityBoundaries(t *testing.T) {
	const owner = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"

	response := &sdk.TxResponse{
		Logs: sdk.ABCIMessageLogs{
			{MsgIndex: 1, Events: sdk.StringEvents{{Type: "wanted", Attributes: []sdk.Attribute{{Key: "value", Value: "wrong-message"}}}}},
			{MsgIndex: 0, Events: sdk.StringEvents{
				{Type: "other", Attributes: []sdk.Attribute{{Key: "value", Value: "wrong-event"}}},
				{Type: "wanted", Attributes: []sdk.Attribute{{Key: "other", Value: "wrong-key"}}},
			}},
		},
		Events: []abci.Event{
			{
				Type: "wanted",
				Attributes: []abci.EventAttribute{
					{Key: "msg_index", Value: "1"},
					{Key: "value", Value: "wrong-aggregate-message"},
				},
			},
			{
				Type: "wanted",
				Attributes: []abci.EventAttribute{
					{Key: "msg_index", Value: "0"},
					{Key: "value", Value: `"aggregate"`},
				},
			},
		},
	}

	require.Equal(t, "aggregate", txEventAttribute(response, 0, "wanted", "value"))
	indexed := responseForMultiMessage(response)
	require.Len(t, indexed.Events, 2)
	require.Equal(t, response.Events, indexed.Events)
	unindexed := &sdk.TxResponse{Events: []abci.Event{{
		Type:       "wanted",
		Attributes: []abci.EventAttribute{{Key: "value", Value: "single-only"}},
	}}}
	require.Equal(t, "single-only", txEventAttribute(unindexed, 0, "wanted", "value"))
	require.Empty(t, txEventAttribute(responseForMultiMessage(unindexed), 0, "wanted", "value"))
	require.Empty(t, txEventAttribute(nil, 0, "wanted", "value"))
	require.Empty(t, txEventAttribute(response, 0, "missing", "value"))
	require.Equal(t, "not-json", normalizeTxEventValue("not-json"))
	require.Equal(t, "not-coins", formatTxEventCoins("not-coins"))

	uncached := &codectypes.Any{TypeUrl: "/cosmos.bank.v1beta1.MsgSend"}
	_, ok := unpackTxMessage(sdkclient.Context{}, uncached)
	require.False(t, ok)
	_, ok = unpackFeeAllowance(sdkclient.Context{}, nil)
	require.False(t, ok)
	_, ok = unpackFeeAllowance(sdkclient.Context{}, uncached)
	require.False(t, ok)

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	feegrant.RegisterInterfaces(interfaceRegistry)
	codecContext := sdkclient.Context{Codec: codec.NewProtoCodec(interfaceRegistry)}
	_, ok = unpackFeeAllowance(codecContext, uncached)
	require.False(t, ok)
	msgAny, err := codectypes.NewAnyWithValue(&banktypes.MsgSend{})
	require.NoError(t, err)
	_, ok = unpackFeeAllowance(sdkclient.Context{}, msgAny)
	require.False(t, ok)

	cases := map[string]ev1.Account{
		"WrongScope":  {Scope: ev1.ScopeBid, XID: owner + "/42"},
		"EmptyOwner":  {Scope: ev1.ScopeDeployment, XID: "/42"},
		"EmptyDSEQ":   {Scope: ev1.ScopeDeployment, XID: owner + "/"},
		"InvalidDSEQ": {Scope: ev1.ScopeDeployment, XID: owner + "/not-a-number"},
		"ZeroDSEQ":    {Scope: ev1.ScopeDeployment, XID: owner + "/0"},
	}
	for name, account := range cases {
		t.Run(name, func(t *testing.T) {
			gotOwner, gotDSEQ, ok := escrowDeploymentIdentity(account)
			require.False(t, ok)
			require.Empty(t, gotOwner)
			require.Zero(t, gotDSEQ)
		})
	}
}

func TestAuthzExecPropagatesNestedFormatterWriterError(t *testing.T) {
	ensureFormattersRegistered()

	const owner = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"
	inner, err := codectypes.NewAnyWithValue(&bmetypes.MsgBurnMint{
		Owner:       owner,
		CoinsToBurn: sdk.NewInt64Coin("uakt", 1_000_000),
		DenomToMint: "uact",
	})
	require.NoError(t, err)

	msg := &authz.MsgExec{Grantee: owner, Msgs: []*codectypes.Any{inner}}
	formatter, ok := LookupTx(msg)
	require.True(t, ok)

	wantErr := errors.New("writer failed")
	require.ErrorIs(t, formatter.FormatTx(txFormatterErrorWriter{err: wantErr}, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0), wantErr)
}

func TestAuthzExecOmitsAmbiguousInnerEventValues(t *testing.T) {
	ensureFormattersRegistered()

	const (
		firstProposer  = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu"
		secondProposer = "akash1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxqy6045"
	)

	first, err := codectypes.NewAnyWithValue(&govv1.MsgSubmitProposal{Proposer: firstProposer})
	require.NoError(t, err)
	second, err := codectypes.NewAnyWithValue(&govv1.MsgSubmitProposal{Proposer: secondProposer})
	require.NoError(t, err)

	msg := &authz.MsgExec{Msgs: []*codectypes.Any{first, second}}
	response := &sdk.TxResponse{Logs: sdk.ABCIMessageLogs{{
		MsgIndex: 0,
		Events: sdk.StringEvents{
			{Type: govtypes.EventTypeSubmitProposal, Attributes: []sdk.Attribute{{Key: govtypes.AttributeKeyProposalID, Value: "41"}}},
			{Type: govtypes.EventTypeSubmitProposal, Attributes: []sdk.Attribute{{Key: govtypes.AttributeKeyProposalID, Value: "42"}}},
		},
	}}}

	formatter, ok := LookupTx(msg)
	require.True(t, ok)

	var output bytes.Buffer
	require.NoError(t, formatter.FormatTx(&output, nil, sdkclient.Context{}, msg, response, 0))
	require.Contains(t, output.String(), firstProposer)
	require.Contains(t, output.String(), secondProposer)
	require.NotContains(t, output.String(), "Proposal ID")
}

func txFormatterRows(fields ...string) string {
	if len(fields)%2 != 0 {
		panic("txFormatterRows requires key/value pairs")
	}

	var output strings.Builder
	for i := 0; i < len(fields); i += 2 {
		fmt.Fprintf(&output, "  %-20s %s\n", fields[i]+":", fields[i+1])
	}

	return output.String()
}

func txFormatterEventResponse(msgIndex uint32, eventType string, attributes ...string) *sdk.TxResponse {
	if len(attributes)%2 != 0 {
		panic("txFormatterEventResponse requires key/value pairs")
	}

	event := sdk.StringEvent{Type: eventType}
	for i := 0; i < len(attributes); i += 2 {
		event.Attributes = append(event.Attributes, sdk.Attribute{Key: attributes[i], Value: attributes[i+1]})
	}

	return &sdk.TxResponse{Logs: sdk.ABCIMessageLogs{{MsgIndex: msgIndex, Events: sdk.StringEvents{event}}}}
}
