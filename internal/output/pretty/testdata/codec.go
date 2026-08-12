// Package prettytestdata contains fixtures whose deliberately unusual method
// signatures implement external interfaces exercised by pretty-output tests.
package prettytestdata

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/gogoproto/proto"
)

// GroupsMarshalCodec injects a JSON payload or error while retaining the rest
// of a real Cosmos SDK codec's behavior.
type GroupsMarshalCodec struct {
	codec.Codec
	Payload []byte
	Err     error
}

// MarshalJSON implements codec.JSONCodec. Its signature is defined upstream
// and intentionally differs from encoding/json.Marshaler.
func (c GroupsMarshalCodec) MarshalJSON(proto.Message) ([]byte, error) {
	return c.Payload, c.Err
}
