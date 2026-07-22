package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"

	aclient "pkg.akt.dev/go/node/client"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

// withActionLog wraps a chain client so every broadcast is recorded in the
// action log carried by ctx (SPEC §5.6) and so a broadcast whose CheckTx
// result carries a non-zero code surfaces as an error (non-zero exit)
// instead of being silently printed as success. The wrapper is applied
// unconditionally; a missing logger only disables recording.
func withActionLog(ctx context.Context, cl aclient.Client) aclient.Client {
	return &loggingClient{Client: cl, log: cliutil.ActionLogFromContext(ctx)}
}

type loggingClient struct {
	aclient.Client
	log *actionlog.Logger
}

func (c *loggingClient) Tx() cv1beta3.TxClient {
	return &loggingTxClient{
		tx:   c.Client.Tx(),
		log:  c.log,
		cctx: c.Client.ClientContext(),
	}
}

type loggingTxClient struct {
	tx   cv1beta3.TxClient
	log  *actionlog.Logger
	cctx sdkclient.Context
}

func (t *loggingTxClient) BroadcastMsgs(ctx context.Context, msgs []sdk.Msg, opts ...cv1beta3.BroadcastOption) (interface{}, error) {
	resp, err := t.tx.BroadcastMsgs(ctx, msgs, opts...)
	t.record(msgs, resp, err)

	return resp, t.failedTxError(resp, err)
}

func (t *loggingTxClient) BroadcastTx(ctx context.Context, tx sdk.Tx, opts ...cv1beta3.BroadcastOption) (interface{}, error) {
	resp, err := t.tx.BroadcastTx(ctx, tx, opts...)
	t.record(tx.GetMsgs(), resp, err)

	return resp, t.failedTxError(resp, err)
}

// failedTxError converts a broadcast whose result carries a non-zero code
// into an error so the CLI exits non-zero on failed transactions. The
// response is still returned to the caller alongside the error.
func (t *loggingTxClient) failedTxError(resp interface{}, err error) error {
	if err != nil {
		return err
	}

	if t.cctx.GenerateOnly || t.cctx.Simulate || t.cctx.Offline {
		return nil
	}

	if r, ok := resp.(*sdk.TxResponse); ok && r != nil && r.Code != 0 {
		return fmt.Errorf("transaction failed: code %d (codespace %s), tx hash %s: %s",
			r.Code, r.Codespace, r.TxHash, r.RawLog)
	}

	return nil
}

// record writes a tx entry for a broadcast. Nothing is recorded when the
// command only generates or simulates the transaction (no state change).
func (t *loggingTxClient) record(msgs []sdk.Msg, resp interface{}, err error) {
	if t.log == nil || len(msgs) == 0 {
		return
	}

	if t.cctx.GenerateOnly || t.cctx.Simulate || t.cctx.Offline {
		return
	}

	entry := actionlog.Entry{
		Type:    actionlog.TypeTx,
		Action:  msgAction(msgs),
		Account: t.cctx.FromName,
		Status:  "success",
	}

	applyMsgIDs(&entry, msgs)

	if r, ok := resp.(*sdk.TxResponse); ok && r != nil {
		entry.TxHash = r.TxHash
		entry.Height = r.Height
		entry.GasUsed = r.GasUsed
		entry.ResultCode = r.Code

		if r.Code != 0 {
			entry.Status = "failed"
			entry.Error = r.RawLog
		}
	}

	if err != nil {
		entry.Status = "failed"
		entry.Error = err.Error()
	}

	_ = t.log.Log(entry)
}

// msgAction derives a compact action identifier from the broadcast messages,
// e.g. "/akash.deployment.v1beta4.MsgCreateDeployment" becomes
// "deployment.MsgCreateDeployment". Additional messages in a multi-msg tx are
// indicated with a "+N" suffix.
func msgAction(msgs []sdk.Msg) string {
	action := shortMsgType(sdk.MsgTypeURL(msgs[0]))
	if len(msgs) > 1 {
		action += "+" + strconv.Itoa(len(msgs)-1)
	}

	return action
}

func shortMsgType(url string) string {
	url = strings.TrimPrefix(url, "/")
	parts := strings.Split(url, ".")

	if len(parts) >= 3 {
		return parts[1] + "." + parts[len(parts)-1]
	}

	return url
}

// applyMsgIDs extracts Akash resource identifiers from well-known message
// types so log entries can be filtered by dseq/provider.
func applyMsgIDs(entry *actionlog.Entry, msgs []sdk.Msg) {
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *dv1beta.MsgCreateDeployment:
			setDeploymentID(entry, m.ID)
		case *dv1beta.MsgUpdateDeployment:
			setDeploymentID(entry, m.ID)
		case *dv1beta.MsgCloseDeployment:
			setDeploymentID(entry, m.ID)
		case *mtypes.MsgCreateLease:
			entry.DSeq = m.BidID.DSeq
			entry.GSeq = m.BidID.GSeq
			entry.OSeq = m.BidID.OSeq
			entry.Provider = m.BidID.Provider
		case *mtypes.MsgCloseLease:
			entry.DSeq = m.ID.DSeq
			entry.GSeq = m.ID.GSeq
			entry.OSeq = m.ID.OSeq
			entry.Provider = m.ID.Provider
		case *mtypes.MsgCloseBid:
			entry.DSeq = m.ID.DSeq
			entry.GSeq = m.ID.GSeq
			entry.OSeq = m.ID.OSeq
			entry.Provider = m.ID.Provider
		}

		if entry.DSeq != 0 {
			return
		}
	}
}

func setDeploymentID(entry *actionlog.Entry, id dv1.DeploymentID) {
	entry.DSeq = id.DSeq
	if entry.Account == "" {
		entry.Account = id.Owner
	}
}
