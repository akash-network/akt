package flags

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pkg.akt.dev/go/node/types/constants"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"pkg.akt.dev/akt/internal/output"
)

const (
	// DefaultGasAdjustment is applied to gas estimates to avoid tx execution
	// failures due to state changes that might occur between the tx simulation
	// and the actual run.
	DefaultGasAdjustment = constants.DefaultGasAdjustment
	GasFlagAuto          = constants.DefaultGas
	DefaultGasLimit      = 200000

	DefaultKeyringBackend = keyring.BackendOS

	// BroadcastSync defines a tx broadcasting mode where the client waits for
	// a CheckTx execution response only.
	BroadcastSync = "sync"
	// BroadcastAsync defines a tx broadcasting mode where the client returns
	// immediately.
	BroadcastAsync = "async"

	BroadcastBlock = "block"

	// SignModeDirect is the value of the --sign-mode flag for SIGN_MODE_DIRECT
	SignModeDirect = "direct"
	// SignModeLegacyAminoJSON is the value of the --sign-mode flag for SIGN_MODE_LEGACY_AMINO_JSON
	SignModeLegacyAminoJSON = "amino-json"
	// SignModeDirectAux is the value of the --sign-mode flag for SIGN_MODE_DIRECT_AUX
	SignModeDirectAux = "direct-aux"
	// SignModeEIP191 is the value of the --sign-mode flag for SIGN_MODE_EIP_191
	SignModeEIP191 = "eip-191"
)

