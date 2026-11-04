package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"pkg.akt.dev/akt/internal/deploymentconfig"
)

// CreateDeploymentOptions selects the saved-definition API and secret sealing.
type CreateDeploymentOptions struct {
	Secrets            SecretValues
	InheritSecretsFrom string
}

// UpdateDeploymentOptions preserves stored secrets while applying SDL changes.
type UpdateDeploymentOptions struct {
	Secrets SecretValues
}

func rejectUnsealedReferences(rawSDL string) error {
	references, err := deploymentconfig.HasReferences(rawSDL)
	if err != nil {
		return errors.New("prepare deployment SDL: invalid deployment SDL")
	}
	if references {
		return errors.New("console: ac-secret references require explicit deployment secret options")
	}
	return nil
}

// sensitiveClient confines response sanitization to this operation. It never
// changes a shared client's behavior while another request is running.
func (c *Client) sensitiveClient() *Client {
	client := *c
	client.sensitiveErrors = true
	return &client
}

func sanitizeSecretError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrUnauthorized, ErrInsufficientFunds, ErrNotFound, ErrAlreadyClosed} {
		if errors.Is(err, known) {
			return known
		}
	}
	var response *HTTPError
	if errors.As(err, &response) {
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal([]byte(response.Body), &body)
		if body.Code == "" {
			body.Code = response.Body
		}
		switch body.Code {
		case "deployment_definition_changed", "deployment_resources_changed", "stored_secrets_unreadable", "stored_sdl_unreadable", "inherited_secrets_unreadable", "deployment_definition_exists", "deployment_definition_mismatch", "manifest_not_delivered", "provider_unreachable":
			return &HTTPError{StatusCode: response.StatusCode, Body: body.Code}
		default:
			return &HTTPError{StatusCode: response.StatusCode, Body: "sensitive response details omitted"}
		}
	}
	return errors.New("console: request failed; sensitive response details omitted")
}

func (c *Client) createDeploymentWithSecrets(ctx context.Context, rawSDL string, options CreateDeploymentOptions) (*CreateDeploymentResult, error) {
	client := c.sensitiveClient()
	if options.InheritSecretsFrom != "" {
		if err := validateDSeq(options.InheritSecretsFrom); err != nil {
			client.record("create-deployment", "", err)
			return nil, err
		}
	}
	// Parse before fetching encryption material, but never return an SDL parser
	// diagnostic that can include credential or variable values.
	if _, _, err := deploymentArtifacts(rawSDL); err != nil {
		err = errors.New("console: invalid deployment SDL; validate its structure before creating")
		client.record("create-deployment", "", err)
		return nil, err
	}
	seal, err := client.sealSecrets(ctx, options.Secrets, rawSDL)
	if err != nil {
		client.record("create-deployment", "", err)
		return nil, err
	}
	body := map[string]any{"sdl": rawSDL, "sealedSecrets": seal}
	if options.InheritSecretsFrom != "" {
		body["inheritSecretsFrom"] = options.InheritSecretsFrom
	}
	var result CreateDeploymentResult
	err = client.doData(ctx, http.MethodPost, "/v1/deployments", envelope(body), &result)
	if err == nil {
		err = sanitizeSecretError(validateCreateDeploymentResult(&result))
	}
	if err == nil {
		// Console returns an unresolved manifest. Do not generate a replacement
		// locally: lease creation can resolve the saved definition itself.
		result.SignTx.RawLog = ""
		client.record("create-deployment", result.DSeq.String(), nil)
		return &result, nil
	}
	if definitiveCreateFailure(err) {
		client.record("create-deployment", "", err)
		return nil, err
	}
	unknown := fmt.Errorf("deployment creation outcome unknown after one submission (%w); inspect `akt console deployment list` and `akt console deployment get <dseq>` before retrying; the request was not replayed", err)
	client.recordOutcome("create-deployment", "", "pending", unknown, nil)
	return nil, unknown
}

// GetDeploymentDefinition returns Console's saved SDL and concurrency token.
func (c *Client) GetDeploymentDefinition(ctx context.Context, dseq string) (*DeploymentDefinition, error) {
	detail, err := c.GetDeployment(ctx, dseq)
	if err != nil {
		return nil, err
	}
	if detail.ConsoleSettings == nil || detail.ConsoleSettings.SDL == "" || detail.ConsoleSettings.ManifestVersion == "" {
		return nil, fmt.Errorf("console: deployment %s has no saved definition; supply its original SDL file", dseq)
	}
	return detail.ConsoleSettings, nil
}

