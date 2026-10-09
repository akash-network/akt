package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	querytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/output/pretty"
)

// GetQueryContractCmd lists contracts or looks one up by address or label.
func GetQueryContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contract [address-or-label]",
		Aliases: []string{"contracts"},
		Short:   "List contracts or show one by address or label",
		Long: `List contract addresses, labels, and code IDs, or show one contract's details.

Labels are matched exactly and may need quoting. Duplicate labels require an
address. Listing and label lookup scan all contracts on the selected network.`,
		Example: `  akt q contracts
  akt q contract akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx
  akt q contract "my contract"`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if len(args) == 1 && strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("contract address or label cannot be empty")
			}
			return nil
		},
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE:              queryContract,
		SilenceUsage:      true,
	}
	cflags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func queryContract(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cl := MustLightClientFromContext(ctx)
	query := cl.Query().Wasm()
	if len(args) == 1 {
		if address, err := sdk.AccAddressFromBech32(args[0]); err == nil {
			info, err := contractInfo(ctx, query, address.String())
			if err != nil {
				return err
			}
			return pretty.PrintQueryResult(cmd, cl.ClientContext(), info)
		}
	}

	contracts, err := listContracts(ctx, query)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return pretty.PrintWasmContractList(cmd, contracts)
	}

	matches := make([]*wasmtypes.QueryContractInfoResponse, 0)
	for _, contract := range contracts {
		if contract.Label == args[0] {
			matches = append(matches, contract)
		}
	}
	switch len(matches) {
	case 0:
		return &cliutil.CLIError{
			Code: cliutil.ExitGeneral, Message: fmt.Sprintf("no contract with label %q", args[0]),
			Suggestion: "Run akt q contracts to see contract addresses and labels.",
		}
	case 1:
		return pretty.PrintQueryResult(cmd, cl.ClientContext(), matches[0])
	default:
		addresses := make([]string, len(matches))
		for i, match := range matches {
			addresses[i] = match.Address
		}
		return &cliutil.CLIError{
			Code: cliutil.ExitUsage, Message: fmt.Sprintf("contract label %q matches multiple addresses:\n  %s", args[0], strings.Join(addresses, "\n  ")),
			Suggestion: "Query one with akt q contract <address>.",
		}
	}
}

func contractInfo(ctx context.Context, query wasmtypes.QueryClient, address string) (*wasmtypes.QueryContractInfoResponse, error) {
	res, err := query.ContractInfo(ctx, &wasmtypes.QueryContractInfoRequest{Address: address})
	if err != nil {
		return nil, fmt.Errorf("query contract %s: %w", address, err)
	}
	if err := requireQueryResponse("contract", res); err != nil {
		return nil, err
	}
	if res.Address == "" || res.Address != address {
		return nil, fmt.Errorf("contract %s query returned malformed node response: address %q", address, res.Address)
	}
	return res, nil
}

// listContracts follows both levels of Wasm pagination before returning a list.
func listContracts(ctx context.Context, query wasmtypes.QueryClient) ([]*wasmtypes.QueryContractInfoResponse, error) {
	contracts := make([]*wasmtypes.QueryContractInfoResponse, 0)
	seenAddresses := make(map[string]bool)
	seenPages := make(map[string]bool)
	page := &querytypes.PageRequest{Limit: 100}
	for {
		codes, err := query.Codes(ctx, &wasmtypes.QueryCodesRequest{Pagination: page})
		if err != nil {
			return nil, fmt.Errorf("list contract codes: %w", err)
		}
		if err := requireQueryResponse("contract codes", codes); err != nil {
			return nil, err
		}
		for _, code := range codes.CodeInfos {
			if err := collectContractsByCode(ctx, query, code.CodeID, seenAddresses, &contracts); err != nil {
				return nil, err
			}
		}
		more, err := advanceContractPage(page, codes.Pagination, seenPages)
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
	}
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].Address < contracts[j].Address })
	return contracts, nil
}

func collectContractsByCode(ctx context.Context, query wasmtypes.QueryClient, codeID uint64, seenAddresses map[string]bool, contracts *[]*wasmtypes.QueryContractInfoResponse) error {
	page := &querytypes.PageRequest{Limit: 100}
	seenPages := make(map[string]bool)
	for {
		res, err := query.ContractsByCode(ctx, &wasmtypes.QueryContractsByCodeRequest{CodeId: codeID, Pagination: page})
		if err != nil {
			return fmt.Errorf("list contracts for code %d: %w", codeID, err)
		}
		if err := requireQueryResponse("contracts by code", res); err != nil {
			return err
		}
		for _, address := range res.Contracts {
			if seenAddresses[address] {
				continue
			}
			info, err := contractInfo(ctx, query, address)
			if err != nil {
				return err
			}
			seenAddresses[address] = true
			*contracts = append(*contracts, info)
		}
		more, err := advanceContractPage(page, res.Pagination, seenPages)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

func advanceContractPage(request *querytypes.PageRequest, response *querytypes.PageResponse, seen map[string]bool) (bool, error) {
	if response == nil || len(response.NextKey) == 0 {
		return false, nil
	}
	key := string(response.NextKey)
	if seen[key] {
		return false, fmt.Errorf("contract listing returned malformed node response: repeated pagination key")
	}
	seen[key] = true
	request.Key = response.NextKey
	return true, nil
}
