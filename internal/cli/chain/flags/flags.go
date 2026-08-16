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
	// OutputPretty is the value for --output that renders human-friendly pretty output.
	OutputPretty = "pretty"
	// OutputJSON is the value for --output that renders machine-readable JSON.
	OutputJSON = "json"
	// OutputYAML is the value for --output that renders machine-readable YAML.
	OutputYAML = "yaml"

	TypeHash   = "hash"
	TypeAccSeq = "acc_seq"
	TypeSig    = "signature"
	TypeHeight = "height"
)

// List of supported output formats
const (
	OutputFormatJSON = "json"
	OutputFormatText = "text"
)

// common flagsets to add to various functions
var (
	fsShares       = pflag.NewFlagSet("", pflag.ContinueOnError)
	fsValidator    = pflag.NewFlagSet("", pflag.ContinueOnError)
	fsRedelegation = pflag.NewFlagSet("", pflag.ContinueOnError)
)

func init() {
	fsShares.String(flagdefs.FlagSharesAmount, "", "Amount of source-shares to either unbond or redelegate as a positive integer or decimal")
	fsShares.String(flagdefs.FlagSharesFraction, "", "Fraction of source-shares to either unbond or redelegate as a positive integer or decimal >0 and <=1")
	fsValidator.String(flagdefs.FlagAddressValidator, "", "The Bech32 address of the validator")
	fsRedelegation.String(flagdefs.FlagAddressValidatorSrc, "", "The Bech32 address of the source validator")
	fsRedelegation.String(flagdefs.FlagAddressValidatorDst, "", "The Bech32 address of the destination validator")
}

// LineBreak can be included in a command list to provide a blank line
// to help with readability
var LineBreak = &cobra.Command{Run: func(*cobra.Command, []string) {}}

// AddQueryFlagsToCmd adds common flags to a module query command.
func AddQueryFlagsToCmd(cmd *cobra.Command) {
	cmd.Flags().String(flagdefs.FlagNode, "tcp://localhost:26657", "<host>:<port> to Tendermint RPC interface for this chain")
	cmd.Flags().String(flagdefs.FlagGRPC, "", "the gRPC endpoint to use for this chain")
	cmd.Flags().Bool(flagdefs.FlagGRPCInsecure, false, "allow gRPC over insecure channels, if not TLS the server must use TLS")
	cmd.Flags().Int64(flagdefs.FlagHeight, 0, "Use a specific height to query state at (this can error if the node is pruning state)")
	cmd.Flags().VarP(output.NewFormatFlag(OutputPretty), flagdefs.FlagOutput, "o", "Output format (pretty|json|yaml)")
}

