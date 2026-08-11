package cliutil

import (
	"errors"
	"testing"
)

func TestUserFacingErrorRemovesRedundantSDKWrappers(t *testing.T) {
	raw := "rpc error: code = Unknown desc = rpc error: code = Unknown desc = " +
		"failed to execute message; message index: 0: spendable balance 5493015uakt " +
		"is smaller than 999999999999uakt: insufficient funds " +
		"[cosmos/cosmos-sdk@v0.53.6/x/bank/keeper/send.go:298]"
	err := ErrTransaction("transaction failed", errors.New(raw))

	want := "Error: transaction failed\n\n" +
		"  Cause:      spendable balance 5493015uakt is smaller than " +
		"999999999999uakt: insufficient funds"
	if got := UserFacingError(err); got != want {
		t.Fatalf("UserFacingError() = %q, want %q", got, want)
	}

	if err.Cause.Error() != raw {
		t.Fatal("display cleanup mutated the underlying diagnostic error")
	}
	if ExitCode(err) != ExitTransaction {
		t.Fatalf("exit code = %d, want %d", ExitCode(err), ExitTransaction)
	}
}

func TestUserFacingErrorLeavesOrdinaryErrorsAlone(t *testing.T) {
	err := errors.New("provider returned [quota exceeded]")
	if got := UserFacingError(err); got != err.Error() {
		t.Fatalf("UserFacingError() = %q, want %q", got, err)
	}
}
