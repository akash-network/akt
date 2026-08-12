package rpc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/output/pretty"
)

func TestGetGovernanceProposalsIncludesRecentActiveAndLiveTallies(t *testing.T) {
	votingEnd := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	requestsByStatus := make(map[govv1.ProposalStatus]int)
	tallyCalls := 0

	writeResponse := func(t *testing.T, w http.ResponseWriter, response interface{ Marshal() ([]byte, error) }) {
		t.Helper()
		encoded, err := response.Marshal()
		if err != nil {
			t.Fatalf("marshal ABCI response: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
			base64.StdEncoding.EncodeToString(encoded))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/abci_query" {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Query().Get("path")
		data, err := hex.DecodeString(strings.TrimPrefix(r.URL.Query().Get("data"), "0x"))
		if err != nil {
			t.Errorf("decode request data: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch {
		case strings.Contains(path, "/cosmos.gov.v1.Query/Proposals"):
			var req govv1.QueryProposalsRequest
			if err := req.Unmarshal(data); err != nil {
				t.Errorf("decode proposals request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			requestsByStatus[req.ProposalStatus]++

			var proposals []*govv1.Proposal
			switch req.ProposalStatus {
			case govv1.StatusNil:
				if req.Pagination == nil || req.Pagination.Limit != 20 || !req.Pagination.Reverse {
					t.Errorf("recent proposal pagination = %#v, want limit 20 reverse", req.Pagination)
				}
				proposals = []*govv1.Proposal{
					{Id: 10, Title: "Voting", Status: govv1.StatusVotingPeriod, VotingEndTime: &votingEnd},
					{Id: 8, Title: "Passed", Status: govv1.StatusPassed, FinalTallyResult: &govv1.TallyResult{YesCount: "99", NoCount: "1"}},
				}
			case govv1.StatusVotingPeriod:
				proposals = []*govv1.Proposal{{Id: 10, Title: "Voting", Status: govv1.StatusVotingPeriod, VotingEndTime: &votingEnd}}
			case govv1.StatusDepositPeriod:
				proposals = []*govv1.Proposal{{Id: 9, Title: "Deposit", Status: govv1.StatusDepositPeriod}}
			default:
				t.Errorf("unexpected proposal status %s", req.ProposalStatus)
			}
			writeResponse(t, w, &govv1.QueryProposalsResponse{Proposals: proposals})

		case strings.Contains(path, "/cosmos.gov.v1.Query/TallyResult"):
			var req govv1.QueryTallyResultRequest
			if err := req.Unmarshal(data); err != nil {
				t.Errorf("decode tally request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			tallyCalls++
			if req.ProposalId != 10 {
				t.Errorf("tally proposal ID = %d, want 10", req.ProposalId)
			}
			writeResponse(t, w, &govv1.QueryTallyResultResponse{Tally: &govv1.TallyResult{
				YesCount: "70", NoCount: "20", AbstainCount: "5", NoWithVetoCount: "5",
			}})

		default:
			t.Errorf("unexpected ABCI path %q", path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.URL).GetGovernanceProposals(context.Background())
	if err != nil {
		t.Fatalf("GetGovernanceProposals: %v", err)
	}
	if len(got.Proposals) != 3 {
		t.Fatalf("proposal count = %d, want 3 de-duplicated proposals", len(got.Proposals))
	}
	for i, wantID := range []uint64{10, 9, 8} {
		if got.Proposals[i].Id != wantID {
			t.Errorf("proposal[%d].id = %d, want %d", i, got.Proposals[i].Id, wantID)
		}
	}
	if got.Proposals[0].FinalTallyResult == nil || got.Proposals[0].FinalTallyResult.YesCount != "70" {
		t.Errorf("live voting tally = %#v, want yes_count 70", got.Proposals[0].FinalTallyResult)
	}
	for _, status := range []govv1.ProposalStatus{govv1.StatusNil, govv1.StatusVotingPeriod, govv1.StatusDepositPeriod} {
		if requestsByStatus[status] != 1 {
			t.Errorf("proposal requests for %s = %d, want 1", status, requestsByStatus[status])
		}
	}
	if tallyCalls != 1 {
		t.Errorf("live tally calls = %d, want 1", tallyCalls)
	}
}

func TestGetAllGovernanceParamsUsesCompleteRPCResponse(t *testing.T) {
	votingPeriod := 7 * 24 * time.Hour
	depositPeriod := 14 * 24 * time.Hour
	response := govv1.QueryParamsResponse{Params: &govv1.Params{
		VotingPeriod:     &votingPeriod,
		MinDeposit:       sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000_000_000)),
		MaxDepositPeriod: &depositPeriod,
		Quorum:           "0.334000000000000000",
		Threshold:        "0.500000000000000000",
		VetoThreshold:    "0.334000000000000000",
		BurnVoteVeto:     true,
	}}
	encoded, err := response.Marshal()
	if err != nil {
		t.Fatalf("marshal governance response: %v", err)
	}

	abciCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/abci_query" {
			http.NotFound(w, r)
			return
		}
		abciCalls++
		if got := r.URL.Query().Get("path"); !strings.Contains(got, "/cosmos.gov.v1.Query/Params") {
			t.Errorf("ABCI path = %q, want governance v1 params", got)
		}
		if got := r.URL.Query().Get("data"); got != "" {
			t.Errorf("ABCI request data = %q, want empty params_type so the chain returns all governance params", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
			base64.StdEncoding.EncodeToString(encoded))
	}))
	defer srv.Close()

	params, err := NewClient(srv.URL, srv.URL).GetAllGovernanceParams(context.Background())
	if err != nil {
		t.Fatalf("GetAllGovernanceParams: %v", err)
	}
	if abciCalls != 1 {
		t.Fatalf("governance ABCI calls = %d, want 1", abciCalls)
	}

	govParams := params.Modules["gov"]
	if govParams == nil || govParams.Error != nil {
		t.Fatalf("governance params = %#v, want complete data", govParams)
	}
	var decoded struct {
		Params struct {
			VotingPeriod string `json:"voting_period"`
			MinDeposit   []struct {
				Denom  string `json:"denom"`
				Amount string `json:"amount"`
			} `json:"min_deposit"`
			Quorum        string `json:"quorum"`
			Threshold     string `json:"threshold"`
			VetoThreshold string `json:"veto_threshold"`
			BurnVoteVeto  bool   `json:"burn_vote_veto"`
		} `json:"params"`
	}
	if err := json.Unmarshal(govParams.RawJSON, &decoded); err != nil {
		t.Fatalf("decode normalized governance JSON: %v", err)
	}
	if decoded.Params.VotingPeriod != "604800s" {
		t.Errorf("voting period = %q, want 604800s", decoded.Params.VotingPeriod)
	}
	if len(decoded.Params.MinDeposit) != 1 ||
		decoded.Params.MinDeposit[0].Amount != "1000000000" {
		t.Errorf("minimum deposit = %#v, want 1000000000 uakt", decoded.Params.MinDeposit)
	}
	if decoded.Params.Quorum != response.Params.Quorum ||
		decoded.Params.Threshold != response.Params.Threshold ||
		decoded.Params.VetoThreshold != response.Params.VetoThreshold {
		t.Errorf("tally params = %#v, want live non-zero values", decoded.Params)
	}
	if !decoded.Params.BurnVoteVeto {
		t.Error("burn_vote_veto = false, want true")
	}

	rendered := pretty.RenderModuleParamsFromJSON("gov", govParams.RawJSON)
	for _, want := range []string{"7 days", "1000 AKT", "33.4%"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered governance params missing %q:\n%s", want, rendered)
		}
	}
}

func TestCompareVersionsOrdersProviderReleasesAndCandidates(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "leading v", left: "v0.14.0", right: "v0.9.0", want: 1},
		{name: "release after candidate", left: "v0.14.0", right: "v0.14.0-rc1", want: 1},
		{name: "candidate before release", left: "v0.14.0-rc1", right: "v0.14.0", want: -1},
		{name: "candidate number", left: "v0.14.0-rc10", right: "v0.14.0-rc2", want: 1},
		{name: "equal", left: "v0.14.0", right: "v0.14.0", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompareVersions(tc.left, tc.right); got != tc.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
			}
		})
	}
}
