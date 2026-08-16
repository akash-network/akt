package flags

import (
	"strings"
	"testing"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/pflag"

	mv1 "pkg.akt.dev/go/node/market/v1"
)

func TestBMELedgerFiltersFromFlags(t *testing.T) {
	tests := []struct {
		name       string
		owner      string
		denom      string
		toDenom    string
		status     string
		wantErr    bool
		wantSource string
	}{
		{
			name:       "complete filter",
			owner:      testOwner,
			denom:      "uakt",
			toDenom:    "uact",
			status:     "ledger_record_status_pending",
			wantSource: testOwner,
		},
		{
			name: "empty filter",
		},
		{
			name:    "invalid owner",
			owner:   "not-an-akash-address",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flagSet := pflag.NewFlagSet(test.name, pflag.ContinueOnError)
			AddBMELedgerFilterFlags(flagSet)

			for name, value := range map[string]string{
				flagdefs.FlagOwner:   test.owner,
				flagdefs.FlagDenom:   test.denom,
				flagdefs.FlagToDenom: test.toDenom,
				flagdefs.FlagStatus:  test.status,
			} {
				if err := flagSet.Set(name, value); err != nil {
					t.Fatalf("set --%s: %v", name, err)
				}
			}

			filters, err := BMELedgerFiltersFromFlags(flagSet)
			if test.wantErr {
				if err == nil {
					t.Fatal("BMELedgerFiltersFromFlags() error = nil, want invalid owner error")
				}
				return
			}
			if err != nil {
				t.Fatalf("BMELedgerFiltersFromFlags() error = %v", err)
			}
			if filters.Source != test.wantSource || filters.Denom != test.denom || filters.ToDenom != test.toDenom || filters.Status != test.status {
				t.Fatalf("BMELedgerFiltersFromFlags() = %+v, want source=%q denom=%q to-denom=%q status=%q",
					filters, test.wantSource, test.denom, test.toDenom, test.status)
			}
		})
	}
}

func TestBidClosedReasonFromFlags(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    mv1.LeaseClosedReason
		wantErr bool
	}{
		{name: "default", want: mv1.LeaseClosedReasonUnspecified},
		{name: "provider reason", value: "10000", want: mv1.LeaseClosedReasonUnstable},
		{name: "below provider range", value: "9999", want: mv1.LeaseClosedReasonInvalid, wantErr: true},
		{name: "above provider range", value: "20000", want: mv1.LeaseClosedReasonInvalid, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flagSet := pflag.NewFlagSet(test.name, pflag.ContinueOnError)
			AddBidClosedReasonFlag(flagSet)
			if test.value != "" {
				if err := flagSet.Set(flagdefs.FlagClosedReason, test.value); err != nil {
					t.Fatalf("set --%s: %v", flagdefs.FlagClosedReason, err)
				}
			}

			got, err := BidClosedReasonFromFlags(flagSet)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "expected range 10000..19999") {
					t.Fatalf("BidClosedReasonFromFlags() error = %v, want provider range error", err)
				}
			} else if err != nil {
				t.Fatalf("BidClosedReasonFromFlags() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("BidClosedReasonFromFlags() = %v, want %v", got, test.want)
			}
		})
	}
}
