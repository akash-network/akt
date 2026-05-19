package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProposals(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/gov/v1/proposals" {
			http.NotFound(w, r)
			return
		}
		resp := proposalsResponse{
			Proposals: []Proposal{
				{ID: "42", Title: "Upgrade v0.30", Status: "PROPOSAL_STATUS_PASSED"},
				{ID: "41", Title: "Param change", Status: "PROPOSAL_STATUS_VOTING_PERIOD"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	proposals, err := client.GetProposals(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(proposals))
	}
	if proposals[0].ID != "42" {
		t.Errorf("expected first proposal ID 42, got %s", proposals[0].ID)
	}
	if proposals[1].Title != "Param change" {
		t.Errorf("expected second proposal title 'Param change', got %s", proposals[1].Title)
	}
}

func TestGetProposals_Empty(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := proposalsResponse{}
		json.NewEncoder(w).Encode(resp)
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	proposals, err := client.GetProposals(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("expected 0 proposals, got %d", len(proposals))
	}
}

func TestGetProposals_Pagination(t *testing.T) {
	callCount := 0
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Query().Get("pagination.key") == "" {
			// First page: return next_key
			resp := proposalsResponse{
				Proposals: []Proposal{
					{ID: "42", Title: "First page"},
				},
			}
			resp.Pagination.NextKey = "page2key"
			json.NewEncoder(w).Encode(resp)
		} else {
			// Second page: no next_key
			resp := proposalsResponse{
				Proposals: []Proposal{
					{ID: "41", Title: "Second page"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	proposals, err := client.GetProposals(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals across pages, got %d", len(proposals))
	}
	if proposals[0].ID != "42" {
		t.Errorf("expected first proposal ID 42, got %s", proposals[0].ID)
	}
	if proposals[1].ID != "41" {
		t.Errorf("expected second proposal ID 41, got %s", proposals[1].ID)
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", callCount)
	}
}

func TestGetProposals_Error(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	_, err := client.GetProposals(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestGetProposalTally(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cosmos/gov/v1/proposals/42/tally" {
			http.NotFound(w, r)
			return
		}
		resp := tallyResponse{
			Tally: TallyResult{
				YesCount:        "1000000",
				NoCount:         "500000",
				AbstainCount:    "200000",
				NoWithVetoCount: "100000",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	tally, err := client.GetProposalTally(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tally.YesCount != "1000000" {
		t.Errorf("expected yes_count 1000000, got %s", tally.YesCount)
	}
	if tally.NoCount != "500000" {
		t.Errorf("expected no_count 500000, got %s", tally.NoCount)
	}
	if tally.AbstainCount != "200000" {
		t.Errorf("expected abstain_count 200000, got %s", tally.AbstainCount)
	}
	if tally.NoWithVetoCount != "100000" {
		t.Errorf("expected no_with_veto_count 100000, got %s", tally.NoWithVetoCount)
	}
}

func TestGetProposalTally_Error(t *testing.T) {
	restServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer restServer.Close()

	client := NewClient("http://unused", restServer.URL)
	_, err := client.GetProposalTally(context.Background(), "999")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}
