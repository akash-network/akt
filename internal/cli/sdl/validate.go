package sdl

import (
	"fmt"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	gosdl "pkg.akt.dev/go/sdl"
)

// Issue is a single validation or lint finding.
type Issue struct {
	Path    string
	Message string
	Hint    string
}

// Result is the outcome of validating one SDL document.
type Result struct {
	Valid    bool
	Services int
	Groups   int
	Errors   []Issue
	Warnings []Issue
}

// Validate parses an SDL document with pkg.akt.dev/go/sdl — the same parser
// and schema/relational validation used by `akt deploy` and the chain tx
// commands — then applies the local lint rules ported from console-axi
// (see lint.go). Parse failures and unpinned images are errors; on-chain
// pricing denoms are warnings. It never panics and collects every issue it
// can find.
func Validate(data []byte) Result {
	doc, err := gosdl.Read(data)
	if err != nil {
		return Result{Errors: []Issue{{
			Path:    "(root)",
			Message: err.Error(),
			Hint:    "Check YAML syntax and SDL structure.",
		}}}
	}

	m, err := doc.Manifest()
	if err != nil {
		return Result{Errors: []Issue{{Path: "(root)", Message: fmt.Sprintf("manifest: %v", err)}}}
	}

	groups, err := doc.DeploymentGroups()
	if err != nil {
		return Result{Errors: []Issue{{Path: "(root)", Message: fmt.Sprintf("deployment groups: %v", err)}}}
	}

	res := Result{
		Services: countServices(m),
		Groups:   len(groups),
	}

	res.Errors = append(res.Errors, lintImages(m)...)

	pricingErrs, pricingWarns := lintPricing(groups)
	res.Errors = append(res.Errors, pricingErrs...)
	res.Warnings = append(res.Warnings, pricingWarns...)

	res.Valid = len(res.Errors) == 0

	return res
}

// countServices counts distinct service names across all manifest groups
// (a service placed in several groups is still one service).
func countServices(m manifest.Manifest) int {
	seen := make(map[string]struct{})
	for _, g := range m {
		for _, s := range g.Services {
			seen[s.Name] = struct{}{}
		}
	}

	return len(seen)
}