// AddTxFlagsToCmd adds common flags to a module tx command.
func AddTxFlagsToCmd(cmd *cobra.Command) {
	f := cmd.Flags()

	f.VarP(output.NewFormatFlag(OutputPretty), flagdefs.FlagOutput, "o", "Output format (pretty|json|yaml)")
	f.String(flagdefs.FlagFrom, "", "Name or address of private key with which to sign")
	f.Uint64P(flagdefs.FlagAccountNumber, "a", 0, "The account number of the signing account (offline mode only)")
	f.Uint64P(flagdefs.FlagSequence, "s", 0, "The sequence number of the signing account (offline mode only)")
	f.String(flagdefs.FlagNote, "", "Note to add a description to the transaction (previously --memo)")
	f.String(flagdefs.FlagFees, "", "Fees to pay along with transaction; eg: 10uatom")
	f.String(flagdefs.FlagGasPrices, constants.DefaultGasPrices, "Gas prices in decimal format to determine the transaction fee (e.g. 0.1uatom)")
	f.String(flagdefs.FlagNode, "tcp://localhost:26657", "<host>:<port> to tendermint rpc interface for this chain")
	f.Bool(flagdefs.FlagUseLedger, false, "Use a connected Ledger device")
	f.Float64(flagdefs.FlagGasAdjustment, DefaultGasAdjustment, "adjustment factor to be multiplied against the estimate returned by the tx simulation; if the gas limit is set manually this flag is ignored ")
	f.VarP(output.NewEnumFlag(BroadcastSync, BroadcastSync, BroadcastAsync, BroadcastBlock), flagdefs.FlagBroadcastMode, "b", "Transaction broadcasting mode (sync|async|block)")
	f.Bool(flagdefs.FlagDryRun, false, "ignore the --gas flag and perform a simulation of a transaction, but don't broadcast it (when enabled, the local Keybase is not accessible)")
	f.Bool(flagdefs.FlagGenerateOnly, false, "Build an unsigned transaction and write it to STDOUT (when enabled, the local Keybase only accessed when providing a key name)")
	f.Bool(flagdefs.FlagOffline, false, "Offline mode (does not allow any online functionality)")
	f.BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip tx broadcasting prompt confirmation")
	f.Var(output.NewEnumFlag(SignModeDirect, SignModeDirect, SignModeLegacyAminoJSON, SignModeDirectAux, SignModeEIP191), flagdefs.FlagSignMode, "Choose sign mode (direct|amino-json|direct-aux|eip-191), this is an advanced feature")
	f.Uint64(flagdefs.FlagTimeoutHeight, 0, "DEPRECATED: Please use --timeout-duration instead. Set a block timeout height to prevent the tx from being committed past a certain height")
	f.Duration(flagdefs.FlagTimeoutDuration, 0, "TimeoutDuration is the duration the transaction will be considered valid in the mempool. The transaction's unordered nonce will be set to the time of transaction creation + the duration value passed. If the transaction is still in the mempool, and the block time has passed the time of submission + TimeoutTimestamp, the transaction will be rejected.")
	f.Bool(flagdefs.FlagUnordered, false, "Enable unordered transaction delivery; must be used in conjunction with --timeout-duration")
	f.String(flagdefs.FlagFeePayer, "", "Fee payer pays fees for the transaction instead of deducting from the signer")
	f.String(flagdefs.FlagFeeGranter, "", "Fee granter grants fees for the transaction")
	f.String(flagdefs.FlagTip, "", "Tip is the amount that is going to be transferred to the fee payer on the target chain. This flag is only valid when used with --aux, and is ignored if the target chain didn't enable the TipDecorator")
	f.Bool(flagdefs.FlagAux, false, "Generate aux signer data instead of sending a tx")
	f.String(flagdefs.FlagChainID, "", "The network chain ID")
	// --gas can accept integers and "auto"
	f.String(flagdefs.FlagGas, GasFlagAuto, fmt.Sprintf("gas limit to set per-transaction; set to %q to calculate sufficient gas automatically. Note: %q option doesn't always report accurate results. Set a valid coin value to adjust the result. Can be used instead of %q. (default %d)",
		GasFlagAuto, GasFlagAuto, flagdefs.FlagFees, DefaultGasLimit))

	cmd.MarkFlagsMutuallyExclusive(flagdefs.FlagTimeoutHeight, flagdefs.FlagTimeoutDuration)
	// unordered transactions must not have sequence values.
	cmd.MarkFlagsMutuallyExclusive(flagdefs.FlagUnordered, flagdefs.FlagSequence)
	cmd.MarkFlagsRequiredTogether(flagdefs.FlagUnordered, flagdefs.FlagTimeoutDuration)

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
	flags.String(flagdefs.FlagKeyringDir, "", "The client Keyring directory; if omitted, the default 'home' directory will be used")
	flags.String(flagdefs.FlagKeyringBackend, DefaultKeyringBackend, "Select keyring's backend (os|file|kwallet|pass|test|memory)")
}

