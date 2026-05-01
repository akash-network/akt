package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	etypes "pkg.akt.dev/go/node/escrow/v1"
)

func init() {
	Register((*etypes.QueryAccountsResponse)(nil), PrettyFormatterFunc(formatEscrowAccounts))
	Register((*etypes.QueryPaymentsResponse)(nil), PrettyFormatterFunc(formatEscrowPayments))
}

// RenderEscrowAccounts renders an escrow accounts list as a styled string.
func RenderEscrowAccounts(res *etypes.QueryAccountsResponse) string {
	var buf strings.Builder
	headers := []string{"SCOPE", "XID", "OWNER", "STATE", "BALANCE", "SPENT", "SETTLED AT"}
	rows := make([][]string, 0, len(res.Accounts))
	for _, a := range res.Accounts {
		balance := "-"
		if len(a.State.Funds) > 0 {
			parts := make([]string, len(a.State.Funds))
			for i, f := range a.State.Funds {
				parts[i] = FormatDecAmount(f.Amount, f.Denom)
			}
			balance = strings.Join(parts, ", ")
		}
		spent := "-"
		if len(a.State.Transferred) > 0 {
			spent = FormatDecCoins(a.State.Transferred)
		}
		rows = append(rows, []string{
			a.ID.Scope.String(), Bold(a.ID.XID), a.State.Owner,
			ColorState(a.State.State.String()), Bold(balance), spent,
			FormatHeight(a.State.SettledAt),
		})
	}
	WriteTable(&buf, headers, rows)
	return buf.String()
}

func formatEscrowAccounts(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderEscrowAccounts(msg.(*etypes.QueryAccountsResponse)))
	return err
}

// RenderEscrowPayments renders an escrow payments list as a styled string.
func RenderEscrowPayments(res *etypes.QueryPaymentsResponse) string {
	var buf strings.Builder
	headers := []string{"ACCOUNT SCOPE", "ACCOUNT XID", "PAYMENT XID", "OWNER", "STATE", "RATE", "BALANCE", "WITHDRAWN"}
	rows := make([][]string, 0, len(res.Payments))
	for _, p := range res.Payments {
		rows = append(rows, []string{
			p.ID.AID.Scope.String(), p.ID.AID.XID, Bold(p.ID.XID),
			p.State.Owner, ColorState(p.State.State.String()),
			FormatDecCoin(p.State.Rate), FormatDecCoin(p.State.Balance),
			FormatCoin(p.State.Withdrawn),
		})
	}
	WriteTable(&buf, headers, rows)
	return buf.String()
}

func formatEscrowPayments(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderEscrowPayments(msg.(*etypes.QueryPaymentsResponse)))
	return err
}
