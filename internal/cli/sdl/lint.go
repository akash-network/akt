package sdl

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/distribution/reference"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
)

// Lint rules ported from console-axi's sdl/lint.ts: best-practice checks
// that the SDL parser does not enforce. Each rule is small and pure so new
// ones are easy to add.

// lintImages enforces the image-pinning rule: every service image must carry
// an explicit tag or a sha256 digest. An untagged image or ":latest" is not
// reproducible and is rejected.
func lintImages(m manifest.Manifest) []Issue {
	var issues []Issue

	seen := make(map[string]struct{})

	for _, g := range m {
		for _, s := range g.Services {
			if _, dup := seen[s.Name]; dup {
				continue
			}
			seen[s.Name] = struct{}{}

			if iss := checkImageTag(s.Name, s.Image); iss != nil {
				issues = append(issues, *iss)
			}
		}
	}

	return issues
}

func checkImageTag(service, image string) *Issue {
	if image == "" {
		return nil // a missing image is the schema validator's problem
	}

	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return invalidImageReferenceIssue(service, image, err)
	}

	if digested, ok := named.(reference.Digested); ok {
		digest := digested.Digest()
		if digest.Algorithm().String() != "sha256" || len(digest.Encoded()) != sha256.Size*2 {
			return invalidImageReferenceIssue(service, image,
				fmt.Errorf("digest must use sha256 with %d hexadecimal characters", sha256.Size*2))
		}

		return nil
	}

	tagged, ok := named.(reference.Tagged)
	if !ok {
		return &Issue{
			Path:    fmt.Sprintf("services/%s/image", service),
			Message: fmt.Sprintf("image %q has no tag; pin an explicit version for reproducible deployments", image),
			Hint:    fmt.Sprintf("use %q instead of an untagged image", image+":<version>"),
		}
	}

	if tagged.Tag() == "latest" {
		return &Issue{
			Path:    fmt.Sprintf("services/%s/image", service),
			Message: fmt.Sprintf("image %q uses \":latest\", which is not reproducible", image),
			Hint:    fmt.Sprintf("pin a specific version, e.g. %q", strings.TrimSuffix(image, ":latest")+":1.2.3"),
		}
	}

	return nil
}

func invalidImageReferenceIssue(service, image string, err error) *Issue {
	return &Issue{
		Path:    fmt.Sprintf("services/%s/image", service),
		Message: fmt.Sprintf("image %q is not a valid container image reference: %v", image, err),
		Hint:    `use a tagged image such as "nginx:1.27" or a valid sha256 digest`,
	}
}

// lintPricing checks placement pricing denoms. "uact" is the deployment pricing
// denom on both rails: akt auto-resolves the deposit to "uact" on the chain rail
// (DetectDeploymentDeposit scans Params.MinDeposits for "uact" only) and on the
// console rail alike, and the chain requires a group's price denom to equal the
// deposit denom — MsgCreateDeployment.ValidateBasic rejects a mismatch with
// "Mismatched denominations (uact != uakt)". A "uakt"-priced SDL therefore fails
// at deploy time unless the user also passes an explicitly matching
// "--deposit <amount>uakt".
//
// This deliberately softens the reference rule, which hard-rejects anything but
// "uact": that explicit-deposit escape hatch is legitimate, so "uakt" warns
// rather than errors. Any other denom is an error, matching the reference.
func lintPricing(groups dtypes.GroupSpecs) (errs, warns []Issue) {
	for _, g := range groups {
		seen := make(map[string]struct{})

		for _, ru := range g.Resources {
			denom := ru.Price.Denom
			if denom == "uact" {
				continue
			}

			if _, dup := seen[denom]; dup {
				continue
			}
			seen[denom] = struct{}{}

			path := fmt.Sprintf("profiles/placement/%s/pricing", g.Name)

			if denom == "uakt" {
				warns = append(warns, Issue{
					Path:    path,
					Message: `pricing denom "uakt" does not match the deposit; deployments are priced in "uact" (micro-ACT, 1:1 USD) on both the on-chain and managed (console-api) rails, and the chain rejects a group whose price denom differs from the deposit denom`,
					Hint:    `switch the pricing denom to "uact", or pass a matching --deposit <amount>uakt when you deploy`,
				})

				continue
			}

			errs = append(errs, Issue{
				Path:    path,
				Message: fmt.Sprintf("pricing denom %q is not accepted; use \"uact\", the deployment pricing denom on both rails", denom),
				Hint:    `change the denom to "uact"`,
			})
		}
	}

	return errs, warns
}
