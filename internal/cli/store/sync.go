package store

import (
	"context"
	"fmt"
	"io"
	"sort"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
	sstore "pkg.akt.dev/akt/internal/store"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

// trackAllAccounts is the tracked-accounts value that selects every key in the
// context's keyring (SPEC §6.7).
const trackAllAccounts = "*"

// syncCmd implements `akt store sync` (SPEC §2.5): the on-demand full
// reconciliation of SPEC §6.4.
//
// Workflow runs record their own outcome (SPEC §6.6), but only what one run
// observed. This is the escape hatch for everything else — deployments made
// before akt or elsewhere, escrow figures that move every block, leases a
// provider closed.
func syncCmd(homeFn func() string, ctxNameFn func() string, mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync [account]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Reconcile the local store with on-chain state",
		Long: `Reconcile the local store with on-chain state.

Queries every deployment owned by the context's tracked accounts, plus their
leases and bids, and writes them to the local store. Local-only fields the
chain does not carry -- SDL path and hash, labels, notes, tags -- are kept.

Without an argument the context's tracked-accounts setting is used, which
defaults to the context's default account. A Console context with neither uses
the unique owners in its local deployment records. If it has no local owners,
pass one explicitly with akt store sync <address>.

The optional account argument reconciles a single account instead; it takes a
bech32 address or the name of a key in the context's keyring.`,
		Example: `  # Reconcile the context's tracked accounts
  akt store sync

  # Reconcile one account
  akt store sync akash1zn43lm...

  # Reconcile the account behind a named key
  akt store sync alice`,
		// Reconciliation is read-only against the chain: it queries
		// deployments, leases, and bids. No signing is involved.
		Annotations: map[string]string{
			capability.AnnotationKey: string(capability.ChainQuery),
		},
		PersistentPreRunE: chaincli.QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cl, err := chaincli.LightClientFromContext(ctx)
			if err != nil {
				return fmt.Errorf("store sync needs a chain connection: %w", err)
			}

			account := ""
			if len(args) == 1 {
				account = args[0]
			}

			// A context that will not resolve is tolerated: the client
			// context still carries a default account to fall back on, and
			// resolveTrackedAccounts reports it if that is missing too.
			var rc *aktctx.Context
			if mgrFn != nil {
				if mgr := mgrFn(); mgr != nil {
					rc, _ = mgr.Resolve(ctxNameFn())
				}
			}

			s, err := openStore(ctx, homeFn, ctxNameFn)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			owners, err := resolveOwnersForSync(ctx, rc, account, cl.ClientContext(), s)
			if err != nil {
				return err
			}

			stats, err := syncpkg.New(s, owners).ReconcileNow(ctx, syncpkg.NewChainQuerier(cl))
			if err != nil {
				return fmt.Errorf("reconcile store with chain state: %w", err)
			}

			return renderSyncResult(cmd, owners, stats)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

func resolveOwnersForSync(
	ctx context.Context,
	rc *aktctx.Context,
	override string,
	cctx sdkclient.Context,
	s sstore.Store,
) ([]string, error) {
	if override != "" || rc == nil || rc.AuthMethod != aktctx.AuthMethodConsoleAPI ||
		len(rc.TrackedAccounts) > 0 || rc.DefaultAccount != "" || !cctx.GetFromAddress().Empty() {
		return resolveTrackedAccounts(rc, override, cctx)
	}

	records, err := s.ListDeployments(ctx, sstore.DeploymentFilter{})
	if err != nil {
		return nil, fmt.Errorf("list local deployments for Console owners: %w", err)
	}

	seen := make(map[string]struct{})
	for _, record := range records {
		if record == nil || record.Owner == "" {
			continue
		}
		addr, err := sdk.AccAddressFromBech32(record.Owner)
		if err != nil {
			return nil, fmt.Errorf("local deployment %d has invalid owner %q: %w", record.DSeq, record.Owner, err)
		}
		seen[addr.String()] = struct{}{}
	}

	owners := make([]string, 0, len(seen))
	for owner := range seen {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	if len(owners) == 0 {
		return nil, fmt.Errorf("no account to sync: pass one explicitly (`akt store sync <address>`); this Console context has no local deployment owners yet")
	}

	return owners, nil
}

// syncResult is the machine-readable shape of a completed reconciliation.
type syncResult struct {
	Accounts    []string `json:"accounts"    yaml:"accounts"`
	Deployments int      `json:"deployments" yaml:"deployments"`
	Leases      int      `json:"leases"      yaml:"leases"`
	Bids        int      `json:"bids"        yaml:"bids"`
	Height      int64    `json:"height"      yaml:"height"`
}

func renderSyncResult(cmd *cobra.Command, owners []string, stats syncpkg.ReconcileStats) error {
	res := syncResult{
		Accounts:    owners,
		Deployments: stats.Deployments,
		Leases:      stats.Leases,
		Bids:        stats.Bids,
		Height:      stats.Height,
	}

	if f := output.FormatFromCmd(cmd); f != output.FormatTable {
		return output.Fprint(cmd.OutOrStdout(), f, res)
	}

	if cliutil.IsQuiet(cmd) {
		return nil
	}

	checked := output.NewCheckedTerminalWriter(cmd.OutOrStdout())
	out := io.Writer(checked)

	fmt.Fprintln(out, pretty.Section("Store Sync"))
	pretty.KV(out, "Accounts", fmt.Sprintf("%d", len(owners)))
	// Addresses are never abbreviated: the reader has to be able to tell
	// which wallet was reconciled, and to copy it.
	for _, owner := range owners {
		pretty.SubKV(out, "Owner", owner)
	}
	pretty.KV(out, "Deployments", fmt.Sprintf("%d", stats.Deployments))
	pretty.KV(out, "Leases", fmt.Sprintf("%d", stats.Leases))
	pretty.KV(out, "Bids", fmt.Sprintf("%d", stats.Bids))
	pretty.KV(out, "Height", pretty.FormatNumber(stats.Height))

	return checked.Err()
}

// resolveTrackedAccounts turns the context's tracked-accounts setting into the
// owner addresses reconciliation covers (SPEC §6.7).
//
// override, when set, replaces the configured list with a single account.
// An empty configured list falls back to the context's default account.
// The single entry "*" expands to every key in the context's keyring.
// Every other entry is a bech32 address used as-is, or a keyring key name
// resolved to its address. An entry that resolves to nothing is an error
// naming it: silently skipping it would report a successful sync that quietly
// left an account out.
func resolveTrackedAccounts(rc *aktctx.Context, override string, cctx sdkclient.Context) ([]string, error) {
	entries := []string{override}

	if override == "" {
		if rc != nil {
			entries = rc.TrackedAccounts
		} else {
			entries = nil
		}

		if len(entries) == 0 {
			owner, err := defaultAccountAddress(rc, cctx)
			if err != nil {
				return nil, err
			}

			return []string{owner}, nil
		}
	}

	if len(entries) == 1 && entries[0] == trackAllAccounts {
		return keyringAddresses(cctx.Keyring)
	}

	seen := make(map[string]struct{}, len(entries))
	owners := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry == trackAllAccounts {
			return nil, fmt.Errorf(
				"tracked-accounts: %q selects every keyring account and cannot be combined with other entries",
				trackAllAccounts,
			)
		}

		addr, err := accountAddress(entry, cctx.Keyring)
		if err != nil {
			return nil, err
		}

		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		owners = append(owners, addr)
	}

	sort.Strings(owners)

	return owners, nil
}

// defaultAccountAddress resolves the context's default account to an address.
func defaultAccountAddress(rc *aktctx.Context, cctx sdkclient.Context) (string, error) {
	if addr := cctx.GetFromAddress(); !addr.Empty() {
		return addr.String(), nil
	}

	if rc != nil && rc.DefaultAccount != "" {
		return accountAddress(rc.DefaultAccount, cctx.Keyring)
	}

	return "", fmt.Errorf(
		"no account to sync: name one (`akt store sync <address|key>`), " +
			"set the context's default-account, or list accounts in its tracked-accounts",
	)
}

// accountAddress resolves one tracked-accounts entry to a bech32 address.
func accountAddress(entry string, kr sdkkeyring.Keyring) (string, error) {
	if addr, err := sdk.AccAddressFromBech32(entry); err == nil {
		return addr.String(), nil
	}

	if kr == nil {
		return "", fmt.Errorf("account %q is not an address and no keyring is configured to look it up in", entry)
	}

	rec, err := kr.Key(entry)
	if err != nil {
		return "", fmt.Errorf("account %q is neither an address nor a key in the context's keyring: %w", entry, err)
	}

	addr, err := rec.GetAddress()
	if err != nil {
		return "", fmt.Errorf("key %q has no address: %w", entry, err)
	}

	return addr.String(), nil
}

// keyringAddresses returns every address in the keyring, for tracked-accounts
// set to "*".
func keyringAddresses(kr sdkkeyring.Keyring) ([]string, error) {
	if kr == nil {
		return nil, fmt.Errorf("tracked-accounts is %q but the context has no keyring to enumerate", trackAllAccounts)
	}

	recs, err := kr.List()
	if err != nil {
		return nil, fmt.Errorf("list keyring accounts: %w", err)
	}

	owners := make([]string, 0, len(recs))
	for _, rec := range recs {
		addr, err := rec.GetAddress()
		if err != nil {
			continue // a key without an address cannot own deployments
		}
		owners = append(owners, addr.String())
	}

	if len(owners) == 0 {
		return nil, fmt.Errorf("tracked-accounts is %q but the context's keyring holds no accounts", trackAllAccounts)
	}

	sort.Strings(owners)

	return owners, nil
}
