package console

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCompatibilityCommandsRejectWrongArgumentCounts(t *testing.T) {
	lease, _, err := leaseCmds(nil).Find([]string{"create"})
	require.NoError(t, err)
	for _, cmd := range []*cobra.Command{logsCmd(nil), lease} {
		for _, args := range [][]string{nil, {"42", "web", "extra"}} {
			require.Error(t, cmd.ValidateArgs(args), cmd.Name())
		}
	}
}
