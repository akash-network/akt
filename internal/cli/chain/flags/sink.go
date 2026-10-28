package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	types "pkg.akt.dev/go/node/sink/v1"
)

// AddSinkDepositFlags adds the destination network flag of a sink deposit.
func AddSinkDepositFlags(flags *pflag.FlagSet) {
	flags.Uint32(flagdefs.FlagDestType, types.DestTypeSolana, "Destination network (1 = Solana, the only supported value)")
}

// AddSinkOpenTransfersFlags adds the paging and hold filter flags of the
// open-transfers query. Its limit is the module's own page size, not the
// Cosmos page request, so the standard pagination flags do not apply.
func AddSinkOpenTransfersFlags(flags *pflag.FlagSet) {
	flags.Uint64(flagdefs.FlagStartAfter, 0, "Return transfers with IDs above this one")
	flags.Uint32(flagdefs.FlagLimit, 0, "Maximum transfers to return; 0 means 100, at most 500")
	flags.Bool(flagdefs.FlagHeldOnly, false, "List every open transfer held for governance (veto or conflict), reading all pages")
}

// AddSinkReportFlags adds the overrides for the reporter epoch and transfer
// round a report names. Both default to the chain's current values.
func AddSinkReportFlags(flags *pflag.FlagSet) {
	flags.Uint64(flagdefs.FlagReporterEpoch, 0, "Reporter epoch to report in (default: the chain's current epoch)")
	flags.Uint64(flagdefs.FlagRound, 0, "Settlement round to report in (default: the transfer's current round)")
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
