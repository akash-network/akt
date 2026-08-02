// Package store implements the `akt store` CLI commands for managing the
// local deployment store.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

// Commands returns the `akt store` command group.
func Commands(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage the local deployment store",
		Long:  "View store status, export records, and import from backups.",
	}

	cmd.AddCommand(
		statusCmd(homeFn, ctxNameFn),
		exportCmd(homeFn, ctxNameFn),
		importCmd(homeFn, ctxNameFn),
	)

	return cmd
}

func storePath(homeFn func() string, ctxNameFn func() string) string {
	return filepath.Join(homeFn(), "contexts", ctxNameFn(), "store", "deployments.db")
}

func openStore(homeFn func() string, ctxNameFn func() string) (sstore.Store, error) {
	p := storePath(homeFn, ctxNameFn)

	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}

	return bbolt.Open(p)
}

func statusCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Args:  cobra.NoArgs,
		Short: "Display local store information",
		Example: `  # Show store status for the current context
  akt store status`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore(homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()

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
					Context       string `json:"context"       yaml:"context"`
					StorePath     string `json:"storePath"     yaml:"storePath"`
					DatabaseBytes int64  `json:"databaseBytes" yaml:"databaseBytes"`
					SchemaVersion uint64 `json:"schemaVersion" yaml:"schemaVersion"`
				}{ctxName, p, dbSize, s.SchemaVersion()})
			}

			out := output.TerminalAwareWriter(cmd.OutOrStdout())

			fmt.Fprintln(out, pretty.Section("Store"))
			pretty.KV(out, "Context", ctxName)
			pretty.KV(out, "Store Path", p)
			pretty.KV(out, "Database", fmt.Sprintf("deployments.db (%s)", pretty.FormatBytes(uint64(dbSize))))
			pretty.KV(out, "Schema", fmt.Sprintf("v%d", s.SchemaVersion()))
			pretty.Newline(out)

			total := stats.ActiveDeployments + stats.ClosedDeployments
			fmt.Fprintln(out, pretty.Section("Records"))
			pretty.KV(out, "Deployments", fmt.Sprintf("%d (%d active, %d closed)",
				total, stats.ActiveDeployments, stats.ClosedDeployments))
			pretty.KV(out, "Leases", fmt.Sprintf("%d", stats.Leases))
			pretty.KV(out, "Bids", fmt.Sprintf("%d", stats.Bids))
			pretty.Newline(out)

			fmt.Fprintln(out, pretty.Section("Sync State"))
			if ss != nil && ss.LastBlockHeight > 0 {
				syncTime := time.Unix(ss.LastSyncTime, 0).UTC().Format(time.RFC3339)
				pretty.KV(out, "Last Block", pretty.FormatNumber(ss.LastBlockHeight))
				pretty.KV(out, "Last Sync", syncTime)
				pretty.KV(out, "Status", "synced")
			} else {
				pretty.KV(out, "Status", "not synced")
			}

			return nil
		},
	}
}

func exportCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "export",
		Args:  cobra.NoArgs,
		Short: "Export the local store to YAML or JSON",
		Example: `  # Export to stdout as YAML
  akt store export

  # Export to a file as JSON
  akt store export -o json --file backup.json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore(homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			format := sstore.FormatYAML
			if o, _ := cmd.Flags().GetString("output"); o == "json" {
				format = sstore.FormatJSON
			}

			var w *os.File
			if file != "" {
				w, err = os.Create(file)
				if err != nil {
					return fmt.Errorf("create file: %w", err)
				}
				defer func() { _ = w.Close() }()
			} else {
				w = os.Stdout
			}

			return s.Export(cmd.Context(), w, format, ctxNameFn())
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Output file (default: stdout)")

	return cmd
}

func importCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	var (
		merge   bool
		replace bool
		dryRun  bool
	)

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
			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open import file: %w", err)
			}
			defer func() { _ = f.Close() }()

			if dryRun {
				if !cliutil.IsQuiet(cmd) {
					fmt.Fprintln(cmd.ErrOrStderr(), "Dry run — no changes will be made.")
				}
				return nil
			}

			s, err := openStore(homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			mergeMode := merge && !replace
			format := sstore.FormatYAML
			if filepath.Ext(args[0]) == ".json" {
				format = sstore.FormatJSON
			}

			if err := s.Import(cmd.Context(), f, format, mergeMode); err != nil {
				return fmt.Errorf("import: %w", err)
			}

			if !cliutil.IsQuiet(cmd) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Import complete.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&merge, "merge", true, "Merge with existing records (default)")
	cmd.Flags().BoolVar(&replace, "replace", false, "Replace entire store contents")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be imported")

	return cmd
}
