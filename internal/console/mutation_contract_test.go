package console

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestValidateCreateDeploymentResultRequiresCompleteReceipt(t *testing.T) {
	tests := []struct {
		name   string
		result CreateDeploymentResult
		want   string
	}{
		{
			name: "valid",
			result: CreateDeploymentResult{
				DSeq:   "42",
				SignTx: &SignTx{Code: 0, TransactionHash: "tx-42", codePresent: true},
			},
		},
		{name: "invalid dseq", result: CreateDeploymentResult{DSeq: "0"}, want: "invalid dseq"},
		{name: "missing receipt", result: CreateDeploymentResult{DSeq: "42"}, want: "omitted the managed-wallet"},
		{
			name:   "missing code",
			result: CreateDeploymentResult{DSeq: "42", SignTx: &SignTx{TransactionHash: "tx-42"}},
			want:   "omitted its code",
		},
		{
			name: "failed receipt",
			result: CreateDeploymentResult{DSeq: "42", SignTx: &SignTx{
				Code: 7, TransactionHash: "tx-failed", RawLog: "out of gas", codePresent: true,
			}},
			want: "transaction tx-failed failed with code 7: out of gas",
		},
		{
			name:   "blank hash",
			result: CreateDeploymentResult{DSeq: "42", SignTx: &SignTx{codePresent: true}},
			want:   "omitted its hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateDeploymentResult(&tt.result)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("valid receipt failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSignTxUnmarshalTracksCodePresence(t *testing.T) {
	var present SignTx
	if err := json.Unmarshal([]byte(`{"code":0,"transactionHash":"tx"}`), &present); err != nil {
		t.Fatalf("unmarshal present code: %v", err)
	}
	if !present.codePresent || present.Code != 0 || present.TransactionHash != "tx" {
		t.Fatalf("present receipt = %+v, want explicit zero code and hash", present)
	}

	var missing SignTx
	if err := json.Unmarshal([]byte(`{"transactionHash":"tx"}`), &missing); err != nil {
		t.Fatalf("unmarshal missing code: %v", err)
	}
	if missing.codePresent {
		t.Fatal("a missing receipt code was recorded as present")
	}

	var malformed SignTx
	if err := malformed.UnmarshalJSON([]byte(`{"code":`)); err == nil {
		t.Fatal("malformed transaction receipt unexpectedly decoded")
	}
}

func TestValidateDeploymentUpdateResultRejectsHashMismatch(t *testing.T) {
	result := &DeploymentDetail{Deployment: Deployment{
		ID:   DeploymentID{DSeq: "42"},
		Hash: "unexpected-hash",
	}}
	err := validateDeploymentUpdateResult(result, "42", "expected-hash")
	if err == nil || !strings.Contains(err.Error(), `SDL hash "unexpected-hash", want "expected-hash"`) {
		t.Fatalf("hash mismatch error = %v", err)
	}
}

func TestDepositContractHelpers(t *testing.T) {
	micros, err := consoleDepositMicros(1.25)
	if err != nil || micros.Cmp(big.NewInt(1_250_000)) != 0 {
		t.Fatalf("consoleDepositMicros(1.25) = %v, %v", micros, err)
	}
	for _, amount := range []float64{0, -1, 0.0000001} {
		if _, err := consoleDepositMicros(amount); err == nil {
			t.Errorf("consoleDepositMicros(%v) unexpectedly succeeded", amount)
		}
	}

	before := map[string]*big.Rat{"uact": big.NewRat(11, 2)}
	after := map[string]*big.Rat{"uact": big.NewRat(31, 2)}
	if !depositTotalDeltaMatches(before, after, big.NewInt(10)) {
		t.Fatal("exact deposit delta across fractional balances did not match")
	}
	if depositTotalDeltaMatches(before, after, big.NewInt(9)) {
		t.Fatal("inexact deposit delta matched")
	}
	if depositTotalDeltaMatches(before, map[string]*big.Rat{}, big.NewInt(10)) {
		t.Fatal("missing settlement denomination matched")
	}
	if !depositTotalDeltaMatches(map[string]*big.Rat{}, map[string]*big.Rat{"uact": big.NewRat(10, 1)}, big.NewInt(10)) {
		t.Fatal("deposit into an escrow with no prior uact balance did not use a zero baseline")
	}
	nearBefore, ok := new(big.Rat).SetString("0.100000000000000001")
	if !ok {
		t.Fatal("construct exact near-miss pre-state")
	}
	nearAfter, ok := new(big.Rat).SetString("10.100000000000000002")
	if !ok {
		t.Fatal("construct exact near-miss post-state")
	}
	if depositTotalDeltaMatches(
		map[string]*big.Rat{"uact": nearBefore},
		map[string]*big.Rat{"uact": nearAfter},
		big.NewInt(10),
	) {
		t.Fatal("deposit delta comparison discarded a one-quantum fractional excess")
	}
	if before["uact"].Cmp(big.NewRat(11, 2)) != 0 || after["uact"].Cmp(big.NewRat(31, 2)) != 0 {
		t.Fatal("depositTotalDeltaMatches mutated an input amount")
	}
}

func TestDeploymentEscrowTotalsRequireFundsAndTransferred(t *testing.T) {
	for _, tt := range []struct {
		name   string
		escrow string
		want   string
	}{
		{name: "missing funds", escrow: `{"state":{"transferred":[]}}`, want: "omitted its funds"},
		{name: "missing transferred", escrow: `{"state":{"funds":[]}}`, want: "omitted its transferred"},
		{name: "malformed envelope", escrow: `{`, want: "decode deployment escrow"},
		{name: "malformed funds array item", escrow: `{"state":{"funds":[true],"transferred":[]}}`, want: "decode deployment escrow funds"},
		{name: "malformed funds object", escrow: `{"state":{"funds":true,"transferred":[]}}`, want: "decode deployment escrow funds"},
		{name: "blank denomination", escrow: `{"state":{"funds":[{"amount":"1"}],"transferred":[]}}`, want: "omitted its denomination"},
		{name: "malformed amount", escrow: `{"state":{"funds":[{"denom":"uact","amount":"not-a-decimal"}],"transferred":[]}}`, want: "not a valid fixed-point decimal"},
		{name: "negative transferred", escrow: `{"state":{"funds":[],"transferred":[{"denom":"uact","amount":"-1"}]}}`, want: "must be non-negative"},
		{name: "too much precision", escrow: `{"state":{"funds":[{"denom":"uact","amount":"0.0000000000000000001"}],"transferred":[]}}`, want: "not a valid fixed-point decimal"},
		{name: "exponent syntax", escrow: `{"state":{"funds":[{"denom":"uact","amount":"1e6"}],"transferred":[]}}`, want: "not a valid fixed-point decimal"},
		{name: "rational syntax", escrow: `{"state":{"funds":[{"denom":"uact","amount":"1/2"}],"transferred":[]}}`, want: "not a valid fixed-point decimal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := deploymentEscrowTotals(&DeploymentDetail{EscrowAccount: json.RawMessage(tt.escrow)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}

	for _, detail := range []*DeploymentDetail{nil, {}, {EscrowAccount: json.RawMessage("null")}} {
		if _, err := deploymentEscrowTotals(detail); err == nil || !strings.Contains(err.Error(), "omitted its escrow account") {
			t.Errorf("missing escrow detail %#v returned %v", detail, err)
		}
	}
}

func TestDeploymentEscrowTotalsAcceptsFixedScaleAmounts(t *testing.T) {
	detail := &DeploymentDetail{EscrowAccount: json.RawMessage(`{
		"state": {
			"funds": [
				{"denom":"uact","amount":"500000.000000000000000000"},
				{"denom":"uact","amount":"-0.100000000000000001"},
				{"denom":"uact","amount":"0.100000000000000002"},
				{"denom":"uusdc","amount":1}
			],
			"transferred": [{"denom":"uact","amount":"250000.000000000000000000"}]
		}
	}`)}

	totals, err := deploymentEscrowTotals(detail)
	if err != nil {
		t.Fatalf("fixed-scale escrow amount failed: %v", err)
	}
	wantUACT, ok := new(big.Rat).SetString("750000.000000000000000001")
	if !ok {
		t.Fatal("construct exact uact expectation")
	}
	if totals["uact"] == nil || totals["uact"].Cmp(wantUACT) != 0 {
		t.Fatalf("uact total = %v, want %v", totals["uact"], wantUACT)
	}
	if totals["uusdc"] == nil || totals["uusdc"].Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("uusdc total = %v, want 1", totals["uusdc"])
	}
}

func TestReconcileDepositRejectsUntrustedPostState(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "wrong deployment identity",
			body: `{"data":{"deployment":{"id":{"dseq":"999"}},"escrow_account":{"state":{"funds":[],"transferred":[]}}}}`,
		},
		{
			name: "malformed escrow state",
			body: `{"data":{"deployment":{"id":{"dseq":"42"}}}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 2 {
					cancel()
					return
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			reconciled, err := New(srv.URL, "test-key").reconcileDeposit(
				ctx,
				"42",
				map[string]*big.Rat{"uact": big.NewRat(1_000_000, 1)},
				big.NewInt(1_000_000),
			)
			if reconciled || err == nil {
				t.Fatalf("reconcile result = %t, %v, want a failed authoritative check", reconciled, err)
			}
			if requests.Load() != 2 {
				t.Fatalf("reconciliation requests = %d, want one classified response before cancellation", requests.Load())
			}
		})
	}
}

func TestReconcileDepositExhaustsBoundedChecksWithoutExactDelta(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
	}))
	defer srv.Close()

	reconciled, err := New(srv.URL, "test-key").reconcileDeposit(
		context.Background(),
		"42",
		map[string]*big.Rat{"uact": big.NewRat(1_000_000, 1)},
		big.NewInt(1_000_000),
	)
	if reconciled || err == nil || !strings.Contains(err.Error(), "did not increase by exactly") {
		t.Fatalf("reconcile result = %t, %v, want bounded no-delta failure", reconciled, err)
	}
	if requests.Load() != maxRetries+2 {
		t.Fatalf("reconciliation requests = %d, want %d", requests.Load(), maxRetries+2)
	}
}
