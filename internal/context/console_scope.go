package context

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// Environment variables that select the Console organization and project for
// one invocation, overriding the context's stored selection (SPEC §7.1.1).
const (
	EnvConsoleOrganization = "AKT_CONSOLE_ORGANIZATION"
	EnvConsoleProject      = "AKT_CONSOLE_PROJECT"
)

// maxConsoleOrganizationLength bounds a stored organization selector. Console
// slugs are derived from names of at most 64 characters, and ids are UUIDs.
const maxConsoleOrganizationLength = 128

// ValidateConsoleOrganization checks a stored organization selector, which
// is the organization's id or slug as the Console API accepts them in
// x-organization-id. Empty clears the selection.
func ValidateConsoleOrganization(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxConsoleOrganizationLength {
		return fmt.Errorf("console organization must be an id or slug of at most %d characters", maxConsoleOrganizationLength)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("console organization %q must be an id or slug without spaces", value)
		}
	}

	return nil
}

// ValidateConsoleProject checks a stored project selector. A context stores
// the project's id, which survives renames; names are resolved to ids by
// `akt console project use`. Empty clears the selection.
func ValidateConsoleProject(value string) error {
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil || len(value) != 36 {
		return fmt.Errorf("console project %q must be a project id (UUID); run `akt console project use <name>` to select a project by name", value)
	}

	return nil
}

func validateConsoleScope(ctx Context) error {
	if err := ValidateConsoleOrganization(ctx.ConsoleOrganization); err != nil {
		return err
	}

	return ValidateConsoleProject(ctx.ConsoleProject)
}

// resolveConsoleSelector applies an environment override to a stored
// selector. A set but blank variable leaves the stored value in place, the
// same way an unset one does.
func resolveConsoleSelector(env, stored string) string {
	if value := strings.TrimSpace(os.Getenv(env)); value != "" {
		return value
	}

	return stored
}
