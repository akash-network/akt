package context

import (
	"fmt"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	"pkg.akt.dev/akt/internal/output"
)

// keyringCommands returns the "context keyring" command tree: management of
// the keyring definitions themselves (SPEC §2.2.2), as opposed to the keys
// they hold (§2.2.3). Without it the only way to change a context's key
// storage after first run was to hand-edit config.yaml -- which is also the
// remedy a host with no system credential store needs (§1.5).
func keyringCommands(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keyring",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage keyring definitions",
		Long: `Keyrings are shared key storage referenced by name from a context.

The backend selects where keys are kept. "os" is an alias for the platform
credential store -- Keychain on macOS, Credential Manager on Windows, Secret
Service or KWallet on Linux -- and akt refuses to open it on a host that has
none rather than quietly storing keys somewhere else.`,
	}

	cmd.AddCommand(
		keyringCreateCmd(mgr),
		keyringListCmd(mgr),
		keyringSetCmd(mgr),
	)

	return cmd
}

func keyringCreateCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create <name> <backend>",
		Short:             "Create a keyring definition",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeKeyringArgs(mgr),
		Example: `  # A file-backed keyring for a headless server
  akt context keyring create headless file

  # Keep the store outside the akt home
  akt context keyring create secure file --dir /mnt/secure/keys`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, backend := args[0], args[1]
			if err := aktkeyring.ValidateBackend(backend); err != nil {
				return err
			}

			m := mgr()
			dir, _ := cmd.Flags().GetString(flagdefs.FlagDir)

			if err := m.CreateKeyring(aktctx.Keyring{Name: name, Backend: backend, Dir: dir}); err != nil {
				return err
			}

			recordContextAction(m.Root(), activeContextFromCmd(cmd, m), "keyring-create", map[string]string{
				"keyring": name,
				"backend": backend,
			})

			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Created keyring %q (backend: %s)\n", name, backend)
			return err
		},
	}

	cmd.Flags().String(flagdefs.FlagDir, "", "Keyring directory (default: <home>/keyrings/<name>/)")

	return cmd
}

func keyringListCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List keyring definitions",
		Args:    cobra.NoArgs,
		Example: `  akt context keyring list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()
			keyrings := m.ListKeyrings()

			if len(keyrings) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(),
					"No keyrings configured. Create one with: akt context keyring create default file")
				return err
			}

			type keyringRow struct {
				Name      string   `json:"name"      yaml:"name"`
				Backend   string   `json:"backend"   yaml:"backend"`
				Effective string   `json:"effective" yaml:"effective"`
				Available bool     `json:"available" yaml:"available"`
				Dir       string   `json:"dir"       yaml:"dir"`
				UsedBy    []string `json:"used_by"   yaml:"used-by"`
			}

			columns := []output.Column{
				{Header: "NAME"},
				{Header: "BACKEND"},
				{Header: "EFFECTIVE"},
				{Header: "DIR"},
				{Header: "USED BY"},
			}

			rows := make([][]string, 0, len(keyrings))
			data := make([]keyringRow, 0, len(keyrings))

			for _, kr := range keyrings {
				backend := kr.Backend
				if backend == "" {
					backend = aktctx.DefaultKeyring().Backend
				}

				// Report what this host actually provides, never the
				// configured value dressed up as a fact (SPEC §1.5).
				effective, available := aktkeyring.EffectiveBackend(m.Root(), kr)
				display := effective
				if !available {
					display = "unavailable"
				}

				users := m.KeyringUsers(kr.Name)
				usedBy := "(none)"
				if len(users) > 0 {
					usedBy = strings.Join(users, ", ")
				}

				rows = append(rows, []string{
					kr.Name, backend, display, aktctx.KeyringDir(m.Root(), kr), usedBy,
				})
				data = append(data, keyringRow{
					Name:      kr.Name,
					Backend:   backend,
					Effective: effective,
					Available: available,
					Dir:       aktctx.KeyringDir(m.Root(), kr),
					UsedBy:    users,
				})
			}

			return output.PrintData(cmd, columns, rows, data)
		},
	}
}

func keyringSetCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set <name> <backend>",
		Short:             "Change a keyring's backend",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeKeyringArgs(mgr),
		Long: `Change where a keyring stores its keys.

Backends do not share storage, so keys added under the previous backend stay
where they are; re-add or import them under the new backend.`,
		Example: `  # Persist the remedy for a host with no system credential store
  akt context keyring set default file

  # Move the store as well
  akt context keyring set default file --dir /mnt/secure/keys`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, backend := args[0], args[1]
			if err := aktkeyring.ValidateBackend(backend); err != nil {
				return err
			}

			m := mgr()

			previous := ""
			if existing := m.GetKeyring(name); existing != nil {
				previous = existing.Backend
			}

			dirChanged := cmd.Flags().Changed(flagdefs.FlagDir)
			dir, _ := cmd.Flags().GetString(flagdefs.FlagDir)

			if err := m.UpdateKeyring(name, func(kr *aktctx.Keyring) error {
				kr.Backend = backend
				if dirChanged {
					kr.Dir = dir
				}
				return nil
			}); err != nil {
				return err
			}

			recordContextAction(m.Root(), activeContextFromCmd(cmd, m), "keyring-set", map[string]string{
				"keyring":  name,
				"backend":  backend,
				"previous": previous,
			})

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Keyring %q now uses backend %s\n", name, backend); err != nil {
				return err
			}

			if previous != "" && previous != backend {
				_, err := fmt.Fprintf(out,
					"Keys stored under the %s backend are not migrated; re-add or import them under %s.\n",
					previous, backend)
				return err
			}

			return nil
		},
	}

	cmd.Flags().String(flagdefs.FlagDir, "", "Keyring directory (default: <home>/keyrings/<name>/)")

	return cmd
}

// completeKeyringArgs completes the keyring name first, then the backend.
func completeKeyringArgs(mgr func() *aktctx.Manager) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 1 {
			return aktkeyring.Backends(), cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) > 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		m := mgr()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		keyrings := m.ListKeyrings()
		names := make([]string, 0, len(keyrings))
		for _, kr := range keyrings {
			names = append(names, kr.Name)
		}

		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