const (
	FlagGenesisTime = flagdefs.FlagGenesisTime
	FlagGenTxDir    = flagdefs.FlagGenTxDir
	FlagRecover     = flagdefs.FlagRecover
	// FlagDefaultBondDenom defines the default denom to use in the genesis file.
	FlagDefaultBondDenom = flagdefs.FlagDefaultBondDenom
	// FlagConsensusKeyAlgo defines the algorithm to use for the consensus signing key.
	FlagConsensusKeyAlgo = flagdefs.FlagConsensusKeyAlgo

	FlagDenom                     = flagdefs.FlagDenom
	FlagVestingStart              = flagdefs.FlagVestingStart
	FlagVestingEnd                = flagdefs.FlagVestingEnd
	FlagVestingAmt                = flagdefs.FlagVestingAmt
	FlagAppendMode                = flagdefs.FlagAppendMode
	FlagEvents                    = flagdefs.FlagEvents
	FlagType                      = flagdefs.FlagType
	FlagMultisig                  = flagdefs.FlagMultisig
	FlagSkipSignatureVerification = flagdefs.FlagSkipSignatureVerification
	FlagOverwrite                 = flagdefs.FlagOverwrite
	FlagSigOnly                   = flagdefs.FlagSigOnly
	FlagAmino                     = flagdefs.FlagAmino
	FlagNoAutoIncrement           = flagdefs.FlagNoAutoIncrement
	FlagAppend                    = flagdefs.FlagAppendMode
	FlagTitle                     = flagdefs.FlagTitle
	FlagMetadata                  = flagdefs.FlagMetadata
	FlagSummary                   = flagdefs.FlagSummary
	FlagExpedited                 = flagdefs.FlagExpedited
	FlagNoValidate                = flagdefs.FlagNoValidate
	FlagDaemonName                = flagdefs.FlagDaemonName
	FlagPeriod                    = flagdefs.FlagPeriod
	FlagPeriodLimit               = flagdefs.FlagPeriodLimit
	FlagAllowedMsgs               = flagdefs.FlagAllowedMsgs
	FlagMsgType                   = flagdefs.FlagMsgType
	FlagAllowedValidators         = flagdefs.FlagAllowedValidators
	FlagDenyValidators            = flagdefs.FlagDenyValidators
	FlagAllowList                 = flagdefs.FlagAllowList
	FlagStatus                    = flagdefs.FlagStatus
	FlagState                     = flagdefs.FlagState
	FlagOwner                     = flagdefs.FlagOwner
	FlagDSeq                      = flagdefs.FlagDSeq
	FlagGSeq                      = flagdefs.FlagGSeq
	FlagOSeq                      = flagdefs.FlagOSeq
	FlagProvider                  = flagdefs.FlagProvider
	FlagClosedReason              = flagdefs.FlagClosedReason
	FlagSerial                    = flagdefs.FlagSerial
	FlagPrice                     = flagdefs.FlagPrice
	FlagDeposit                   = flagdefs.FlagDeposit
	FlagDepositSources            = flagdefs.FlagDepositSources
	FlagExpiration                = flagdefs.FlagExpiration
	FlagSpendLimit                = flagdefs.FlagSpendLimit
	FlagScope                     = flagdefs.FlagScope
	FlagHome                      = flagdefs.FlagHome
	FlagKeyringDir                = flagdefs.FlagKeyringDir
	FlagUseLedger                 = flagdefs.FlagUseLedger
	FlagChainID                   = flagdefs.FlagChainID
	FlagNode                      = flagdefs.FlagNode
	FlagGRPC                      = flagdefs.FlagGRPC
	FlagGRPCInsecure              = flagdefs.FlagGRPCInsecure
	FlagHeight                    = flagdefs.FlagHeight
	FlagGasAdjustment             = flagdefs.FlagGasAdjustment
	FlagFrom                      = flagdefs.FlagFrom
	FlagName                      = flagdefs.FlagName
	FlagAccountNumber             = flagdefs.FlagAccountNumber
	FlagSequence                  = flagdefs.FlagSequence
	FlagNote                      = flagdefs.FlagNote
	FlagFees                      = flagdefs.FlagFees
	FlagGas                       = flagdefs.FlagGas
	FlagGasPrices                 = flagdefs.FlagGasPrices
	FlagBroadcastMode             = flagdefs.FlagBroadcastMode
	FlagDryRun                    = flagdefs.FlagDryRun
	FlagGenerateOnly              = flagdefs.FlagGenerateOnly
	FlagOffline                   = flagdefs.FlagOffline
	FlagModulesToExport           = flagdefs.FlagModulesToExport
	FlagOutputDocument            = flagdefs.FlagOutputDocument // inspired by wget -O
	FlagForZeroHeight             = flagdefs.FlagForZeroHeight
	FlagJailAllowedAddrs          = flagdefs.FlagJailAllowedAddrs
	FlagSkipConfirmation          = flagdefs.FlagSkipConfirmation
	FlagProve                     = flagdefs.FlagProve
	FlagKeyringBackend            = flagdefs.FlagKeyringBackend
	FlagPage                      = flagdefs.FlagPage
	FlagLimit                     = flagdefs.FlagLimit
	FlagSignMode                  = flagdefs.FlagSignMode
	FlagPageKey                   = flagdefs.FlagPageKey
	FlagOffset                    = flagdefs.FlagOffset
	FlagCountTotal                = flagdefs.FlagCountTotal
	FlagTimeoutHeight             = flagdefs.FlagTimeoutHeight
	FlagKeyType                   = flagdefs.FlagKeyType
	FlagFeePayer                  = flagdefs.FlagFeePayer
	FlagFeeGranter                = flagdefs.FlagFeeGranter
	FlagReverse                   = flagdefs.FlagReverse
	FlagTip                       = flagdefs.FlagTip
	FlagAux                       = flagdefs.FlagAux
	FlagInitHeight                = flagdefs.FlagInitHeight
	FlagDelayed                   = flagdefs.FlagDelayed
	FlagSkipRPCInit               = flagdefs.FlagSkipRPCInit
	// FlagOutput is the flag to set the output format.
	// This differs from FlagOutputDocument that is used to set the output file.
	FlagOutput = flagdefs.FlagOutput

	// OutputPretty is the value for --output that renders human-friendly pretty output.
	OutputPretty = "pretty"
	// OutputJSON is the value for --output that renders machine-readable JSON.
	OutputJSON = "json"
	// OutputYAML is the value for --output that renders machine-readable YAML.
	OutputYAML = "yaml"
	FlagSplit  = flagdefs.FlagSplit

	TimeoutDuration  = flagdefs.FlagTimeoutDuration
	FlagUnordered    = flagdefs.FlagUnordered
	FlagKeyAlgorithm = flagdefs.FlagKeyAlgorithm

	// CometBFT logging flags

	FlagLogLevel     = flagdefs.FlagLogLevel
	FlagLogFormat    = flagdefs.FlagLogFormat
	FlagLogNoColor   = flagdefs.FlagLogNoColor
	FlagLogColor     = flagdefs.FlagLogColor
	FlagLogTimestamp = flagdefs.FlagLogTimestamp
	FlagTrace        = flagdefs.FlagTrace

	FlagAddressValidator    = flagdefs.FlagAddressValidator
	FlagAddressValidatorSrc = flagdefs.FlagAddressValidatorSrc
	FlagAddressValidatorDst = flagdefs.FlagAddressValidatorDst
	FlagPubKey              = flagdefs.FlagPubKey
	FlagAmount              = flagdefs.FlagAmount
	FlagSharesAmount        = flagdefs.FlagSharesAmount
	FlagSharesFraction      = flagdefs.FlagSharesFraction

	FlagMoniker         = flagdefs.FlagMoniker
	FlagEditMoniker     = flagdefs.FlagEditMoniker
	FlagIdentity        = flagdefs.FlagIdentity
	FlagWebsite         = flagdefs.FlagWebsite
	FlagSecurityContact = flagdefs.FlagSecurityContact
	FlagDetails         = flagdefs.FlagDetails

	FlagCommission              = flagdefs.FlagCommission
	FlagCommissionRate          = flagdefs.FlagCommissionRate
	FlagCommissionMaxRate       = flagdefs.FlagCommissionMaxRate
	FlagCommissionMaxChangeRate = flagdefs.FlagCommissionMaxChangeRate
	FlagMinSelfDelegation       = flagdefs.FlagMinSelfDelegation

	FlagGenesisFormat = flagdefs.FlagGenesisFormat
	FlagNodeID        = flagdefs.FlagNodeID
	FlagIP            = flagdefs.FlagIP
	FlagP2PPort       = flagdefs.FlagP2PPort

	FlagNoChecksumRequired = flagdefs.FlagNoChecksumRequired
	FlagAuthority          = flagdefs.FlagAuthority

	FlagModuleName = flagdefs.FlagModuleName

	// Tendermint full-node start flags

	FlagWithTendermint     = flagdefs.FlagWithTendermint
	FlagWithComet          = flagdefs.FlagWithComet
	FlagAddress            = flagdefs.FlagAddress
	FlagTransport          = flagdefs.FlagTransport
	FlagTraceStore         = flagdefs.FlagTraceStore
	FlagCPUProfile         = flagdefs.FlagCPUProfile
	FlagMinGasPrices       = flagdefs.FlagMinGasPrices
	FlagQueryGasLimit      = flagdefs.FlagQueryGasLimit
	FlagHaltHeight         = flagdefs.FlagHaltHeight
	FlagHaltTime           = flagdefs.FlagHaltTime
	FlagInterBlockCache    = flagdefs.FlagInterBlockCache
	FlagUnsafeSkipUpgrades = flagdefs.FlagUnsafeSkipUpgrades
	FlagInvCheckPeriod     = flagdefs.FlagInvCheckPeriod

	FlagPruning             = flagdefs.FlagPruning
	FlagPruningKeepRecent   = flagdefs.FlagPruningKeepRecent
	FlagPruningInterval     = flagdefs.FlagPruningInterval
	FlagIndexEvents         = flagdefs.FlagIndexEvents
	FlagMinRetainBlocks     = flagdefs.FlagMinRetainBlocks
	FlagIAVLCacheSize       = flagdefs.FlagIAVLCacheSize
	FlagDisableIAVLFastNode = flagdefs.FlagDisableIAVLFastNode
	FlagIAVLLazyLoading     = flagdefs.FlagIAVLLazyLoading
	FlagIAVLSyncPruning     = flagdefs.FlagIAVLSyncPruning
	FlagShutdownGrace       = flagdefs.FlagShutdownGrace

	// state sync-related flags

	FlagStateSyncSnapshotInterval   = flagdefs.FlagStateSyncSnapshotInterval
	FlagStateSyncSnapshotKeepRecent = flagdefs.FlagStateSyncSnapshotKeepRecent

	// api-related flags

	FlagAPIEnable             = flagdefs.FlagAPIEnable
	FlagAPISwagger            = flagdefs.FlagAPISwagger
	FlagAPIAddress            = flagdefs.FlagAPIAddress
	FlagAPIMaxOpenConnections = flagdefs.FlagAPIMaxOpenConnections
	FlagRPCReadTimeout        = flagdefs.FlagRPCReadTimeout
	FlagRPCWriteTimeout       = flagdefs.FlagRPCWriteTimeout
	FlagRPCMaxBodyBytes       = flagdefs.FlagRPCMaxBodyBytes
	FlagAPIEnableUnsafeCORS   = flagdefs.FlagAPIEnableUnsafeCORS

	// gRPC-related flags

	FlagGRPCOnly            = flagdefs.FlagGRPCOnly
	FlagGRPCEnable          = flagdefs.FlagGRPCEnable
	FlagGRPCAddress         = flagdefs.FlagGRPCAddress
	FlagGRPCWebEnable       = flagdefs.FlagGRPCWebEnable
	FlagGRPCSkipCheckHeader = flagdefs.FlagGRPCSkipCheckHeader

	// mempool flags

	FlagMempoolMaxTxs = flagdefs.FlagMempoolMaxTxs

	FlagQuery   = flagdefs.FlagQuery
	FlagOrderBy = flagdefs.FlagOrderBy

	TypeHash   = "hash"
	TypeAccSeq = "acc_seq"
	TypeSig    = "signature"
	TypeHeight = "height"

	// testnet keys

	FlagTestnetRootDir = flagdefs.FlagTestnetRootDir
	KeyTestnetRootDir  = FlagTestnetRootDir

	KeyIsTestnet             = "is-testnet"
	KeyTestnetConfig         = "testnet-config"
	KeyTestnetTriggerUpgrade = "testnet-trigger-upgrade"

	FlagLabel                     = flagdefs.FlagLabel
	FlagSource                    = flagdefs.FlagSource
	FlagBuilder                   = flagdefs.FlagBuilder
	FlagCodeHash                  = flagdefs.FlagCodeHash
	FlagAdmin                     = flagdefs.FlagAdmin
	FlagNoAdmin                   = flagdefs.FlagNoAdmin
	FlagFixMsg                    = flagdefs.FlagFixMsg
	FlagRunAs                     = flagdefs.FlagRunAs
	FlagInstantiateByEverybody    = flagdefs.FlagInstantiateByEverybody
	FlagInstantiateNobody         = flagdefs.FlagInstantiateNobody
	FlagInstantiateByAddress      = flagdefs.FlagInstantiateByAddress
	FlagInstantiateByAnyOfAddress = flagdefs.FlagInstantiateByAnyOfAddress
	FlagUnpinCode                 = flagdefs.FlagUnpinCode
	FlagAllowedMsgKeys            = flagdefs.FlagAllowedMsgKeys
	FlagAllowedRawMsgs            = flagdefs.FlagAllowedRawMsgs
	FlagMaxCalls                  = flagdefs.FlagMaxCalls
	FlagMaxFunds                  = flagdefs.FlagMaxFunds
	FlagAllowAllMsgs              = flagdefs.FlagAllowAllMsgs
	FlagNoTokenTransfer           = flagdefs.FlagNoTokenTransfer //nolint: gosec
	FlagExpedite                  = flagdefs.FlagExpedite

	// oracle flags

	FlagAssetDenom = flagdefs.FlagAssetDenom
	FlagBaseDenom  = flagdefs.FlagBaseDenom

	// bme flags

	FlagTo      = flagdefs.FlagTo
	FlagToDenom = flagdefs.FlagToDenom

	// wasm flags

	FlagWasmMemoryCacheSize        = flagdefs.FlagWasmMemoryCacheSize
	FlagWasmQueryGasLimit          = flagdefs.FlagWasmQueryGasLimit
	FlagWasmSimulationGasLimit     = flagdefs.FlagWasmSimulationGasLimit
	FlagWasmSkipWasmVMVersionCheck = flagdefs.FlagWasmSkipWasmVMVersionCheck //nolint: gosec
)

