package pretty

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	mtypes "pkg.akt.dev/go/node/market/v1"
	leasev1 "pkg.akt.dev/go/provider/lease/v1"
)

func TestRenderLeaseAttestationWithoutPlatform(t *testing.T) {
	lid := mtypes.LeaseID{
		Owner:    "akash1ownerfulladdress",
		DSeq:     42,
		GSeq:     2,
		OSeq:     3,
		Provider: "akash1providerfulladdress",
	}

	got := RenderLeaseAttestation(
		lid,
		"nonce-value",
		&leasev1.AttestationQuoteResponse{},
		144,
		false,
		false,
	)
	plain := ansi.Strip(got)

	for _, want := range []string{
		"akash1ownerfulladdress/42/2/3/akash1providerfulladdress",
		"TEE Platform:        -",
		"Nonce Verified:      No",
		"Mock Report:         No",
		"Report Size:         144 bytes",
		"GPU Reports:         0",
		"TLS Bound:           No",
		"Nonce:               nonce-value",
		"-o json or -o yaml",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("attestation output missing %q:\n%s", want, got)
		}
	}
}
