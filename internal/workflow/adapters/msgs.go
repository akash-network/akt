package adapters

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	depositv1 "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdl"
)

// buildCreateDeploymentMsg builds a MsgCreateDeployment from an SDL file,
// mirroring `akt tx deployment create`: group specs and version hash come
// from the SDL, the deployment ID is owner + dseq, and the deposit is
// attached as-is.
func buildCreateDeploymentMsg(owner sdk.AccAddress, sdlPath string, dseq uint64, dep depositv1.Deposit) (*dv1beta.MsgCreateDeployment, error) {
	if sdlPath == "" {
		return nil, fmt.Errorf("%s: required param %q missing", msgCreateDeployment, "sdl")
	}

	if dseq == 0 {
		return nil, fmt.Errorf("%s: dseq must be non-zero", msgCreateDeployment)
	}

	sdlManifest, err := sdl.ReadFile(sdlPath)
	if err != nil {
		return nil, fmt.Errorf("read SDL file %q: %w", sdlPath, err)
	}

	groups, err := sdlManifest.DeploymentGroups()
	if err != nil {
		return nil, err
	}

	version, err := sdlManifest.Version()
	if err != nil {
		return nil, err
	}

	msg := &dv1beta.MsgCreateDeployment{
		ID: dv1.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq,
		},
		Hash:    version,
		Groups:  make(dv1beta.GroupSpecs, 0, len(groups)),
		Deposit: dep,
	}

	msg.Groups = append(msg.Groups, groups...)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	return msg, nil
}

// buildUpdateDeploymentMsg builds a MsgUpdateDeployment from the "sdl" and
// "dseq" params, mirroring `akt tx deployment update` (the version hash is
// recomputed from the SDL). It also returns the SDL's group specs so the
// caller can verify they match the existing on-chain deployment.
func buildUpdateDeploymentMsg(owner sdk.AccAddress, params map[string]string) (*dv1beta.MsgUpdateDeployment, dv1beta.GroupSpecs, error) {
	sdlPath := params["sdl"]
	if sdlPath == "" {
		return nil, nil, fmt.Errorf("%s: required param %q missing", msgUpdateDeployment, "sdl")
	}

	dseq, err := requiredUint64Param(params, "dseq")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", msgUpdateDeployment, err)
	}

	sdlManifest, err := sdl.ReadFile(sdlPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read SDL file %q: %w", sdlPath, err)
	}

	hash, err := sdlManifest.Version()
	if err != nil {
		return nil, nil, err
	}

	groups, err := sdlManifest.DeploymentGroups()
	if err != nil {
		return nil, nil, err
	}

	msg := &dv1beta.MsgUpdateDeployment{
		ID: dv1.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq,
		},
		Hash: hash,
	}

	return msg, groups, nil
}

// buildCloseDeploymentMsg builds a MsgCloseDeployment from the "dseq" param.
func buildCloseDeploymentMsg(owner sdk.AccAddress, params map[string]string) (*dv1beta.MsgCloseDeployment, error) {
	dseq, err := requiredUint64Param(params, "dseq")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCloseDeployment, err)
	}

	return &dv1beta.MsgCloseDeployment{
		ID: dv1.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq,
		},
	}, nil
}

// buildCreateLeaseMsg builds a MsgCreateLease from the "dseq", "gseq",
// "oseq", and "provider" params. gseq and oseq default to 1 when absent; the
// bid owner is the client's from-address.
func buildCreateLeaseMsg(owner sdk.AccAddress, params map[string]string) (*mtypes.MsgCreateLease, error) {
	dseq, err := requiredUint64Param(params, "dseq")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateLease, err)
	}

	provider := params["provider"]
	if provider == "" {
		return nil, fmt.Errorf("%s: required param %q missing", msgCreateLease, "provider")
	}

	gseq, err := uint32ParamWithDefault(params, "gseq", 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateLease, err)
	}

	oseq, err := uint32ParamWithDefault(params, "oseq", 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgCreateLease, err)
	}

	msg := &mtypes.MsgCreateLease{
		BidID: mv1.BidID{
			Owner:    owner.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: provider,
		},
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	return msg, nil
}

// depositFromString parses a coin string (e.g. "5000000uakt") into a deposit
// with the default sources used by the CLI deposit flags (grant, balance).
func depositFromString(s string) (depositv1.Deposit, error) {
	coin, err := sdk.ParseCoinNormalized(s)
	if err != nil {
		return depositv1.Deposit{}, fmt.Errorf("parse deposit %q: %w", s, err)
	}

	return depositv1.Deposit{
		Amount:  coin,
		Sources: defaultDepositSources(),
	}, nil
}

// defaultDepositSources mirrors the default of the CLI --deposit-sources
// flag: grant first, then balance.
func defaultDepositSources() depositv1.Sources {
	return depositv1.Sources{depositv1.SourceGrant, depositv1.SourceBalance}
}

// requiredUint64Param parses a required unsigned integer param.
func requiredUint64Param(params map[string]string, key string) (uint64, error) {
	v := params[key]
	if v == "" {
		return 0, fmt.Errorf("required param %q missing", key)
	}

	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse param %q: %w", key, err)
	}

	return n, nil
}

// optionalUint64Param parses an optional unsigned integer param, returning 0
// when absent.
func optionalUint64Param(params map[string]string, key string) (uint64, error) {
	if params[key] == "" {
		return 0, nil
	}

	return requiredUint64Param(params, key)
}

// uint32ParamWithDefault parses an optional 32-bit unsigned integer param.
func uint32ParamWithDefault(params map[string]string, key string, def uint32) (uint32, error) {
	v := params[key]
	if v == "" {
		return def, nil
	}

	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse param %q: %w", key, err)
	}

	return uint32(n), nil
}

// optionalUint32Param parses an optional 32-bit unsigned integer param,
// returning 0 (no filter) when absent.
func optionalUint32Param(params map[string]string, key string) (uint32, error) {
	return uint32ParamWithDefault(params, key, 0)
}
