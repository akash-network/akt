package console

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Console API error codes for teams and projects (SPEC §7.6). The API sends
// them in the error body's `code` field.
const (
	CodeOrganizationForbidden = "organization_forbidden"
	CodeOrganizationMismatch  = "organization_mismatch"
	CodeProjectForbidden      = "project_forbidden"
	CodeProjectRequired       = "project_required"
	CodeProjectBudgetExceeded = "project_budget_exceeded"
	CodeSessionRequired       = "session_required"
	CodeProjectNameTaken      = "project_name_taken"
	CodeProjectIsDefault      = "project_is_default"
	CodeProjectNotEmpty       = "project_not_empty"
	CodeProjectLimitReached   = "project_limit_reached"
	// CodeDepositPriceUnavailable comes with a 503: Console could not price
	// a deposit against the project's budget and refused before signing.
	CodeDepositPriceUnavailable = "deposit_price_unavailable"
)

// maxScopeErrorDetail bounds the API's own message quoted in an explanation.
const maxScopeErrorDetail = 300

type scopeErrorExplanation struct {
	summary string
	remedy  string
}

func scopeErrorExplanations() map[string]scopeErrorExplanation {
	return map[string]scopeErrorExplanation{
		CodeProjectRequired: {
			summary: "this request needs a project",
			remedy: "Select one with --console-project <project>, AKT_CONSOLE_PROJECT, or " +
				"`akt console project use <project>`; `akt console project list` shows the projects you can deploy into.",
		},
		CodeProjectForbidden: {
			summary: "the selected project is not reachable with this API key",
			remedy: "Pick one from `akt console project list`, or clear the context's project with " +
				"`akt context edit <context> --console-project \"\"`. A key limited to one project acts only in that project.",
		},
		CodeOrganizationMismatch: {
			summary: "the selected organization is not the one this API key acts in",
			remedy: "`akt console organization list` shows the key's organization. Select it with " +
				"`akt context edit <context> --console-organization <id-or-slug>`, or clear the selection with --console-organization \"\".",
		},
		CodeOrganizationForbidden: {
			summary: "this API key cannot act in the organization",
			remedy:  "Use an API key created in an organization you are a member of.",
		},
		CodeProjectBudgetExceeded: {
			summary: "this would take the project past its monthly budget, so nothing was signed",
			remedy:  "`akt console project list` shows the other projects you can deploy into.",
		},
		CodeSessionRequired: {
			summary: "this action needs a signed-in Console session",
			remedy:  "Do it in the Console web app at " + DefaultUIBaseURL + "; API keys cannot create, delete, or leave a team, or accept an invitation.",
		},
		CodeProjectNameTaken: {
			summary: "another project of the organization has this name",
			remedy:  "Choose another name; `akt console project list` shows the names in use.",
		},
		CodeProjectIsDefault: {
			summary: "the organization's default project cannot be deleted",
		},
		CodeProjectNotEmpty: {
			summary: "the project still holds open deployments",
			remedy:  "Move them with `akt console deployment move <dseq> <project>` or close them first.",
		},
		CodeProjectLimitReached: {
			summary: "the organization has reached the number of projects it can have",
			remedy: "Delete a project you no longer use with `akt console project delete <project>`, or reuse one from " +
				"`akt console project list`; the default project counts toward the limit.",
		},
		CodeDepositPriceUnavailable: {
			summary: "Console could not price the deposit against the project's budget",
			remedy:  "Nothing was signed; run the same command again in a moment.",
		},
	}
}

// scopeErrorCode returns the recognized team or project code of an error
// body, or "".
func scopeErrorCode(body []byte) string {
	var envelope struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	if _, ok := scopeErrorExplanations()[envelope.Code]; !ok {
		return ""
	}

	return envelope.Code
}

// scopeErrorMessage explains a recognized refusal: the API's own message
// when the body carries one, the code and status, and the remedy.
func scopeErrorMessage(e *HTTPError) (string, bool) {
	explanation, ok := scopeErrorExplanations()[e.Code]
	if !ok {
		return "", false
	}

	detail := explanation.summary
	var envelope struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &envelope) == nil {
		if message := boundedErrorDetail(envelope.Message); message != "" {
			detail = message
		}
	}

	var message strings.Builder
	fmt.Fprintf(&message, "console: %s (%s, HTTP %d)", strings.TrimSuffix(detail, "."), e.Code, e.StatusCode)
	if explanation.remedy != "" {
		message.WriteString(". ")
		message.WriteString(explanation.remedy)
	}

	return message.String(), true
}

func boundedErrorDetail(message string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(message))
	if runes := []rune(cleaned); len(runes) > maxScopeErrorDetail {
		cleaned = string(runes[:maxScopeErrorDetail]) + " [truncated]"
	}

	return cleaned
}