// List of supported output formats
const (
	OutputFormatJSON = "json"
	OutputFormatText = "text"
)

const (
	// FlagProposal only used for v1beta1 legacy proposals.
	FlagProposal = flagdefs.FlagProposal
	// FlagDescription only used for v1beta1 legacy proposals.
	FlagDescription = flagdefs.FlagDescription
	// FlagProposalType only used for v1beta1 legacy proposals.
	FlagProposalType = flagdefs.FlagType
	// FlagUpgradeHeight only used for v1beta1 legacy proposals.
	FlagUpgradeHeight = flagdefs.FlagUpgradeHeight
	// FlagUpgradeInfo only used for v1beta1 legacy proposals.
	FlagUpgradeInfo = flagdefs.FlagUpgradeInfo
)

// common flagsets to add to various functions
var (
	fsShares       = pflag.NewFlagSet("", pflag.ContinueOnError)
	fsValidator    = pflag.NewFlagSet("", pflag.ContinueOnError)
	fsRedelegation = pflag.NewFlagSet("", pflag.ContinueOnError)
)

func init() {
	fsShares.String(FlagSharesAmount, "", "Amount of source-shares to either unbond or redelegate as a positive integer or decimal")
	fsShares.String(FlagSharesFraction, "", "Fraction of source-shares to either unbond or redelegate as a positive integer or decimal >0 and <=1")
	fsValidator.String(FlagAddressValidator, "", "The Bech32 address of the validator")
	fsRedelegation.String(FlagAddressValidatorSrc, "", "The Bech32 address of the source validator")
	fsRedelegation.String(FlagAddressValidatorDst, "", "The Bech32 address of the destination validator")
}

