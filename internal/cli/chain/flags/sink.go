package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	types "pkg.akt.dev/go/node/sink/v1"
)

// AddSinkDepositFlags adds the destination network flag of a sink deposit.
func AddSinkDepositFlags(flags *pflag.FlagSet) {
	flags.Uint32(flagdefs.FlagDestType, uint32(types.DestTypeSolana), "Destination network (1 = Solana, the only supported value)")
}

// AddSinkReportFlags adds overrides for the current policy version and round.
func AddSinkReportFlags(flags *pflag.FlagSet) {
	flags.Uint64(flagdefs.FlagPolicyVersion, 0, "Reporter policy version (default: the chain's current policy)")
	flags.Uint64(flagdefs.FlagRound, 0, "Settlement round (default: the transfer's current round)")
}

// AddSinkExpectedRoundFlag binds governance actions to one settlement round.
func AddSinkExpectedRoundFlag(flags *pflag.FlagSet) {
	flags.Uint64(flagdefs.FlagExpectedRound, 0, "Expected settlement round (default: read from the transfer before proposing)")
}

// AddSinkProposalFlags adds the governance proposal flags shared by the
// sink propose-* commands, plus the standard transaction flags.
func AddSinkProposalFlags(cmd *cobra.Command, govAuthority string) {
	flags := cmd.Flags()
	flags.String(flagdefs.FlagTitle, "", "Proposal title (default: a description of the action)")
	flags.String(flagdefs.FlagSummary, "", "Proposal summary (default: a description of the action)")
	flags.String(flagdefs.FlagDeposit, "", "Initial proposal deposit, for example 50000000uakt")
	flags.String(flagdefs.FlagAuthority, govAuthority, "Authority the sink message names (default: the governance module account)")
	flags.Bool(flagdefs.FlagExpedite, false, "Submit as an expedited proposal: shorter voting period, higher threshold")
	AddTxFlagsToCmd(cmd)
}
