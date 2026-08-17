package context_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func newConfiguredManager(t *testing.T, mutate func(*aktctx.Config)) *aktctx.Manager {
	t.Helper()

	root := t.TempDir()
	cfg := aktctx.DefaultConfig()
	cfg.Networks = []aktctx.Network{{
		Name:          "mainnet",
		ChainID:       "akashnet-2",
		Endpoints:     aktctx.Endpoints{RPC: []string{"https://rpc.example"}},
		GasPrices:     "0.025uakt",
		GasAdjustment: "1.5",
	}}
	cfg.Contexts = []aktctx.Context{{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
		Keyring: aktctx.Keyring{Name: "default"},
		Gas:     "auto",
	}}
	if mutate != nil {
		mutate(&cfg)
	}
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig fixture: %v", err)
	}

	mgr, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func TestManagerSurfacesCorruptConfigOnOpenAndReload(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(aktctx.ConfigPath(root), []byte("contexts: [\n"), 0o600); err != nil {
			t.Fatalf("write malformed config: %v", err)
		}
		if _, err := aktctx.NewManager(root); err == nil {
			t.Fatal("NewManager must reject malformed config")
		}
	})

	t.Run("reload keeps last valid state", func(t *testing.T) {
		mgr := newConfiguredManager(t, nil)
		if err := os.WriteFile(aktctx.ConfigPath(mgr.Root()), []byte("contexts: [\n"), 0o600); err != nil {
			t.Fatalf("write malformed config: %v", err)
		}
		if err := mgr.Reload(); err == nil {
			t.Fatal("Reload must reject malformed config")
		}
		if got := mgr.GetContext("prod"); got == nil || got.Network.Name != "mainnet" {
			t.Fatalf("failed reload replaced last valid state: %+v", got)
		}
	})
}

func TestManagerRejectsInvalidNetworkAndContextOperations(t *testing.T) {
	sentinel := errors.New("validation failed")

	t.Run("network operations", func(t *testing.T) {
		mgr := newConfiguredManager(t, nil)
		if err := mgr.CreateNetwork(aktctx.Network{}); err == nil {
			t.Error("unnamed network was accepted")
		}
		if err := mgr.UpdateNetwork("missing", func(*aktctx.Network) error { return nil }); err == nil {
			t.Error("missing network update was accepted")
		}
		if err := mgr.UpdateNetwork("mainnet", func(*aktctx.Network) error { return sentinel }); !errors.Is(err, sentinel) {
			t.Errorf("callback error = %v, want sentinel", err)
		}
		if err := mgr.ForkNetwork("missing", "fork"); err == nil {
			t.Error("missing source network was forked")
		}
		if err := mgr.ForkNetwork("mainnet", "mainnet"); err == nil {
			t.Error("duplicate destination network was accepted")
		}
	})

	t.Run("context operations", func(t *testing.T) {
		mgr := newConfiguredManager(t, nil)
		cases := []struct {
			name string
			ctx  aktctx.Context
		}{
			{name: "empty name", ctx: aktctx.Context{}},
			{name: "duplicate", ctx: aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}}},
			{name: "networkless keyring auth", ctx: aktctx.Context{Name: "offline"}},
			{name: "missing keyring", ctx: aktctx.Context{Name: "bad-keyring", Network: aktctx.Network{Name: "mainnet"}, Keyring: aktctx.Keyring{Name: "missing"}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := mgr.CreateContext(tc.ctx); err == nil {
					t.Fatalf("CreateContext(%+v) unexpectedly succeeded", tc.ctx)
				}
			})
		}

		if err := mgr.UseContext("missing"); err == nil {
			t.Error("missing context became current")
		}
		if err := mgr.DeleteContext("missing", false); err == nil {
			t.Error("missing context deletion succeeded")
		}
		if err := mgr.RenameContext("missing", "new"); err == nil {
			t.Error("missing context rename succeeded")
		}
		if err := mgr.RenameContext("prod", "prod"); err == nil {
			t.Error("duplicate context rename succeeded")
		}
	})
}

