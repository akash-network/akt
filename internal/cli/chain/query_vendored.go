package cli

import (
	"context"
	"fmt"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcclienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

const localVendoredQueryAnnotation = "akt.local-vendored-query"

// adoptVendoredQueryCmds prepares a query subtree that is imported wholesale
// from a dependency, rather than declared here.
//
// Such a subtree resolves its endpoint through the SDK's own
// GetClientQueryContext, which reads the --node flag and falls back to its
// default of tcp://localhost:26657. akt keeps the endpoint in the active
// context instead, so every one of these commands dialled localhost and failed
// on a perfectly good remote-RPC setup -- 38 commands across ibc and
// ibc-transfer. Two of them did worse than fail: upstream discards the query
// error and dereferences the nil result, so `query ibc client params` and
// `query ibc connection params` segfaulted with a Go stack trace and exit 2.
//
// Resolving the context here and storing it back on the command means the SDK
// finds a client already attached and leaves it alone. The few upstream
// handlers that violate akt's public contract are replaced at this boundary:
// transport errors are returned, pagination lookahead is removed, scalar
// output follows the selected format, and duplicate paths are discarded.
//
// The hook is set on every command in the subtree, not just its root: cobra
// runs only the closest PersistentPreRunE, so a descendant that declares its
// own would otherwise skip this.
func adoptVendoredQueryCmd(cmd *cobra.Command) *cobra.Command {
	dedupeVendoredQueryCmd(cmd)
	installVendoredQueryOverrides(cmd)
	applyVendoredQueryPreRun(cmd)

	return cmd
}

func applyVendoredQueryPreRun(cmd *cobra.Command) {
	if inner := cmd.PersistentPreRunE; inner != nil {
		cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			if err := vendoredQueryPreRunE(c, args); err != nil {
				return err
			}

			return inner(c, args)
		}
	} else {
		cmd.PersistentPreRunE = vendoredQueryPreRunE
	}

	for _, sub := range cmd.Commands() {
		applyVendoredQueryPreRun(sub)
	}
}

func vendoredQueryPreRunE(cmd *cobra.Command, _ []string) error {
	if cmd.Annotations[localVendoredQueryAnnotation] == "true" {
		return localQueryPreRunE(cmd, nil)
	}

	ctx := cmd.Context()

	if cmd.Flags().Changed(flagdefs.FlagNode) {
		rpcURI, _ := cmd.Flags().GetString(flagdefs.FlagNode)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, rpcURI)
		cmd.SetContext(ctx)
	}

	cctx, err := GetClientQueryContext(cmd)
	if err != nil {
		return err
	}
	cctx = normalizeVendoredQueryOutput(cmd, cctx)

	if err := sdkclient.SetCmdClientContext(cmd, cctx); err != nil {
		return err
	}

	verboseQueryEndpoint(cmd, cctx)
	return nil
}

// normalizeVendoredQueryOutput translates akt's public YAML spelling to the
// Cosmos SDK's internal "text" spelling. The SDK otherwise accepts "yaml" but
// treats it as JSON when PrintProto dispatches. Keep the public flag value so
// akt's enum remains truthful, but mark it consumed so an upstream handler's
// second GetClientQueryContext call does not overwrite the translated context.
func normalizeVendoredQueryOutput(cmd *cobra.Command, cctx sdkclient.Context) sdkclient.Context {
	flag := cmd.Flags().Lookup(flagdefs.FlagOutput)
	if flag == nil {
		return cctx
	}

	format, _ := cmd.Flags().GetString(flagdefs.FlagOutput)
	if format != cflags.OutputYAML {
		return cctx
	}

	flag.Changed = false
	return cctx.WithOutputFormat(cflags.OutputFormatText)
}

func dedupeVendoredQueryCmd(cmd *cobra.Command) {
	seen := make(map[string]struct{})
	for _, child := range append([]*cobra.Command(nil), cmd.Commands()...) {
		name := child.Name()
		if name != "" {
			if _, exists := seen[name]; exists {
				cmd.RemoveCommand(child)
				continue
			}
			seen[name] = struct{}{}
		}

		dedupeVendoredQueryCmd(child)
	}
}

