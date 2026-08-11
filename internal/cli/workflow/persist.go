package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
	wf "pkg.akt.dev/akt/internal/workflow"
)

// depositAuto is the deposit param value meaning "the chain minimum". The
// resolved amount is never reported back to the run, so it is not recorded.
const depositAuto = "auto"

// recordWorkflowOutcome persists a finished run to the context's local store
// (SPEC §6.6), best-effort.
//
// Every failure is a warning on stderr and nothing more: by this point the
// transactions are already on chain, so failing the command over a bookkeeping
// error would report a real deployment as a failed one. Warnings go to stderr,
// so --output jsonl stdout stays pure.
func recordWorkflowOutcome(cmd *cobra.Command, rc *aktctx.Context, state *wf.RunState) {
	if state == nil || rc == nil || rc.Root == "" || rc.Name == "" {
		return
	}

	// The run may have ended because its context was cancelled (Ctrl-C, or a
	// timeout). What already happened on chain still has to be recorded, so
	// the store write does not inherit that cancellation.
	ctx := context.WithoutCancel(cmd.Context())

	s, err := bbolt.OpenContext(ctx, rc.Root, rc.Name)
	if err != nil {
		warnStoreFailure(cmd, err)

		return
	}
	defer func() { _ = s.Close() }()

	if err := persistWorkflowOutcome(ctx, s, state, time.Now().Unix()); err != nil {
		warnStoreFailure(cmd, err)
	}
}

// warnStoreFailure reports a store write that did not happen without changing
// the command's outcome.
func warnStoreFailure(cmd *cobra.Command, err error) {
	if cliutil.IsQuiet(cmd) {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "warning: the local store was not updated: %v\n", err)
}

// errNoOwner is returned when a run's owner address cannot be determined.
// Records are keyed <owner>:<dseq> (SPEC §4.4), so writing one under an empty
// owner would corrupt the key space and produce a record no lookup can find —
// the run is left unrecorded instead, with the warning naming the fix.
var errNoOwner = errors.New(
	"the deployment owner address could not be determined from the run or the context's default-account; " +
		"run `akt store sync <address>` to record it from chain state",
)

// persistWorkflowOutcome writes what a workflow run observed to the local
// store (SPEC §6.6).
//
// Only steps that actually succeeded contribute, so a partially failed run
// still records the deployment it created — the same one the recovery advice
// tells the user to close. Workflows other than the built-ins are ignored: a
// user-defined definition has no known mapping onto the record types.
func persistWorkflowOutcome(ctx context.Context, s sstore.Store, state *wf.RunState, now int64) error {
	if state == nil || s == nil {
		return nil
	}

	switch state.Workflow {
	case "deploy":
		return persistDeploy(ctx, s, state, now)
	case "update":
		return persistUpdate(ctx, s, state, now)
	case "close":
		return persistClose(ctx, s, state, now)
	default:
		return nil
	}
}

