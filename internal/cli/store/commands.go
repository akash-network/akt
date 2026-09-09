// Package store implements the `akt store` CLI commands for managing the
// local deployment store.
package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

// Commands returns the `akt store` command group.
func Commands(homeFn func() string, ctxNameFn func() string, mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage the local deployment store",
		Long:  "View store status, sync with the chain, export records, and import from backups.",
	}

	cmd.AddCommand(
		statusCmd(homeFn, ctxNameFn),
		syncCmd(homeFn, ctxNameFn, mgrFn),
		exportCmd(homeFn, ctxNameFn),
		importCmd(homeFn, ctxNameFn),
	)

	return cmd
}

// storePath resolves the context's store database through the shared path
// helper, so every consumer (this group, the workflow persistence of SPEC
// §6.6) is guaranteed to open the same file.
func storePath(homeFn func() string, ctxNameFn func() string) string {
	return aktctx.StoreDBPath(homeFn(), ctxNameFn())
}

func openStore(ctx context.Context, homeFn func() string, ctxNameFn func() string) (*bbolt.BoltStore, error) {
	return bbolt.OpenContext(ctx, homeFn(), ctxNameFn())
}

func statusCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Args:  cobra.NoArgs,
		Short: "Display local store information",
		Example: `  # Show store status for the current context
  akt store status`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			s, err := openStore(ctx, homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			stats, err := s.Stats(ctx)
			if err != nil {
				return fmt.Errorf("read stats: %w", err)
			}

			ss, err := s.GetSyncState(ctx)
			if err != nil {
				return fmt.Errorf("read sync state: %w", err)
			}

			p := storePath(homeFn, ctxNameFn)
			ctxName := ctxNameFn()

			// Get DB file size.
			var dbSize int64
			if fi, statErr := os.Stat(p); statErr == nil {
				dbSize = fi.Size()
			}

			if f := output.FormatFromCmd(cmd); f != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), f, struct {
					Context               string               `json:"context"               yaml:"context"`
					StorePath             string               `json:"storePath"             yaml:"storePath"`
					DatabaseBytes         int64                `json:"databaseBytes"         yaml:"databaseBytes"`
					SchemaVersion         uint64               `json:"schemaVersion"         yaml:"schemaVersion"`
					Records               *sstore.StoreStats   `json:"records"               yaml:"records"`
					NetworkReconciliation reconciliationStatus `json:"networkReconciliation" yaml:"networkReconciliation"`
				}{ctxName, p, dbSize, s.SchemaVersion(), stats, describeReconciliation(ss)})
			}

			checked := output.NewCheckedTerminalWriter(cmd.OutOrStdout())
			out := io.Writer(checked)

			fmt.Fprintln(out, pretty.Section("Store"))
			pretty.KV(out, "Context", ctxName)
			pretty.KV(out, "Store Path", p)
			pretty.KV(out, "Database", fmt.Sprintf("deployments.db (%s)", pretty.FormatBytes(uint64(dbSize))))
			pretty.KV(out, "Schema", fmt.Sprintf("v%d", s.SchemaVersion()))
			pretty.Newline(out)

			fmt.Fprintln(out, pretty.Section("Records"))
			pretty.KV(out, "Deployments", formatStateCounts(stats.Deployments,
				stateCount{"active", stats.ActiveDeployments},
				stateCount{"closed", stats.ClosedDeployments},
			))
			pretty.KV(out, "Leases", formatStateCounts(stats.Leases,
				stateCount{"active", stats.ActiveLeases},
				stateCount{"closed", stats.ClosedLeases},
				stateCount{"insufficient funds", stats.InsufficientFundsLeases},
			))
			pretty.KV(out, "Bids", formatStateCounts(stats.Bids,
				stateCount{"open", stats.OpenBids},
				stateCount{"matched", stats.MatchedBids},
				stateCount{"lost", stats.LostBids},
				stateCount{"closed", stats.ClosedBids},
			))
			pretty.Newline(out)

			fmt.Fprintln(out, pretty.Section("Network Reconciliation"))
			if ss != nil && ss.LastBlockHeight > 0 {
				syncTime := time.Unix(ss.LastSyncTime, 0).UTC().Format(time.RFC3339)
				pretty.KV(out, "Last Block", pretty.FormatNumber(ss.LastBlockHeight))
				pretty.KV(out, "Last Run", syncTime)
				pretty.KV(out, "Status", "completed")
			} else {
				pretty.KV(out, "Status", "not yet run")
			}
			pretty.KV(out, "Run", "akt store sync")

			return checked.Err()
		},
	}
}

type stateCount struct {
	name  string
	count int64
}

func formatStateCounts(total int64, counts ...stateCount) string {
	if total == 0 {
		return "0"
	}

	parts := make([]string, 0, len(counts)+1)
	var known int64
	for _, count := range counts {
		known += count.count
		if count.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count.count, count.name))
		}
	}

	if other := total - known; other > 0 {
		parts = append(parts, fmt.Sprintf("%d other", other))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d", total)
	}

	return fmt.Sprintf("%d (%s)", total, strings.Join(parts, ", "))
}

type reconciliationStatus struct {
	Status          string `json:"status"                    yaml:"status"`
	LastBlockHeight int64  `json:"lastBlockHeight,omitempty" yaml:"lastBlockHeight,omitempty"`
	LastRun         string `json:"lastRun,omitempty"         yaml:"lastRun,omitempty"`
	Command         string `json:"command,omitempty"         yaml:"command,omitempty"`
}

