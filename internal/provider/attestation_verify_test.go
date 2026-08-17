package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testNonce() [64]byte {
	var nonce [64]byte
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	return nonce
}

func reportWithNonceAt(size, offset int, nonce [64]byte) []byte {
	report := make([]byte, size)
	copy(report[offset:], nonce[:])
	return report
}

func TestVerifyNonceEcho(t *testing.T) {
	nonce := testNonce()

	t.Run("mock format", func(t *testing.T) {
		report := reportWithNonceAt(144, 80, nonce)
		copy(report[0:4], "MOCK")

		verified, mock := VerifyNonceEcho(report, nonce)
		require.True(t, verified)
		require.True(t, mock)
	})

	t.Run("truncated mock format", func(t *testing.T) {
		report := reportWithNonceAt(143, 80, nonce)
		copy(report[0:4], "MOCK")

		verified, mock := VerifyNonceEcho(report, nonce)
		require.False(t, verified)
		require.True(t, mock)
	})

	t.Run("mock format wrong nonce", func(t *testing.T) {
		wrong := nonce
		wrong[0] ^= 0xff
		report := reportWithNonceAt(144, 80, wrong)
		copy(report[0:4], "MOCK")

		verified, mock := VerifyNonceEcho(report, nonce)
		require.False(t, verified)
		require.True(t, mock)
	})

	t.Run("tdx quote v4", func(t *testing.T) {
		report := reportWithNonceAt(48+520+64, 48+520, nonce)

		verified, mock := VerifyNonceEcho(report, nonce)
		require.True(t, verified)
		require.False(t, mock)
	})

	t.Run("snp", func(t *testing.T) {
		report := reportWithNonceAt(0x90, 0x50, nonce)

		verified, mock := VerifyNonceEcho(report, nonce)
		require.True(t, verified)
		require.False(t, mock)
	})

	for _, tc := range []struct {
		name   string
		size   int
		offset int
	}{
		{name: "truncated tdx boundary", size: 48 + 520 + 63, offset: 48 + 520},
		{name: "truncated snp boundary", size: 0x8f, offset: 0x50},
		{name: "tdx nonce at wrong offset", size: 48 + 520 + 64, offset: 48 + 519},
		{name: "snp nonce at wrong offset", size: 0x90, offset: 0x4f},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := reportWithNonceAt(tc.size, tc.offset, nonce)

			verified, mock := VerifyNonceEcho(report, nonce)
			require.False(t, verified)
			require.False(t, mock)
		})
	}

	t.Run("tdx mismatch falls back to snp", func(t *testing.T) {
		wrong := nonce
		wrong[0] ^= 0xff
		report := reportWithNonceAt(48+520+64, 48+520, wrong)
		copy(report[0x50:], nonce[:])

		verified, mock := VerifyNonceEcho(report, nonce)
		require.True(t, verified)
		require.False(t, mock)
	})

	t.Run("mock format takes precedence", func(t *testing.T) {
		report := reportWithNonceAt(48+520+64, 48+520, nonce)
		copy(report[0:4], "MOCK")

		verified, mock := VerifyNonceEcho(report, nonce)
		require.False(t, verified)
		require.True(t, mock)
	})

	t.Run("mismatch", func(t *testing.T) {
		var wrong [64]byte
		for i := range wrong {
			wrong[i] = byte(i + 100)
		}
		report := reportWithNonceAt(48+520+64, 48+520, wrong)

		verified, mock := VerifyNonceEcho(report, nonce)
		require.False(t, verified)
		require.False(t, mock)
	})
}
