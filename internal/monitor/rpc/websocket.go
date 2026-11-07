package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	cstypes "github.com/cometbft/cometbft/consensus/types"
	"github.com/gorilla/websocket"

	"pkg.akt.dev/akt/internal/monitor/consensus"
)

// ConsensusSnapshot carries a live update, committed timestamp, or stream status.
type ConsensusSnapshot struct {
	State *consensus.State
	Block *CommittedBlock
	Err   error
	Reset bool // connection bootstrap invalidates incomplete live history
}

// CommittedBlock carries chain time, independent of when the observer receives it.
type CommittedBlock struct {
	Height int64
	Time   time.Time
}

// ConsensusSubscription exposes snapshots and the lifetime of their producer.
// Owners of resources used by the producer wait for Done before closing them.
type ConsensusSubscription struct {
	Snapshots <-chan ConsensusSnapshot
	done      <-chan struct{}
}

// Done closes after the producer has closed its socket and snapshot channel.
func (s *ConsensusSubscription) Done() <-chan struct{} {
	return s.done
}

const (
	consensusWebSocketTimeout = 10 * time.Second
	consensusReconnectDelay   = 250 * time.Millisecond
)

// SubscribeConsensusState connects to the RPC endpoint over WebSocket,
// subscribes to vote, round, step, and block events, and locally tracks
// prevote/precommit power to produce real-time consensus snapshots.
//
// The returned subscription delivers a snapshot after every vote and step
// transition. Reconnects restore subscriptions and bootstrap current votes.
// Snapshot and completion channels close when reconnect/resubscribe is
// exhausted or ctx is cancelled.
func (c *Client) SubscribeConsensusState(ctx context.Context) (*ConsensusSubscription, error) {
	wsURL, err := consensusWebSocketURL(c.rpcEndpoint)
	if err != nil {
		return nil, fmt.Errorf("create websocket client: %w", err)
	}

	ws, tracker, buffered, err := c.openConsensusConnection(ctx, wsURL)
	if err != nil {
		return nil, err
	}

	ch := make(chan ConsensusSnapshot, 16)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(ch)
		for ws != nil {
			consumeErr := c.consumeConsensusConnection(ctx, ws, tracker, buffered, ch)
			_ = ws.Close()
			if ctx.Err() == nil {
				select {
				case ch <- ConsensusSnapshot{Err: fmt.Errorf("consensus stream disconnected: %w", consumeErr)}:
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
			// Validator-set failures are state-integrity failures, not transport
			// failures. Let the UI's bounded retry begin a new connection cycle
			// rather than spinning this stream against a bad height response.
			if consumeErr != nil && !isConsensusTransportError(consumeErr) {
				return
			}
			if !waitForConsensusReconnect(ctx) {
				return
			}

			// A failed reconnect setup closes this channel. The UI owns the
			// longer-lived capped retry schedule and will start a fresh cycle.
			ws, tracker, buffered, err = c.openConsensusConnection(ctx, wsURL)
			if err != nil {
				return
			}
		}
	}()

	return &ConsensusSubscription{
		Snapshots: ch,
		done:      done,
	}, nil
}

type consensusTransportError struct {
	err error
}

func (err consensusTransportError) Error() string { return err.err.Error() }
func (err consensusTransportError) Unwrap() error { return err.err }

func isConsensusTransportError(err error) bool {
	var transportErr consensusTransportError
	return errors.As(err, &transportErr)
}

func (c *Client) openConsensusConnection(
	ctx context.Context,
	wsURL string,
) (*websocket.Conn, *consensusTracker, []consensusWSResponse, error) {
	setupCtx, cancel := context.WithTimeout(ctx, consensusWebSocketTimeout)
	defer cancel()

	ws, response, err := websocket.DefaultDialer.DialContext(setupCtx, wsURL, nil)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("websocket connect to %s: %w", c.rpcEndpoint, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = ws.Close()
		}
	}()

	// Make cancellation interrupt acknowledgement reads as well as the dial.
	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-setupCtx.Done():
			_ = ws.Close()
		case <-stopWatch:
		}
	}()
	defer func() {
		close(stopWatch)
		<-watchDone
	}()

	// Each connection cycle starts with a fresh validator set. A prior
	// successful response may belong to an earlier height.
	_, err = c.refreshValidators(setupCtx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load validators: %w", err)
	}
	buffered, err := subscribeConsensusEvents(setupCtx, ws)
	if err != nil {
		if setupCtx.Err() != nil {
			return nil, nil, nil, fmt.Errorf("subscribe to consensus events: %w", setupCtx.Err())
		}
		return nil, nil, nil, err
	}

	snapshotResponse, err := c.GetConsensusState(setupCtx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("bootstrap consensus: %w", err)
	}
	height, _, _, err := consensus.ParseHeightRoundStep(snapshotResponse.Result.RoundState.HeightRoundStep)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("bootstrap consensus at height %d: %w", height, err)
	}
	validators, err := c.refreshValidatorsAtHeight(setupCtx, height)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("bootstrap consensus at height %d: %w", height, err)
	}
	state, err := consensus.ParseConsensusState(snapshotResponse, validators)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("bootstrap consensus at height %d: %w", height, err)
	}
	tracker := newConsensusTracker(validators)
	tracker.seed(state)
	closeOnError = false
	return ws, tracker, buffered, nil
}