// LineBreak can be included in a command list to provide a blank line
// to help with readability
var LineBreak = &cobra.Command{Run: func(*cobra.Command, []string) {}}

// AddQueryFlagsToCmd adds common flags to a module query command.
func AddQueryFlagsToCmd(cmd *cobra.Command) {
	cmd.Flags().String(FlagNode, "tcp://localhost:26657", "<host>:<port> to Tendermint RPC interface for this chain")
	cmd.Flags().String(FlagGRPC, "", "the gRPC endpoint to use for this chain")
	cmd.Flags().Bool(FlagGRPCInsecure, false, "allow gRPC over insecure channels, if not TLS the server must use TLS")
	cmd.Flags().Int64(FlagHeight, 0, "Use a specific height to query state at (this can error if the node is pruning state)")
	cmd.Flags().VarP(output.NewFormatFlag(OutputPretty), FlagOutput, "o", "Output format (pretty|json|yaml)")
}

// AddTxFlagsToCmd adds common flags to a module tx command.
func AddTxFlagsToCmd(cmd *cobra.Command) {
	f := cmd.Flags()

	f.VarP(output.NewFormatFlag(OutputPretty), FlagOutput, "o", "Output format (pretty|json|yaml)")
	f.String(FlagFrom, "", "Name or address of private key with which to sign")
	f.Uint64P(FlagAccountNumber, "a", 0, "The account number of the signing account (offline mode only)")
	f.Uint64P(FlagSequence, "s", 0, "The sequence number of the signing account (offline mode only)")
	f.String(FlagNote, "", "Note to add a description to the transaction (previously --memo)")
	f.String(FlagFees, "", "Fees to pay along with transaction; eg: 10uatom")
	f.String(FlagGasPrices, constants.DefaultGasPrices, "Gas prices in decimal format to determine the transaction fee (e.g. 0.1uatom)")
	f.String(FlagNode, "tcp://localhost:26657", "<host>:<port> to tendermint rpc interface for this chain")
	f.Bool(FlagUseLedger, false, "Use a connected Ledger device")
	f.Float64(FlagGasAdjustment, DefaultGasAdjustment, "adjustment factor to be multiplied against the estimate returned by the tx simulation; if the gas limit is set manually this flag is ignored ")
	f.VarP(output.NewEnumFlag(BroadcastSync, BroadcastSync, BroadcastAsync, BroadcastBlock), FlagBroadcastMode, "b", "Transaction broadcasting mode (sync|async|block)")
	f.Bool(FlagDryRun, false, "ignore the --gas flag and perform a simulation of a transaction, but don't broadcast it (when enabled, the local Keybase is not accessible)")
	f.Bool(FlagGenerateOnly, false, "Build an unsigned transaction and write it to STDOUT (when enabled, the local Keybase only accessed when providing a key name)")
	f.Bool(FlagOffline, false, "Offline mode (does not allow any online functionality)")
	f.BoolP(FlagSkipConfirmation, "y", false, "Skip tx broadcasting prompt confirmation")
	f.Var(output.NewEnumFlag(SignModeDirect, SignModeDirect, SignModeLegacyAminoJSON, SignModeDirectAux, SignModeEIP191), FlagSignMode, "Choose sign mode (direct|amino-json|direct-aux|eip-191), this is an advanced feature")
	f.Uint64(FlagTimeoutHeight, 0, "DEPRECATED: Please use --timeout-duration instead. Set a block timeout height to prevent the tx from being committed past a certain height")
	f.Duration(TimeoutDuration, 0, "TimeoutDuration is the duration the transaction will be considered valid in the mempool. The transaction's unordered nonce will be set to the time of transaction creation + the duration value passed. If the transaction is still in the mempool, and the block time has passed the time of submission + TimeoutTimestamp, the transaction will be rejected.")
	f.Bool(FlagUnordered, false, "Enable unordered transaction delivery; must be used in conjunction with --timeout-duration")
	f.String(FlagFeePayer, "", "Fee payer pays fees for the transaction instead of deducting from the signer")
	f.String(FlagFeeGranter, "", "Fee granter grants fees for the transaction")
	f.String(FlagTip, "", "Tip is the amount that is going to be transferred to the fee payer on the target chain. This flag is only valid when used with --aux, and is ignored if the target chain didn't enable the TipDecorator")
	f.Bool(FlagAux, false, "Generate aux signer data instead of sending a tx")
	f.String(FlagChainID, "", "The network chain ID")
	// --gas can accept integers and "auto"
	f.String(FlagGas, GasFlagAuto, fmt.Sprintf("gas limit to set per-transaction; set to %q to calculate sufficient gas automatically. Note: %q option doesn't always report accurate results. Set a valid coin value to adjust the result. Can be used instead of %q. (default %d)",
		GasFlagAuto, GasFlagAuto, FlagFees, DefaultGasLimit))

	cmd.MarkFlagsMutuallyExclusive(FlagTimeoutHeight, TimeoutDuration)
	// unordered transactions must not have sequence values.
	cmd.MarkFlagsMutuallyExclusive(FlagUnordered, FlagSequence)
	cmd.MarkFlagsRequiredTogether(FlagUnordered, TimeoutDuration)

	// --keyring-backend and --keyring-dir are deliberately NOT registered
	// here. They are global (SPEC §3.1), registered once on the root command
	// so key management, queries, and transactions all read the same value.
	// A transaction-local copy shadowed the global one, which kept the
	// documented AKT_KEYRING_BACKEND/AKT_KEYRING_DIR variables from ever
	// reaching a tx invocation, and its non-empty "os" default stood ready to
	// override the context's persisted backend.
}

