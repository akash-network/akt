package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	cclient "pkg.akt.dev/go/node/client/v1beta3"
)

func TestSplitAndApplyUnlimitedRunsOnce(t *testing.T) {
	calls := 0
	responses, err := newSplitAndApply(
		context.Background(),
		func(_ context.Context, msgs []sdk.Msg, _ ...cclient.BroadcastOption) (interface{}, error) {
			calls++
			if len(msgs) != 2 {
				t.Fatalf("message count = %d, want 2", len(msgs))
			}
			return nil, nil
		},
		[]sdk.Msg{nil, nil},
		0,
	)
	if err != nil {
		t.Fatalf("newSplitAndApply: %v", err)
	}
	if calls != 1 {
		t.Fatalf("unlimited batch called broadcaster %d times, want 1", calls)
	}
	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
}

func TestSplitAndApplyRejectsEmptyMessageSet(t *testing.T) {
	calls := 0
	responses, err := newSplitAndApply(
		context.Background(),
		func(_ context.Context, _ []sdk.Msg, _ ...cclient.BroadcastOption) (interface{}, error) {
			calls++
			return nil, nil
		},
		nil,
		0,
	)
	if err == nil || !strings.Contains(err.Error(), "no messages") {
		t.Fatalf("empty batch error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("empty batch called broadcaster %d times", calls)
	}
	if responses != nil {
		t.Fatalf("empty batch responses = %#v", responses)
	}
}

func TestSplitAndApplyBatchesEachMessageOnce(t *testing.T) {
	calls := 0
	seen := 0
	responses, err := newSplitAndApply(
		context.Background(),
		func(_ context.Context, msgs []sdk.Msg, _ ...cclient.BroadcastOption) (interface{}, error) {
			calls++
			seen += len(msgs)
			return calls, nil
		},
		[]sdk.Msg{nil, nil, nil, nil, nil},
		2,
	)
	if err != nil {
		t.Fatalf("newSplitAndApply: %v", err)
	}
	if calls != 3 || seen != 5 || len(responses) != 3 {
		t.Fatalf("calls=%d messages=%d responses=%d, want 3/5/3", calls, seen, len(responses))
	}
}

func TestDraftProposalRequiresTTY(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(&bytes.Buffer{})
	err := requireDraftProposalTTY(cmd)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "terminal") {
		t.Fatalf("non-TTY error = %v", err)
	}
}
