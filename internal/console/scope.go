package console

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// Request scope headers of the Console API (SPEC §7.1.1). The organization
// header takes an organization id or slug; the project header takes a
// project id.
const (
	HeaderOrganizationID = "x-organization-id"
	HeaderProjectID      = "x-project-id"
)

// requestScope selects the organization and project a client's requests act
// in. The project is kept as given (id, slug, or name) and resolved to its
// id once, on the first request that needs it.
type requestScope struct {
	organization string
	project      string

	mu        sync.Mutex
	projectID string
}

// WithScope selects the Console organization (id or slug) and project (id,
// slug, or name) every request of this client acts in. Empty values send no
// header, which leaves the API key's own scope in place: an account without
// teams sees no change. Returns the client for chaining.
func (c *Client) WithScope(organization, project string) *Client {
	organization = strings.TrimSpace(organization)
	project = strings.TrimSpace(project)
	if organization == "" && project == "" {
		c.scope = nil
		return c
	}

	c.scope = &requestScope{organization: organization, project: project}
	return c
}

// ForProject returns a copy of the client acting in another project of the
// same organization; an empty project acts across every project the API key
// reaches. The receiver is unchanged, so a long-lived client (the MCP server's)
// can serve one call in a different project.
func (c *Client) ForProject(project string) *Client {
	copied := *c
	organization := ""
	if c.scope != nil {
		organization = c.scope.organization
	}

	return copied.WithScope(organization, project)
}

// Organization returns the organization selector this client sends, or "".
func (c *Client) Organization() string {
	if c.scope == nil {
		return ""
	}

	return c.scope.organization
}

// Project returns the project selector this client was given, or "".
func (c *Client) Project() string {
	if c.scope == nil {
		return ""
	}

	return c.scope.project
}

// scopeProjectID resolves the selected project to its id, or returns "" when
// no project is selected. A name or slug costs one project listing the first
// time; the id is reused afterwards.
func (c *Client) scopeProjectID(ctx context.Context) (string, error) {
	if c.scope == nil || c.scope.project == "" {
		return "", nil
	}

	scope := c.scope
	scope.mu.Lock()
	defer scope.mu.Unlock()

	if scope.projectID != "" {
		return scope.projectID, nil
	}

	if id, ok := canonicalUUID(scope.project); ok {
		scope.projectID = id
		return id, nil
	}

	id, err := c.ForProject("").lookupProjectID(ctx, scope.project)
	if err != nil {
		return "", err
	}
	scope.projectID = id

	return id, nil
}

func (c *Client) lookupProjectID(ctx context.Context, selector string) (string, error) {
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return "", fmt.Errorf("console: resolve project %q: %w", selector, err)
	}

	project, err := MatchProject(projects, selector)
	if err != nil {
		return "", err
	}

	return project.ID, nil
}

// MatchProject finds the one project a selector names: its id, its slug, or
// its name, compared without regard to case as Console compares names.
func MatchProject(projects []ListedProject, selector string) (*ListedProject, error) {
	selector = strings.TrimSpace(selector)
	if id, ok := canonicalUUID(selector); ok {
		for i := range projects {
			if strings.EqualFold(projects[i].ID, id) {
				return &projects[i], nil
			}
		}

		return nil, fmt.Errorf("console: no project with id %s is reachable with this API key; `akt console project list` shows the projects it reaches", id)
	}

	var matches []*ListedProject
	for i := range projects {
		if strings.EqualFold(projects[i].Slug, selector) || strings.EqualFold(projects[i].Name, selector) {
			matches = append(matches, &projects[i])
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("console: no project named %q is reachable with this API key; `akt console project list` shows the projects it reaches", selector)
	default:
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.ID)
		}
		return nil, fmt.Errorf("console: %q names more than one project (%s); select one by id", selector, strings.Join(ids, ", "))
	}
}

// fileInProject names the project a new deployment is filed in. The header
// alone would already narrow the request to it; the body field makes the
// filing explicit and turns a project the key cannot deploy into into a 404
// instead of a fallback.
func fileInProject(payload map[string]any, projectID string) {
	if projectID != "" {
		payload["projectId"] = projectID
	}
}

func (c *Client) setScopeHeaders(req *http.Request, projectID string) {
	if c.scope == nil {
		return
	}
	if c.scope.organization != "" {
		req.Header.Set(HeaderOrganizationID, c.scope.organization)
	}
	if projectID != "" {
		req.Header.Set(HeaderProjectID, projectID)
	}
}

func canonicalUUID(value string) (string, bool) {
	if len(value) != 36 {
		return "", false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", false
	}

	return parsed.String(), true
}
