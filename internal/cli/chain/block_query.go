package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	clioutput "pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
)

func positiveBlockHeight(value string) (int64, error) {
	height, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block height: %w", err)
	}
	if height <= 0 {
		return 0, fmt.Errorf("block height must be positive: %d", height)
	}

	return height, nil
}

func latestBlockHeight(ctx context.Context, node sdkclient.CometRPC) (int64, error) {
	status, err := node.Status(ctx)
	if err != nil {
		return 0, err
	}
	if status == nil {
		return 0, errors.New("missing status result")
	}
	if status.SyncInfo.LatestBlockHeight <= 0 {
		return 0, errors.New("node has no committed block height")
	}

	return status.SyncInfo.LatestBlockHeight, nil
}

func blockQueryOutputFormat(cmd *cobra.Command) clioutput.Format {
	if clioutput.FormatFromCmd(cmd) == clioutput.FormatYAML {
		return clioutput.FormatYAML
	}

	return clioutput.FormatJSON
}

func responseBlock(result *coretypes.ResultBlock) *cmtproto.Block {
	return sdk.NewResponseResultBlock(result, result.Block.Time.Format(time.RFC3339))
}

// QueryBlocksCmd returns a command to search through blocks by events.
func QueryBlocksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocks [query]",
		Short: "Query for paginated blocks that match a set of events",
		Long: `Search for blocks that match the exact given events where results are paginated.
The events query is directly passed to CometBFT's RPC BlockSearch method and must
conform to CometBFT's query syntax.
Refer to the documentation of the module you are querying for the events it
emits.
`,
		Example: fmt.Sprintf(
			"$ %[1]s query blocks \"message.sender='cosmos1...' AND block.height > 7\" --page 1 --limit 30 --order_by asc",
			version.AppName,
		),
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if cflags.ExprFromArgs(args, "") == "" && !cmd.Flags().Changed(flagdefs.FlagHeight) {
				return errors.New("query expression is required: pass it positionally")
			}

			return nil
		},
		PreRunE: directQueryWithoutHeightPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			// FEEDBACK(2026-07): --query disabled for the positional-only UX
			// trial; the positional expression is the only source (zero
			// fallback). Restore by uncommenting if users ask for the flag
			// form back.
			// query, _ := cmd.Flags().GetString(flagdefs.FlagQuery)
			queryExpr := cflags.ExprFromArgs(args, "")
			page, _ := cmd.Flags().GetInt(flagdefs.FlagPage)
			limit, _ := cmd.Flags().GetInt(flagdefs.FlagLimit)
			orderBy, _ := cmd.Flags().GetString(flagdefs.FlagOrderBy)

			cctx, err := GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			node, err := cctx.GetNode()
			if err != nil {
				return err
			}

			result, err := node.BlockSearch(cmd.Context(), queryExpr, &page, &limit, orderBy)
			if err != nil {
				return err
			}
			if result == nil {
				return errors.New("missing block search result")
			}

			blocks := make([]*cmtproto.Block, len(result.Blocks))
			for i, resultBlock := range result.Blocks {
				if resultBlock == nil || resultBlock.Block == nil {
					return fmt.Errorf("invalid block at index %d: missing block body", i)
				}
				blocks[i] = responseBlock(resultBlock)
				if blocks[i] == nil {
					return fmt.Errorf("invalid block at index %d: cannot encode block", i)
				}
			}

			output := sdk.NewSearchBlocksResult(
				int64(result.TotalCount),
				int64(len(blocks)),
				int64(page),
				int64(limit),
				blocks,
			)

			return pretty.PrintQueryResult(cmd, cctx, output)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cmd.Flags().Int(flagdefs.FlagPage, query.DefaultPage, "Query a specific page of paginated results")
	cmd.Flags().Int(flagdefs.FlagLimit, query.DefaultLimit, "Query number of transactions results per page returned")
	// FEEDBACK(2026-07): --query disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String(flagdefs.FlagQuery, "", "The blocks events query per CometBFT's query semantics")
	cmd.Flags().Var(
		clioutput.NewEnumFlag("", "", "asc", "desc"),
		flagdefs.FlagOrderBy,
		"The ordering semantics (asc|desc; empty uses the node default)",
	)

	return cmd
}

