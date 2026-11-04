package codec

import (
	"cosmossdk.io/x/tx/signing"
	"cosmossdk.io/x/tx/signing/aminojson"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/std"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/gogoproto/proto"
	"google.golang.org/protobuf/reflect/protodesc"

	offchain "pkg.akt.dev/go/node/types/offchain/sign"
	"pkg.akt.dev/go/sdkutil"
)

// makeEncodingConfig preserves the SDK signing configuration while resolving
// the legacy Kubernetes import present in the full CLI's descriptor graph.
func makeEncodingConfig() sdkutil.EncodingConfig {
	options := sdkutil.NewCodecOptions()
	registry := options.NewInterfaceRegistry()
	amino := codec.NewLegacyAmino()
	std.RegisterLegacyAminoCodec(amino)
	std.RegisterInterfaces(registry)
	amino.RegisterConcrete(&offchain.MsgSignData{}, "sign/"+(&offchain.MsgSignData{}).Type(), nil)
	cdc := codec.NewProtoCodec(registry)

	signingContext := mustCodecValue(signing.NewContext(options.Options))
	files := mustCodecValue(proto.MergedGlobalFileDescriptors())
	// Quantity's generated descriptor names this same gogo file by its old
	// repository path. Normalize the local copy, not the process registries.
	for _, file := range files.File {
		for i, dependency := range file.Dependency {
			if dependency == "github.com/gogo/protobuf/gogoproto/gogo.proto" {
				file.Dependency[i] = "gogoproto/gogo.proto"
			}
		}
	}
	resolved := mustCodecValue(protodesc.NewFiles(files))

	txConfig := mustCodecValue(tx.NewTxConfigWithOptions(cdc, tx.ConfigOptions{
		EnabledSignModes: []signingtypes.SignMode{
			signingtypes.SignMode_SIGN_MODE_DIRECT,
			signingtypes.SignMode_SIGN_MODE_DIRECT_AUX,
		},
		CustomSignModes: []signing.SignModeHandler{
			aminojson.NewSignModeHandler(aminojson.SignModeHandlerOptions{
				FileResolver: resolved,
				TypeResolver: options.Options.TypeResolver,
			}),
		},
		SigningOptions: &options.Options,
		SigningContext: signingContext,
	}))

	return sdkutil.EncodingConfig{
		InterfaceRegistry: registry,
		Codec:             cdc,
		TxConfig:          txConfig,
		SigningOptions:    options.Options,
		Amino:             amino,
	}
}

// mustCodecValue rejects a failed SDK constructor during codec startup.
func mustCodecValue[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
