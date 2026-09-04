package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestMutationRequestAndReconciliationPolicy(t *testing.T) {
	if requestTimeout(http.MethodGet) != 30*time.Second {
		t.Fatalf("GET request timeout = %s, want 30s", requestTimeout(http.MethodGet))
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if requestTimeout(method) != 2*time.Minute {
			t.Fatalf("%s request timeout = %s, want 2m", method, requestTimeout(method))
		}
	}
	if mutationReconciliationWindow != 30*time.Second {
		t.Fatalf("mutation reconciliation window = %s, want 30s", mutationReconciliationWindow)
	}
	if mutationReconciliationPollInterval != 2*time.Second {
		t.Fatalf("mutation reconciliation poll interval = %s, want 2s", mutationReconciliationPollInterval)
	}
}
