package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestTxSurfaceOmitsUnsupportedAndEmptyGroups(t *testing.T) {
	tx := TxCmd()
	for _, child := range tx.Commands() {
		require.NotEmpty(t, strings.TrimSpace(child.Name()), "transaction tree must not expose separator sentinels as leaves")
	}

	require.Nil(t, directChild(tx, "crisis"), "Akash does not register the crisis message handler")
	require.Nil(t, directChild(tx, "evidence"), "no concrete evidence transaction exists")

	ibc := directChild(tx, "ibc")
	require.NotNil(t, ibc)
	require.Nil(t, directChild(ibc, "channelv2"), "upstream channel-v2 has no transaction actions")
}

func TestWithoutEmptyVendoredGroupPreservesFutureActions(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	empty := &cobra.Command{Use: "future"}
	parent.AddCommand(empty)

	withoutEmptyVendoredGroup(parent, "future")
	require.Nil(t, directChild(parent, "future"))

	actionable := &cobra.Command{Use: "future"}
	actionable.AddCommand(&cobra.Command{Use: "send", RunE: func(*cobra.Command, []string) error { return nil }})
	parent.AddCommand(actionable)

	withoutEmptyVendoredGroup(parent, "future")
	require.Same(t, actionable, directChild(parent, "future"))
}

func directChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}

	return nil
}