// AddKeyringFlags sets common keyring flags on a standalone command tree that
// does not inherit akt's global flags (SPEC §3.1).
func AddKeyringFlags(flags *pflag.FlagSet) {
	flags.String(FlagKeyringDir, "", "The client Keyring directory; if omitted, the default 'home' directory will be used")
	flags.String(FlagKeyringBackend, DefaultKeyringBackend, "Select keyring's backend (os|file|kwallet|pass|test|memory)")
}

// AddPaginationFlagsToCmd adds common pagination flags to cmd
func AddPaginationFlagsToCmd(cmd *cobra.Command, query string) {
	cmd.Flags().Uint64(FlagPage, 1, fmt.Sprintf("pagination page of %s to query for. This sets offset to a multiple of limit", query))
	cmd.Flags().String(FlagPageKey, "", fmt.Sprintf("pagination page-key of %s to query for", query))
	cmd.Flags().Uint64(FlagOffset, 0, fmt.Sprintf("pagination offset of %s to query for", query))
	cmd.Flags().Uint64(FlagLimit, 100, fmt.Sprintf("pagination limit of %s to query for", query))
	cmd.Flags().Bool(FlagCountTotal, false, fmt.Sprintf("count total number of records in %s to query for", query))
	cmd.Flags().Bool(FlagReverse, false, "results are sorted in descending order")
}