// persistDeploy records the deployment, the won lease, and every bid seen.
func persistDeploy(ctx context.Context, s sstore.Store, state *wf.RunState, now int64) error {
	created := successfulStep(state, "create-deployment")
	if created == nil {
		return nil // nothing reached the chain
	}

	dseq, err := workflowOutputUint64(created.Output, "dseq")
	if err != nil || dseq == 0 {
		return fmt.Errorf("deploy result carries no usable dseq, so it cannot be recorded locally")
	}

	owner := runOwner(state)
	if owner == "" {
		return errNoOwner
	}

	rec := &sstore.DeploymentRecord{
		Owner:         owner,
		DSeq:          dseq,
		State:         "active",
		CreatedHeight: created.Height,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	applySDL(rec, state)

	// "auto" is a request for the chain minimum, not an amount; recording the
	// literal word would be worse than recording nothing.
	if dep := paramString(state, "deposit"); dep != "" && dep != depositAuto {
		rec.Deposit = dep
	}

	if err := s.PutDeployment(ctx, rec); err != nil {
		return fmt.Errorf("record deployment %d: %w", dseq, err)
	}

	winner := leaseProvider(state)

	for _, b := range observedBids(state) {
		bid := &sstore.BidRecord{
			ID:                 b.id,
			State:              "lost",
			Price:              b.price,
			ProviderAttributes: b.attributes,
			ProviderAudited:    b.audited,
			CreatedAt:          now,
		}
		if bid.ID.Owner == "" {
			bid.ID.Owner = owner
		}
		if bid.ID.DSeq == 0 {
			bid.ID.DSeq = dseq
		}
		if winner != "" && bid.ID.Provider == winner {
			bid.State = "matched"
		}

		if err := s.PutBid(ctx, bid); err != nil {
			return fmt.Errorf("record bid from %s: %w", bid.ID.Provider, err)
		}
	}

	return persistWonLease(ctx, s, state, owner, dseq, now)
}

// persistWonLease records the lease a deploy run opened.
//
// Provider gateway URI and service endpoints are left empty: the run never
// learns them (send-manifest returns no output), and `akt store sync` reads
// them from chain state later.
func persistWonLease(ctx context.Context, s sstore.Store, state *wf.RunState, owner string, dseq uint64, now int64) error {
	leased := successfulStep(state, "create-lease")
	if leased == nil {
		return nil
	}

	provider := leaseProvider(state)
	if provider == "" {
		return fmt.Errorf("lease result names no provider, so the lease cannot be recorded locally")
	}

	gseq, _ := asUint64(leaseField(state, "gseq"))
	oseq, _ := asUint64(leaseField(state, "oseq"))

	lease := &sstore.LeaseRecord{
		ID: sstore.LeaseID{
			Owner:    owner,
			DSeq:     dseq,
			GSeq:     uint32(gseq), //nolint:gosec // group sequences are small by construction
			OSeq:     uint32(oseq), //nolint:gosec // order sequences are small by construction
			Provider: provider,
		},
		State:     "active",
		Price:     normalizeCoin(workflowOutputString(state.Steps["select-bid"], "price")),
		CreatedAt: now,
	}

	if err := s.PutLease(ctx, lease); err != nil {
		return fmt.Errorf("record lease %s: %w", sstore.LeaseKey(lease.ID), err)
	}

	return nil
}

// leaseField reads a lease identity field from the lease step, falling back to
// the bid selection that produced it.
func leaseField(state *wf.RunState, key string) string {
	if v := workflowOutputString(state.Steps["create-lease"], key); v != "" {
		return v
	}

	return workflowOutputString(state.Steps["select-bid"], key)
}

// persistUpdate refreshes the stored SDL identity of an updated deployment.
func persistUpdate(ctx context.Context, s sstore.Store, state *wf.RunState, now int64) error {
	updated := successfulStep(state, "update-deployment")
	if updated == nil {
		return nil
	}

	dseq := runDSeq(state, updated)
	if dseq == 0 {
		return fmt.Errorf("update result carries no usable dseq, so it cannot be recorded locally")
	}

	owner, err := existingRunOwner(ctx, s, state, dseq)
	if err != nil {
		return err
	}

	rec, err := s.GetDeployment(ctx, owner, dseq)
	if err != nil {
		return fmt.Errorf("read deployment %d: %w", dseq, err)
	}
	if rec == nil {
		// The deployment was created outside this store (another machine, or
		// before workflow persistence existed). Recording what this run knows
		// beats leaving it invisible; `akt store sync` fills in the rest.
		rec = &sstore.DeploymentRecord{
			Owner:     owner,
			DSeq:      dseq,
			State:     "active",
			CreatedAt: now,
		}
	}

	applySDL(rec, state)
	rec.UpdatedAt = now

	if err := s.PutDeployment(ctx, rec); err != nil {
		return fmt.Errorf("record deployment %d: %w", dseq, err)
	}

	return nil
}

// persistClose marks a closed deployment and its leases closed.
func persistClose(ctx context.Context, s sstore.Store, state *wf.RunState, now int64) error {
	closed := successfulStep(state, "close-deployment")
	if closed == nil {
		return nil
	}

	dseq := runDSeq(state, closed)
	if dseq == 0 {
		return fmt.Errorf("close result carries no usable dseq, so it cannot be recorded locally")
	}

	owner, err := existingRunOwner(ctx, s, state, dseq)
	if err != nil {
		return err
	}

	if err := s.MarkDeploymentClosed(ctx, owner, dseq, now); err != nil {
		return fmt.Errorf("record deployment %d close: %w", dseq, err)
	}

	return nil
}

// existingRunOwner resolves ownerless Console update/close results against
// local state. A DSEQ is only safe as an owner lookup when exactly one local
// account carries it.
func existingRunOwner(ctx context.Context, s sstore.Store, state *wf.RunState, dseq uint64) (string, error) {
	if owner := runOwner(state); owner != "" {
		return owner, nil
	}

	owner, err := sstore.UniqueDeploymentOwner(ctx, s, dseq)
	if err != nil {
		return "", err
	}
	if owner == "" {
		return "", errNoOwner
	}

	return owner, nil
}

// applySDL records the SDL file a run used. A file that can no longer be read
// leaves the hash empty rather than failing: the path is still worth keeping.
func applySDL(rec *sstore.DeploymentRecord, state *wf.RunState) {
	path := paramString(state, "sdl-file")
	if path == "" {
		return
	}

	rec.SDLPath = path

	if hash, err := sdlHash(path); err == nil {
		rec.SDLHash = hash
	}
}

// sdlHash returns the SHA256 of an SDL file's contents (SPEC §6.6).
func sdlHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// runOwner determines the owner address a run's records are keyed by.
//
// The transaction result is preferred, then the market identity returned with
// the lease or the winning bid (which is what a rail that omits the owner from
// its deployment response still provides), and finally the run's account — but
// only when that is an address, since a keyring key name would key records
// under a string no lookup could match.
func runOwner(state *wf.RunState) string {
	for _, step := range []string{
		"create-deployment",
		"update-deployment",
		"close-deployment",
		"create-lease",
		"select-bid",
	} {
		if v := workflowOutputString(state.Steps[step], "owner"); v != "" {
			return v
		}
	}

	if addr, err := sdk.AccAddressFromBech32(strings.TrimSpace(state.Account)); err == nil {
		return addr.String()
	}

	return ""
}

// runDSeq reads the deployment sequence from a step result, falling back to
// the run's own dseq parameter.
func runDSeq(state *wf.RunState, sr *wf.StepResult) uint64 {
	if sr != nil {
		if dseq, err := workflowOutputUint64(sr.Output, "dseq"); err == nil && dseq != 0 {
			return dseq
		}
	}

	if state.Params == nil {
		return 0
	}

	dseq, _ := asUint64(state.Params["dseq"])

	return dseq
}

// leaseProvider returns the provider the run leased from.
func leaseProvider(state *wf.RunState) string {
	if p := workflowOutputString(state.Steps["create-lease"], "provider"); p != "" {
		return p
	}

	return workflowOutputString(state.Steps["select-bid"], "provider")
}

// successfulStep returns a step result only when the step succeeded.
func successfulStep(state *wf.RunState, name string) *wf.StepResult {
	sr := state.Steps[name]
	if sr == nil || sr.Status != "success" {
		return nil
	}

	return sr
}

// paramString reads a string workflow parameter.
func paramString(state *wf.RunState, name string) string {
	if state.Params == nil {
		return ""
	}

	v, _ := state.Params[name].(string)

	return strings.TrimSpace(v)
}

// observedBid is the identity and price of one bid a run saw.
type observedBid struct {
	id         sstore.BidID
	price      string
	attributes map[string]string
	audited    bool
}

// observedBids extracts every bid the wait-for-bids step returned.
//
// The shape is tolerated rather than assumed: the chain rail returns the query
// proto ({"bid":{"id":{...},"price":{...}}}) and the Console rail a flatter
// object, with sequence numbers arriving as JSON numbers or strings depending
// on the rail.
func observedBids(state *wf.RunState) []observedBid {
	sr := successfulStep(state, "wait-for-bids")
	if sr == nil || sr.Output == nil {
		return nil
	}

	list, ok := sr.Output["bids"].([]any)
	if !ok {
		return nil
	}

	out := make([]observedBid, 0, len(list))
	metadata := observedProviderMetadata(sr.Output["provider_metadata"])
	for _, item := range list {
		inner, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if b, nested := inner["bid"].(map[string]any); nested {
			inner = b
		}

		id := inner
		if m, nested := inner["id"].(map[string]any); nested {
			id = m
		}

		provider, _ := id["provider"].(string)
		if provider == "" {
			continue // a bid with no provider cannot be keyed
		}

		owner, _ := id["owner"].(string)
		dseq, _ := asUint64(id["dseq"])
		gseq, _ := asUint64(id["gseq"])
		oseq, _ := asUint64(id["oseq"])

		providerMetadata := metadata[provider]
		out = append(out, observedBid{
			id: sstore.BidID{
				Owner:    owner,
				DSeq:     dseq,
				GSeq:     uint32(gseq), //nolint:gosec // group sequences are small by construction
				OSeq:     uint32(oseq), //nolint:gosec // order sequences are small by construction
				Provider: provider,
			},
			price:      coinString(inner["price"]),
			attributes: providerMetadata.attributes,
			audited:    providerMetadata.audited,
		})
	}

	return out
}

type observedMetadata struct {
	attributes map[string]string
	audited    bool
}

func observedProviderMetadata(value any) map[string]observedMetadata {
	entries, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	result := make(map[string]observedMetadata, len(entries))
	for provider, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		attributes := map[string]string{}
		switch rawAttributes := entry["attributes"].(type) {
		case map[string]any:
			for key, value := range rawAttributes {
				if stringValue, ok := value.(string); ok {
					attributes[key] = stringValue
				}
			}
		case map[string]string:
			for key, value := range rawAttributes {
				attributes[key] = value
			}
		default:
			continue
		}

		audited, _ := entry["audited"].(bool)
		result[provider] = observedMetadata{attributes: attributes, audited: audited}
	}

	return result
}