func (c *Client) updateDeploymentDefinition(ctx context.Context, dseq, rawSDL string, options UpdateDeploymentOptions) (*DeploymentDetail, error) {
	client := c.sensitiveClient()
	if err := validateDSeq(dseq); err != nil {
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	expectedHash, _, err := deploymentArtifacts(rawSDL)
	if err != nil {
		err = errors.New("prepare deployment SDL: invalid deployment SDL")
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	detail, err := client.requireMutableDeployment(ctx, dseq)
	if err != nil {
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	if detail.ConsoleSettings == nil {
		references, parseErr := deploymentconfig.HasReferences(rawSDL)
		if parseErr != nil || references || options.Secrets.Supplied() {
			err := errors.New("console: deployment has no saved definition; legacy updates accept only ordinary SDL without Console references or secret input")
			client.record("update-deployment", dseq, err)
			return nil, err
		}
		return client.updateDeploymentSDL(ctx, dseq, rawSDL, expectedHash)
	}
	patch, err := deploymentconfig.Diff(detail.ConsoleSettings.SDL, rawSDL)
	if err != nil {
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	return client.patchDeploymentDefinition(ctx, dseq, patch, options.Secrets, detail)
}

// PatchDeployment changes only the named fields and seals only supplied secret
// replacements. Omitted secret names retain their values on the server.
func (c *Client) PatchDeployment(ctx context.Context, dseq string, patch deploymentconfig.Patch, secrets SecretValues) (*DeploymentDetail, error) {
	client := c.sensitiveClient()
	if err := deploymentconfig.Validate(patch); err != nil {
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	if !patch.HasChanges() && !secrets.Supplied() {
		err := errors.New("console: an empty patch requires an explicit secrets file for rotation")
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	detail, err := client.requireMutableDeployment(ctx, dseq)
	if err != nil {
		client.record("update-deployment", dseq, err)
		return nil, err
	}
	return client.patchDeploymentDefinition(ctx, dseq, patch, secrets, detail)
}

func (c *Client) patchDeploymentDefinition(ctx context.Context, dseq string, patch deploymentconfig.Patch, secrets SecretValues, current *DeploymentDetail) (*DeploymentDetail, error) {
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		patch.Name = &name
	}
	serviceChanges := patch
	serviceChanges.Name = nil
	renameOnly := !serviceChanges.HasChanges() && patch.Name != nil && !secrets.Supplied()
	if renameOnly {
		if patch.IfManifestVersion != "" {
			err := errors.New("console: rename-only patches cannot enforce ifManifestVersion; remove the version guard or include a configuration change")
			c.record("update-deployment", dseq, err)
			return nil, err
		}
		patch.Services = nil
	}
	if !renameOnly {
		if current.ConsoleSettings == nil || current.ConsoleSettings.SDL == "" || current.ConsoleSettings.ManifestVersion == "" {
			err := fmt.Errorf("console: deployment %s has no saved definition; supply an ordinary SDL update first", dseq)
			c.record("update-deployment", dseq, err)
			return nil, err
		}
		if patch.IfManifestVersion == "" {
			patch.IfManifestVersion = current.ConsoleSettings.ManifestVersion
		}
		if _, err := deploymentconfig.Apply(current.ConsoleSettings.SDL, patch); err != nil {
			c.record("update-deployment", dseq, err)
			return nil, err
		}
	}
	request := struct {
		deploymentconfig.Patch
		SealedSecrets string `json:"sealedSecrets,omitempty"`
	}{Patch: patch}
	if !renameOnly {
		seal, err := c.sealSecrets(ctx, secrets, "")
		if err != nil {
			c.record("update-deployment", dseq, err)
			return nil, err
		}
		request.SealedSecrets = seal
	}
	var result DeploymentDetail
	err := c.doData(ctx, http.MethodPatch, "/v1/deployments/"+url.PathEscape(dseq), envelope(request), &result)
	if err == nil {
		switch {
		case result.Deployment.ID.DSeq.String() != dseq:
			err = errors.New("console: deployment patch response did not identify the requested deployment")
		case renameOnly && (result.Name == nil || *result.Name != *patch.Name):
			err = errors.New("console: deployment patch response did not acknowledge the requested name")
		case !renameOnly && (result.ManifestVersion == "" || result.Deployment.Hash != result.ManifestVersion):
			err = errors.New("console: deployment patch response did not confirm the resulting chain version")
		}
	}
	if err == nil {
		c.record("update-deployment", dseq, nil)
		return &result, nil
	}
	if definitiveCreateFailure(err) {
		c.record("update-deployment", dseq, err)
		return nil, err
	}
	// The stored SDL is committed before chain/provider work. Neither its
	// version nor a GET response can establish successful manifest delivery.
	unknown := fmt.Errorf("deployment update outcome unknown (%w); inspect `akt console deployment get %s`, then retry the identical update to complete delivery; the patch was not replayed", err, dseq)
	c.recordOutcome("update-deployment", dseq, "pending", unknown, nil)
	return nil, unknown
}
