package pretty

import (
	"io"
	"sync"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Query formatters
// ---------------------------------------------------------------------------

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
// Query formatters are registered in init() functions within per-module formatter files.
// Tx formatters are registered via RegisterAllTxFormatters() called during root command setup.
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

// ---------------------------------------------------------------------------
// Transaction formatters (SPEC §10.11)
// ---------------------------------------------------------------------------

// TxPrettyFormatter formats a transaction message for pretty output.
// It renders the message-specific detail section (Section 2 in the two-section
// layout). The common transaction summary (Section 1) is rendered by the caller.
type TxPrettyFormatter interface {
	// FormatTx renders the message-specific detail section.
	FormatTx(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, msgIndex int) error

	// Title returns the human-readable section header for this message type.
	// Examples: "Send", "Deployment Created", "Delegate", "Vote"
	Title() string
}

// TxPrettyFormatterFunc is a convenience adapter for simple tx formatters.
type TxPrettyFormatterFunc struct {
	TitleStr string
	FormatFn func(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, msgIndex int) error
}

func (f TxPrettyFormatterFunc) FormatTx(w io.Writer, cmd *cobra.Command, cctx sdkclient.Context, msg sdk.Msg, resp *sdk.TxResponse, msgIndex int) error {
	return f.FormatFn(w, cmd, cctx, msg, resp, msgIndex)
}

func (f TxPrettyFormatterFunc) Title() string {
	return f.TitleStr
}

var (
	txMu       sync.RWMutex
	txRegistry = make(map[string]TxPrettyFormatter)
)

// RegisterTx registers a TxPrettyFormatter for the given sdk.Msg type.
func RegisterTx(msg sdk.Msg, formatter TxPrettyFormatter) {
	txMu.Lock()
	defer txMu.Unlock()

	key := proto.MessageName(msg)
	txRegistry[key] = formatter
}

// LookupTx returns the TxPrettyFormatter registered for the given sdk.Msg type, if any.
func LookupTx(msg sdk.Msg) (TxPrettyFormatter, bool) {
	txMu.RLock()
	defer txMu.RUnlock()

	key := proto.MessageName(msg)
	f, ok := txRegistry[key]
	return f, ok
}
