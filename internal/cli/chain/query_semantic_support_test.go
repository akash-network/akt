package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"io"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type semanticQueryLightClient struct {
	query clientv1beta3.QueryClient
	node  clientv1beta3.NodeClient
	cctx  sdkclient.Context
}

func (client *semanticQueryLightClient) Query() clientv1beta3.QueryClient {
	return client.query
}

func (client *semanticQueryLightClient) Node() clientv1beta3.NodeClient {
	return client.node
}

func (client *semanticQueryLightClient) ClientContext() sdkclient.Context {
	return client.cctx
}

func (*semanticQueryLightClient) PrintMessage(interface{}) error { return nil }
func (*semanticQueryLightClient) PrintJSON(interface{}) error    { return nil }

func runSemanticQuery(
	t *testing.T,
	cmd *cobra.Command,
	query clientv1beta3.QueryClient,
	node clientv1beta3.NodeClient,
	output io.Writer,
	args ...string,
) error {
	t.Helper()

	if output == nil {
		output = io.Discard
	}

	encoding := aktcodec.MakeEncodingConfig()
	cctx := sdkclient.Context{}.
		WithCodec(encoding.Codec).
		WithLegacyAmino(encoding.Amino).
		WithInterfaceRegistry(encoding.InterfaceRegistry).
		WithOutput(output)
	client := &semanticQueryLightClient{query: query, node: node, cctx: cctx}

	ctx := context.WithValue(context.Background(), ClientContextKey, &cctx)
	ctx = context.WithValue(ctx, ContextTypeQueryClient, client)
	cmd.SetContext(ctx)
	cmd.SetOut(output)
	cmd.SetErr(output)
	if flag := cmd.Flags().Lookup(flagdefs.FlagOutput); flag != nil {
		if err := cmd.Flags().Set(flagdefs.FlagOutput, "json"); err != nil {
			t.Fatalf("set query output: %v", err)
		}
	}

	return cmd.RunE(cmd, args)
}

func runSemanticQueryBuffer(
	t *testing.T,
	cmd *cobra.Command,
	query clientv1beta3.QueryClient,
	node clientv1beta3.NodeClient,
	args ...string,
) (string, error) {
	t.Helper()

	var output bytes.Buffer
	err := runSemanticQuery(t, cmd, query, node, &output, args...)
	return output.String(), err
}
