package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Proposal represents a governance proposal from the Cosmos REST API.
type Proposal struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	Summary          string       `json:"summary"`
	Status           string       `json:"status"`
	VotingStartTime  time.Time    `json:"voting_start_time"`
	VotingEndTime    time.Time    `json:"voting_end_time"`
	SubmitTime       time.Time    `json:"submit_time"`
	DepositEndTime   time.Time    `json:"deposit_end_time"`
	FinalTallyResult *TallyResult `json:"final_tally_result"`
}

// TallyResult represents the vote tally for a governance proposal.
type TallyResult struct {
	YesCount        string `json:"yes_count"`
	NoCount         string `json:"no_count"`
	AbstainCount    string `json:"abstain_count"`
	NoWithVetoCount string `json:"no_with_veto_count"`
}

// proposalsResponse represents the REST API response for governance proposals.
type proposalsResponse struct {
	Proposals  []Proposal `json:"proposals"`
	Pagination struct {
		NextKey string `json:"next_key"`
		Total   string `json:"total"`
	} `json:"pagination"`
}

// tallyResponse represents the REST API response for a proposal tally.
type tallyResponse struct {
	Tally TallyResult `json:"tally"`
}

// GetProposals fetches governance proposals from the REST endpoint, newest first.
// It handles pagination automatically, following next_key until all pages are retrieved.
func (c *Client) GetProposals(ctx context.Context) ([]Proposal, error) {
	var all []Proposal
	nextKey := ""

	for {
		proposals, newNextKey, err := c.fetchProposalsPage(ctx, nextKey)
		if err != nil {
			return nil, err
		}

		all = append(all, proposals...)

		if newNextKey == "" {
			break
		}
		nextKey = newNextKey
	}

	return all, nil
}

func (c *Client) fetchProposalsPage(ctx context.Context, nextKey string) ([]Proposal, string, error) {
	reqURL := fmt.Sprintf("%s/cosmos/gov/v1/proposals?pagination.limit=100&pagination.reverse=true", c.restEndpoint)
	if nextKey != "" {
		reqURL += "&pagination.key=" + url.QueryEscape(nextKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create proposals request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch proposals: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("proposals returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read proposals response: %w", err)
	}

	var result proposalsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse proposals: %w", err)
	}

	return result.Proposals, result.Pagination.NextKey, nil
}

// GetProposalTally fetches the live tally for a specific governance proposal.
// This is useful for proposals currently in the voting period.
func (c *Client) GetProposalTally(ctx context.Context, proposalID string) (*TallyResult, error) {
	reqURL := fmt.Sprintf("%s/cosmos/gov/v1/proposals/%s/tally", c.restEndpoint, proposalID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create tally request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch proposal tally: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proposal tally returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tally response: %w", err)
	}

	var result tallyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tally: %w", err)
	}

	return &result.Tally, nil
}
