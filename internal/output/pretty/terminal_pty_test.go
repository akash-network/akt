//go:build !windows

package pretty

import (
	"bytes"
	"os"
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/ansi"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func TestPrettyCommandBoundariesPreserveANSIOnTerminal(t *testing.T) {
	unsetNoColor(t)

	tests := []struct {
		name string
		run  func(*os.File) error
		want string
	}{
		{
			name: "query",
			run: func(terminal *os.File) error {
				cmd := prettyPTYCommand(terminal)
				return PrintQueryResult(cmd, sdkclient.Context{}, &stakingtypes.Pool{
					BondedTokens:    math.NewInt(1_000_000),
					NotBondedTokens: math.NewInt(2_000_000),
				})
			},
			want: "Staking Pool",
		},
		{
			name: "transaction",
			run: func(terminal *os.File) error {
				return PrintTxResult(
					prettyPTYCommand(terminal),
					sdkclient.Context{},
					&sdk.TxResponse{TxHash: "ABC123", Height: 10},
				)
			},
			want: "Transaction",
		},
		{
			name: "simulation",
			run: func(terminal *os.File) error {
				cmd := prettyPTYCommand(terminal)
				cmd.Flags().Float64(flagdefs.FlagGasAdjustment, 1.5, "")
				cmd.Flags().String(flagdefs.FlagFees, "", "")
				cmd.Flags().String(flagdefs.FlagGasPrices, "0.0025uakt", "")
				return PrintTxResult(cmd, sdkclient.Context{}, &txtypes.SimulateResponse{
					GasInfo: &sdk.GasInfo{GasUsed: 42},
				})
			},
			want: "Simulation",
		},
		{
			name: "deployment groups",
			run: func(terminal *os.File) error {
				return PrintGroupsList(
					prettyPTYCommand(terminal),
					sdkclient.Context{},
					dvbeta.Groups{},
				)
			},
			want: "(no groups)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := capturePrettyPTY(t, test.run)
			require.Contains(t, ansi.Strip(output), test.want)
			require.Contains(t, output, "\x1b[", "pretty terminal output must retain ANSI styling")
		})
	}
}

func TestPrettyQueryHonorsNoColorOnTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	output := capturePrettyPTY(t, func(terminal *os.File) error {
		return PrintQueryResult(prettyPTYCommand(terminal), sdkclient.Context{}, &stakingtypes.Pool{
			BondedTokens:    math.NewInt(1_000_000),
			NotBondedTokens: math.NewInt(2_000_000),
		})
	})
	require.Contains(t, ansi.Strip(output), "Staking Pool")
	require.NotContains(t, output, "\x1b[")
}

func prettyPTYCommand(terminal *os.File) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String(flagdefs.FlagOutput, cflags.OutputPretty, "")
	cmd.SetOut(terminal)
	return cmd
}

func capturePrettyPTY(t *testing.T, run func(*os.File) error) string {
	t.Helper()

	master, terminal, err := pty.Open()
	require.NoError(t, err)
	defer master.Close()
	defer terminal.Close()

	const endMarker = "__AKT_PTY_CAPTURE_COMPLETE__"
	runDone := make(chan error, 1)
	go func() {
		runErr := run(terminal)
		if _, markerErr := terminal.WriteString(endMarker); runErr == nil {
			runErr = markerErr
		}
		runDone <- runErr
	}()

	var output bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := master.Read(buf)
		if n > 0 {
			_, _ = output.Write(buf[:n])
			if markerIndex := bytes.Index(output.Bytes(), []byte(endMarker)); markerIndex >= 0 {
				require.NoError(t, <-runDone)
				return output.String()[:markerIndex]
			}
		}
		if readErr != nil {
			t.Fatalf("read pseudo-terminal output: %v", readErr)
		}
	}
}

func unsetNoColor(t *testing.T) {
	t.Helper()

	value, present := os.LookupEnv("NO_COLOR")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", value)
			return
		}
		_ = os.Unsetenv("NO_COLOR")
	})
}
