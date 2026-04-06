package pretty

import (
	"io"
	"sync"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

// PrettyFormatter formats a protobuf query response for human-readable output.
type PrettyFormatter interface {
	// Format writes pretty output to w. cmd provides access to flags for
	// reading --output format preferences. cctx provides codec access for
	// unpacking Any types.
	Format(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg proto.Message) error
}

// PrettyFormatterFunc is an adapter to allow the use of ordinary functions as
// PrettyFormatter implementations.
type PrettyFormatterFunc func(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg proto.Message) error

func (f PrettyFormatterFunc) Format(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	return f(w, cmd, cctx, msg)
}

var (
	mu       sync.RWMutex
	registry = make(map[string]PrettyFormatter)
)

// Register registers a PrettyFormatter for the given protobuf message type.
// The key is the proto full name (e.g., "akash.deployment.v1beta4.QueryDeploymentsResponse").
// Registration is typically done in init() functions within per-module formatter files.
func Register(msg proto.Message, f PrettyFormatter) {
	mu.Lock()
	defer mu.Unlock()

	key := proto.MessageName(msg)
	registry[key] = f
}

// Lookup returns the PrettyFormatter registered for the given message type, if any.
func Lookup(msg proto.Message) (PrettyFormatter, bool) {
	mu.RLock()
	defer mu.RUnlock()

	key := proto.MessageName(msg)
	f, ok := registry[key]
	return f, ok
}