func (c *Client) consumeConsensusConnection(
	ctx context.Context,
	ws *websocket.Conn,
	tracker *consensusTracker,
	buffered []consensusWSResponse,
	ch chan<- ConsensusSnapshot,
) error {
	select {
	case ch <- ConsensusSnapshot{
		State: tracker.buildState(),
		Reset: true,
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	readDone := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-readDone:
		}
	}()
	defer func() {
		close(readDone)
		<-watchDone
	}()

	for {
		var resp consensusWSResponse
		if len(buffered) > 0 {
			resp = buffered[0]
			buffered = buffered[1:]
		} else {
			if err := ws.ReadJSON(&resp); err != nil {
				return consensusTransportError{err: err}
			}
		}
		if resp.Error != nil {
			return fmt.Errorf("consensus subscription failed: RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if len(resp.Result) == 0 {
			continue
		}

		eventHeight, hasHeight := consensusMessageHeight(resp.Result)
		if hasHeight && (!tracker.initialized || eventHeight > tracker.height) {
			validators, err := c.refreshValidatorsAtHeight(ctx, eventHeight)
			if err != nil {
				return err
			}
			tracker = newConsensusTracker(validators)
		}

		snap, changed := tracker.handleMessage(resp.Result)
		if !changed {
			continue
		}

		select {
		case ch <- snap:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func waitForConsensusReconnect(ctx context.Context) bool {
	timer := time.NewTimer(consensusReconnectDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func consensusMessageHeight(raw json.RawMessage) (int64, bool) {
	var result struct {
		Query string          `json:"query"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Data) == 0 {
		return 0, false
	}

	var height string
	switch {
	case strings.Contains(result.Query, "Vote"):
		var event struct {
			Value struct {
				Vote struct {
					Height string `json:"height"`
				} `json:"Vote"`
			} `json:"value"`
		}
		if err := json.Unmarshal(result.Data, &event); err != nil {
			return 0, false
		}
		height = event.Value.Vote.Height
	case strings.Contains(result.Query, "NewRound"):
		var event struct {
			Value struct {
				Height string `json:"height"`
			} `json:"value"`
		}
		if err := json.Unmarshal(result.Data, &event); err != nil {
			return 0, false
		}
		height = event.Value.Height
	default:
		return 0, false
	}

	parsed, err := strconv.ParseInt(height, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

type consensusWSRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      int               `json:"id"`
	Method  string            `json:"method"`
	Params  map[string]string `json:"params"`
}

type consensusWSResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type consensusSubscriptionSocket interface {
	SetWriteDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	WriteJSON(any) error
	ReadJSON(any) error
}

func consensusWebSocketURL(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported RPC scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/websocket"
	return parsed.String(), nil
}

func subscribeConsensusEvents(ctx context.Context, ws consensusSubscriptionSocket) ([]consensusWSResponse, error) {
	queries := []struct {
		name  string
		query string
	}{
		{name: "Vote", query: "tm.event='Vote'"},
		{name: "NewRoundStep", query: "tm.event='NewRoundStep'"},
		{name: "NewRound", query: "tm.event='NewRound'"},
		{name: "NewBlock", query: "tm.event='NewBlock'"},
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := ws.SetWriteDeadline(deadline); err != nil {
			return nil, fmt.Errorf("set subscription write deadline: %w", err)
		}
		if err := ws.SetReadDeadline(deadline); err != nil {
			return nil, fmt.Errorf("set subscription read deadline: %w", err)
		}
		defer ws.SetWriteDeadline(time.Time{}) //nolint:errcheck
		defer ws.SetReadDeadline(time.Time{})  //nolint:errcheck
	}

	for index, subscription := range queries {
		id := index + 1
		request := consensusWSRequest{
			JSONRPC: "2.0",
			ID:      id,
			Method:  "subscribe",
			Params:  map[string]string{"query": subscription.query},
		}
		if err := ws.WriteJSON(request); err != nil {
			return nil, fmt.Errorf("subscribe to %s events: %w", subscription.name, err)
		}
	}

	subscriptions := make(map[int]string, len(queries))
	pending := make(map[int]struct{}, len(queries))
	for index, query := range queries {
		subscriptions[index+1] = query.name
		pending[index+1] = struct{}{}
	}
	buffered := make([]consensusWSResponse, 0)
	for len(pending) > 0 {
		var response consensusWSResponse
		if err := ws.ReadJSON(&response); err != nil {
			return nil, fmt.Errorf("await subscription acknowledgement: %w", err)
		}
		name, ok := subscriptions[response.ID]
		if !ok {
			return nil, fmt.Errorf("subscription acknowledgement has unexpected response id %d", response.ID)
		}
		if response.Error != nil {
			detail := response.Error.Message
			if response.Error.Data != "" {
				detail += ": " + response.Error.Data
			}
			return nil, fmt.Errorf("subscribe to %s events: RPC error %d: %s", name, response.Error.Code, detail)
		}
		if isConsensusSubscriptionAck(response.Result) {
			if _, waiting := pending[response.ID]; !waiting {
				return nil, fmt.Errorf("subscribe to %s events: duplicate acknowledgement", name)
			}
			delete(pending, response.ID)
			continue
		}
		if !isConsensusEventResult(response.Result) {
			return nil, fmt.Errorf("subscribe to %s events: malformed acknowledgement", name)
		}
		// CometBFT starts the subscription producer before its request handler
		// writes the empty acknowledgement. Preserve a valid event that races
		// ahead of either acknowledgement and process it after setup completes.
		buffered = append(buffered, response)
	}

	return buffered, nil
}

func isConsensusSubscriptionAck(result json.RawMessage) bool {
	var acknowledgement map[string]json.RawMessage
	if err := json.Unmarshal(result, &acknowledgement); err != nil {
		return false
	}
	return acknowledgement != nil && len(acknowledgement) == 0
}

func isConsensusEventResult(result json.RawMessage) bool {
	var event struct {
		Query string          `json:"query"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(result, &event); err != nil {
		return false
	}
	return event.Query != "" && len(event.Data) > 0 && string(event.Data) != "null"
}

// consensusTracker accumulates vote events to produce real-time state.
type consensusTracker struct {
	validators      []consensus.Validator
	validatorPowers map[int]int64 // validator_index -> voting_power
	totalPower      int64

	initialized bool
	height      int64
	round       int
	step        int
	startTime   time.Time

	proposerAddr  string
	proposerIndex int

	// Per-round vote tracking
	prevoted    map[int]bool // validator indices that prevoted
	precommited map[int]bool // validator indices that precommited
	pvPower     int64        // accumulated prevote voting power
	pcPower     int64        // accumulated precommit voting power
}

func newConsensusTracker(validators []consensus.Validator) *consensusTracker {
	powers := make(map[int]int64, len(validators))
	var totalPower int64
	for i, v := range validators {
		p, _ := strconv.ParseInt(v.VotingPower, 10, 64)
		powers[i] = p
		totalPower += p
	}

	return &consensusTracker{
		validators:      validators,
		validatorPowers: powers,
		totalPower:      totalPower,
		proposerIndex:   -1, // unknown until the bootstrap snapshot or a NewRound event
		prevoted:        make(map[int]bool),
		precommited:     make(map[int]bool),
	}
}

// handleMessage processes a raw event result and returns a snapshot
// if the state changed.
func (t *consensusTracker) handleMessage(raw json.RawMessage) (ConsensusSnapshot, bool) {
	// The CometBFT WSClient delivers the "result" field which wraps
	// query + data for subscription events.
	var result struct {
		Query string          `json:"query"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return ConsensusSnapshot{}, false
	}

	if len(result.Data) == 0 || string(result.Data) == "{}" {
		return ConsensusSnapshot{}, false
	}

	switch {
	case strings.Contains(result.Query, "Vote"):
		return t.handleVoteEvent(result.Data)
	case strings.Contains(result.Query, "NewRound"):
		return t.handleNewRoundStepEvent(result.Data)
	case strings.Contains(result.Query, "NewBlock"):
		var event struct {
			Value struct {
				Block struct {
					Header struct {
						Height string    `json:"height"`
						Time   time.Time `json:"time"`
					} `json:"header"`
				} `json:"block"`
			} `json:"value"`
		}
		if err := json.Unmarshal(result.Data, &event); err != nil {
			return ConsensusSnapshot{}, false
		}
		height, err := strconv.ParseInt(event.Value.Block.Header.Height, 10, 64)
		if err != nil || height <= 0 || event.Value.Block.Header.Time.IsZero() {
			return ConsensusSnapshot{}, false
		}
		return ConsensusSnapshot{
			Block: &CommittedBlock{
				Height: height,
				Time:   event.Value.Block.Header.Time,
			},
		}, true
	}

	return ConsensusSnapshot{}, false
}

func (t *consensusTracker) handleVoteEvent(data json.RawMessage) (ConsensusSnapshot, bool) {
	var evt struct {
		Type  string `json:"type"`
		Value struct {
			Vote struct {
				Type           int    `json:"type"` // 1=prevote, 2=precommit
				Height         string `json:"height"`
				Round          int    `json:"round"`
				ValidatorIndex int    `json:"validator_index"`
			} `json:"Vote"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		return ConsensusSnapshot{}, false
	}

	vote := evt.Value.Vote
	height, err := strconv.ParseInt(vote.Height, 10, 64)
	if err != nil || height < 0 || vote.Round < 0 || vote.ValidatorIndex < 0 || vote.ValidatorIndex >= len(t.validators) || (vote.Type != 1 && vote.Type != 2) {
		return ConsensusSnapshot{}, false
	}

	// A vote can be the first forward-progress event observed after reconnect.
	// advanceTo accepts only monotonic height/round movement, so a newer vote
	// may advance the model while a delayed older vote cannot rewind it.
	if !t.advanceTo(height, vote.Round) {
		return ConsensusSnapshot{}, false
	}

	power := t.validatorPowers[vote.ValidatorIndex]

	switch vote.Type {
	case 1: // Prevote
		if !t.prevoted[vote.ValidatorIndex] {
			t.prevoted[vote.ValidatorIndex] = true
			t.pvPower += power
		}
	case 2: // Precommit
		if !t.precommited[vote.ValidatorIndex] {
			t.precommited[vote.ValidatorIndex] = true
			t.pcPower += power
		}
	}

	return ConsensusSnapshot{State: t.buildState()}, true
}

func (t *consensusTracker) handleNewRoundStepEvent(data json.RawMessage) (ConsensusSnapshot, bool) {
	var evt struct {
		Type  string `json:"type"`
		Value struct {
			Height   string `json:"height"`
			Round    int32  `json:"round"`
			Step     string `json:"step"`
			Proposer *struct {
				Address string `json:"address"`
				Index   int    `json:"index"`
			} `json:"proposer"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		return ConsensusSnapshot{}, false
	}

	height, err := strconv.ParseInt(evt.Value.Height, 10, 64)
	if err != nil || height < 0 || evt.Value.Round < 0 {
		return ConsensusSnapshot{}, false
	}

	round := int(evt.Value.Round)
	step := 0
	for candidate := cstypes.RoundStepNewHeight; candidate <= cstypes.RoundStepCommit; candidate++ {
		if candidate.String() == evt.Value.Step {
			step = int(candidate)
			break
		}
	}
	if step == 0 {
		return ConsensusSnapshot{}, false
	}
	proposer := evt.Value.Proposer
	if proposer != nil && (proposer.Index < 0 || proposer.Index >= len(t.validators) ||
		!strings.EqualFold(t.validators[proposer.Index].Address, proposer.Address)) {
		return ConsensusSnapshot{}, false
	}
	if t.initialized && height == t.height && round == t.round && step < t.step {
		if proposer == nil {
			return ConsensusSnapshot{}, false
		}
		// Separate subscriptions can deliver NewRound after a later step. Its
		// proposer is still valid for this round, but its step must not rewind it.
		step = t.step
	}
	if !t.advanceTo(height, round) {
		return ConsensusSnapshot{}, false
	}

	t.step = step
	if proposer != nil {
		t.proposerAddr = proposer.Address
		t.proposerIndex = proposer.Index
	}

	return ConsensusSnapshot{State: t.buildState()}, true
}

func (t *consensusTracker) advanceTo(height int64, round int) bool {
	if t.initialized {
		if height < t.height || (height == t.height && round < t.round) {
			return false
		}
		if height == t.height && round == t.round {
			return true
		}
	}

	t.initialized = true
	t.height = height
	t.resetForRound(round)

	return true
}

func (t *consensusTracker) resetForRound(round int) {
	t.round = round
	t.step = 0
	t.proposerAddr = ""
	t.proposerIndex = -1
	t.startTime = time.Now()
	t.prevoted = make(map[int]bool)
	t.precommited = make(map[int]bool)
	t.pvPower = 0
	t.pcPower = 0
}

// seed merges the node's existing votes before queued events are replayed.
// Percentages use integer power, not the rounded fractions in bit-array text.
func (t *consensusTracker) seed(state *consensus.State) {
	t.advanceTo(state.Height, state.Round)
	t.step = state.Step
	t.proposerAddr = state.ProposerAddress
	t.proposerIndex = state.ProposerIndex
	for _, validator := range state.Validators {
		if validator.Prevoted {
			t.prevoted[validator.Index] = true
			t.pvPower += validator.VotingPower
		}
		if validator.Precommited {
			t.precommited[validator.Index] = true
			t.pcPower += validator.VotingPower
		}
	}
}

func (t *consensusTracker) buildState() *consensus.State {
	var pvPct, pcPct float64
	if t.totalPower > 0 {
		pvPct = float64(t.pvPower) / float64(t.totalPower)
		pcPct = float64(t.pcPower) / float64(t.totalPower)
	}

	vals := make([]consensus.ValidatorStatus, len(t.validators))
	for i, v := range t.validators {
		power, _ := strconv.ParseInt(v.VotingPower, 10, 64)
		vals[i] = consensus.ValidatorStatus{
			Index:       i,
			Address:     v.Address,
			PubKey:      v.PubKey.Value,
			VotingPower: power,
			Prevoted:    t.prevoted[i],
			Precommited: t.precommited[i],
			IsProposer:  i == t.proposerIndex,
		}
	}

	var bitArray strings.Builder
	for i := range t.validators {
		if t.prevoted[i] {
			bitArray.WriteByte('x')
		} else {
			bitArray.WriteByte('_')
		}
	}

	return &consensus.State{
		Height:           t.height,
		Round:            t.round,
		Step:             t.step,
		StartTime:        t.startTime,
		Elapsed:          time.Since(t.startTime),
		ProposerAddress:  t.proposerAddr,
		ProposerIndex:    t.proposerIndex,
		TotalValidators:  len(t.validators),
		TotalVotingPower: t.totalPower,
		PrevoteCount:     len(t.prevoted),
		PrevotePower:     t.pvPower,
		PrevotePercent:   pvPct,
		PrevoteBitArray:  bitArray.String(),
		PrecommitCount:   len(t.precommited),
		PrecommitPower:   t.pcPower,
		PrecommitPercent: pcPct,
		Validators:       vals,
	}
}