func TestCreateContextFilesystemFailureDoesNotRegisterGhostContext(t *testing.T) {
	t.Run("data directory failure", func(t *testing.T) {
		root := t.TempDir()
		mgr, err := aktctx.NewManager(root)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "contexts"), []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err = mgr.CreateContext(aktctx.Context{
			Name:       "console",
			AuthMethod: aktctx.AuthMethodConsoleAPI,
		})
		if err == nil {
			t.Fatal("CreateContext unexpectedly succeeded")
		}
		if ghost := mgr.GetContext("console"); ghost != nil {
			t.Fatalf("failed create left an in-memory context: %+v", ghost)
		}
	})

	t.Run("config save failure", func(t *testing.T) {
		root := t.TempDir()
		mgr, err := aktctx.NewManager(root)
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		if err := os.Mkdir(aktctx.ConfigPath(root), 0o700); err != nil {
			t.Fatalf("block config path: %v", err)
		}

		err = mgr.CreateContext(aktctx.Context{
			Name:       "console",
			AuthMethod: aktctx.AuthMethodConsoleAPI,
		})
		if err == nil {
			t.Fatal("CreateContext unexpectedly persisted through a directory")
		}
		if ghost := mgr.GetContext("console"); ghost != nil {
			t.Fatalf("failed save left an in-memory context: %+v", ghost)
		}
	})
}

func TestRenameContextReportsDataDirectoryCollision(t *testing.T) {
	mgr := newConfiguredManager(t, nil)
	if err := aktctx.EnsureContextDirs(mgr.Root(), "prod"); err != nil {
		t.Fatalf("create source data directory: %v", err)
	}
	newDir := aktctx.ContextDir(mgr.Root(), "renamed")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatalf("mkdir destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "existing"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write destination data: %v", err)
	}

	err := mgr.RenameContext("prod", "renamed")
	if err == nil || !strings.Contains(err.Error(), "rename context directory") {
		t.Fatalf("RenameContext error = %v, want directory collision", err)
	}
	if mgr.GetContext("prod") == nil || mgr.GetContext("renamed") != nil {
		t.Fatal("failed rename mutated context registry")
	}
}

func TestUpdateContextAndNetworkRejectsInvalidChangesWithoutMutation(t *testing.T) {
	sentinel := errors.New("callback rejected change")
	tests := []struct {
		name         string
		mutateConfig func(*aktctx.Config)
		contextName  string
		forkName     string
		applyContext func(*aktctx.Context) error
		applyNetwork func(*aktctx.Network) error
	}{
		{name: "missing context", contextName: "missing"},
		{name: "context callback", contextName: "prod", applyContext: func(ctx *aktctx.Context) error {
			ctx.DefaultAccount = "should-not-stick"
			return sentinel
		}},
		{name: "networkless context", mutateConfig: func(cfg *aktctx.Config) {
			cfg.Contexts[0].Network = aktctx.Network{}
			cfg.Contexts[0].AuthMethod = aktctx.AuthMethodConsoleAPI
		}, contextName: "prod", applyNetwork: func(*aktctx.Network) error { return nil }},
		{name: "missing selected network", contextName: "prod", applyContext: func(ctx *aktctx.Context) error {
			ctx.Network = aktctx.Network{Name: "missing"}
			return nil
		}, applyNetwork: func(*aktctx.Network) error { return nil }},
		{name: "duplicate fork", contextName: "prod", forkName: "mainnet", applyNetwork: func(*aktctx.Network) error { return nil }},
		{name: "network callback", contextName: "prod", applyNetwork: func(network *aktctx.Network) error {
			network.ChainID = "should-not-stick"
			return sentinel
		}},
		{name: "fork without network changes", contextName: "prod", forkName: "fork"},
		{name: "missing context network reference", contextName: "prod", applyContext: func(ctx *aktctx.Context) error {
			ctx.Network = aktctx.Network{Name: "missing"}
			return nil
		}},
		{name: "missing keyring reference", contextName: "prod", applyContext: func(ctx *aktctx.Context) error {
			ctx.Keyring = aktctx.Keyring{Name: "missing"}
			return nil
		}},
		{name: "invalid provider auth", contextName: "prod", applyContext: func(ctx *aktctx.Context) error {
			ctx.ProviderDefaults.AuthType = "password"
			return nil
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newConfiguredManager(t, tc.mutateConfig)
			before := mgr.Config()

			err := mgr.UpdateContextAndNetwork(tc.contextName, tc.forkName, tc.applyContext, tc.applyNetwork)
			if err == nil {
				t.Fatal("invalid update unexpectedly succeeded")
			}
			after := mgr.Config()
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected update mutated config\nbefore: %+v\nafter:  %+v", before, after)
			}
		})
	}
}