// AddPaginationFlagsToCmd adds common pagination flags to cmd
func AddPaginationFlagsToCmd(cmd *cobra.Command, query string) {
	cmd.Flags().Uint64(flagdefs.FlagPage, 1, fmt.Sprintf("pagination page of %s to query for. This sets offset to a multiple of limit", query))
	cmd.Flags().String(flagdefs.FlagPageKey, "", fmt.Sprintf("pagination page-key of %s to query for", query))
	cmd.Flags().Uint64(flagdefs.FlagOffset, 0, fmt.Sprintf("pagination offset of %s to query for", query))
	cmd.Flags().Uint64(flagdefs.FlagLimit, 100, fmt.Sprintf("pagination limit of %s to query for", query))
	cmd.Flags().Bool(flagdefs.FlagCountTotal, false, fmt.Sprintf("count total number of records in %s to query for", query))
	cmd.Flags().Bool(flagdefs.FlagReverse, false, "results are sorted in descending order")
}

// FlagSetCommissionCreate Returns the FlagSet used for commission create.
func FlagSetCommissionCreate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(flagdefs.FlagCommissionRate, "", "The initial commission rate percentage")
	fs.String(flagdefs.FlagCommissionMaxRate, "", "The maximum commission rate percentage")
	fs.String(flagdefs.FlagCommissionMaxChangeRate, "", "The maximum commission change rate percentage (per day)")

	return fs
}

// FlagSetAmount Returns the FlagSet for amount related operations.
func FlagSetAmount() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(flagdefs.FlagAmount, "", "Amount of coins to bond")
	return fs
}

// FlagSetPublicKey Returns the flagset for Public Key related operations.
func FlagSetPublicKey() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(flagdefs.FlagPubKey, "", "The validator's Protobuf JSON encoded public key")
	return fs
}

func FlagSetDescriptionEdit() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(flagdefs.FlagEditMoniker, stakingtypes.DoNotModifyDesc, "The validator's name")
	fs.String(flagdefs.FlagIdentity, stakingtypes.DoNotModifyDesc, "The (optional) identity signature (ex. UPort or Keybase)")
	fs.String(flagdefs.FlagWebsite, stakingtypes.DoNotModifyDesc, "The validator's (optional) website")
	fs.String(flagdefs.FlagSecurityContact, stakingtypes.DoNotModifyDesc, "The validator's (optional) security contact email")
	fs.String(flagdefs.FlagDetails, stakingtypes.DoNotModifyDesc, "The validator's (optional) details")

	return fs
}

func FlagSetCommissionUpdate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(flagdefs.FlagCommissionRate, "", "The new commission rate percentage")

	return fs
}

func FlagSetDescriptionCreate() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)

	fs.String(flagdefs.FlagMoniker, "", "The validator's name")
	fs.String(flagdefs.FlagIdentity, "", "The optional identity signature (ex. UPort or Keybase)")
	fs.String(flagdefs.FlagWebsite, "", "The validator's (optional) website")
	fs.String(flagdefs.FlagSecurityContact, "", "The validator's (optional) security contact email")
	fs.String(flagdefs.FlagDetails, "", "The validator's (optional) details")

	return fs
}

// FlagSetMinSelfDelegation Returns the FlagSet used for minimum set delegation.
func FlagSetMinSelfDelegation() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.String(flagdefs.FlagMinSelfDelegation, "", "The minimum self delegation required on the validator")
	return fs
}

// AddGovPropFlagsToCmd adds flags for defining MsgSubmitProposal fields.
//
// See also ReadGovPropFlags.
func AddGovPropFlagsToCmd(cmd *cobra.Command) {
	cmd.Flags().String(flagdefs.FlagDeposit, "", "The deposit to include with the governance proposal")
	cmd.Flags().String(flagdefs.FlagMetadata, "", "The metadata to include with the governance proposal")
	cmd.Flags().String(flagdefs.FlagTitle, "", "The title to put on the governance proposal")
	cmd.Flags().String(flagdefs.FlagSummary, "", "The summary to include with the governance proposal")
	cmd.Flags().Bool(flagdefs.FlagExpedited, false, "Whether to expedite the governance proposal") // cannot be enabled because of IBC redefining this flag in `upgrade-channels` command.
}
