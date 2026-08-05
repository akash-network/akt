package pretty

import (
	"fmt"
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

func init() {
	Register((*types.QueryAccountResponse)(nil), PrettyFormatterFunc(formatAccountResponse))
	Register((*types.QueryAccountsResponse)(nil), PrettyFormatterFunc(formatAccountsResponse))
	Register((*types.QueryModuleAccountsResponse)(nil), PrettyFormatterFunc(formatModuleAccountsResponse))
}

func formatAccountResponse(w io.Writer, _ *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryAccountResponse)
	if res.Account == nil {
		fmt.Fprintln(w, Dim("(no account)"))
		return nil
	}

	acct, err := unpackAccount(cctx.InterfaceRegistry, res.Account)
	if err != nil {
		// Can't unpack — show type URL as fallback.
		fmt.Fprintln(w, Section("Account"))
		KV(w, "Type", res.Account.TypeUrl)
		//nolint:nilerr // the fallback above already rendered what is known
		// about the account; a renderer that cannot decode one field should
		// degrade, not fail the query the user just ran.
		return nil
	}

	fmt.Fprintln(w, Section("Account"))
	KV(w, "Address", acct.GetAddress().String())
	KV(w, "Account #", fmt.Sprintf("%d", acct.GetAccountNumber()))
	KV(w, "Sequence", fmt.Sprintf("%d", acct.GetSequence()))
	pk := acct.GetPubKey()
	if pk != nil {
		KV(w, "Pub Key Type", fmt.Sprintf("%T", pk))
	}

	// Show module name for module accounts.
	if ma, ok := acct.(*types.ModuleAccount); ok {
		KV(w, "Name", Bold(ma.Name))
		if len(ma.Permissions) > 0 {
			KV(w, "Permissions", fmt.Sprintf("%v", ma.Permissions))
		}
	}

	return nil
}

func formatAccountsResponse(w io.Writer, _ *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryAccountsResponse)

	headers := []string{"ADDRESS", "TYPE", "ACCOUNT #", "SEQUENCE"}
	rows := make([][]string, 0, len(res.Accounts))

	for _, anyAcct := range res.Accounts {
		acct, err := unpackAccount(cctx.InterfaceRegistry, anyAcct)
		if err != nil {
			rows = append(rows, []string{"?", shortTypeName(anyAcct.TypeUrl), "-", "-"})
			continue
		}

		rows = append(rows, []string{
			acct.GetAddress().String(),
			shortTypeName(anyAcct.TypeUrl),
			fmt.Sprintf("%d", acct.GetAccountNumber()),
			fmt.Sprintf("%d", acct.GetSequence()),
		})
	}

	WriteTableOrEmpty(w, headers, rows, "(no accounts)")
	return nil
}

func formatModuleAccountsResponse(w io.Writer, _ *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryModuleAccountsResponse)

	headers := []string{"NAME", "ADDRESS", "PERMISSIONS"}
	rows := make([][]string, 0, len(res.Accounts))

	for _, anyAcct := range res.Accounts {
		acct, err := unpackAccount(cctx.InterfaceRegistry, anyAcct)
		if err != nil {
			rows = append(rows, []string{"-", shortTypeName(anyAcct.TypeUrl), "-"})
			continue
		}

		name := "-"
		perms := "-"
		if ma, ok := acct.(*types.ModuleAccount); ok {
			name = Bold(ma.Name)
			if len(ma.Permissions) > 0 {
				perms = fmt.Sprintf("%v", ma.Permissions)
			}
		}

		rows = append(rows, []string{
			name,
			acct.GetAddress().String(),
			perms,
		})
	}

	WriteTableOrEmpty(w, headers, rows, "(no module accounts)")
	return nil
}

// unpackAccount unpacks a codectypes.Any into an AccountI using the interface
// registry from the client context.
func unpackAccount(registry codectypes.InterfaceRegistry, anyAcct *codectypes.Any) (sdk.AccountI, error) {
	if registry == nil {
		return nil, fmt.Errorf("no interface registry available")
	}

	var acct sdk.AccountI
	if err := registry.UnpackAny(anyAcct, &acct); err != nil {
		return nil, err
	}

	return acct, nil
}
