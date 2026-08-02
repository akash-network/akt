package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestEveryClientBackedTransactionLeafHasPreflight(t *testing.T) {
	t.Parallel()

	root := TxCmd()
	paths := []string{
		"distribution fund-validator-rewards-pool",
		"gov cancel-proposal",
		"vesting create-vesting-account",
		"vesting create-permanent-locked-account",
		"vesting create-periodic-vesting-account",
		"ibc client add-counterparty",
		"ibc-transfer transfer",
		"upgrade software-upgrade",
		"upgrade cancel-software-upgrade",
		"validate-signatures",
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			cmd, _, err := root.Find(strings.Fields(path))
			if err != nil {
				t.Fatalf("find %q: %v", path, err)
			}

			for current := cmd; current != nil; current = current.Parent() {
				if current.PersistentPreRunE != nil {
					return
				}
			}

			t.Fatalf("%q can reach its handler without transaction preflight", path)
		})
	}
}

func TestVendoredTransactionGroupHelpDoesNotInitializeClient(t *testing.T) {
	root := TxCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"ibc", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("tx ibc --help: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("help output missing usage:\n%s", stdout.String())
	}
}

func TestVendoredTransactionUnknownGroupDoesNotInitializeClient(t *testing.T) {
	root := TxCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"ibc", "channelv2", "--generate-only"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "channelv2" for "ibc"`) {
		t.Fatalf("tx ibc channelv2 error = %v; want unknown command before preflight", err)
	}
	if strings.Contains(err.Error(), "sign mode") {
		t.Fatalf("empty group reached transaction preflight: %v", err)
	}
}
