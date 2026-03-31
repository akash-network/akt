package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	cmtclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"

	"pkg.akt.dev/akt/internal/top/consensus"
)

// ConsensusSnapshot is a real-time consensus state built from WebSocket
// Vote events.
type ConsensusSnapshot struct {
	State *consensus.State
}

// SubscribeConsensusState connects to the RPC endpoint over WebSocket,
// subscribes to Vote and NewRoundStep events, and locally tracks
// prevote/precommit power to produce real-time consensus snapshots.
//
// The returned channel receives a snapshot after every vote and step
// transition. It is closed when the connection drops or ctx is cancelled.
func (c *Client) SubscribeConsensusState(ctx context.Context) (<-chan ConsensusSnapshot, error) {
	ws, err := cmtclient.NewWS(c.rpcEndpoint, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("create websocket client: %w", err)
	}

	if err := ws.Start(); err != nil {
		return nil, fmt.Errorf("websocket connect to %s: %w", c.rpcEndpoint, err)
	}

	// Fetch the validator set (cached after first call).
	validators, err := c.GetValidators()
	if err != nil {
		ws.Stop() //nolint:errcheck
		return nil, fmt.Errorf("load validators: %w", err)
	}

	// Subscribe to Vote and NewRoundStep events.
	subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
	defer subCancel()

	if err := ws.Subscribe(subCtx, "tm.event='Vote'"); err != nil {
		ws.Stop() //nolint:errcheck
		return nil, fmt.Errorf("subscribe to Vote events: %w", err)
	}
	if err := ws.Subscribe(subCtx, "tm.event='NewRoundStep'"); err != nil {
		ws.Stop() //nolint:errcheck
		return nil, fmt.Errorf("subscribe to NewRoundStep events: %w", err)
	}

	ch := make(chan ConsensusSnapshot, 16)
	tracker := newConsensusTracker(validators)

	go func() {
		defer close(ch)
		defer ws.Stop() //nolint:errcheck

		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-ws.ResponsesCh:
				if !ok {
					return
				}
				if len(resp.Result) == 0 {
					continue
				}

				snap, changed := tracker.handleMessage(resp.Result)
				if !changed {
					continue
				}

				select {
				case ch <- snap:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// consensusTracker accumulates vote events to produce real-time state.
type consensusTracker struct {
	validators      []consensus.Validator
	validatorPowers map[int]int64 // validator_index -> voting_power
	totalPower      int64

	height    int64
	round     int
	step      int
	startTime time.Time

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
	case strings.Contains(result.Query, "NewRoundStep"):
		return t.handleNewRoundStepEvent(result.Data)
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
	height, _ := strconv.ParseInt(vote.Height, 10, 64)

	if height != t.height {
		t.resetForHeight(height)
	}

	if vote.Round != t.round {
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
	default:
		return ConsensusSnapshot{}, false
	}

	return ConsensusSnapshot{State: t.buildState()}, true
}

func (t *consensusTracker) handleNewRoundStepEvent(data json.RawMessage) (ConsensusSnapshot, bool) {
	var evt struct {
		Type  string `json:"type"`
		Value struct {
			Height string `json:"height"`
			Round  int32  `json:"round"`
			Step   int32  `json:"step"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		return ConsensusSnapshot{}, false
	}

	height, _ := strconv.ParseInt(evt.Value.Height, 10, 64)

	if height != t.height {
		t.resetForHeight(height)
	}

	t.round = int(evt.Value.Round)
	t.step = int(evt.Value.Step)

	return ConsensusSnapshot{State: t.buildState()}, true
}

func (t *consensusTracker) resetForHeight(height int64) {
	t.height = height
	t.round = 0
	t.step = 0
	t.startTime = time.Now()
	t.prevoted = make(map[int]bool)
	t.precommited = make(map[int]bool)
	t.pvPower = 0
	t.pcPower = 0
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