// FlagSetCommissionCreate Returns the FlagSet used for commission create.
func FlagSetCommissionCreate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(FlagCommissionRate, "", "The initial commission rate percentage")
	fs.String(FlagCommissionMaxRate, "", "The maximum commission rate percentage")
	fs.String(FlagCommissionMaxChangeRate, "", "The maximum commission change rate percentage (per day)")

	return fs
}

// FlagSetAmount Returns the FlagSet for amount related operations.
func FlagSetAmount() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(FlagAmount, "", "Amount of coins to bond")
	return fs
}

// FlagSetPublicKey Returns the flagset for Public Key related operations.
func FlagSetPublicKey() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(FlagPubKey, "", "The validator's Protobuf JSON encoded public key")
	return fs
}

func FlagSetDescriptionEdit() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(FlagEditMoniker, stakingtypes.DoNotModifyDesc, "The validator's name")
	fs.String(FlagIdentity, stakingtypes.DoNotModifyDesc, "The (optional) identity signature (ex. UPort or Keybase)")
	fs.String(FlagWebsite, stakingtypes.DoNotModifyDesc, "The validator's (optional) website")
	fs.String(FlagSecurityContact, stakingtypes.DoNotModifyDesc, "The validator's (optional) security contact email")
	fs.String(FlagDetails, stakingtypes.DoNotModifyDesc, "The validator's (optional) details")

	return fs
}

