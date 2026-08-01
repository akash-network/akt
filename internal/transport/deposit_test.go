package transport

import (
	"strings"
	"testing"
)

func TestParseDeposit(t *testing.T) {
	tests := []struct {
		in   string
		want Deposit
	}{
		// Rail-default forms.
		{"", Deposit{Raw: "", Auto: true}},
		{"auto", Deposit{Raw: "auto", Auto: true}},
		// Raw carries the trimmed value: it is passed downstream (e.g. into
		// a workflow's tx params), where surrounding whitespace from a YAML
		// definition would fail the rail's own parsing.
		{"  auto  ", Deposit{Raw: "auto", Auto: true}},

		// USD forms.
		{"5usd", Deposit{Raw: "5usd", IsUSD: true, USD: 5}},
		{"5.50usd", Deposit{Raw: "5.50usd", IsUSD: true, USD: 5.5}},
		{"5USD", Deposit{Raw: "5USD", IsUSD: true, USD: 5}},
		{"5 usd", Deposit{Raw: "5 usd", IsUSD: true, USD: 5}},
		{"$5", Deposit{Raw: "$5", IsUSD: true, USD: 5}},
		{"$5.50", Deposit{Raw: "$5.50", IsUSD: true, USD: 5.5}},
		{"0.5usd", Deposit{Raw: "0.5usd", IsUSD: true, USD: 0.5}},

		// Bare numbers: USD on console, rejected on chain (per RailValue).
		{"5", Deposit{Raw: "5", IsUSD: true, USD: 5, Bare: true}},
		{"5.50", Deposit{Raw: "5.50", IsUSD: true, USD: 5.5, Bare: true}},
		{"0", Deposit{Raw: "0", IsUSD: true, USD: 0, Bare: true}},

		// Coin forms.
		{"5000000uakt", Deposit{Raw: "5000000uakt", Coin: "5000000uakt"}},
		{"5akt", Deposit{Raw: "5akt", Coin: "5akt"}},
		{"5.5akt", Deposit{Raw: "5.5akt", Coin: "5.5akt"}},
	}

	for _, tt := range tests {
		got, err := ParseDeposit(tt.in)
		if err != nil {
			t.Errorf("ParseDeposit(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseDeposit(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseDepositErrors(t *testing.T) {
	for _, in := range []string{
		"abc",     // not a USD, bare, or coin form
		"$",       // missing USD amount
		"$abc",    // non-numeric USD amount
		"$-5",     // negative USD
		"-5usd",   // negative USD
		"usd",     // missing USD amount
		"-5",      // negative bare amount
		"$5usd",   // mixed USD forms
		"AUTO",    // auto is lowercase only (historical chain behavior)
		"-5uakt",  // negative coin
		"5milusd", // usd suffix always wins; "5mil" is not a USD amount
	} {
		if _, err := ParseDeposit(in); err == nil {
			t.Errorf("ParseDeposit(%q): expected error, got nil", in)
		}
	}
}

func TestDepositRailValueChain(t *testing.T) {
	tests := []struct {
		in      string
		want    string // rail-native value when wantErr is empty
		wantErr string // substring of the expected error
	}{
		{"", "", ""},
		{"auto", "auto", ""},
		{"5000000uakt", "5000000uakt", ""},
		{"5akt", "5akt", ""},
		{"5usd", "", "USD deposits require a console-api context; use auto (recommended)"},
		{"$5", "", "USD deposits require a console-api context; use auto (recommended)"},
		{"5", "", "console-api context; use auto (recommended)"},
	}

	for _, tt := range tests {
		dep, err := ParseDeposit(tt.in)
		if err != nil {
			t.Fatalf("ParseDeposit(%q): %v", tt.in, err)
		}

		got, err := dep.RailValue(KindChain)
		if tt.wantErr != "" {
			if err == nil {
				t.Errorf("RailValue(chain) for %q: expected error, got %q", tt.in, got)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("RailValue(chain) for %q: error %q does not contain %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("RailValue(chain) for %q: unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("RailValue(chain) for %q = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDepositRailValueChainDoesNotAdvertiseOneNetworkDenomination(t *testing.T) {
	dep, err := ParseDeposit("5usd")
	if err != nil {
		t.Fatalf("ParseDeposit: %v", err)
	}

	_, err = dep.RailValue(KindChain)
	if err == nil {
		t.Fatal("RailValue(chain): expected cross-rail error")
	}
	if strings.Contains(err.Error(), "uakt") {
		t.Errorf("error %q advertises a network-specific denomination", err)
	}
	if !strings.Contains(err.Error(), "network's deployment deposit denomination") {
		t.Errorf("error %q does not explain the explicit-coin requirement", err)
	}
}

func TestDepositRailValueConsole(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{"", "", ""},         // passes through; the console adapter demands explicit USD
		{"auto", "auto", ""}, // same
		{"5usd", "5", ""},
		{"$5.50", "5.5", ""},
		{"5", "5", ""},
		{"5.50", "5.5", ""},
		{"0.5usd", "0.5", ""},
		{"5000000uakt", "", "console deposits are in USD; use e.g. 5usd"},
		{"5akt", "", "console deposits are in USD; use e.g. 5usd"},
	}

	for _, tt := range tests {
		dep, err := ParseDeposit(tt.in)
		if err != nil {
			t.Fatalf("ParseDeposit(%q): %v", tt.in, err)
		}

		got, err := dep.RailValue(KindConsole)
		if tt.wantErr != "" {
			if err == nil {
				t.Errorf("RailValue(console) for %q: expected error, got %q", tt.in, got)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("RailValue(console) for %q: error %q does not contain %q", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("RailValue(console) for %q: unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("RailValue(console) for %q = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseDepositTrimsRawForDownstreamUse(t *testing.T) {
	// A user workflow YAML can easily carry padded values; Raw is what the
	// rail adapters receive, so it must be usable as-is.
	for _, in := range []string{"  5usd ", "\t5000000uakt\n", " 5 "} {
		dep, err := ParseDeposit(in)
		if err != nil {
			t.Fatalf("ParseDeposit(%q): %v", in, err)
		}
		if dep.Raw != strings.TrimSpace(in) {
			t.Errorf("ParseDeposit(%q).Raw = %q, want the trimmed value %q", in, dep.Raw, strings.TrimSpace(in))
		}
	}
}
