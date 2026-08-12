package provider

import "bytes"

// VerifyNonceEcho reports whether the attestation report echoes the caller's
// nonce in its report_data, the freshness check that stops a provider from
// replaying an old quote. report is the raw decoded evidence.
//
// The layout is platform-specific and duplicates the offsets provider-services
// derives in its own lease-attestation command. mock is the sidecar's
// MOCK-prefixed synthetic format, carrying the nonce at byte 80. A real quote
// is TDX Quote v4 (report_data after the 48-byte header and 520 into the body)
// or SNP (report_data at 0x50); both are tried because the platform is not
// known here.
func VerifyNonceEcho(report []byte, nonce [64]byte) (verified bool, mock bool) {
	mock = len(report) >= 4 && string(report[0:4]) == "MOCK"

	if mock {
		if len(report) >= 144 {
			verified = bytes.Equal(report[80:144], nonce[:])
		}
		return verified, mock
	}

	const tdxReportDataOffset = 48 + 520
	if len(report) >= tdxReportDataOffset+64 {
		verified = bytes.Equal(report[tdxReportDataOffset:tdxReportDataOffset+64], nonce[:])
	}

	if !verified && len(report) >= 0x90 {
		verified = bytes.Equal(report[0x50:0x50+64], nonce[:])
	}

	return verified, mock
}
