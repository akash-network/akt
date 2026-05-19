package data

import (
	tea "charm.land/bubbletea/v2"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

// Service defines the contract for data loading that views depend on.
// Each method returns a tea.Cmd that produces the corresponding message
// from the messages package when executed by the bubbletea runtime.
type Service interface {
	LoadDeployments(owner string) tea.Cmd
	LoadLeases(owner string) tea.Cmd
	LoadDeploymentLeases(owner string, dseq uint64) tea.Cmd
	LoadBids(owner string, dseq uint64) tea.Cmd
	LoadProviders() tea.Cmd
	LoadProposals() tea.Cmd
	LoadTallies(proposals []*govv1.Proposal) tea.Cmd
	LoadValidators() tea.Cmd
	LoadStakingPool() tea.Cmd
	LoadBalance(account string) tea.Cmd
	LoadStoreStats() tea.Cmd
	LoadSyncState() tea.Cmd
}