func FlagSetCommissionUpdate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(FlagCommissionRate, "", "The new commission rate percentage")

	return fs
}

func FlagSetDescriptionCreate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(FlagMoniker, "", "The validator's name")
	fs.String(FlagIdentity, "", "The optional identity signature (ex. UPort or Keybase)")
	fs.String(FlagWebsite, "", "The validator's (optional) website")
	fs.String(FlagSecurityContact, "", "The validator's (optional) security contact email")
	fs.String(FlagDetails, "", "The validator's (optional) details")

	return fs
}

// FlagSetMinSelfDelegation Returns the FlagSet used for minimum set delegation.
func FlagSetMinSelfDelegation() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(FlagMinSelfDelegation, "", "The minimum self delegation required on the validator")
	return fs
}

// AddGovPropFlagsToCmd adds flags for defining MsgSubmitProposal fields.
//
// See also ReadGovPropFlags.
func AddGovPropFlagsToCmd(cmd *cobra.Command) {
	cmd.Flags().String(FlagDeposit, "", "The deposit to include with the governance proposal")
	cmd.Flags().String(FlagMetadata, "", "The metadata to include with the governance proposal")
	cmd.Flags().String(FlagTitle, "", "The title to put on the governance proposal")
	cmd.Flags().String(FlagSummary, "", "The summary to include with the governance proposal")
	cmd.Flags().Bool(FlagExpedited, false, "Whether to expedite the governance proposal") // cannot be enabled because of IBC redefining this flag in `upgrade-channels` command.
}
