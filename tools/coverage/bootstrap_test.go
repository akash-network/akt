package main

import (
	"strings"
	"testing"
)

func TestSourcePackagesFromTreeFindsOnlyReleaseSource(t *testing.T) {
	t.Parallel()

	packages, err := sourcePackagesFromTree("pkg.akt.dev/akt", []byte(strings.Join([]string{
		"main.go",
		"main_test.go",
		"internal/existing/existing.go",
		"internal/existing/existing_test.go",
		"internal/native/helper.c",
		"tools/new/main_test.go",
		"README.md",
	}, "\x00")))
	if err != nil {
		t.Fatalf("sourcePackagesFromTree() error = %v", err)
	}
	want := map[string]bool{
		"pkg.akt.dev/akt":                   true,
		"pkg.akt.dev/akt/internal/existing": true,
	}
	if len(packages) != len(want) {
		t.Fatalf("sourcePackagesFromTree() = %#v, want %#v", packages, want)
	}
	for name := range want {
		if !packages[name] {
			t.Errorf("sourcePackagesFromTree() omitted %q", name)
		}
	}
}

func TestGuardInitialBaselineGrandfathersOnlyLegacyPackages(t *testing.T) {
	t.Parallel()

	const (
		legacy = "pkg.akt.dev/akt/internal/legacy"
		added  = "pkg.akt.dev/akt/internal/added"
	)
	packages := packageSet{
		Entries: []packageEntry{
			{Name: legacy, Class: classActive, Critical: true},
			{Name: added, Class: classActive},
		},
		ByName: map[string]packageEntry{
			legacy: {Name: legacy, Class: classActive, Critical: true},
			added:  {Name: added, Class: classActive},
		},
	}
	current := map[string]statementCount{
		legacy: {Covered: 1, Total: 100},
		added:  {Covered: 79, Total: 100},
	}

	err := guardInitialBaseline(packages, current, map[string]bool{legacy: true}, classActive)
	if err == nil || !strings.Contains(err.Error(), "initial package "+added+" is 79.00%; minimum is 80%") {
		t.Fatalf("guardInitialBaseline() error = %v, want only the new package floor", err)
	}

	current[added] = statementCount{Covered: 80, Total: 100}
	if err := guardInitialBaseline(packages, current, map[string]bool{legacy: true}, classActive); err != nil {
		t.Fatalf("guardInitialBaseline() rejected reviewed legacy snapshot: %v", err)
	}
}

func TestSourcePackagesFromTreeRejectsMalformedModule(t *testing.T) {
	t.Parallel()

	for _, module := range []string{"", "pkg.akt.dev/akt\nother"} {
		if _, err := sourcePackagesFromTree(module, []byte("main.go\n")); err == nil {
			t.Fatalf("sourcePackagesFromTree(%q) accepted a malformed module", module)
		}
	}
}
