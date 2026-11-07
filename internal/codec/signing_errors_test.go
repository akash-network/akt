package codec

import (
	"testing"

	"cosmossdk.io/x/tx/signing"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"

	"pkg.akt.dev/go/sdkutil"
)

func TestCodecStartupRejectsInvalidSDKConfiguration(t *testing.T) {
	t.Run("missing address codec", func(t *testing.T) {
		options := sdkutil.NewSigningOptions()
		options.AddressCodec = nil
		require.PanicsWithError(t, "address codec is required", func() {
			mustCodecValue(signing.NewContext(options))
		})
	})

	t.Run("unresolved descriptor import", func(t *testing.T) {
		files := &descriptorpb.FileDescriptorSet{
			File: []*descriptorpb.FileDescriptorProto{
				{
					Name:       proto.String("broken.proto"),
					Syntax:     proto.String("proto3"),
					Dependency: []string{"missing.proto"},
				},
			},
		}
		_, err := protodesc.NewFiles(files)
		require.ErrorContains(t, err, "missing.proto")
		require.PanicsWithError(t, err.Error(), func() {
			mustCodecValue(protodesc.NewFiles(files))
		})
	})

	t.Run("textual signing without metadata", func(t *testing.T) {
		encoding := MakeEncodingConfig()
		require.PanicsWithError(t, "cannot enable SIGN_MODE_TEXTUAL without a TextualCoinMetadataQueryFn", func() {
			mustCodecValue(tx.NewTxConfigWithOptions(encoding.Codec, tx.ConfigOptions{
				EnabledSignModes: []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_TEXTUAL},
				SigningContext:   encoding.TxConfig.SigningContext(),
				SigningOptions:   &encoding.SigningOptions,
			}))
		})
	})
}
