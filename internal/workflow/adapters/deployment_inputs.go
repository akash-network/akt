package adapters

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/deploymentconfig"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

// DeploymentInputs belongs to one workflow invocation. Prepared chain SDLs
// stay here so transaction construction and provider delivery use identical
// bytes without putting expanded configuration into workflow state or files.
type DeploymentInputs struct {
	root    string
	context string
	input   io.Reader
	sdls    map[uint64]string
}

// NewDeploymentInputs creates the private input state shared by one run's
// transaction and provider adapters. Input is used only for a secrets-file of -.
func NewDeploymentInputs(root, contextName string, input io.Reader) *DeploymentInputs {
	return &DeploymentInputs{root: root, context: contextName, input: input, sdls: make(map[uint64]string)}
}

func selectDeploymentInputs(inputs []*DeploymentInputs, root, contextName string) *DeploymentInputs {
	if len(inputs) != 0 && inputs[0] != nil {
		return inputs[0]
	}
	return NewDeploymentInputs(root, contextName, nil)
}

func (d *DeploymentInputs) sourcePath(ctx context.Context, owner, dseq, override, flag string) (string, error) {
	sequence, err := strconv.ParseUint(dseq, 10, 64)
	if err != nil || sequence == 0 {
		return "", fmt.Errorf("source deployment sequence must be a positive integer")
	}
	if override != "" {
		return override, nil
	}
	missing := fmt.Errorf("deployment %s has no readable local SDL; supply --%s", dseq, flag)
	if d == nil || d.root == "" || d.context == "" {
		return "", missing
	}
	if _, err := os.Stat(aktctx.StoreDBPath(d.root, d.context)); err != nil {
		if os.IsNotExist(err) {
			return "", missing
		}
		return "", fmt.Errorf("read deployment store: %w", err)
	}
	store, err := bbolt.OpenContext(ctx, d.root, d.context)
	if err != nil {
		return "", err
	}
	defer func() { _ = store.Close() }()
	if owner == "" {
		owner, err = sstore.UniqueDeploymentOwner(ctx, store, sequence)
		if err != nil {
			return "", err
		}
	}
	record, err := store.GetDeployment(ctx, owner, sequence)
	if err != nil {
		return "", err
	}
	if record == nil || record.SDLPath == "" {
		return "", missing
	}
	return record.SDLPath, nil
}

// ValidateChain resolves the same source and patch as execution without reading
// secret values or retaining an expanded SDL. A resolved deposit additionally
// checks the denomination of every deployment group. An omitted owner requires
// an unambiguous source deployment in the context store.
func (d *DeploymentInputs) ValidateChain(ctx context.Context, owner string, params map[string]string) error {
	raw, _, err := d.prepareChain(ctx, owner, params)
	if err != nil {
		return err
	}
	document, err := readSDL([]byte(raw))
	if err != nil {
		return err
	}
	deposit := params["deposit"]
	if deposit == "" || deposit == depositAuto {
		return nil
	}
	coin, err := sdk.ParseDecCoin(deposit)
	if err != nil {
		return fmt.Errorf("parse resolved deployment deposit: %w", err)
	}
	// readSDL returns a fully initialized document with deployment groups.
	groups, _ := document.DeploymentGroups()
	for _, group := range groups {
		if group.Price().Denom != coin.Denom {
			return fmt.Errorf("SDL price denomination %q does not match the effective deposit denomination %q for deployment group %q; set the SDL pricing denomination to %s or supply a matching deposit", group.Price().Denom, coin.Denom, group.Name, coin.Denom)
		}
	}
	return nil
}

func (d *DeploymentInputs) prepareChain(ctx context.Context, owner string, params map[string]string) (string, string, error) {
	if params["secrets-file"] != "" {
		return "", "", fmt.Errorf("--secrets-file requires a Console context; switch with `akt context edit --deploy-via console`")
	}
	path := params["sdl"]
	if source := params["source-dseq"]; source != "" {
		var err error
		path, err = d.sourcePath(ctx, owner, source, path, "sdl-file")
		if err != nil {
			return "", "", err
		}
	}
	raw, err := sdlContent(path)
	if err != nil {
		return "", "", err
	}
	if params["patch"] == "true" {
		patch, err := deploymentconfig.ParsePatch([]byte(raw))
		if err != nil {
			return "", "", err
		}
		if patch.Name != nil || patch.IfManifestVersion != "" {
			return "", "", fmt.Errorf("patch name and ifManifestVersion require a Console context")
		}
		if !patch.HasChanges() {
			return "", "", fmt.Errorf("deployment patch contains no changes")
		}
		path, err = d.sourcePath(ctx, owner, params["dseq"], params["base-sdl"], "base-sdl")
		if err != nil {
			return "", "", err
		}
		base, err := sdlContent(path)
		if err != nil {
			return "", "", err
		}
		raw, err = deploymentconfig.Apply(base, patch)
		if err != nil {
			return "", "", err
		}
	}
	references, err := deploymentconfig.HasReferences(raw)
	if err != nil {
		return "", "", err
	}
	if references {
		return "", "", fmt.Errorf("ac-secret:// references require a Console context; switch with `akt context edit --deploy-via console`")
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		path = ""
	}
	return raw, path, nil
}

func (d *DeploymentInputs) remember(dseq uint64, raw string) {
	if d != nil {
		d.sdls[dseq] = raw
	}
}

func (d *DeploymentInputs) providerSDL(dseq uint64, fallback []byte) []byte {
	if d != nil {
		if raw, ok := d.sdls[dseq]; ok {
			return []byte(raw)
		}
	}
	return fallback
}
