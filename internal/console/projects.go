package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// ErrTeamsUnavailable reports that the Console API answers its organization
// and project routes as unknown for this account: teams are not enabled for
// it yet, so the key acts in its personal account as before.
const ErrTeamsUnavailable sentinelError = "console: teams and projects are not enabled for this Console account"

// sentinelError is an error value that can be declared as a constant and
// matched with errors.Is.
type sentinelError string

func (e sentinelError) Error() string {
	return string(e)
}

// Limits the Console API applies to project fields (POST/PATCH /v1/projects).
const (
	MaxProjectNameLength        = 64
	MaxProjectDescriptionLength = 140
)

// ListOrganizations lists the organizations the caller acts in. An API key
// acts in exactly one, which is returned with IsActive set.
//
// Wire: GET /v1/organizations, response {"data":[...]}.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var out []Organization
	if err := c.doData(ctx, http.MethodGet, "/v1/organizations", nil, &out); err != nil {
		return nil, teamsListError(err)
	}

	return out, nil
}

// ActiveOrganization returns the organization this client's requests run in.
func (c *Client) ActiveOrganization(ctx context.Context) (*Organization, error) {
	organizations, err := c.ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range organizations {
		if organizations[i].IsActive {
			return &organizations[i], nil
		}
	}

	return nil, errors.New("console: the organization list marks no organization as the active one")
}

// ListProjects lists the projects of the active organization the caller can
// reach, most recently active first.
//
// Wire: GET /v1/projects, response {"data":[...]}.
func (c *Client) ListProjects(ctx context.Context) ([]ListedProject, error) {
	var out []ListedProject
	if err := c.doData(ctx, http.MethodGet, "/v1/projects", nil, &out); err != nil {
		return nil, teamsListError(err)
	}

	return out, nil
}

// FindProject resolves a project id, slug, or name among the projects the
// caller can reach.
func (c *Client) FindProject(ctx context.Context, selector string) (*ListedProject, error) {
	projects, err := c.ForProject("").ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	return MatchProject(projects, selector)
}

// CreateProject creates a project in the active organization. An empty
// description is omitted.
//
// Wire: POST /v1/projects, body {"data":{"name":..., "description"?:...}},
// data-enveloped response.
func (c *Client) CreateProject(ctx context.Context, name, description string) (*Project, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if err := validateProjectName(name); err != nil {
		c.record("create-project", "", err)
		return nil, err
	}
	if err := validateProjectDescription(description); err != nil {
		c.record("create-project", "", err)
		return nil, err
	}

	payload := map[string]any{"name": name}
	if description != "" {
		payload["description"] = description
	}

	var out Project
	err := c.doData(ctx, http.MethodPost, "/v1/projects", envelope(payload), &out)
	if err == nil && strings.TrimSpace(out.ID) == "" {
		err = errors.New("console: project response omitted its id")
	}
	if err == nil {
		c.recordOutcome("create-project", "", "success", nil, map[string]string{"projectId": out.ID, "name": out.Name})
		return &out, nil
	}
	if definitiveCreateFailure(err) {
		err = teamsListError(err)
		c.recordOutcome("create-project", "", "failed", err, map[string]string{"name": name})
		return nil, err
	}

	unknown := fmt.Errorf("project creation outcome unknown after one submission (%w); the request was not replayed: `akt console project list` shows whether %q exists", err, name)
	c.recordOutcome("create-project", "", "pending", unknown, map[string]string{"name": name})
	return nil, unknown
}

// RenameProject gives a project a new name; its slug and id are unchanged.
//
// Wire: PATCH /v1/projects/{id}, body {"data":{"name":...}}, data-enveloped
// response.
func (c *Client) RenameProject(ctx context.Context, projectID, name string) (*Project, error) {
	name = strings.TrimSpace(name)
	params := map[string]string{"projectId": projectID, "name": name}
	id, err := projectPathID(projectID)
	if err == nil {
		err = validateProjectName(name)
	}
	if err != nil {
		c.recordOutcome("rename-project", "", "failed", err, params)
		return nil, err
	}

	var out Project
	err = c.doData(ctx, http.MethodPatch, "/v1/projects/"+url.PathEscape(id), envelope(map[string]any{"name": name}), &out)
	if err == nil && out.Name != name {
		err = fmt.Errorf("console: project rename response named %q instead of %q", out.Name, name)
	}
	c.recordOutcome("rename-project", "", outcomeStatus(err), err, params)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// DeleteProject deletes a project that holds no open deployment. Console
// refuses the organization's default project.
//
// Wire: DELETE /v1/projects/{id} → 204.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	params := map[string]string{"projectId": projectID}
	id, err := projectPathID(projectID)
	if err == nil {
		err = c.doJSON(ctx, http.MethodDelete, "/v1/projects/"+url.PathEscape(id), nil, nil)
	}
	c.recordOutcome("delete-project", "", outcomeStatus(err), err, params)

	return err
}

// MoveDeployment files a deployment in another project of the active
// organization. The request deliberately carries no project header: a
// request narrowed to the deployment's current project could not reach the
// target.
//
// Wire: PATCH /v1/deployments/{dseq}/project, body {"data":{"projectId":...}},
// response {"data":{"dseq":...,"projectId":...}}.
func (c *Client) MoveDeployment(ctx context.Context, dseq, projectID string) (*DeploymentProject, error) {
	params := map[string]string{"projectId": projectID}
	id, err := projectPathID(projectID)
	if err == nil {
		err = validateDSeq(dseq)
	}
	if err != nil {
		c.recordOutcome("move-deployment", dseq, "failed", err, params)
		return nil, err
	}

	var out DeploymentProject
	err = c.ForProject("").doData(
		ctx,
		http.MethodPatch,
		"/v1/deployments/"+url.PathEscape(dseq)+"/project",
		envelope(map[string]any{"projectId": id}),
		&out,
	)
	if err == nil && (out.DSeq.String() != dseq || !strings.EqualFold(out.ProjectID, id)) {
		err = fmt.Errorf("console: move response reported deployment %s in project %s, not %s in %s", out.DSeq, out.ProjectID, dseq, id)
	}
	c.recordOutcome("move-deployment", dseq, outcomeStatus(err), err, params)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func projectPathID(projectID string) (string, error) {
	id, ok := canonicalUUID(strings.TrimSpace(projectID))
	if !ok {
		return "", fmt.Errorf("console: project id must be a UUID, got %q", projectID)
	}

	return id, nil
}

func validateProjectName(name string) error {
	if name == "" {
		return errors.New("console: project name must not be blank")
	}
	if utf8.RuneCountInString(name) > MaxProjectNameLength {
		return fmt.Errorf("console: project name must be at most %d characters", MaxProjectNameLength)
	}

	return nil
}

func validateProjectDescription(description string) error {
	if utf8.RuneCountInString(description) > MaxProjectDescriptionLength {
		return fmt.Errorf("console: project description must be at most %d characters", MaxProjectDescriptionLength)
	}

	return nil
}

// teamsListError explains the 404 a collection route of organizations or
// projects answers while teams are not enabled for the account. Routes that
// name one project or deployment keep their plain not-found error, since
// there the 404 also means the id is out of reach.
func teamsListError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrTeamsUnavailable, err)
	}

	return err
}

func outcomeStatus(err error) string {
	if err != nil {
		return "failed"
	}

	return "success"
}
