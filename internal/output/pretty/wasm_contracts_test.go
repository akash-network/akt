package pretty

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func TestPrintWasmContractListPreservesValues(t *testing.T) {
	address := "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"
	label := "a contract label with spaces and a distinguishing suffix"
	contracts := []*types.QueryContractInfoResponse{{Address: address, ContractInfo: types.ContractInfo{CodeID: ^uint64(0), Label: label}}}
	for _, format := range []string{"pretty", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd := &cobra.Command{}
			cflags.AddQueryFlagsToCmd(cmd)
			require.NoError(t, cmd.Flags().Set(flagdefs.FlagOutput, format))
			var out bytes.Buffer
			cmd.SetOut(&out)
			require.NoError(t, PrintWasmContractList(cmd, contracts))
			require.Contains(t, out.String(), address)
			require.Contains(t, out.String(), label)
			require.NotContains(t, out.String(), "\x1b")
			if format == "pretty" {
				require.Contains(t, out.String(), "CODE ID")
				require.Contains(t, out.String(), "18446744073709551615")
			} else {
				var rows []map[string]any
				if format == "json" {
					require.NoError(t, json.Unmarshal(out.Bytes(), &rows))
				} else {
					require.NoError(t, yaml.Unmarshal(out.Bytes(), &rows))
				}
				require.Equal(t, "18446744073709551615", rows[0]["code_id"])
			}
		})
	}
}

func TestPrintWasmContractListPropagatesWriteErrors(t *testing.T) {
	failure := errors.New("stdout closed")
	for _, format := range []string{"pretty", "json", "yaml"} {
		for _, tc := range []struct {
			name   string
			writer io.Writer
			want   error
		}{
			{"error", errorWriter{err: failure}, failure},
			{"short write", shortWriter{}, io.ErrShortWrite},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				cmd := &cobra.Command{}
				cflags.AddQueryFlagsToCmd(cmd)
				require.NoError(t, cmd.Flags().Set(flagdefs.FlagOutput, format))
				cmd.SetOut(tc.writer)
				require.ErrorIs(t, PrintWasmContractList(cmd, nil), tc.want)
			})
		}
	}
}
