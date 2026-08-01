package rpc

import "testing"

func TestProviderTLSConfigVerifiesByDefault(t *testing.T) {
	if providerTLSConfig(false).InsecureSkipVerify {
		t.Fatal("default provider TLS config skips certificate verification")
	}
	if !providerTLSConfig(true).InsecureSkipVerify {
		t.Fatal("explicit insecure provider TLS config still verifies certificates")
	}
}

func TestConvertToGRPCEndpointSupportsIPv6(t *testing.T) {
	got, err := convertToGRPCEndpoint("https://[2001:db8::1]:8443")
	if err != nil {
		t.Fatalf("convertToGRPCEndpoint: %v", err)
	}
	if got != "[2001:db8::1]:8444" {
		t.Fatalf("gRPC endpoint = %q, want bracketed IPv6", got)
	}
}