func describeReconciliation(ss *sstore.SyncState) reconciliationStatus {
	if ss == nil || ss.LastBlockHeight == 0 {
		return reconciliationStatus{Status: "not_yet_run", Command: "akt store sync"}
	}

	return reconciliationStatus{
		Status:          "completed",
		LastBlockHeight: ss.LastBlockHeight,
		LastRun:         time.Unix(ss.LastSyncTime, 0).UTC().Format(time.RFC3339),
	}
}

func exportCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Args:  cobra.NoArgs,
		Short: "Export the local store to YAML or JSON",
		Long: "Export the local store to YAML or JSON. The top-level version is " +
			"the export format, schema_version is the database layout, and each " +
			"record_version is that record's update revision.",
		Example: `  # Export to stdout as YAML
  akt store export

  # Export to a file as JSON
  akt store export -o json --file backup.json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore(cmd.Context(), homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			format := sstore.FormatYAML
			if o, _ := cmd.Flags().GetString(flagdefs.FlagOutput); o == "json" {
				format = sstore.FormatJSON
			}

			file, _ := cmd.Flags().GetString(flagdefs.FlagFile)
			if file != "" {
				return exportFileAtomically(cmd.Context(), s, file, format, ctxNameFn())
			}

			return s.Export(cmd.Context(), cmd.OutOrStdout(), format, ctxNameFn())
		},
	}

	cmd.Flags().String(flagdefs.FlagFile, "", "Output file (default: stdout)")

	return cmd
}

func exportFileAtomically(ctx context.Context, s *bbolt.BoltStore, destination string, format sstore.ExportFormat, contextName string) error {
	temp, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary export: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := writeStoreExport(ctx, s, temp, format, contextName); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return fmt.Errorf("replace export file: %w", err)
	}

	return nil
}

type syncedWriteCloser interface {
	io.Writer
	Sync() error
	Close() error
}

func writeStoreExport(ctx context.Context, s *bbolt.BoltStore, destination syncedWriteCloser, format sstore.ExportFormat, contextName string) error {
	if err := s.Export(ctx, destination, format, contextName); err != nil {
		return err
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("flush temporary export: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close temporary export: %w", err)
	}

	return nil
}

func importCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import records from a previously exported file",
		Args:  cobra.ExactArgs(1),
		Example: `  # Import with merge (default)
  akt store import backup.yaml

  # Import replacing all existing data
  akt store import backup.yaml --replace

  # Dry run — show what would be imported
  akt store import backup.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			merge, _ := cmd.Flags().GetBool(flagdefs.FlagMerge)
			replace, _ := cmd.Flags().GetBool(flagdefs.FlagReplace)
			dryRun, _ := cmd.Flags().GetBool(flagdefs.FlagDryRun)
			if !merge && !replace {
				return errors.New("replacing store contents requires the explicit --replace flag")
			}
			mergeMode := !replace

			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open import file: %w", err)
			}
			defer func() { _ = f.Close() }()

			format := sstore.FormatYAML
			if filepath.Ext(args[0]) == ".json" {
				format = sstore.FormatJSON
			}

			if dryRun {
				if err := bbolt.ValidateImportSnapshot(cmd.Context(), storePath(homeFn, ctxNameFn), f, format, mergeMode); err != nil {
					return fmt.Errorf("validate import: %w", err)
				}
				if !cliutil.IsQuiet(cmd) {
					notice := output.NewCheckedWriter(cmd.ErrOrStderr())
					if _, err := fmt.Fprintln(notice, "Dry run — input is valid; no changes were made."); err != nil {
						return notice.Complete(err)
					}
				}
				return nil
			}
			if replace {
				yes, _ := cmd.Flags().GetBool(flagdefs.FlagSkipConfirmation)
				if !yes {
					confirmed, err := confirmStoreReplacement(cmd, ctxNameFn())
					if err != nil {
						return err
					}
					if !confirmed {
						if !cliutil.IsQuiet(cmd) {
							notice := output.NewCheckedWriter(cmd.ErrOrStderr())
							if _, err := fmt.Fprintln(notice, "Import cancelled."); err != nil {
								return notice.Complete(err)
							}
						}
						return nil
					}
				}
			}

			s, err := openStore(cmd.Context(), homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			if err := s.Import(cmd.Context(), f, format, mergeMode); err != nil {
				return fmt.Errorf("import: %w", err)
			}

			if !cliutil.IsQuiet(cmd) {
				notice := output.NewCheckedWriter(cmd.ErrOrStderr())
				if _, err := fmt.Fprintln(notice, "Import complete."); err != nil {
					return notice.Complete(err)
				}
			}
			return nil
		},
	}

	cmd.Flags().Bool(flagdefs.FlagMerge, true, "Merge with existing records (default)")
	cmd.Flags().Bool(flagdefs.FlagReplace, false, "Replace entire store contents")
	cmd.Flags().Bool(flagdefs.FlagDryRun, false, "Show what would be imported")
	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip replacement confirmation")

	return cmd
}

func confirmStoreReplacement(cmd *cobra.Command, contextName string) (bool, error) {
	diagnostics := output.NewCheckedWriter(cmd.ErrOrStderr())
	if _, err := fmt.Fprintf(
		diagnostics,
		"Replace every record in store context %q? This cannot be undone. [y/N]: ",
		contextName,
	); err != nil {
		return false, fmt.Errorf("write replacement confirmation: %w", diagnostics.Complete(err))
	}

	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || len(answer) == 0) {
		return false, fmt.Errorf("read replacement confirmation: %w", err)
	}
	answer = strings.TrimSpace(answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}
