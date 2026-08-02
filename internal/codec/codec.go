// Package codec provides the application-wide encoding configuration.
//
// MakeEncodingConfig builds and registers all Akash and Cosmos SDK module
// interfaces into an EncodingConfig. It should be called once at startup
// and the result passed to all consumers.
package codec

import (
	evidencetypes "cosmossdk.io/x/evidence/types"
	feegranttypes "cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	constypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibctypes "github.com/cosmos/ibc-go/v10/modules/core/types"
	ibclightclienttypes "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"

	audittypes "pkg.akt.dev/go/node/audit/v1"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	depltypes "pkg.akt.dev/go/node/deployment/v1beta4"
	escrowtypes "pkg.akt.dev/go/node/escrow/v1"
	markettypes "pkg.akt.dev/go/node/market/v1beta5"
	ov1 "pkg.akt.dev/go/node/oracle/v1"
	ov2 "pkg.akt.dev/go/node/oracle/v2"
	providertypes "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/sdkutil"
)

// MakeEncodingConfig builds the encoding config and registers all module interfaces.
// Call once at startup; pass the result to keyring, client, and any other consumer.
func MakeEncodingConfig() sdkutil.EncodingConfig {
	encCfg := sdkutil.MakeEncodingConfig()

	// akash modules
	audittypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	bmetypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	certtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	depltypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	escrowtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	markettypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	ov1.RegisterInterfaces(encCfg.InterfaceRegistry)
	ov2.RegisterInterfaces(encCfg.InterfaceRegistry)

	providertypes.RegisterInterfaces(encCfg.InterfaceRegistry)

	// cosmos sdk modules
	authtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	authztypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	banktypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	stakingtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	minttypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	distrtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	govv1.RegisterInterfaces(encCfg.InterfaceRegistry)
	// Proposals predating gov v1 are still returned by the chain as
	// v1beta1 content (TextProposal and friends). Without this the registry
	// cannot resolve them, and `query gov proposals -o json` failed outright
	// while the pretty renderer worked -- the only machine-readable path to
	// the proposal list was broken.
	govv1beta1.RegisterInterfaces(encCfg.InterfaceRegistry)
	paramstypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	constypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	crisistypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	slashingtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	ibclightclienttypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	ibctypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	upgradetypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	evidencetypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	transfertypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	vestingtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	feegranttypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	wasmtypes.RegisterInterfaces(encCfg.InterfaceRegistry)

	return encCfg
}