func installVendoredQueryOverrides(root *cobra.Command) {
	switch root.Name() {
	case "ibc":
		if cmd := vendoredSubcommand(root, "client", "params"); cmd != nil {
			cmd.RunE = queryIBCClientParams
		}
		if cmd := vendoredSubcommand(root, "client", "states"); cmd != nil {
			cmd.RunE = queryIBCClientStates
		}
		if cmd := vendoredSubcommand(root, "connection", "params"); cmd != nil {
			cmd.RunE = queryIBCConnectionParams
		}
		if cmd := vendoredSubcommand(root, "client", "consensus-state-heights"); cmd != nil {
			cmd.RunE = queryIBCConsensusStateHeights
		}
		if cmd := vendoredSubcommand(root, "client", "consensus-states"); cmd != nil {
			cmd.RunE = queryIBCConsensusStates
		}
		if cmd := vendoredSubcommand(root, "channel", "connections"); cmd != nil {
			cmd.RunE = queryIBCConnectionChannels
		}
	case "ibc-transfer":
		if cmd := vendoredSubcommand(root, "escrow-address"); cmd != nil {
			if cmd.Annotations == nil {
				cmd.Annotations = make(map[string]string)
			}
			cmd.Annotations[localVendoredQueryAnnotation] = "true"
			cmd.RunE = queryIBCTransferEscrowAddress
		}
	}
}

func vendoredSubcommand(root *cobra.Command, path ...string) *cobra.Command {
	current := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}

	return current
}

func queryIBCClientParams(cmd *cobra.Command, _ []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	res, err := ibcclienttypes.NewQueryClient(cctx).ClientParams(cmd.Context(), &ibcclienttypes.QueryClientParamsRequest{})
	if err != nil {
		return err
	}
	if res == nil || res.Params == nil {
		return fmt.Errorf("ibc client params response did not include params")
	}

	return cctx.PrintProto(res.Params)
}

func queryIBCClientStates(cmd *cobra.Command, _ []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
	if err != nil {
		return err
	}

	res, err := ibcclienttypes.NewQueryClient(cctx).ClientStates(cmd.Context(), &ibcclienttypes.QueryClientStatesRequest{
		Pagination: pageReq,
	})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("ibc client states query returned an empty response")
	}

	res.ClientStates = normalizeVendoredPage(res.ClientStates, pageReq)
	return cctx.PrintProto(res)
}

func queryIBCConnectionParams(cmd *cobra.Command, _ []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	res, err := connectiontypes.NewQueryClient(cctx).ConnectionParams(cmd.Context(), &connectiontypes.QueryConnectionParamsRequest{})
	if err != nil {
		return err
	}
	if res == nil || res.Params == nil {
		return fmt.Errorf("ibc connection params response did not include params")
	}

	return cctx.PrintProto(res.Params)
}

func queryIBCConsensusStateHeights(cmd *cobra.Command, args []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
	if err != nil {
		return err
	}

	res, err := ibcclienttypes.NewQueryClient(cctx).ConsensusStateHeights(cmd.Context(), &ibcclienttypes.QueryConsensusStateHeightsRequest{
		ClientId:   args[0],
		Pagination: pageReq,
	})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("ibc consensus-state-heights query returned an empty response")
	}

	res.ConsensusStateHeights = normalizeVendoredPage(res.ConsensusStateHeights, pageReq)
	return cctx.PrintProto(res)
}

func queryIBCConsensusStates(cmd *cobra.Command, args []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
	if err != nil {
		return err
	}

	res, err := ibcclienttypes.NewQueryClient(cctx).ConsensusStates(cmd.Context(), &ibcclienttypes.QueryConsensusStatesRequest{
		ClientId:   args[0],
		Pagination: pageReq,
	})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("ibc consensus-states query returned an empty response")
	}

	res.ConsensusStates = normalizeVendoredPage(res.ConsensusStates, pageReq)
	return cctx.PrintProto(res)
}

func queryIBCConnectionChannels(cmd *cobra.Command, args []string) error {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
	if err != nil {
		return err
	}

	res, err := channeltypes.NewQueryClient(cctx).ConnectionChannels(cmd.Context(), &channeltypes.QueryConnectionChannelsRequest{
		Connection: args[0],
		Pagination: pageReq,
	})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("ibc connection channels query returned an empty response")
	}

	res.Channels = normalizeVendoredPage(res.Channels, pageReq)
	return cctx.PrintProto(res)
}

// normalizeVendoredPage enforces the requested record limit while compensating
// for ibc-go offset paths that include a skipped prefix or lookahead item.
// Pagination metadata remains untouched.
func normalizeVendoredPage[T any](records []T, req *sdkquery.PageRequest) []T {
	if req == nil {
		req = &sdkquery.PageRequest{}
	}

	limit := req.Limit
	if limit == 0 {
		limit = sdkquery.DefaultLimit
	}

	start := uint64(0)
	if len(req.Key) == 0 && req.Offset != 0 {
		start = min(req.Offset, uint64(len(records)))
	}

	available := uint64(len(records)) - start
	count := min(limit, available)
	return records[start : start+count]
}

func queryIBCTransferEscrowAddress(cmd *cobra.Command, args []string) error {
	address := transfertypes.GetEscrowAddress(args[0], args[1])
	return printQueryScalar(cmd, address.String())
}
