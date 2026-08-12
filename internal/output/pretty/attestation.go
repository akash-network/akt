package pretty

import (
	"fmt"
	"strings"

	mtypes "pkg.akt.dev/go/node/market/v1"
	leasev1 "pkg.akt.dev/go/provider/lease/v1"
)

// RenderLeaseAttestation renders a confidential-compute attestation verdict as a
// styled summary. The raw quote (report, GPU evidence, cert chain) is large
// base64, so the summary carries the verdict and the full evidence stays in the
// json/yaml representations.
func RenderLeaseAttestation(
	lid mtypes.LeaseID,
	nonce string,
	quote *leasev1.AttestationQuoteResponse,
	reportSize int,
	nonceVerified bool,
	mockReport bool,
) string {
	platform := quote.GetTeePlatform()
	if platform == "" {
		platform = "-"
	}

	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Lease Attestation"))
	KV(&buf, "Lease", fmt.Sprintf("%s/%d/%d/%d/%s", lid.Owner, lid.DSeq, lid.GSeq, lid.OSeq, lid.Provider))
	KV(&buf, "TEE Platform", platform)
	KV(&buf, "Nonce Verified", FormatBool(nonceVerified))
	KV(&buf, "Mock Report", FormatBool(mockReport))
	KV(&buf, "Report Size", fmt.Sprintf("%d bytes", reportSize))
	KV(&buf, "GPU Reports", fmt.Sprintf("%d", len(quote.GetGpuReports())))
	KV(&buf, "TLS Bound", FormatBool(quote.GetTlsBound()))
	KV(&buf, "Nonce", nonce)

	Newline(&buf)
	fmt.Fprintln(&buf, Dim("Full report (raw quote, GPU evidence, cert chain) available with -o json or -o yaml"))

	return buf.String()
}