// coinString renders a {amount, denom} object as a coin string, normalized
// through the SDK decimal parser when it accepts the value so that records
// written here and by reconciliation compare equal.
func coinString(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}

	denom, _ := m["denom"].(string)
	amount := scalarString(m["amount"])
	if amount == "" || denom == "" {
		return ""
	}

	return normalizeCoin(amount + denom)
}

// normalizeCoin renders a coin string through the SDK decimal parser so that
// records written by a workflow run and by reconciliation compare equal. A
// value the parser rejects is kept verbatim rather than dropped.
func normalizeCoin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if coin, err := sdk.ParseDecCoin(raw); err == nil {
		return coin.String()
	}

	return raw
}

// scalarString renders a JSON scalar as a plain decimal string, never
// scientific notation.
func scalarString(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(n)
	case json.Number:
		return n.String()
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// asUint64 reads an unsigned integer from a JSON scalar or a workflow param.
func asUint64(v any) (uint64, bool) {
	switch n := v.(type) {
	case nil:
		return 0, false
	case int:
		if n < 0 {
			return 0, false
		}

		return uint64(n), true
	case uint64:
		return n, true
	case float64:
		if n < 0 {
			return 0, false
		}

		return uint64(n), true
	}

	parsed, err := strconv.ParseUint(scalarString(v), 10, 64)
	if err != nil {
		return 0, false
	}

	return parsed, true
}