// QueryBlockCmd implements the default command for a Block query.
func QueryBlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block --type=[height|hash] [height|hash]",
		Short: "Query for a committed block by height, hash, or event(s)",
		Long:  "Query for a specific committed block using the CometBFT RPC `block` and `block_by_hash` method",
		Example: strings.TrimSpace(fmt.Sprintf(`
$ %s query block --%s=%s <height>
$ %s query block --%s=%s <hash>
`,
			version.AppName, flagdefs.FlagType, cflags.TypeHeight,
			version.AppName, flagdefs.FlagType, cflags.TypeHash)),
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return err
			}

			typ, _ := cmd.Flags().GetString(flagdefs.FlagType)
			if typ == cflags.TypeHash && cmd.Flags().Changed(flagdefs.FlagType) && (len(args) == 0 || args[0] == "") {
				return errors.New("block hash is required when --type=hash")
			}
			if cmd.Flags().Changed(flagdefs.FlagHeight) {
				height, _ := cmd.Flags().GetInt64(flagdefs.FlagHeight)
				if height <= 0 {
					return fmt.Errorf("block height must be positive: %d", height)
				}
				// Preserve the dedicated positional/flag conflict error from PreRunE.
				if len(args) > 0 {
					return nil
				}
			}
			if len(args) == 0 {
				return nil
			}
			if typ == cflags.TypeHeight {
				_, err := positiveBlockHeight(args[0])
				return err
			}
			if _, err := hex.DecodeString(args[0]); err != nil {
				return fmt.Errorf("failed to decode block hash: %w", err)
			}

			return nil
		},
		PreRunE: directQueryHeightPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx, err := GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			typ, _ := cmd.Flags().GetString(flagdefs.FlagType)
			if len(args) == 0 {
				// Preserve the established no-argument behavior: query latest height.
				typ = cflags.TypeHeight
			}
			node, err := cctx.GetNode()
			if err != nil {
				return err
			}

			switch typ {
			case cflags.TypeHeight:
				var height int64
				heightStr := ""
				if len(args) > 0 {
					heightStr = args[0]
				}

				switch {
				case heightStr == "" && cctx.Height != 0:
					// --height is the documented way to pin a query to a block.
					// It was parsed into the client context and then ignored
					// here, so `query block --height N` answered with the latest
					// block at exit 0 -- every conclusion drawn from it about
					// historical state was about the wrong point in time.
					if cctx.Height < 0 {
						return fmt.Errorf("block height must be positive: %d", cctx.Height)
					}
					height = cctx.Height
				case heightStr == "":
					height, err = latestBlockHeight(cmd.Context(), node)
					if err != nil {
						return fmt.Errorf("failed to get chain height: %w", err)
					}

					cliutil.Statusf(cmd, "no height given; using latest block %d", height)
				default:
					height, err = positiveBlockHeight(heightStr)
					if err != nil {
						return err
					}
				}

				result, err := node.Block(cmd.Context(), &height)
				if err != nil {
					return err
				}
				if result == nil || result.Block == nil || result.Block.Height == 0 {
					return fmt.Errorf("no block found with height %d", height)
				}
				output := responseBlock(result)
				if output == nil {
					return fmt.Errorf("invalid block returned for height %d", height)
				}

				return pretty.PrintQueryResult(cmd, cctx, output)

			default: // --type is constrained to height|hash at flag parse time.
				hash, err := hex.DecodeString(args[0])
				if err != nil {
					return fmt.Errorf("failed to decode block hash: %w", err)
				}
				result, err := node.BlockByHash(cmd.Context(), hash)
				if err != nil {
					return err
				}
				if result == nil || result.Block == nil {
					return fmt.Errorf("no block found with hash %s", args[0])
				}
				output := responseBlock(result)
				if output == nil {
					return fmt.Errorf("invalid block returned for hash %s", args[0])
				}

				return pretty.PrintQueryResult(cmd, cctx, output)
			}
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cmd.Flags().Var(
		clioutput.NewEnumFlag(cflags.TypeHash, cflags.TypeHeight, cflags.TypeHash),
		flagdefs.FlagType,
		fmt.Sprintf("The block identifier type (%s|%s)", cflags.TypeHeight, cflags.TypeHash),
	)

	return cmd
}

// QueryBlockResultsCmd implements the default command for a BlockResults query.
func QueryBlockResultsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block-results [height]",
		Short: "Query for a committed block's results by height",
		Long:  "Query for a specific committed block's results using the CometBFT RPC `block_results` method",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.RangeArgs(0, 1)(cmd, args); err != nil {
				return err
			}
			if cmd.Flags().Changed(flagdefs.FlagHeight) {
				height, _ := cmd.Flags().GetInt64(flagdefs.FlagHeight)
				if height <= 0 {
					return fmt.Errorf("block height must be positive: %d", height)
				}
				// Preserve the dedicated positional/flag conflict error from PreRunE.
				if len(args) > 0 {
					return nil
				}
			}
			if len(args) == 0 {
				return nil
			}

			_, err := positiveBlockHeight(args[0])
			return err
		},
		PreRunE: directQueryHeightPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx, err := GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			var height int64

			switch {
			case len(args) > 0:
				height, err = positiveBlockHeight(args[0])
				if err != nil {
					return err
				}
			case cctx.Height != 0:
				// See QueryBlockCmd: --height was read and then dropped here.
				if cctx.Height < 0 {
					return fmt.Errorf("block height must be positive: %d", cctx.Height)
				}
				height = cctx.Height
			default:
				node, nodeErr := cctx.GetNode()
				if nodeErr != nil {
					return nodeErr
				}
				height, err = latestBlockHeight(cmd.Context(), node)
				if err != nil {
					return fmt.Errorf("failed to get chain height: %w", err)
				}

				cliutil.Statusf(cmd, "no height given; using latest block %d", height)
			}

			node, err := cctx.GetNode()
			if err != nil {
				return err
			}
			blockRes, err := node.BlockResults(cmd.Context(), &height)
			if err != nil {
				return err
			}
			if blockRes == nil {
				return fmt.Errorf("no block results found with height %d", height)
			}

			return clioutput.FprintJSONSemantics(cmd.OutOrStdout(), blockQueryOutputFormat(cmd), blockRes)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