func TestUpdateContextAndNetworkUpdatesParentAndRollsBackSaveFailure(t *testing.T) {
	t.Run("update shared network in place", func(t *testing.T) {
		mgr := newConfiguredManager(t, nil)
		err := mgr.UpdateContextAndNetwork("prod", "", nil, func(network *aktctx.Network) error {
			network.Endpoints.RPC[0] = "https://new-rpc.example"
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateContextAndNetwork: %v", err)
		}
		if got := mgr.GetNetwork("mainnet").Endpoints.RPC[0]; got != "https://new-rpc.example" {
			t.Fatalf("RPC = %q, want updated endpoint", got)
		}
	})

	t.Run("save failure", func(t *testing.T) {
		mgr := newConfiguredManager(t, nil)
		before := cloneConfigForComparison(mgr.Config())
		path := aktctx.ConfigPath(mgr.Root())
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove config: %v", err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("replace config with directory: %v", err)
		}

		err := mgr.UpdateContextAndNetwork(
			"prod",
			"mainnet-fork",
			func(ctx *aktctx.Context) error {
				ctx.DefaultAccount = "alice"
				return nil
			},
			func(network *aktctx.Network) error {
				network.ChainID = "fork-2"
				return nil
			},
		)
		if err == nil {
			t.Fatal("save to a directory unexpectedly succeeded")
		}
		if after := mgr.Config(); !reflect.DeepEqual(after, before) {
			t.Fatalf("save failure mutated in-memory config\nbefore: %+v\nafter:  %+v", before, after)
		}
	})
}

func cloneConfigForComparison(cfg aktctx.Config) aktctx.Config {
	cfg.Networks = append([]aktctx.Network(nil), cfg.Networks...)
	for i := range cfg.Networks {
		cfg.Networks[i].Endpoints.RPC = append([]string(nil), cfg.Networks[i].Endpoints.RPC...)
		cfg.Networks[i].Endpoints.API = append([]string(nil), cfg.Networks[i].Endpoints.API...)
		cfg.Networks[i].Endpoints.GRPC = append([]string(nil), cfg.Networks[i].Endpoints.GRPC...)
	}
	cfg.Keyrings = append([]aktctx.Keyring(nil), cfg.Keyrings...)
	cfg.Contexts = append([]aktctx.Context(nil), cfg.Contexts...)
	for i := range cfg.Contexts {
		cfg.Contexts[i].TrackedAccounts = append([]string(nil), cfg.Contexts[i].TrackedAccounts...)
	}
	return cfg
}

func TestUpdateKeyringAndKeyringUsers(t *testing.T) {
	mgr := newConfiguredManager(t, func(cfg *aktctx.Config) {
		cfg.Contexts = append(cfg.Contexts,
			aktctx.Context{Name: "implicit", Network: aktctx.Network{Name: "mainnet"}},
			aktctx.Context{Name: "other", Network: aktctx.Network{Name: "mainnet"}, Keyring: aktctx.Keyring{Name: "hardware"}},
		)
		cfg.Keyrings = append(cfg.Keyrings, aktctx.Keyring{Name: "hardware", Backend: "test"})
	})

	users := mgr.KeyringUsers("default")
	if !reflect.DeepEqual(users, []string{"prod", "implicit"}) {
		t.Fatalf("default keyring users = %v, want [prod implicit]", users)
	}
	if users := mgr.KeyringUsers("hardware"); !reflect.DeepEqual(users, []string{"other"}) {
		t.Fatalf("hardware keyring users = %v, want [other]", users)
	}

	if err := mgr.UpdateKeyring("missing", func(*aktctx.Keyring) error { return nil }); err == nil {
		t.Fatal("missing keyring update succeeded")
	}
	sentinel := errors.New("reject backend")
	if err := mgr.UpdateKeyring("hardware", func(*aktctx.Keyring) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("callback error = %v, want sentinel", err)
	}
	if err := mgr.UpdateKeyring("hardware", func(kr *aktctx.Keyring) error {
		kr.Backend = "file"
		kr.Dir = "/keys"
		return nil
	}); err != nil {
		t.Fatalf("UpdateKeyring: %v", err)
	}
	if got := mgr.GetKeyring("hardware"); got.Backend != "file" || got.Dir != "/keys" {
		t.Fatalf("updated keyring = %+v", got)
	}
}

func TestResolveRejectsBrokenReferencesAndInvalidAuth(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aktctx.Config)
		want   string
	}{
		{name: "missing context", want: `context "missing" not found`},
		{name: "missing network", mutate: func(cfg *aktctx.Config) {
			cfg.Contexts[0].Network.Name = "missing"
		}, want: `network "missing"`},
		{name: "missing keyring", mutate: func(cfg *aktctx.Config) {
			cfg.Contexts[0].Keyring.Name = "missing"
		}, want: `keyring "missing"`},
		{name: "invalid provider auth", mutate: func(cfg *aktctx.Config) {
			cfg.Contexts[0].ProviderDefaults.AuthType = "password"
		}, want: "invalid provider auth type"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newConfiguredManager(t, tc.mutate)
			name := "prod"
			if tc.name == "missing context" {
				name = "missing"
			}
			_, err := mgr.Resolve(name)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Resolve error = %v, want %q", err, tc.want)
			}
		})
	}
}
