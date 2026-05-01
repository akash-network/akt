package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	ctypes "pkg.akt.dev/go/node/cert/v1"
)

func TestRenderCertificateList(t *testing.T) {
	tests := map[string]struct {
		res *ctypes.QueryCertificatesResponse
	}{
		"Empty": {
			res: &ctypes.QueryCertificatesResponse{},
		},
		"WithCerts": {
			res: &ctypes.QueryCertificatesResponse{
				Certificates: []ctypes.CertificateResponse{
					{
						Serial: "1234567890",
						Certificate: ctypes.Certificate{
							State: ctypes.CertificateValid,
						},
					},
					{
						Serial: "9876543210",
						Certificate: ctypes.Certificate{
							State: ctypes.CertificateRevoked,
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderCertificateList(tc.res))
		})
	}
}
