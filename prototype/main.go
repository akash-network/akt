package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"akash-tui-v2/theme"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ────────────────────────────────────────────────────────────────────────────────
// View IDs
// ────────────────────────────────────────────────────────────────────────────────

type viewID int

const (
	// Landing
	viewDashboard viewID = iota

	// Primary nav (1-6)
	viewDeployments
	viewLeases
	viewProviders
	viewMonitorHub // entry into monitor; resolves to sub-view via monitorTab
	viewGovernance
	viewStaking

	// Secondary (drill-down from primary)
	viewDeploymentDetail
	viewLeaseDetail
	viewProviderDetail
	viewProposalDetail
	viewValidatorDetail

	// Monitor sub-views (tertiary, cycled via Tab inside Monitor Hub)
	viewMonitorNetwork
	viewMonitorProvider
	viewMonitorOracleBME

	// Deploy workflow
	viewDeployWorkflow
)

// primaryNavItems maps number keys to views (1=Home, 2-7=categories)
var primaryNavItems = []struct {
	key  string
	name string
	view viewID
}{
	{"1", "Home", viewDashboard},
	{"2", "Deployments", viewDeployments},
	{"3", "Leases", viewLeases},
	{"4", "Providers", viewProviders},
	{"5", "Monitor", viewMonitorHub},
	{"6", "Governance", viewGovernance},
	{"7", "Staking", viewStaking},
}

// ────────────────────────────────────────────────────────────────────────────────
// Data types
// ────────────────────────────────────────────────────────────────────────────────

type deployment struct {
	dseq      int
	state     string
	provider  string
	price     string
	escrow    string
	age       string
	image     string
	cpu       string
	memory    string
	gpu       string
	owner     string
	created   int
	deposit   string
	endpoints []endpoint
}

type endpoint struct {
	name string
	url  string
}

type lease struct {
	dseq     int
	gseq     int
	oseq     int
	state    string
	provider string
	price    string
	age      string
	image    string

	// Order info (Artur feedback)
	orderID    string
	bidID      string
	orderState string
	createdH   int // block height created

	// Settlement info (Artur feedback)
	lastSettlement   string // time ago
	settledAmount    string
	fundsRemaining   string
	withdrawnAmount  string
	settlementHeight int
}

type providerEntry struct {
	address string
	url     string
	version string
	cpu     string
	memory  string
	gpu     string
	region  string
	active  int
	uptime  string
}

type proposal struct {
	id       int
	title    string
	propType string
	status   string
	yes      float64
	no       float64
	abstain  float64
	veto     float64
	endTime  string
}

type validator struct {
	rank      int
	moniker   string
	address   string
	tokens    string
	comission string
	status    string
	uptime    string
}

type versionDist struct {
	version string
	count   int
	pct     float64
}

type paletteCommand struct {
	name string
	desc string
	view viewID
}

// ────────────────────────────────────────────────────────────────────────────────
// Mock data
// ────────────────────────────────────────────────────────────────────────────────

var mockDeployments = []deployment{
	{dseq: 18542, state: "active", provider: "akash1q7spv...m6rx6", price: "12.5 uakt", escrow: "4.2 AKT", age: "2d 14h", image: "nginx:latest", cpu: "0.5", memory: "512Mi", gpu: "", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18200000, deposit: "5 AKT", endpoints: []endpoint{{name: "web", url: "http://abc123.provider1.akash.network"}, {name: "api", url: "http://def456.provider1.akash.network:8080"}}},
	{dseq: 18539, state: "active", provider: "akash1f5k9...p3wn8", price: "45.0 uakt", escrow: "12.8 AKT", age: "3d 2h", image: "llama-3:70b", cpu: "8", memory: "64Gi", gpu: "1x A100", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18197000, deposit: "20 AKT", endpoints: []endpoint{{name: "inference", url: "http://ghi789.provider2.akash.network:8000"}}},
	{dseq: 18535, state: "insufficient_funds", provider: "akash1q7spv...m6rx6", price: "8.2 uakt", escrow: "0.1 AKT", age: "5d 8h", image: "postgres:16", cpu: "1", memory: "2Gi", gpu: "", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18190000, deposit: "3 AKT", endpoints: []endpoint{{name: "db", url: "tcp://jkl012.provider1.akash.network:5432"}}},
	{dseq: 18520, state: "active", provider: "akash1h8g2...k4mn9", price: "120.0 uakt", escrow: "85.0 AKT", age: "7d 0h", image: "stable-diffusion:xl", cpu: "4", memory: "32Gi", gpu: "2x H100", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18180000, deposit: "100 AKT", endpoints: []endpoint{{name: "web", url: "http://mno345.provider3.akash.network"}}},
	{dseq: 18501, state: "closed", provider: "akash1f5k9...p3wn8", price: "15.0 uakt", escrow: "0 AKT", age: "12d 3h", image: "redis:7", cpu: "0.25", memory: "256Mi", gpu: "", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18160000, deposit: "5 AKT", endpoints: nil},
	{dseq: 18490, state: "closed", provider: "akash1q7spv...m6rx6", price: "22.0 uakt", escrow: "0 AKT", age: "15d 6h", image: "grafana:latest", cpu: "0.5", memory: "1Gi", gpu: "", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18150000, deposit: "8 AKT", endpoints: nil},
	{dseq: 18475, state: "paused", provider: "akash1h8g2...k4mn9", price: "30.0 uakt", escrow: "2.1 AKT", age: "18d 1h", image: "jupyter:scipy", cpu: "2", memory: "8Gi", gpu: "1x T4", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18140000, deposit: "10 AKT", endpoints: []endpoint{{name: "notebook", url: "http://pqr678.provider3.akash.network:8888"}}},
	{dseq: 18460, state: "active", provider: "akash1m3xr...j7tn2", price: "55.0 uakt", escrow: "42.0 AKT", age: "21d 5h", image: "vllm:0.4", cpu: "16", memory: "128Gi", gpu: "4x A100", owner: "akash1abcdefghijklmnopqrstuvwxyz012345678901", created: 18120000, deposit: "60 AKT", endpoints: []endpoint{{name: "api", url: "http://stu901.provider4.akash.network:8000"}}},
}

var mockLeases = []lease{
	{dseq: 18542, gseq: 1, oseq: 1, state: "active", provider: "akash1q7spv...m6rx6", price: "12.5 uakt", age: "2d 14h", image: "nginx:latest",
		orderID: "18542/1/1", bidID: "18542/1/1/akash1q7spv", orderState: "matched", createdH: 18200000,
		lastSettlement: "12m ago", settledAmount: "1,250 uakt", fundsRemaining: "3.95 AKT", withdrawnAmount: "0.25 AKT", settlementHeight: 18234200},
	{dseq: 18539, gseq: 1, oseq: 1, state: "active", provider: "akash1f5k9...p3wn8", price: "45.0 uakt", age: "3d 2h", image: "llama-3:70b",
		orderID: "18539/1/1", bidID: "18539/1/1/akash1f5k9", orderState: "matched", createdH: 18197000,
		lastSettlement: "8m ago", settledAmount: "4,500 uakt", fundsRemaining: "10.2 AKT", withdrawnAmount: "2.6 AKT", settlementHeight: 18234300},
	{dseq: 18535, gseq: 1, oseq: 1, state: "active", provider: "akash1q7spv...m6rx6", price: "8.2 uakt", age: "5d 8h", image: "postgres:16",
		orderID: "18535/1/1", bidID: "18535/1/1/akash1q7spv", orderState: "matched", createdH: 18190000,
		lastSettlement: "15m ago", settledAmount: "820 uakt", fundsRemaining: "0.08 AKT", withdrawnAmount: "2.92 AKT", settlementHeight: 18234100},
	{dseq: 18520, gseq: 1, oseq: 1, state: "active", provider: "akash1h8g2...k4mn9", price: "120.0 uakt", age: "7d 0h", image: "stable-diffusion:xl",
		orderID: "18520/1/1", bidID: "18520/1/1/akash1h8g2", orderState: "matched", createdH: 18180000,
		lastSettlement: "5m ago", settledAmount: "12,000 uakt", fundsRemaining: "78.5 AKT", withdrawnAmount: "6.5 AKT", settlementHeight: 18234400},
	{dseq: 18520, gseq: 2, oseq: 1, state: "active", provider: "akash1m3xr...j7tn2", price: "55.0 uakt", age: "7d 0h", image: "redis:7-sidecar",
		orderID: "18520/2/1", bidID: "18520/2/1/akash1m3xr", orderState: "matched", createdH: 18180000,
		lastSettlement: "5m ago", settledAmount: "5,500 uakt", fundsRemaining: "38.2 AKT", withdrawnAmount: "3.8 AKT", settlementHeight: 18234400},
	{dseq: 18501, gseq: 1, oseq: 1, state: "closed", provider: "akash1f5k9...p3wn8", price: "15.0 uakt", age: "12d 3h", image: "redis:7",
		orderID: "18501/1/1", bidID: "18501/1/1/akash1f5k9", orderState: "closed", createdH: 18160000,
		lastSettlement: "12d ago", settledAmount: "0 uakt", fundsRemaining: "0 AKT", withdrawnAmount: "5.0 AKT", settlementHeight: 18200000},
	{dseq: 18475, gseq: 1, oseq: 1, state: "active", provider: "akash1h8g2...k4mn9", price: "30.0 uakt", age: "18d 1h", image: "jupyter:scipy",
		orderID: "18475/1/1", bidID: "18475/1/1/akash1h8g2", orderState: "matched", createdH: 18140000,
		lastSettlement: "10m ago", settledAmount: "3,000 uakt", fundsRemaining: "1.8 AKT", withdrawnAmount: "0.3 AKT", settlementHeight: 18234350},
	{dseq: 18460, gseq: 1, oseq: 1, state: "active", provider: "akash1m3xr...j7tn2", price: "55.0 uakt", age: "21d 5h", image: "vllm:0.4",
		orderID: "18460/1/1", bidID: "18460/1/1/akash1m3xr", orderState: "matched", createdH: 18120000,
		lastSettlement: "3m ago", settledAmount: "5,500 uakt", fundsRemaining: "35.8 AKT", withdrawnAmount: "6.2 AKT", settlementHeight: 18234500},
}

var mockProviderList = []providerEntry{
	{address: "akash1q7spv...m6rx6", url: "provider1.akash.network", version: "0.6.4", cpu: "42/64", memory: "128/256Gi", gpu: "4 H100", region: "US-East", active: 156, uptime: "99.8%"},
	{address: "akash1f5k9...p3wn8", url: "provider2.akash.network", version: "0.6.4", cpu: "18/32", memory: "64/128Gi", gpu: "-", region: "EU-West", active: 82, uptime: "99.9%"},
	{address: "akash1h8g2...k4mn9", url: "provider3.example.com", version: "0.6.4", cpu: "120/256", memory: "512/1024Gi", gpu: "8 A100", region: "US-West", active: 312, uptime: "99.5%"},
	{address: "akash1m3xr...j7tn2", url: "provider4.akash.network", version: "0.6.4", cpu: "8/16", memory: "32/64Gi", gpu: "2 T4", region: "AP-SE", active: 45, uptime: "98.2%"},
	{address: "akash1p8nw...r2kx5", url: "provider5.cloud.io", version: "0.6.3", cpu: "64/128", memory: "256/512Gi", gpu: "4 A100", region: "US-Central", active: 201, uptime: "99.7%"},
	{address: "akash1t4js...w9pl3", url: "provider6.akash.network", version: "0.6.3", cpu: "24/48", memory: "96/192Gi", gpu: "-", region: "EU-Central", active: 67, uptime: "99.4%"},
	{address: "akash1v7mq...n3hd8", url: "provider7.example.net", version: "0.6.2", cpu: "16/32", memory: "64/128Gi", gpu: "1 T4", region: "AP-NE", active: 23, uptime: "97.1%"},
	{address: "akash1x2bf...s5gy4", url: "provider8.akash.network", version: "0.6.4", cpu: "32/64", memory: "128/256Gi", gpu: "2 H100", region: "US-East", active: 178, uptime: "99.6%"},
}

var mockProposals = []proposal{
	{id: 268, title: "Upgrade to v0.38.0", propType: "Software Upgrade", status: "active", yes: 72.4, no: 3.1, abstain: 24.5, veto: 0.0, endTime: "in 2d 8h"},
	{id: 267, title: "Community Pool Spend: Marketing Q2", propType: "Community Pool Spend", status: "active", yes: 58.9, no: 12.3, abstain: 26.8, veto: 2.0, endTime: "in 4d 12h"},
	{id: 266, title: "Parameter Change: Max Validators 120", propType: "Parameter Change", status: "passed", yes: 91.2, no: 2.1, abstain: 6.7, veto: 0.0, endTime: "3d ago"},
	{id: 265, title: "IBC Client Update: Osmosis", propType: "Client Update", status: "passed", yes: 88.7, no: 1.2, abstain: 10.1, veto: 0.0, endTime: "7d ago"},
	{id: 264, title: "Increase Deployment Deposit Min", propType: "Parameter Change", status: "rejected", yes: 32.1, no: 45.6, abstain: 18.3, veto: 4.0, endTime: "12d ago"},
	{id: 263, title: "Oracle Module Activation", propType: "Software Upgrade", status: "passed", yes: 95.3, no: 0.8, abstain: 3.9, veto: 0.0, endTime: "18d ago"},
}

var mockValidators = []validator{
	{rank: 1, moniker: "Forbole", address: "akashvaloper1...abc", tokens: "12.4M AKT", comission: "5%", status: "bonded", uptime: "100%"},
	{rank: 2, moniker: "Cosmostation", address: "akashvaloper1...def", tokens: "10.8M AKT", comission: "10%", status: "bonded", uptime: "99.9%"},
	{rank: 3, moniker: "Witval", address: "akashvaloper1...ghi", tokens: "8.2M AKT", comission: "5%", status: "bonded", uptime: "99.8%"},
	{rank: 4, moniker: "Stakefish", address: "akashvaloper1...jkl", tokens: "7.6M AKT", comission: "8%", status: "bonded", uptime: "100%"},
	{rank: 5, moniker: "Figment", address: "akashvaloper1...mno", tokens: "6.1M AKT", comission: "10%", status: "bonded", uptime: "99.7%"},
	{rank: 6, moniker: "Chorus One", address: "akashvaloper1...pqr", tokens: "5.5M AKT", comission: "7.5%", status: "bonded", uptime: "99.9%"},
	{rank: 7, moniker: "Imperator.co", address: "akashvaloper1...stu", tokens: "4.9M AKT", comission: "5%", status: "bonded", uptime: "99.6%"},
	{rank: 8, moniker: "Everstake", address: "akashvaloper1...vwx", tokens: "3.2M AKT", comission: "8%", status: "bonded", uptime: "99.5%"},
	{rank: 9, moniker: "InfStones", address: "akashvaloper1...yza", tokens: "2.1M AKT", comission: "10%", status: "unbonding", uptime: "98.1%"},
	{rank: 10, moniker: "Moonlet", address: "akashvaloper1...bcd", tokens: "1.8M AKT", comission: "5%", status: "bonded", uptime: "99.3%"},
}

var mockVersions = []versionDist{
	{version: "0.6.4", count: 62, pct: 63.3},
	{version: "0.6.3", count: 18, pct: 18.4},
	{version: "0.6.2", count: 10, pct: 10.2},
}

var paletteCommands = []paletteCommand{
	{name: "Dashboard", desc: "Home / overview", view: viewDashboard},
	{name: "Deployments", desc: "View all deployments", view: viewDeployments},
	{name: "Leases", desc: "View all leases", view: viewLeases},
	{name: "Providers", desc: "Browse provider list", view: viewProviders},
	{name: "Monitor", desc: "Real-time network monitor", view: viewMonitorHub},
	{name: "Governance", desc: "Governance proposals", view: viewGovernance},
	{name: "Staking", desc: "Validators & delegations", view: viewStaking},
	// Deploy workflow removed per Artur — being replaced by generic `akt workflow` system
	// {name: "Deploy", desc: "Create new deployment", view: viewDeployWorkflow},
	{name: "Quit", desc: "Quit application", view: viewDashboard}, // sentinel
}

// ────────────────────────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────────────────────────

type model struct {
	width  int
	height int

	// Navigation stack
	viewStack []viewID

	// Overlays
	paletteOpen bool
	confirmOpen bool
	helpOpen    bool
	voteOpen    bool
	voteCursor  int // 0=Yes,1=No,2=Abstain,3=NoWithVeto
	delegateOpen   bool
	delegateAction int // 0=delegate, 1=redelegate, 2=undelegate
	delegateStep   int // 0=amount input, 1=confirm

	// Deployments list
	deplCursor  int
	filterInput textinput.Model
	filterOpen  bool
	stateFilter string // "", "active", "closed"

	// Leases list
	leaseCursor  int
	leaseFilter  string

	// Providers list
	provListCursor int

	// Monitor hub
	monitorTab    int // 0=Network, 1=Provider, 2=Oracle/BME
	networkSubTab int // 0=Overview, 1=Validators, 2=Governance

	// Monitor provider sub-view cursors
	monProvCursor  int
	versionCursor  int

	// Governance
	govCursor int

	// Staking
	stakeCursor int

	// Command palette
	paletteInput  textinput.Model
	paletteCursor int

	// Confirm dialog
	confirmTarget int
	confirmCursor int // 0=cancel, 1=confirm

	// Deploy workflow
	deployStep   int // 0-6 for the 7 steps
	deployBidCur int // cursor within bid selection (step 2)

	// Log viewer
	logViewerOpen bool
	logLines      []string
	logScroll     int

	// Spinner
	spinner spinner.Model
}

func initialModel() model {
	fi := textinput.New()
	fi.Placeholder = "Filter..."
	fi.CharLimit = 40
	fi.SetWidth(30)

	pi := textinput.New()
	pi.Placeholder = ""
	pi.CharLimit = 60
	pi.SetWidth(40)

	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	return model{
		viewStack:     []viewID{viewDashboard},
		deplCursor:    0,
		filterInput:   fi,
		stateFilter:   "",
		monitorTab:    0,
		networkSubTab: 0,
		paletteInput:  pi,
		paletteCursor: 0,
		confirmCursor: 0,
		spinner:       s,
		width:         120,
		height:        40,
	}
}

func (m *model) currentView() viewID {
	if len(m.viewStack) == 0 {
		return viewDashboard
	}
	return m.viewStack[len(m.viewStack)-1]
}

func (m *model) pushView(v viewID) {
	m.viewStack = append(m.viewStack, v)
}

func (m *model) popView() {
	if len(m.viewStack) > 1 {
		m.viewStack = m.viewStack[:len(m.viewStack)-1]
	}
}

// switchPrimary replaces the entire stack with Dashboard + the target primary view
func (m *model) switchPrimary(v viewID) {
	m.viewStack = []viewID{viewDashboard, v}
}

// isPrimaryView returns true if the view is one of the primary nav destinations
func isPrimaryView(v viewID) bool {
	switch v {
	case viewDashboard, viewDeployments, viewLeases, viewProviders,
		viewMonitorHub, viewMonitorNetwork, viewMonitorProvider, viewMonitorOracleBME,
		viewGovernance, viewStaking:
		return true
	}
	return false
}

// activePrimaryIndex returns which primary nav item is active (-1 for dashboard)
func (m model) activePrimaryIndex() int {
	cur := m.currentView()
	// Walk the stack to find the primary-level view
	for i := len(m.viewStack) - 1; i >= 0; i-- {
		v := m.viewStack[i]
		for pi, nav := range primaryNavItems {
			if v == nav.view {
				return pi
			}
		}
		// Monitor sub-views map to Monitor Hub
		if v == viewMonitorNetwork || v == viewMonitorProvider || v == viewMonitorOracleBME {
			return 4 // Monitor is index 4 (1=Home,2=Depl,3=Leases,4=Prov,5=Monitor...)
		}
		// Dashboard maps to Home
		if v == viewDashboard {
			return 0 // Home is index 0
		}
	}
	_ = cur
	return -1 // Dashboard
}

// ────────────────────────────────────────────────────────────────────────────────
// Bubbletea lifecycle
// ────────────────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		key := msg.String()

		// ctrl+c always quits
		if key == "ctrl+c" {
			return m, tea.Quit
		}

		// Handle overlays first
		if m.helpOpen {
			if key == "?" || key == "esc" || key == "q" {
				m.helpOpen = false
			}
			return m, nil
		}
		if m.paletteOpen {
			return m.updatePalette(msg)
		}
		if m.confirmOpen {
			return m.updateConfirm(msg)
		}
		if m.voteOpen {
			return m.updateVoteDialog(msg)
		}
		if m.delegateOpen {
			return m.updateDelegateDialog(msg)
		}
		if m.logViewerOpen {
			return m.updateLogViewer(msg)
		}

		// Global keys (when no overlay and not in a text input)
		if !m.filterOpen {
			switch key {
			case "?":
				m.helpOpen = true
				return m, nil
			case ":", "ctrl+p":
				m.paletteOpen = true
				m.paletteCursor = 0
				m.paletteInput.SetValue("")
				cmd := m.paletteInput.Focus()
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			case "1":
				m.viewStack = []viewID{viewDashboard}
				return m, nil
			case "2":
				m.switchPrimary(viewDeployments)
				return m, nil
			case "3":
				m.switchPrimary(viewLeases)
				return m, nil
			case "4":
				m.switchPrimary(viewProviders)
				return m, nil
			case "5":
				m.switchPrimary(viewMonitorHub)
				m.monitorTab = 0
				m.networkSubTab = 0
				// Resolve to actual monitor sub-view
				m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
				return m, nil
			case "6":
				m.switchPrimary(viewGovernance)
				return m, nil
			case "7":
				m.switchPrimary(viewStaking)
				return m, nil
			case "q":
				if m.currentView() == viewDashboard {
					return m, tea.Quit
				}
				// q on non-dashboard goes home
				m.viewStack = []viewID{viewDashboard}
				return m, nil
			}
		}

		// View-specific keys
		switch m.currentView() {
		case viewDashboard:
			return m.updateDashboard(msg)
		case viewDeployments:
			return m.updateDeployments(msg)
		case viewDeploymentDetail:
			return m.updateDeploymentDetail(msg)
		case viewLeases:
			return m.updateLeases(msg)
		case viewLeaseDetail:
			return m.updateLeaseDetail(msg)
		case viewProviders:
			return m.updateProviderList(msg)
		case viewProviderDetail:
			return m.updateProviderDetail(msg)
		case viewMonitorNetwork, viewMonitorOracleBME:
			return m.updateMonitorNetwork(msg)
		case viewMonitorProvider:
			return m.updateMonitorProviderView(msg)
		case viewGovernance:
			return m.updateGovernance(msg)
		case viewProposalDetail:
			return m.updateProposalDetail(msg)
		case viewStaking:
			return m.updateStaking(msg)
		case viewValidatorDetail:
			return m.updateValidatorDetail(msg)
		// case viewDeployWorkflow:
		//	return m.updateDeployWorkflow(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Dashboard
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Deployments
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateDeployments(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.filterOpen {
		switch key {
		case "esc":
			m.filterOpen = false
			m.filterInput.Blur()
			m.filterInput.SetValue("")
			m.deplCursor = 0
			return m, nil
		case "enter":
			m.filterOpen = false
			m.filterInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.deplCursor = 0
			return m, cmd
		}
	}

	switch key {
	case "j", "down":
		filtered := m.filteredDeployments()
		if m.deplCursor < len(filtered)-1 {
			m.deplCursor++
		}
	case "k", "up":
		if m.deplCursor > 0 {
			m.deplCursor--
		}
	case "/":
		m.filterOpen = true
		m.filterInput.SetValue("")
		return m, m.filterInput.Focus()
	case "f":
		switch m.stateFilter {
		case "":
			m.stateFilter = "active"
		case "active":
			m.stateFilter = "closed"
		case "closed":
			m.stateFilter = ""
		}
		m.deplCursor = 0
	case "enter":
		filtered := m.filteredDeployments()
		if len(filtered) > 0 {
			m.pushView(viewDeploymentDetail)
		}
		return m, nil
	case "d":
		filtered := m.filteredDeployments()
		if len(filtered) > 0 && m.deplCursor < len(filtered) {
			m.confirmOpen = true
			m.confirmTarget = m.deplCursor
			m.confirmCursor = 0
		}
		return m, nil
	case "esc":
		if m.stateFilter != "" {
			m.stateFilter = ""
			m.deplCursor = 0
		} else {
			m.popView()
		}
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Deployment Detail
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateDeploymentDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	case "d":
		m.confirmOpen = true
		m.confirmTarget = m.deplCursor
		m.confirmCursor = 0
	case "l":
		m.logViewerOpen = true
		m.logScroll = 0
		m.logLines = mockLogLines()
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Leases
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateLeases(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.leaseCursor < len(mockLeases)-1 {
			m.leaseCursor++
		}
	case "k", "up":
		if m.leaseCursor > 0 {
			m.leaseCursor--
		}
	case "f":
		switch m.leaseFilter {
		case "":
			m.leaseFilter = "active"
		case "active":
			m.leaseFilter = "closed"
		case "closed":
			m.leaseFilter = ""
		}
		m.leaseCursor = 0
	case "enter":
		if len(mockLeases) > 0 {
			m.pushView(viewLeaseDetail)
		}
	case "esc":
		if m.leaseFilter != "" {
			m.leaseFilter = ""
			m.leaseCursor = 0
		} else {
			m.popView()
		}
	}
	return m, nil
}

func (m model) updateLeaseDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Providers
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateProviderList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.provListCursor < len(mockProviderList)-1 {
			m.provListCursor++
		}
	case "k", "up":
		if m.provListCursor > 0 {
			m.provListCursor--
		}
	case "enter":
		if len(mockProviderList) > 0 {
			m.pushView(viewProviderDetail)
		}
	case "esc":
		m.popView()
	}
	return m, nil
}

func (m model) updateProviderDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Monitor Network
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateMonitorNetwork(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	case "tab":
		m.monitorTab = (m.monitorTab + 1) % 3
		switch m.monitorTab {
		case 0:
			m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
		case 1:
			m.viewStack[len(m.viewStack)-1] = viewMonitorProvider
		case 2:
			m.viewStack[len(m.viewStack)-1] = viewMonitorOracleBME
		}
	case "shift+tab":
		m.monitorTab = (m.monitorTab + 2) % 3 // reverse cycle
		switch m.monitorTab {
		case 0:
			m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
		case 1:
			m.viewStack[len(m.viewStack)-1] = viewMonitorProvider
		case 2:
			m.viewStack[len(m.viewStack)-1] = viewMonitorOracleBME
		}
	}

	// Network sub-tabs (a/s/g — number keys conflict with global nav 1-7)
	if m.currentView() == viewMonitorNetwork {
		switch msg.String() {
		case "a":
			m.networkSubTab = 0
		case "s":
			m.networkSubTab = 1
		case "g":
			m.networkSubTab = 2
		}
	}
	return m, nil
}

func (m model) updateMonitorProviderView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	case "tab":
		m.monitorTab = (m.monitorTab + 1) % 3
		switch m.monitorTab {
		case 0:
			m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
		case 1:
			m.viewStack[len(m.viewStack)-1] = viewMonitorProvider
		case 2:
			m.viewStack[len(m.viewStack)-1] = viewMonitorOracleBME
		}
	case "shift+tab":
		m.monitorTab = (m.monitorTab + 2) % 3
		switch m.monitorTab {
		case 0:
			m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
		case 1:
			m.viewStack[len(m.viewStack)-1] = viewMonitorProvider
		case 2:
			m.viewStack[len(m.viewStack)-1] = viewMonitorOracleBME
		}
	case "j", "down":
		if m.monProvCursor < len(mockProviderList)-1 {
			m.monProvCursor++
		}
	case "k", "up":
		if m.monProvCursor > 0 {
			m.monProvCursor--
		}
	case "h":
		if m.versionCursor > 0 {
			m.versionCursor--
		}
	case "l":
		if m.versionCursor < len(mockVersions)-1 {
			m.versionCursor++
		}
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Governance
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateGovernance(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.govCursor < len(mockProposals)-1 {
			m.govCursor++
		}
	case "k", "up":
		if m.govCursor > 0 {
			m.govCursor--
		}
	case "enter":
		if len(mockProposals) > 0 {
			m.pushView(viewProposalDetail)
		}
	case "esc":
		m.popView()
	}
	return m, nil
}

func (m model) updateProposalDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	case "v":
		m.voteOpen = true
		m.voteCursor = 0
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Staking
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateStaking(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.stakeCursor < len(mockValidators)-1 {
			m.stakeCursor++
		}
	case "k", "up":
		if m.stakeCursor > 0 {
			m.stakeCursor--
		}
	case "enter":
		if len(mockValidators) > 0 {
			m.pushView(viewValidatorDetail)
		}
	case "esc":
		m.popView()
	}
	return m, nil
}

func (m model) updateValidatorDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
	case "d", "D":
		m.delegateOpen = true
		m.delegateAction = 0
		m.delegateStep = 0
	case "r", "R":
		m.delegateOpen = true
		m.delegateAction = 1
		m.delegateStep = 0
	case "u", "U":
		m.delegateOpen = true
		m.delegateAction = 2
		m.delegateStep = 0
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Command Palette
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.paletteOpen = false
		m.paletteInput.Blur()
		return m, nil
	case "enter":
		filtered := m.filteredPaletteCommands()
		if len(filtered) > 0 && m.paletteCursor < len(filtered) {
			cmd := filtered[m.paletteCursor]
			m.paletteOpen = false
			m.paletteInput.Blur()
			if cmd.name == "Quit" {
				return m, tea.Quit
			}
			if cmd.view == viewMonitorHub {
				m.switchPrimary(viewMonitorHub)
				m.monitorTab = 0
				m.networkSubTab = 0
				m.viewStack[len(m.viewStack)-1] = viewMonitorNetwork
			} else if cmd.view == viewDashboard {
				m.viewStack = []viewID{viewDashboard}
			} else {
				m.switchPrimary(cmd.view)
			}
		}
		return m, nil
	case "j", "down":
		filtered := m.filteredPaletteCommands()
		if m.paletteCursor < len(filtered)-1 {
			m.paletteCursor++
		}
		return m, nil
	case "k", "up":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		m.paletteCursor = 0
		return m, cmd
	}
}

// ────────────────────────────────────────────────────────────────────────────────
// Update: Confirm Dialog
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.confirmOpen = false
		return m, nil
	case "tab":
		m.confirmCursor = (m.confirmCursor + 1) % 2
		return m, nil
	case "enter":
		m.confirmOpen = false
		return m, nil
	}
	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Filters
// ────────────────────────────────────────────────────────────────────────────────

func (m model) filteredDeployments() []deployment {
	var out []deployment
	q := strings.ToLower(m.filterInput.Value())
	for _, d := range mockDeployments {
		if m.stateFilter != "" && d.state != m.stateFilter {
			continue
		}
		if q != "" {
			dseqStr := fmt.Sprintf("%d", d.dseq)
			if !strings.Contains(strings.ToLower(d.image), q) &&
				!strings.Contains(dseqStr, q) &&
				!strings.Contains(strings.ToLower(d.provider), q) &&
				!strings.Contains(strings.ToLower(d.state), q) {
				continue
			}
		}
		out = append(out, d)
	}
	if out == nil {
		return []deployment{}
	}
	return out
}

func (m model) filteredLeases() []lease {
	var out []lease
	for _, l := range mockLeases {
		if m.leaseFilter != "" && l.state != m.leaseFilter {
			continue
		}
		out = append(out, l)
	}
	if out == nil {
		return []lease{}
	}
	return out
}

func (m model) filteredPaletteCommands() []paletteCommand {
	q := strings.ToLower(m.paletteInput.Value())
	if q == "" {
		return paletteCommands
	}
	var out []paletteCommand
	for _, c := range paletteCommands {
		if strings.Contains(strings.ToLower(c.name), q) ||
			strings.Contains(strings.ToLower(c.desc), q) {
			out = append(out, c)
		}
	}
	return out
}

// ────────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────────

func commaGroup(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func hintPair(key, desc string) string {
	return theme.FooterKey.Render(key) + " " + theme.FooterDesc.Render(desc) + "  "
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ────────────────────────────────────────────────────────────────────────────────
// View (top-level)
// ────────────────────────────────────────────────────────────────────────────────

func (m model) View() tea.View {
	w := m.width
	if w < 40 {
		w = 40
	}

	header := m.renderHeader(w)
	navBar := m.renderPrimaryNav(w)
	var content string

	switch m.currentView() {
	case viewDashboard:
		content = m.renderDashboard(w)
	case viewDeployments:
		content = m.renderDeploymentList(w)
	case viewDeploymentDetail:
		content = m.renderDeploymentDetail(w)
	case viewLeases:
		content = m.renderLeaseList(w)
	case viewLeaseDetail:
		content = m.renderLeaseDetail(w)
	case viewProviders:
		content = m.renderProviderListView(w)
	case viewProviderDetail:
		content = m.renderProviderDetailView(w)
	case viewMonitorNetwork:
		content = m.renderMonitorNetwork(w)
	case viewMonitorProvider:
		content = m.renderMonitorProviderFleet(w)
	case viewMonitorOracleBME:
		content = m.renderMonitorOracleBME(w)
	case viewGovernance:
		content = m.renderGovernance(w)
	case viewProposalDetail:
		content = m.renderProposalDetail(w)
	case viewStaking:
		content = m.renderStaking(w)
	case viewValidatorDetail:
		content = m.renderValidatorDetailView(w)
	// Deploy workflow disabled — pending akt workflow system definition
	// case viewDeployWorkflow:
	//	content = m.renderDeployWorkflow(w)
	}

	footer := m.renderFooter(w)

	// Breadcrumb sits inside the content area, rendered as first line of content zone
	breadcrumb := m.renderBreadcrumb()

	// Calculate vertical padding
	headerH := strings.Count(header, "\n") + 1
	navH := strings.Count(navBar, "\n") + 1
	contentLines := strings.Count(content, "\n") + 1
	breadcrumbH := 1
	footerH := strings.Count(footer, "\n") + 1
	usedH := headerH + navH + breadcrumbH + contentLines + footerH + 2 // +2 for blank lines
	padH := m.height - usedH
	if padH < 0 {
		padH = 0
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		navBar,
		breadcrumb,
		content,
		strings.Repeat("\n", padH),
		footer,
	)

	if m.helpOpen {
		body = m.overlayHelp(body, w)
	}
	if m.paletteOpen {
		body = m.overlayPalette(body, w)
	}
	if m.confirmOpen {
		body = m.overlayConfirm(body, w)
	}
	if m.voteOpen {
		body = m.overlayVote(body, w)
	}
	if m.delegateOpen {
		body = m.overlayDelegate(body, w)
	}
	if m.logViewerOpen {
		body = m.overlayLogViewer(body, w)
	}

	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

// ────────────────────────────────────────────────────────────────────────────────
// Header Bar
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderHeader(w int) string {
	appName := theme.HeaderAppName.Render("akt")

	sep := theme.HeaderMeta.Render(" · ")

	ctx := theme.HeaderContext.Render("prod") +
		theme.HeaderMeta.Render(":akashnet-2")

	acct := theme.HeaderContext.Render("alice") +
		theme.HeaderMeta.Render(" akash1abc…def")

	block := theme.HeaderMeta.Render("⎡ ") +
		theme.HeaderValue.Render(commaGroup(18234567)) +
		theme.HeaderMeta.Render(" ⎤")

	sync := theme.SyncOK.Render("● synced")

	left := appName + sep + ctx + sep + acct
	right := block + "  " + sync

	innerW := w - 2
	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return theme.HeaderStyle.Width(w).Render(
		left + strings.Repeat(" ", gap) + right,
	)
}

// ────────────────────────────────────────────────────────────────────────────────
// Primary Navigation Bar
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderPrimaryNav(w int) string {
	active := m.activePrimaryIndex()

	var parts []string
	for i, nav := range primaryNavItems {
		label := nav.key + " " + nav.name
		if i == active {
			parts = append(parts, theme.NavTabActive.Render(label))
		} else {
			parts = append(parts, theme.NavTabInactive.Render(label))
		}
	}

	bar := " " + strings.Join(parts, " ")

	rule := lipgloss.NewStyle().Foreground(theme.Slate700).Render(
		strings.Repeat("─", w))

	return bar + "\n" + rule
}

// ────────────────────────────────────────────────────────────────────────────────
// Breadcrumb
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderBreadcrumb() string {
	sep := theme.BreadcrumbSeparator.Render(" / ")

	var segments []string

	// Skip Dashboard in breadcrumb if it's the only item — it's redundant with nav
	startIdx := 0
	if len(m.viewStack) > 1 && m.viewStack[0] == viewDashboard {
		startIdx = 1
	}

	for i := startIdx; i < len(m.viewStack); i++ {
		v := m.viewStack[i]
		isLast := i == len(m.viewStack)-1
		var name string

		switch v {
		case viewDashboard:
			name = "Home"
		case viewDeployments:
			name = "Deployments"
		case viewDeploymentDetail:
			filtered := m.filteredDeployments()
			if m.deplCursor < len(filtered) {
				name = fmt.Sprintf("#%d", filtered[m.deplCursor].dseq)
			} else {
				name = "Detail"
			}
		case viewLeases:
			name = "Leases"
		case viewLeaseDetail:
			fl := m.filteredLeases()
			if m.leaseCursor < len(fl) {
				l := fl[m.leaseCursor]
				name = fmt.Sprintf("#%d/%d/%d", l.dseq, l.gseq, l.oseq)
			} else {
				name = "Detail"
			}
		case viewProviders:
			name = "Providers"
		case viewProviderDetail:
			if m.provListCursor < len(mockProviderList) {
				name = mockProviderList[m.provListCursor].url
			} else {
				name = "Detail"
			}
		case viewMonitorHub, viewMonitorNetwork:
			if isLast && v == viewMonitorNetwork {
				subNames := []string{"Overview", "Validators", "Governance"}
				sub := m.networkSubTab
				if sub >= len(subNames) {
					sub = 0
				}
				segments = append(segments, theme.BreadcrumbSegment.Render("Monitor"))
				segments = append(segments, theme.BreadcrumbSegment.Render("Network"))
				segments = append(segments, theme.BreadcrumbActive.Render(subNames[sub]))
				return " " + theme.BreadcrumbSeparator.Render(" ") + strings.Join(segments, sep)
			}
			name = "Monitor"
		case viewMonitorProvider:
			if isLast {
				segments = append(segments, theme.BreadcrumbSegment.Render("Monitor"))
				segments = append(segments, theme.BreadcrumbActive.Render("Provider"))
				return " " + theme.BreadcrumbSeparator.Render(" ") + strings.Join(segments, sep)
			}
			name = "Monitor"
		case viewMonitorOracleBME:
			if isLast {
				segments = append(segments, theme.BreadcrumbSegment.Render("Monitor"))
				segments = append(segments, theme.BreadcrumbActive.Render("Oracle/BME"))
				return " " + theme.BreadcrumbSeparator.Render(" ") + strings.Join(segments, sep)
			}
			name = "Monitor"
		case viewGovernance:
			name = "Governance"
		case viewProposalDetail:
			if m.govCursor < len(mockProposals) {
				name = fmt.Sprintf("#%d", mockProposals[m.govCursor].id)
			} else {
				name = "Proposal"
			}
		case viewStaking:
			name = "Staking"
		case viewDeployWorkflow:
			if isLast {
				segments = append(segments, theme.BreadcrumbSegment.Render("Deploy"))
				segments = append(segments, theme.BreadcrumbActive.Render(deployStepName(m.deployStep)))
				return " " + theme.BreadcrumbSeparator.Render(" ") + strings.Join(segments, sep)
			}
			name = "Deploy"
		case viewValidatorDetail:
			if m.stakeCursor < len(mockValidators) {
				name = mockValidators[m.stakeCursor].moniker
			} else {
				name = "Validator"
			}
		default:
			name = "?"
		}

		if isLast {
			segments = append(segments, theme.BreadcrumbActive.Render(name))
		} else {
			segments = append(segments, theme.BreadcrumbSegment.Render(name))
		}
	}

	if len(segments) == 0 {
		return ""
	}
	return " " + theme.BreadcrumbSeparator.Render(" ") + strings.Join(segments, sep)
}

// ────────────────────────────────────────────────────────────────────────────────
// Footer / Status Bar
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderFooter(w int) string {
	var hints string

	switch m.currentView() {
	case viewDashboard:
		hints = hintPair("1-7", "Navigate") + hintPair(":", "Command") + hintPair("?", "Help") + hintPair("q", "Quit")
	case viewDeployments:
		stateLabel := "all"
		if m.stateFilter != "" {
			stateLabel = m.stateFilter
		}
		hints = hintPair("j/k", "Navigate") + hintPair("Enter", "Detail") +
			hintPair("/", "Filter") + hintPair("f", "State:"+stateLabel) +
			hintPair("d", "Close") + hintPair(":", "Command")
	case viewDeploymentDetail:
		hints = hintPair("Esc", "Back") + hintPair("l", "Logs") +
			hintPair("s", "Shell") + hintPair("u", "Update") +
			hintPair("d", "Close") + hintPair("y", "YAML")
	case viewLeases:
		stateLabel := "all"
		if m.leaseFilter != "" {
			stateLabel = m.leaseFilter
		}
		hints = hintPair("j/k", "Navigate") + hintPair("Enter", "Detail") +
			hintPair("f", "State:"+stateLabel) + hintPair("Esc", "Back")
	case viewLeaseDetail:
		hints = hintPair("Esc", "Back") + hintPair("l", "Logs") +
			hintPair("s", "Shell") + hintPair("e", "Events")
	case viewProviders:
		hints = hintPair("j/k", "Navigate") + hintPair("Enter", "Detail") +
			hintPair("Esc", "Back")
	case viewProviderDetail:
		hints = hintPair("Esc", "Back")
	case viewMonitorNetwork:
		hints = hintPair("Tab", "Next Dashboard") + hintPair("S-Tab", "Prev Dashboard") +
			hintPair("a", "Overview") + hintPair("s", "Validators") +
			hintPair("g", "Governance") + hintPair("Esc", "Back")
	case viewMonitorProvider:
		hints = hintPair("Tab", "Next Dashboard") + hintPair("S-Tab", "Prev Dashboard") +
			hintPair("j/k", "Scroll") + hintPair("h/l", "Version") +
			hintPair("Esc", "Back")
	case viewMonitorOracleBME:
		hints = hintPair("Tab", "Next Dashboard") + hintPair("S-Tab", "Prev Dashboard") +
			hintPair("Esc", "Back")
	case viewGovernance:
		hints = hintPair("j/k", "Navigate") + hintPair("Enter", "Detail") +
			hintPair("v", "Vote") + hintPair("Esc", "Back")
	case viewProposalDetail:
		hints = hintPair("Esc", "Back") + hintPair("v", "Vote")
	case viewStaking:
		hints = hintPair("j/k", "Navigate") + hintPair("Enter", "Detail") +
			hintPair("d", "Delegate") + hintPair("Esc", "Back")
	case viewValidatorDetail:
		hints = hintPair("Esc", "Back") + hintPair("d", "Delegate") +
			hintPair("r", "Redelegate") + hintPair("u", "Undelegate")
	case viewDeployWorkflow:
		if m.deployStep == 2 {
			hints = hintPair("j/k", "Select bid") + hintPair("Enter", "Accept bid") +
				hintPair("Esc", "Cancel")
		} else if m.deployStep == 6 {
			hints = hintPair("Enter", "Go to Deployments") + hintPair("Esc", "Cancel")
		} else {
			hints = hintPair("Enter", "Next step") + hintPair("Esc", "Cancel")
		}
	}

	return theme.HRule(w) + "\n" + hints
}

// ────────────────────────────────────────────────────────────────────────────────
// Dashboard View (START HERE)
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderDashboard(w int) string {
	var lines []string

	// Account summary — inline KV row instead of cards (works at any width)
	activeCount := 0
	for _, d := range mockDeployments {
		if d.state == "active" {
			activeCount++
		}
	}
	activeLeases := 0
	for _, l := range mockLeases {
		if l.state == "active" {
			activeLeases++
		}
	}

	lines = append(lines, "")

	// Summary cards as bordered boxes
	cardW := (w - 6) / 3
	if cardW < 18 {
		cardW = 18
	}

	cardStyle := lipgloss.NewStyle().
		Width(cardW).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.Slate700).
		Padding(0, 1)

	balanceCard := cardStyle.Render(
		theme.KVKey.Render("Balance") + "\n" +
			theme.KVValueBold.Render("148.52 AKT") +
			theme.KVValueMuted.Render("  ≈ $742.60"))

	deplCard := cardStyle.Render(
		theme.KVKey.Render("Deployments") + "\n" +
			theme.KVValueBold.Render(fmt.Sprintf("%d", activeCount)) +
			theme.KVValue.Render(" active") +
			theme.KVValueMuted.Render(fmt.Sprintf("  %d total", len(mockDeployments))))

	leaseCard := cardStyle.Render(
		theme.KVKey.Render("Leases") + "\n" +
			theme.KVValueBold.Render(fmt.Sprintf("%d", activeLeases)) +
			theme.KVValue.Render(" active") +
			theme.KVValueMuted.Render("  187.7 uakt/blk"))

	cards := lipgloss.JoinHorizontal(lipgloss.Top, " "+balanceCard, " ", deplCard, " ", leaseCard)
	lines = append(lines, cards)
	lines = append(lines, "")

	// Recent Deployments
	lines = append(lines, " "+theme.SectionTitle.Render("Recent Deployments"))
	lines = append(lines, " "+theme.HRule(w-2))

	dashStateColW := 13
	lines = append(lines, "  "+
		col(theme.ColHeader, 7, "DSEQ")+
		col(theme.ColHeader, dashStateColW, "STATE")+
		col(theme.ColHeader, 20, "IMAGE")+
		col(theme.ColHeader, 12, "PRICE/BLK")+
		col(theme.ColHeader, 10, "ESCROW")+
		theme.ColHeader.Render("AGE"))

	limit := 5
	if limit > len(mockDeployments) {
		limit = len(mockDeployments)
	}
	for _, d := range mockDeployments[:limit] {
		tag := stateTag(d.state)
		tagW := stateTagWidth(d.state)
		tagPad := ""
		if tagW < dashStateColW {
			tagPad = strings.Repeat(" ", dashStateColW-tagW)
		}

		row := "  " +
			col(theme.ColBold, 7, fmt.Sprintf("%d", d.dseq)) +
			tag + tagPad +
			col(theme.Col, 20, d.image) +
			col(theme.Col, 12, d.price) +
			col(theme.Col, 10, d.escrow) +
			theme.ColMuted.Render(d.age)
		lines = append(lines, row)
	}

	lines = append(lines, "  "+theme.ColMuted.Render(
		fmt.Sprintf("Press 2 to see all %d deployments", len(mockDeployments))))
	lines = append(lines, "")

	// Network status strip
	lines = append(lines, " "+theme.SectionTitle.Render("Network"))
	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines,
		"  "+theme.KVKey.Width(8).Render("Chain")+theme.KVValue.Render("akashnet-2")+
			"   "+theme.KVKey.Width(8).Render("Height")+theme.KVValueBold.Render(commaGroup(18234567))+
			"   "+theme.KVKey.Width(12).Render("Validators")+theme.KVValue.Render("100 active")+
			"   "+theme.KVKey.Width(12).Render("Proposals")+theme.KVValue.Render("2 voting"),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Deployments List View
// ────────────────────────────────────────────────────────────────────────────────

// shortState returns a compact display label for deployment states
func shortState(state string) string {
	switch state {
	case "insufficient_funds":
		return "low funds"
	default:
		return state
	}
}

// stateTag renders a state label as an inline bordered tag using │text│ with colored border chars
func stateTag(state string) string {
	label := shortState(state)

	var border, text lipgloss.Style
	switch state {
	case "active", "open", "bonded", "passed", "valid":
		text = lipgloss.NewStyle().Foreground(theme.Green)
		border = lipgloss.NewStyle().Foreground(theme.GreenDim)
	case "paused", "insufficient_funds", "overdrawn", "unbonding":
		text = lipgloss.NewStyle().Foreground(theme.Yellow)
		border = lipgloss.NewStyle().Foreground(theme.YellowDim)
	case "closed", "lost", "unbonded", "rejected", "failed", "revoked":
		text = lipgloss.NewStyle().Foreground(theme.Slate500)
		border = lipgloss.NewStyle().Foreground(theme.Slate700)
	default:
		text = lipgloss.NewStyle().Foreground(theme.Slate500)
		border = lipgloss.NewStyle().Foreground(theme.Slate700)
	}

	return border.Render("│") + text.Render(label) + border.Render("│")
}

// stateTagWidth returns the display width of a state tag for column alignment
func stateTagWidth(state string) int {
	label := shortState(state)
	return len(label) + 2 // 2 for the border chars │ │
}

// col renders text into a fixed-width column using a lipgloss style (no padding).
func col(style lipgloss.Style, width int, text string) string {
	padded := fmt.Sprintf("%-*s", width, text)
	return style.Render(padded)
}

func (m model) renderDeploymentList(w int) string {
	deps := m.filteredDeployments()
	var lines []string

	// Filter bar
	if m.filterOpen {
		prompt := theme.PalettePrompt.Render("/") + " "
		lines = append(lines, " "+prompt+m.filterInput.View())
		lines = append(lines, "")
	} else if m.filterInput.Value() != "" {
		lines = append(lines, " "+theme.PalettePrompt.Render("/")+
			" "+theme.HeaderValue.Render(m.filterInput.Value())+
			"  "+theme.HeaderMeta.Render("(Esc to clear)"))
		lines = append(lines, "")
	}

	if m.stateFilter != "" {
		lines = append(lines, " "+theme.HeaderMeta.Render("Showing: ")+
			theme.StateBadge(m.stateFilter).Render(m.stateFilter))
		lines = append(lines, "")
	}

	// Table header
	stateColW := 13 // enough for │low funds│ + padding
	lines = append(lines, "  "+
		col(theme.ColHeader, 7, "DSEQ")+
		col(theme.ColHeader, stateColW, "STATE")+
		col(theme.ColHeader, 20, "IMAGE")+
		col(theme.ColHeader, 20, "PROVIDER")+
		col(theme.ColHeader, 12, "PRICE/BLK")+
		col(theme.ColHeader, 10, "ESCROW")+
		theme.ColHeader.Render("AGE"))
	lines = append(lines, " "+theme.HRule(w-2))

	cursor := m.deplCursor
	if cursor >= len(deps) {
		cursor = len(deps) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	for i, d := range deps {
		cur := "  "
		if i == cursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		// Render state as a bordered tag, then pad to fixed column width
		tag := stateTag(d.state)
		tagW := stateTagWidth(d.state)
		tagPad := ""
		if tagW < stateColW {
			tagPad = strings.Repeat(" ", stateColW-tagW)
		}
		stateCell := tag + tagPad

		var row string
		if i == cursor {
			row = cur +
				col(theme.ColBold, 7, fmt.Sprintf("%d", d.dseq)) +
				stateCell +
				col(theme.ColBold, 20, d.image) +
				col(theme.Col, 20, d.provider) +
				col(theme.ColBold, 12, d.price) +
				col(theme.ColBold, 10, d.escrow) +
				theme.ColBold.Render(d.age)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.ColBold, 7, fmt.Sprintf("%d", d.dseq)) +
				stateCell +
				col(theme.Col, 20, d.image) +
				col(theme.ColMuted, 20, d.provider) +
				col(theme.Col, 12, d.price) +
				col(theme.Col, 10, d.escrow) +
				theme.ColMuted.Render(d.age)
		}
		lines = append(lines, row)
	}

	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "  "+theme.ColMuted.Render(fmt.Sprintf("%d deployments", len(deps))))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Deployment Detail View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderDeploymentDetail(w int) string {
	filtered := m.filteredDeployments()
	idx := m.deplCursor
	if idx >= len(filtered) {
		idx = 0
	}
	if len(filtered) == 0 {
		return "  No deployment selected"
	}
	d := filtered[idx]

	var lines []string

	lines = append(lines, "  "+theme.SectionTitle.Render("Deployment"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("DSEQ")+theme.KVValueBold.Render(fmt.Sprintf("%d", d.dseq)),
		"    "+theme.KVKey.Render("Owner")+theme.KVValue.Render(d.owner),
		"    "+theme.KVKey.Render("State")+theme.StateBadge(d.state).Render(d.state),
		"    "+theme.KVKey.Render("Created")+theme.KVValue.Render(commaGroup(d.created)),
		"    "+theme.KVKey.Render("Deposit")+theme.KVValue.Render(d.deposit),
		"    "+theme.KVKey.Render("Escrow")+theme.KVValue.Render(d.escrow),
	)
	lines = append(lines, "")

	lines = append(lines, "  "+theme.SectionTitle.Render("Active Lease"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Provider")+theme.KVValue.Render(d.provider),
		"    "+theme.KVKey.Render("Price")+theme.KVValue.Render(d.price+"/block"),
	)
	if len(d.endpoints) > 0 {
		lines = append(lines, "    "+theme.KVKey.Render("Endpoints"))
		for _, ep := range d.endpoints {
			lines = append(lines,
				"      "+theme.KVKey.Width(10).Render(ep.name+":")+
					theme.KVValue.Render(ep.url),
			)
		}
	}
	lines = append(lines, "")

	lines = append(lines, "  "+theme.SectionTitle.Render("Groups"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	gpuStr := d.gpu
	if gpuStr == "" {
		gpuStr = "-"
	}
	lines = append(lines,
		"    "+theme.KVKey.Render("Group 1")+theme.KVValueMuted.Render("\""+d.image+"\""),
		"      "+theme.KVKey.Width(10).Render("CPU")+theme.KVValue.Render(d.cpu),
		"      "+theme.KVKey.Width(10).Render("Memory")+theme.KVValue.Render(d.memory),
		"      "+theme.KVKey.Width(10).Render("GPU")+theme.KVValue.Render(gpuStr),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Leases List View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderLeaseList(w int) string {
	leases := m.filteredLeases()
	var lines []string

	if m.leaseFilter != "" {
		lines = append(lines, " "+theme.HeaderMeta.Render("Showing: ")+
			theme.StateBadge(m.leaseFilter).Render(m.leaseFilter))
		lines = append(lines, "")
	}

	leaseStateColW := 10
	lines = append(lines, "  "+
		col(theme.ColHeader, 7, "DSEQ")+
		col(theme.ColHeader, 5, "G")+
		col(theme.ColHeader, 5, "O")+
		col(theme.ColHeader, leaseStateColW, "STATE")+
		col(theme.ColHeader, 20, "PROVIDER")+
		col(theme.ColHeader, 12, "PRICE/BLK")+
		theme.ColHeader.Render("AGE"))
	lines = append(lines, " "+theme.HRule(w-2))

	cursor := m.leaseCursor
	if cursor >= len(leases) {
		cursor = len(leases) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	for i, l := range leases {
		cur := "  "
		if i == cursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		tag := stateTag(l.state)
		tagW := stateTagWidth(l.state)
		tagPad := ""
		if tagW < leaseStateColW {
			tagPad = strings.Repeat(" ", leaseStateColW-tagW)
		}

		var row string
		if i == cursor {
			row = cur +
				col(theme.ColBold, 7, fmt.Sprintf("%d", l.dseq)) +
				col(theme.ColBold, 5, fmt.Sprintf("%d", l.gseq)) +
				col(theme.ColBold, 5, fmt.Sprintf("%d", l.oseq)) +
				tag + tagPad +
				col(theme.ColBold, 20, l.provider) +
				col(theme.ColBold, 12, l.price) +
				theme.ColBold.Render(l.age)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.ColBold, 7, fmt.Sprintf("%d", l.dseq)) +
				col(theme.Col, 5, fmt.Sprintf("%d", l.gseq)) +
				col(theme.Col, 5, fmt.Sprintf("%d", l.oseq)) +
				tag + tagPad +
				col(theme.ColMuted, 20, l.provider) +
				col(theme.Col, 12, l.price) +
				theme.ColMuted.Render(l.age)
		}
		lines = append(lines, row)
	}

	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "  "+theme.ColMuted.Render(fmt.Sprintf("%d leases", len(leases))))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Lease Detail View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderLeaseDetail(w int) string {
	leases := m.filteredLeases()
	idx := m.leaseCursor
	if idx >= len(leases) {
		idx = 0
	}
	if len(leases) == 0 {
		return "  No lease selected"
	}
	l := leases[idx]

	var lines []string

	// Lease overview
	lines = append(lines, "  "+theme.SectionTitle.Render("Lease"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("DSEQ")+theme.KVValueBold.Render(fmt.Sprintf("%d", l.dseq)),
		"    "+theme.KVKey.Render("GSEQ/OSEQ")+theme.KVValue.Render(fmt.Sprintf("%d/%d", l.gseq, l.oseq)),
		"    "+theme.KVKey.Render("State")+theme.StateBadge(l.state).Render(l.state),
		"    "+theme.KVKey.Render("Provider")+theme.KVValue.Render(l.provider),
		"    "+theme.KVKey.Render("Price")+theme.KVValue.Render(l.price+"/block"),
		"    "+theme.KVKey.Render("Age")+theme.KVValue.Render(l.age),
		"    "+theme.KVKey.Render("Image")+theme.KVValue.Render(l.image),
	)
	lines = append(lines, "")

	// Order info (per Artur's feedback)
	lines = append(lines, "  "+theme.SectionTitle.Render("Order"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Order ID")+theme.KVValueBold.Render(l.orderID),
		"    "+theme.KVKey.Render("Bid ID")+theme.KVValue.Render(l.bidID),
		"    "+theme.KVKey.Render("Order State")+theme.StateBadge(l.orderState).Render(l.orderState),
		"    "+theme.KVKey.Render("Created At")+theme.KVValue.Render("Block "+commaGroup(l.createdH)),
	)
	lines = append(lines, "")

	// Settlement info (per Artur's feedback)
	lines = append(lines, "  "+theme.SectionTitle.Render("Settlement"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Last Settled")+theme.KVValue.Render(l.lastSettlement)+
			theme.KVValueMuted.Render("  (block "+commaGroup(l.settlementHeight)+")"),
		"    "+theme.KVKey.Render("Settled Amt")+theme.KVValue.Render(l.settledAmount),
		"    "+theme.KVKey.Render("Funds Left")+theme.KVValueBold.Render(l.fundsRemaining),
		"    "+theme.KVKey.Render("Withdrawn")+theme.KVValue.Render(l.withdrawnAmount),
	)
	lines = append(lines, "")

	// Provider status
	lines = append(lines, "  "+theme.SectionTitle.Render("Provider Status"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Status")+theme.BadgeActive.Render("online"),
		"    "+theme.KVKey.Render("Forwarded Ports")+theme.KVValue.Render("80:30812, 443:30813"),
		"    "+theme.KVKey.Render("Available IPs")+theme.KVValue.Render("1"),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Providers List View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderProviderListView(w int) string {
	var lines []string

	lines = append(lines, "  "+
		col(theme.ColHeader, 26, "URL")+
		col(theme.ColHeader, 9, "VERSION")+
		col(theme.ColHeader, 10, "CPU")+
		col(theme.ColHeader, 12, "MEMORY")+
		col(theme.ColHeader, 9, "GPU")+
		col(theme.ColHeader, 7, "LEASES")+
		theme.ColHeader.Render("UPTIME"))
	lines = append(lines, " "+theme.HRule(w-2))

	cursor := m.provListCursor
	if cursor >= len(mockProviderList) {
		cursor = len(mockProviderList) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	for i, p := range mockProviderList {
		cur := "  "
		if i == cursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		var row string
		if i == cursor {
			row = cur +
				col(theme.ColBold, 26, p.url) +
				col(theme.ColBold, 9, p.version) +
				col(theme.ColBold, 10, p.cpu) +
				col(theme.ColBold, 12, p.memory) +
				col(theme.ColBold, 9, p.gpu) +
				col(theme.ColBold, 7, fmt.Sprintf("%d", p.active)) +
				theme.ColBold.Render(p.uptime)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.ColBold, 26, p.url) +
				col(theme.Col, 9, p.version) +
				col(theme.Col, 10, p.cpu) +
				col(theme.Col, 12, p.memory) +
				col(theme.Col, 9, p.gpu) +
				col(theme.Col, 7, fmt.Sprintf("%d", p.active)) +
				theme.ColMuted.Render(p.uptime)
		}
		lines = append(lines, row)
	}

	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "  "+theme.ColMuted.Render(fmt.Sprintf("%d providers", len(mockProviderList))))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Provider Detail View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderProviderDetailView(w int) string {
	idx := m.provListCursor
	if idx >= len(mockProviderList) {
		idx = 0
	}
	p := mockProviderList[idx]

	var lines []string

	lines = append(lines, "  "+theme.SectionTitle.Render("Provider"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Address")+theme.KVValue.Render(p.address),
		"    "+theme.KVKey.Render("URL")+theme.KVValueBold.Render(p.url),
		"    "+theme.KVKey.Render("Version")+theme.KVValue.Render(p.version),
		"    "+theme.KVKey.Render("Region")+theme.KVValue.Render(p.region),
		"    "+theme.KVKey.Render("Status")+theme.BadgeActive.Render("online"),
		"    "+theme.KVKey.Render("Active Leases")+theme.KVValue.Render(fmt.Sprintf("%d", p.active)),
		"    "+theme.KVKey.Render("Uptime")+theme.KVValue.Render(p.uptime),
	)
	lines = append(lines, "")

	lines = append(lines, "  "+theme.SectionTitle.Render("Resources"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	barW := w - 50
	if barW < 20 {
		barW = 20
	}
	// Parse CPU utilization from "42/64" format
	lines = append(lines, renderProgressBar("CPU", 65.6, p.cpu, barW))
	lines = append(lines, renderProgressBar("Memory", 50.0, p.memory, barW))
	lines = append(lines,
		"    "+theme.KVKey.Render("GPU")+theme.KVValue.Render(p.gpu),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Monitor Hub: Network Dashboard
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderMonitorNetwork(w int) string {
	var lines []string

	// Monitor tab bar
	lines = append(lines, "  "+m.renderMonitorTabs())
	lines = append(lines, "")

	// Network sub-tabs
	subTabs := []string{"Overview", "Validators", "Governance"}
	var subTabBar string
	for i, name := range subTabs {
		if i == m.networkSubTab {
			subTabBar += theme.TabActive.Render(name)
		} else {
			subTabBar += theme.TabInactive.Render(name)
		}
		if i < len(subTabs)-1 {
			subTabBar += " "
		}
	}
	lines = append(lines, "  "+subTabBar)
	lines = append(lines, "")

	// Consensus State
	lines = append(lines, "  "+theme.SectionTitle.Render("Consensus State"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	row1 := "    " +
		theme.KVKey.Render("Height") + theme.KVValueBold.Render(commaGroup(18234567)) +
		"    " +
		theme.KVKey.Render("Round") + theme.KVValue.Render("0")
	row2 := "    " +
		theme.KVKey.Render("Step") + theme.KVValue.Render("Precommit") +
		"    " +
		theme.KVKey.Render("Elapsed") + theme.KVValue.Render("1.2s")
	row3 := "    " +
		theme.KVKey.Render("Proposer") + theme.KVValue.Render("akash1abc...xyz") +
		theme.KVValueMuted.Render(" (idx 42)")

	lines = append(lines, row1, row2, row3)
	lines = append(lines, "")

	// Vote Progress
	lines = append(lines, "  "+theme.SectionTitle.Render("Vote Progress"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	barWidth := w - 50
	if barWidth < 20 {
		barWidth = 20
	}

	lines = append(lines, renderProgressBar("Prevotes", 67.2, "71.3M/106.1M", barWidth))
	lines = append(lines, renderProgressBar("Precommits", 82.1, "87.1M/106.1M", barWidth))
	lines = append(lines, "")

	// Validator Votes
	lines = append(lines, "  "+theme.SectionTitle.Render("Validator Votes"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	votedCount := 89
	notVotedCount := 11
	totalValidators := votedCount + notVotedCount

	dotsPerLine := w - 6
	if dotsPerLine < 20 {
		dotsPerLine = 20
	}

	var dots strings.Builder
	for i := 0; i < totalValidators; i++ {
		if i > 0 && i%dotsPerLine == 0 {
			dots.WriteString("\n    ")
		}
		if i < votedCount {
			dots.WriteString(lipgloss.NewStyle().Foreground(theme.Slate200).Render("●"))
		} else {
			dots.WriteString(lipgloss.NewStyle().Foreground(theme.Slate600).Render("○"))
		}
	}

	lines = append(lines, "    "+dots.String())
	lines = append(lines, "")
	legend := "    " +
		lipgloss.NewStyle().Foreground(theme.Slate200).Render("●") +
		theme.HeaderMeta.Render(fmt.Sprintf(" voted (%d)", votedCount)) + "  " +
		lipgloss.NewStyle().Foreground(theme.Slate600).Render("○") +
		theme.HeaderMeta.Render(fmt.Sprintf(" not voted (%d)", notVotedCount))
	lines = append(lines, legend)

	return strings.Join(lines, "\n")
}

func renderProgressBar(label string, pct float64, detail string, barWidth int) string {
	filled := int(math.Round(float64(barWidth) * pct / 100.0))
	empty := barWidth - filled
	if empty < 0 {
		empty = 0
	}

	bar := theme.ProgressFilled.Render(strings.Repeat("█", filled)) +
		theme.ProgressEmpty.Render(strings.Repeat("░", empty))
	pctStr := theme.ProgressPct.Render(fmt.Sprintf("%.1f%%", pct))
	detailStr := theme.HeaderMeta.Render(" (" + detail + ")")

	return "    " + theme.ProgressLabel.Render(label) + bar + " " + pctStr + detailStr
}

// ────────────────────────────────────────────────────────────────────────────────
// Monitor Hub: Provider Fleet
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderMonitorProviderFleet(w int) string {
	var lines []string

	lines = append(lines, "  "+m.renderMonitorTabs())
	lines = append(lines, "")

	// Scan progress
	scanProgress := 57
	scanBarWidth := 20
	scanFilled := scanBarWidth * scanProgress / 100
	scanEmpty := scanBarWidth - scanFilled
	scanBar := theme.ProgressFilled.Render(strings.Repeat("█", scanFilled)) +
		theme.ProgressEmpty.Render(strings.Repeat("░", scanEmpty))
	lines = append(lines, "  "+theme.SpinnerText.Render("Scanning providers... ")+
		theme.HeaderValue.Render("142/247")+
		theme.HeaderMeta.Render(" checked, ")+
		theme.HeaderValue.Render("98")+
		theme.HeaderMeta.Render(" online  ")+
		scanBar+" "+theme.ProgressPct.Render(fmt.Sprintf("%d%%", scanProgress)))
	lines = append(lines, "")

	// Version Distribution
	lines = append(lines, "  "+theme.SectionTitle.Render("Version Distribution"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	dotBarWidth := 28
	for i, vd := range mockVersions {
		indicator := "  "
		versionStyle := theme.KVValue
		if i == m.versionCursor {
			indicator = theme.TableCursor.Render("▸ ")
			versionStyle = theme.KVValueBold
		}

		filledDots := int(math.Round(float64(dotBarWidth) * vd.pct / 100.0))
		emptyDots := dotBarWidth - filledDots

		var dotBar string
		if i == m.versionCursor {
			dotBar = lipgloss.NewStyle().Foreground(theme.Slate200).Render(strings.Repeat("●", filledDots)) +
				lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("○", emptyDots))
		} else {
			dotBar = lipgloss.NewStyle().Foreground(theme.Slate500).Render(strings.Repeat("●", filledDots)) +
				lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("○", emptyDots))
		}

		lines = append(lines, "  "+indicator+
			versionStyle.Width(12).Render(vd.version)+
			dotBar+"  "+
			theme.KVValue.Width(4).Render(fmt.Sprintf("%d", vd.count))+"  "+
			theme.HeaderMeta.Render(fmt.Sprintf("%.1f%%", vd.pct)))
	}
	lines = append(lines, "")

	// Provider List
	lines = append(lines, " "+theme.SectionTitle.Render("Provider List"))
	lines = append(lines, " "+theme.HRuleAccent(w-2))

	lines = append(lines, "  "+
		col(theme.ColHeader, 4, "#")+
		col(theme.ColHeader, 26, "URL")+
		col(theme.ColHeader, 9, "VERSION")+
		col(theme.ColHeader, 10, "CPU")+
		col(theme.ColHeader, 12, "MEMORY")+
		theme.ColHeader.Render("GPU"))

	for i, p := range mockProviderList {
		cur := "  "
		if i == m.monProvCursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		var row string
		if i == m.monProvCursor {
			row = cur +
				col(theme.ColBold, 4, fmt.Sprintf("%d", i+1)) +
				col(theme.ColBold, 26, p.url) +
				col(theme.ColBold, 9, p.version) +
				col(theme.ColBold, 10, p.cpu) +
				col(theme.ColBold, 12, p.memory) +
				theme.ColBold.Render(p.gpu)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.Col, 4, fmt.Sprintf("%d", i+1)) +
				col(theme.Col, 26, p.url) +
				col(theme.Col, 9, p.version) +
				col(theme.Col, 10, p.cpu) +
				col(theme.Col, 12, p.memory) +
				theme.ColMuted.Render(p.gpu)
		}
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Monitor Hub: Oracle/BME
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderMonitorOracleBME(w int) string {
	var lines []string

	lines = append(lines, "  "+m.renderMonitorTabs())
	lines = append(lines, "")
	lines = append(lines, "  "+theme.SectionTitle.Render("Oracle / BME"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines, "")

	// Oracle prices
	lines = append(lines, "  "+theme.SectionTitle.Render("Oracle Prices"))
	lines = append(lines, "  "+theme.HRule(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("AKT/USD")+theme.KVValueBold.Render("$4.98")+
			theme.KVValueMuted.Render("  ▲ 2.3%"),
		"    "+theme.KVKey.Render("USDC/USD")+theme.KVValue.Render("$1.00")+
			theme.KVValueMuted.Render("  — 0.0%"),
		"    "+theme.KVKey.Render("ATOM/USD")+theme.KVValue.Render("$12.45")+
			theme.KVValueMuted.Render("  ▼ 1.1%"),
	)
	lines = append(lines, "")

	// BME Status
	lines = append(lines, "  "+theme.SectionTitle.Render("BME Vault"))
	lines = append(lines, "  "+theme.HRule(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Total Minted")+theme.KVValue.Render("2,450,000 AKT"),
		"    "+theme.KVKey.Render("Vault Balance")+theme.KVValue.Render("890,000 AKT"),
		"    "+theme.KVKey.Render("Mint Rate")+theme.KVValue.Render("12,500 AKT/day"),
		"    "+theme.KVKey.Render("Last Mint")+theme.KVValue.Render("Block 18,234,200"),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Monitor Tab Bar
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderMonitorTabs() string {
	tabNames := []string{"Network", "Provider", "Oracle/BME"}
	var tabs string
	for i, name := range tabNames {
		if i == m.monitorTab {
			tabs += theme.TabActive.Render(name)
		} else {
			tabs += theme.TabInactive.Render(name)
		}
		if i < len(tabNames)-1 {
			tabs += " "
		}
	}
	return tabs
}

// ────────────────────────────────────────────────────────────────────────────────
// Governance View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderGovernance(w int) string {
	var lines []string

	lines = append(lines, "  "+
		col(theme.ColHeader, 5, "ID")+
		col(theme.ColHeader, 38, "TITLE")+
		col(theme.ColHeader, 22, "TYPE")+
		col(theme.ColHeader, 12, "STATUS")+
		theme.ColHeader.Render("ENDS"))
	lines = append(lines, " "+theme.HRule(w-2))

	cursor := m.govCursor
	if cursor >= len(mockProposals) {
		cursor = len(mockProposals) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	govStateColW := 12
	for i, p := range mockProposals {
		cur := "  "
		if i == cursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		tag := stateTag(p.status)
		tagW := stateTagWidth(p.status)
		tagPad := ""
		if tagW < govStateColW {
			tagPad = strings.Repeat(" ", govStateColW-tagW)
		}

		var row string
		if i == cursor {
			row = cur +
				col(theme.ColBold, 5, fmt.Sprintf("%d", p.id)) +
				col(theme.ColBold, 38, p.title) +
				col(theme.Col, 22, p.propType) +
				tag + tagPad +
				theme.ColBold.Render(p.endTime)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.ColBold, 5, fmt.Sprintf("%d", p.id)) +
				col(theme.Col, 38, p.title) +
				col(theme.ColMuted, 22, p.propType) +
				tag + tagPad +
				theme.ColMuted.Render(p.endTime)
		}
		lines = append(lines, row)
	}

	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "  "+theme.ColMuted.Render(fmt.Sprintf("%d proposals", len(mockProposals))))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Proposal Detail View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderProposalDetail(w int) string {
	idx := m.govCursor
	if idx >= len(mockProposals) {
		idx = 0
	}
	p := mockProposals[idx]

	var lines []string

	lines = append(lines, "  "+theme.SectionTitle.Render(fmt.Sprintf("Proposal #%d", p.id)))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Title")+theme.KVValueBold.Render(p.title),
		"    "+theme.KVKey.Render("Type")+theme.KVValue.Render(p.propType),
		"    "+theme.KVKey.Render("Status")+theme.StateBadge(p.status).Render(p.status),
		"    "+theme.KVKey.Render("Ends")+theme.KVValue.Render(p.endTime),
	)
	lines = append(lines, "")

	lines = append(lines, "  "+theme.SectionTitle.Render("Vote Tally"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	barW := w - 50
	if barW < 20 {
		barW = 20
	}
	lines = append(lines, renderProgressBar("Yes", p.yes, fmt.Sprintf("%.1f%%", p.yes), barW))
	lines = append(lines, renderProgressBar("No", p.no, fmt.Sprintf("%.1f%%", p.no), barW))
	lines = append(lines, renderProgressBar("Abstain", p.abstain, fmt.Sprintf("%.1f%%", p.abstain), barW))
	lines = append(lines, renderProgressBar("No w/ Veto", p.veto, fmt.Sprintf("%.1f%%", p.veto), barW))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Staking View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderStaking(w int) string {
	var lines []string

	// Staking summary
	lines = append(lines, " "+theme.SectionTitle.Render("Your Delegations"))
	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines,
		"  "+theme.KVKey.Width(14).Render("Total Staked")+theme.KVValueBold.Render("5,000 AKT")+
			"   "+theme.KVKey.Width(10).Render("Rewards")+theme.KVValue.Render("12.4 AKT")+
			"   "+theme.KVKey.Width(5).Render("APR")+theme.KVValue.Render("~18.2%"),
	)
	lines = append(lines, "")

	// Validator table
	lines = append(lines, " "+theme.SectionTitle.Render("Validators"))
	lines = append(lines, " "+theme.HRule(w-2))

	lines = append(lines, "  "+
		col(theme.ColHeader, 5, "#")+
		col(theme.ColHeader, 16, "MONIKER")+
		col(theme.ColHeader, 13, "TOKENS")+
		col(theme.ColHeader, 8, "COMM")+
		col(theme.ColHeader, 13, "STATUS")+
		theme.ColHeader.Render("UPTIME"))

	cursor := m.stakeCursor
	if cursor >= len(mockValidators) {
		cursor = len(mockValidators) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	valStateColW := 13
	for i, v := range mockValidators {
		cur := "  "
		if i == cursor {
			cur = theme.TableCursor.Render("▸ ")
		}

		tag := stateTag(v.status)
		tagW := stateTagWidth(v.status)
		tagPad := ""
		if tagW < valStateColW {
			tagPad = strings.Repeat(" ", valStateColW-tagW)
		}

		var row string
		if i == cursor {
			row = cur +
				col(theme.ColBold, 5, fmt.Sprintf("%d", v.rank)) +
				col(theme.ColBold, 16, v.moniker) +
				col(theme.ColBold, 13, v.tokens) +
				col(theme.Col, 8, v.comission) +
				tag + tagPad +
				theme.ColBold.Render(v.uptime)
			row = theme.TableRowSelected.Width(w).Render(row)
		} else {
			row = cur +
				col(theme.Col, 5, fmt.Sprintf("%d", v.rank)) +
				col(theme.ColBold, 16, v.moniker) +
				col(theme.Col, 13, v.tokens) +
				col(theme.ColMuted, 8, v.comission) +
				tag + tagPad +
				theme.ColMuted.Render(v.uptime)
		}
		lines = append(lines, row)
	}

	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "  "+theme.ColMuted.Render(fmt.Sprintf("%d validators", len(mockValidators))))

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Validator Detail View
// ────────────────────────────────────────────────────────────────────────────────

func (m model) renderValidatorDetailView(w int) string {
	idx := m.stakeCursor
	if idx >= len(mockValidators) {
		idx = 0
	}
	v := mockValidators[idx]

	var lines []string

	lines = append(lines, "  "+theme.SectionTitle.Render(v.moniker))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Rank")+theme.KVValueBold.Render(fmt.Sprintf("#%d", v.rank)),
		"    "+theme.KVKey.Render("Address")+theme.KVValue.Render(v.address),
		"    "+theme.KVKey.Render("Tokens")+theme.KVValue.Render(v.tokens),
		"    "+theme.KVKey.Render("Commission")+theme.KVValue.Render(v.comission),
		"    "+theme.KVKey.Render("Status")+theme.StateBadge(v.status).Render(v.status),
		"    "+theme.KVKey.Render("Uptime")+theme.KVValue.Render(v.uptime),
	)
	lines = append(lines, "")

	lines = append(lines, "  "+theme.SectionTitle.Render("Your Delegation"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVKey.Render("Delegated")+theme.KVValue.Render("2,500 AKT"),
		"    "+theme.KVKey.Render("Rewards")+theme.KVValue.Render("6.2 AKT"),
		"    "+theme.KVKey.Render("Share")+theme.KVValueMuted.Render("0.02%"),
	)

	return strings.Join(lines, "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Overlay: Command Palette
// ────────────────────────────────────────────────────────────────────────────────

func (m model) overlayPalette(base string, w int) string {
	paletteW := 50
	if paletteW > w-4 {
		paletteW = w - 4
	}

	filtered := m.filteredPaletteCommands()

	var content strings.Builder

	promptStr := theme.PalettePrompt.Render(": ")
	content.WriteString(promptStr + m.paletteInput.View())
	content.WriteString("\n")
	content.WriteString(theme.HRule(paletteW - 4))
	content.WriteString("\n")

	for i, cmd := range filtered {
		var line string
		nameW := 16
		if i == m.paletteCursor {
			line = theme.PaletteItemSelected.Width(nameW).Render("> "+cmd.name) +
				"  " + theme.PaletteItemDesc.Render(cmd.desc)
		} else {
			line = theme.PaletteItemNormal.Width(nameW).Render("  "+cmd.name) +
				"  " + theme.PaletteItemDesc.Render(cmd.desc)
		}
		content.WriteString(line + "\n")
	}

	box := theme.PaletteBorder.
		Width(paletteW).
		Padding(1, 1).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Overlay: Confirmation Dialog
// ────────────────────────────────────────────────────────────────────────────────

func (m model) overlayConfirm(base string, w int) string {
	filtered := m.filteredDeployments()
	idx := m.confirmTarget
	if idx >= len(filtered) {
		idx = 0
	}
	if len(filtered) == 0 {
		return base
	}
	d := filtered[idx]

	dialogW := 56
	if dialogW > w-4 {
		dialogW = w - 4
	}

	var content strings.Builder

	content.WriteString(theme.DialogTitle.Render(fmt.Sprintf("Close Deployment #%d?", d.dseq)))
	content.WriteString("\n\n")
	content.WriteString(theme.DialogBody.Render("This will close all active leases and return\nremaining escrow balance."))
	content.WriteString("\n\n")
	content.WriteString(theme.KVKey.Width(14).Render("Owner") + theme.KVValue.Render(d.owner[:20]+"...") + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Balance") + theme.KVValue.Render(d.escrow) + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Gas estimate") + theme.KVValue.Render("~200,000") + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Fee estimate") + theme.KVValue.Render("~0.02 AKT") + "\n")
	content.WriteString("\n")

	var cancelBtn, confirmBtn string
	if m.confirmCursor == 0 {
		cancelBtn = theme.DialogButtonPrimary.Render(" Cancel (Esc) ")
		confirmBtn = theme.DialogButtonSecondary.Render(" Confirm (Enter) ")
	} else {
		cancelBtn = theme.DialogButtonSecondary.Render(" Cancel (Esc) ")
		confirmBtn = theme.DialogButtonPrimary.Render(" Confirm (Enter) ")
	}
	content.WriteString(cancelBtn + "  " + confirmBtn)

	box := theme.DialogBorder.
		Width(dialogW).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Overlay helper
// ────────────────────────────────────────────────────────────────────────────────

func overlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)
	overlayW := 0
	for _, l := range overlayLines {
		lw := lipgloss.Width(l)
		if lw > overlayW {
			overlayW = lw
		}
	}

	startY := (h - overlayH) / 2
	if startY < 2 {
		startY = 2
	}
	startX := (w - overlayW) / 2
	if startX < 0 {
		startX = 0
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		var newLine strings.Builder
		if startX > 0 {
			newLine.WriteString(strings.Repeat(" ", startX))
		}
		newLine.WriteString(ol)
		baseLines[row] = newLine.String()
	}

	return strings.Join(baseLines[:minInt(len(baseLines), h)], "\n")
}

// ────────────────────────────────────────────────────────────────────────────────
// Screenshot mode
// ────────────────────────────────────────────────────────────────────────────────

func renderScreenshot() {
	w := 120

	m := initialModel()
	m.width = w
	m.height = 40

	divider := func(title string) {
		fmt.Println()
		fmt.Println(lipgloss.NewStyle().Foreground(theme.Slate600).Bold(true).Render(
			"─── " + title + " " + strings.Repeat("─", w-len(title)-5)))
		fmt.Println()
	}

	// 1. Dashboard
	divider("DASHBOARD (START)")
	fmt.Println(m.renderHeader(w))
	fmt.Println(m.renderPrimaryNav(w))
	m.viewStack = []viewID{viewDashboard}
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderDashboard(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 2. Deployments List
	divider("1 DEPLOYMENTS")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewDeployments}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderDeploymentList(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 3. Deployment Detail
	divider("1 DEPLOYMENTS › DETAIL")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewDeployments, viewDeploymentDetail}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderDeploymentDetail(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 4. Leases List
	divider("2 LEASES")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewLeases}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderLeaseList(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 5. Providers List
	divider("3 PROVIDERS")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewProviders}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderProviderListView(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 6. Monitor Network
	divider("4 MONITOR › NETWORK")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewMonitorNetwork}
	m.monitorTab = 0
	m.networkSubTab = 0
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderMonitorNetwork(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 7. Monitor Provider
	divider("4 MONITOR › PROVIDER")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewMonitorProvider}
	m.monitorTab = 1
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderMonitorProviderFleet(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 8. Monitor Oracle/BME
	divider("4 MONITOR › ORACLE/BME")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewMonitorOracleBME}
	m.monitorTab = 2
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderMonitorOracleBME(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 9. Governance
	divider("5 GOVERNANCE")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewGovernance}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderGovernance(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 10. Proposal Detail
	divider("5 GOVERNANCE › PROPOSAL DETAIL")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewGovernance, viewProposalDetail}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderProposalDetail(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 11. Staking
	divider("6 STAKING")
	fmt.Println(m.renderHeader(w))
	m.viewStack = []viewID{viewDashboard, viewStaking}
	fmt.Println(m.renderPrimaryNav(w))
	fmt.Println(m.renderBreadcrumb())
	fmt.Println()
	fmt.Println(m.renderStaking(w))
	fmt.Println()
	fmt.Println(m.renderFooter(w))

	// 12. Command Palette
	divider("COMMAND PALETTE OVERLAY")
	paletteW := 50
	var palContent strings.Builder
	palContent.WriteString(theme.PalettePrompt.Render(": ") + theme.PaletteInput.Render("_"))
	palContent.WriteString("\n")
	palContent.WriteString(theme.HRule(paletteW - 4))
	palContent.WriteString("\n")
	for i, cmd := range paletteCommands {
		if i == 0 {
			palContent.WriteString(theme.PaletteItemSelected.Width(16).Render("> "+cmd.name) +
				"  " + theme.PaletteItemDesc.Render(cmd.desc) + "\n")
		} else {
			palContent.WriteString(theme.PaletteItemNormal.Width(16).Render("  "+cmd.name) +
				"  " + theme.PaletteItemDesc.Render(cmd.desc) + "\n")
		}
	}
	box := theme.PaletteBorder.Width(paletteW).Padding(1, 1).Render(palContent.String())
	fmt.Println(box)

	// 13. Confirm Dialog
	divider("CONFIRMATION DIALOG OVERLAY")
	d := mockDeployments[0]
	var confContent strings.Builder
	confContent.WriteString(theme.DialogTitle.Render(fmt.Sprintf("Close Deployment #%d?", d.dseq)))
	confContent.WriteString("\n\n")
	confContent.WriteString(theme.DialogBody.Render("This will close all active leases and return\nremaining escrow balance."))
	confContent.WriteString("\n\n")
	confContent.WriteString(theme.KVKey.Width(14).Render("Owner") + theme.KVValue.Render(d.owner[:20]+"...") + "\n")
	confContent.WriteString(theme.KVKey.Width(14).Render("Balance") + theme.KVValue.Render(d.escrow) + "\n")
	confContent.WriteString(theme.KVKey.Width(14).Render("Gas estimate") + theme.KVValue.Render("~200,000") + "\n")
	confContent.WriteString(theme.KVKey.Width(14).Render("Fee estimate") + theme.KVValue.Render("~0.02 AKT") + "\n")
	confContent.WriteString("\n")
	confContent.WriteString(theme.DialogButtonSecondary.Render(" Cancel (Esc) ") + "  " +
		theme.DialogButtonPrimary.Render(" Confirm (Enter) "))
	confBox := theme.DialogBorder.Width(56).Render(confContent.String())
	fmt.Println(confBox)

	fmt.Println()
}

// ────────────────────────────────────────────────────────────────────────────────
// Main
// ────────────────────────────────────────────────────────────────────────────────

// ────────────────────────────────────────────────────────────────────────────────
// Help Overlay (? key)
// ────────────────────────────────────────────────────────────────────────────────

func (m model) helpBindings() [][]string {
	global := [][]string{
		{"1-7", "Navigate primary views"},
		{":", "Command palette"},
		{"?", "This help"},
		{"q", "Quit / Go home"},
		{"Ctrl+C", "Force quit"},
	}

	var contextual [][]string
	switch m.currentView() {
	case viewDashboard:
		contextual = [][]string{
			{"2", "Jump to Deployments"},
		}
	case viewDeployments:
		contextual = [][]string{
			{"j/k", "Move cursor up/down"},
			{"Enter", "View deployment detail"},
			{"/", "Search / filter"},
			{"f", "Cycle state filter"},
			{"d", "Close deployment"},
			{"Esc", "Clear filter / go back"},
		}
	case viewDeploymentDetail:
		contextual = [][]string{
			{"l", "Open log viewer"},
			{"s", "Open shell"},
			{"d", "Close deployment"},
			{"Esc", "Back to list"},
		}
	case viewLeases:
		contextual = [][]string{
			{"j/k", "Move cursor"},
			{"Enter", "View lease detail"},
			{"f", "Cycle state filter"},
			{"Esc", "Back"},
		}
	case viewLeaseDetail:
		contextual = [][]string{
			{"l", "View logs"},
			{"s", "Open shell"},
			{"Esc", "Back to list"},
		}
	case viewProviders:
		contextual = [][]string{
			{"j/k", "Move cursor"},
			{"Enter", "View provider detail"},
			{"Esc", "Back"},
		}
	case viewProviderDetail:
		contextual = [][]string{
			{"Esc", "Back to list"},
		}
	case viewMonitorNetwork:
		contextual = [][]string{
			{"Tab", "Next monitor dashboard"},
			{"S-Tab", "Previous dashboard"},
			{"a", "Overview sub-tab"},
			{"s", "Validators sub-tab"},
			{"g", "Governance sub-tab"},
			{"Esc", "Back"},
		}
	case viewMonitorProvider:
		contextual = [][]string{
			{"Tab", "Next monitor dashboard"},
			{"S-Tab", "Previous dashboard"},
			{"j/k", "Scroll providers"},
			{"h/l", "Select version"},
			{"Esc", "Back"},
		}
	case viewMonitorOracleBME:
		contextual = [][]string{
			{"Tab", "Next monitor dashboard"},
			{"S-Tab", "Previous dashboard"},
			{"Esc", "Back"},
		}
	case viewGovernance:
		contextual = [][]string{
			{"j/k", "Move cursor"},
			{"Enter", "View proposal detail"},
			{"Esc", "Back"},
		}
	case viewProposalDetail:
		contextual = [][]string{
			{"v", "Cast vote"},
			{"Esc", "Back to list"},
		}
	case viewStaking:
		contextual = [][]string{
			{"j/k", "Move cursor"},
			{"Enter", "View validator detail"},
			{"Esc", "Back"},
		}
	case viewValidatorDetail:
		contextual = [][]string{
			{"d", "Delegate AKT"},
			{"r", "Redelegate"},
			{"u", "Undelegate"},
			{"Esc", "Back to list"},
		}
	case viewDeployWorkflow:
		contextual = [][]string{
			{"Enter", "Advance to next step"},
			{"j/k", "Select bid (step 3)"},
			{"Esc", "Cancel workflow"},
		}
	}

	return append(contextual, global...)
}

func (m model) overlayHelp(base string, w int) string {
	helpW := 54
	if helpW > w-4 {
		helpW = w - 4
	}

	var content strings.Builder
	content.WriteString(theme.DialogTitle.Render("Keyboard Shortcuts"))
	content.WriteString("\n")
	content.WriteString(theme.HRule(helpW - 6))
	content.WriteString("\n\n")

	// Context header
	viewName := "Global"
	switch m.currentView() {
	case viewDashboard:
		viewName = "Dashboard"
	case viewDeployments:
		viewName = "Deployments"
	case viewDeploymentDetail:
		viewName = "Deployment Detail"
	case viewLeases:
		viewName = "Leases"
	case viewLeaseDetail:
		viewName = "Lease Detail"
	case viewProviders:
		viewName = "Providers"
	case viewProviderDetail:
		viewName = "Provider Detail"
	case viewMonitorNetwork:
		viewName = "Monitor: Network"
	case viewMonitorProvider:
		viewName = "Monitor: Provider"
	case viewMonitorOracleBME:
		viewName = "Monitor: Oracle/BME"
	case viewGovernance:
		viewName = "Governance"
	case viewProposalDetail:
		viewName = "Proposal Detail"
	case viewStaking:
		viewName = "Staking"
	case viewValidatorDetail:
		viewName = "Validator Detail"
	case viewDeployWorkflow:
		viewName = "Deploy Workflow"
	}

	content.WriteString(theme.ColMuted.Render("Context: ") + theme.ColBold.Render(viewName) + "\n\n")

	bindings := m.helpBindings()
	for _, b := range bindings {
		key := fmt.Sprintf("%-10s", b[0])
		content.WriteString("  " + theme.FooterKey.Render(key) + " " + theme.Col.Render(b[1]) + "\n")
	}

	content.WriteString("\n" + theme.ColMuted.Render("Press ? or Esc to close"))

	box := theme.DialogBorder.
		Width(helpW).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Vote Dialog (v key from Proposal Detail)
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateVoteDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.voteOpen = false
	case "j", "down":
		if m.voteCursor < 3 {
			m.voteCursor++
		}
	case "k", "up":
		if m.voteCursor > 0 {
			m.voteCursor--
		}
	case "enter":
		// Submit vote (mock — just close)
		m.voteOpen = false
	}
	return m, nil
}

func (m model) overlayVote(base string, w int) string {
	idx := m.govCursor
	if idx >= len(mockProposals) {
		idx = 0
	}
	p := mockProposals[idx]

	dialogW := 48
	if dialogW > w-4 {
		dialogW = w - 4
	}

	voteOptions := []string{"Yes", "No", "Abstain", "No with Veto"}

	var content strings.Builder
	content.WriteString(theme.DialogTitle.Render(fmt.Sprintf("Vote on Proposal #%d", p.id)))
	content.WriteString("\n")
	content.WriteString(theme.DialogBody.Render(p.title))
	content.WriteString("\n\n")

	for i, opt := range voteOptions {
		if i == m.voteCursor {
			content.WriteString("  " + theme.ColAccent.Render("▸ "+opt) + "\n")
		} else {
			content.WriteString("    " + theme.Col.Render(opt) + "\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(theme.KVKey.Width(14).Render("Gas estimate") + theme.KVValue.Render("~100,000") + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Fee estimate") + theme.KVValue.Render("~0.01 AKT") + "\n")
	content.WriteString("\n")

	var cancelBtn, confirmBtn string
	cancelBtn = theme.DialogButtonSecondary.Render(" Cancel (Esc) ")
	confirmBtn = theme.DialogButtonPrimary.Render(" Submit (Enter) ")
	content.WriteString(cancelBtn + "  " + confirmBtn)

	box := theme.DialogBorder.
		Width(dialogW).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Delegate / Redelegate / Undelegate Dialog
// ────────────────────────────────────────────────────────────────────────────────

func (m model) updateDelegateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.delegateOpen = false
	case "tab":
		m.delegateStep = (m.delegateStep + 1) % 2
	case "enter":
		if m.delegateStep == 1 {
			m.delegateOpen = false // confirm
		} else {
			m.delegateStep = 1 // advance to confirm
		}
	}
	return m, nil
}

func (m model) overlayDelegate(base string, w int) string {
	idx := m.stakeCursor
	if idx >= len(mockValidators) {
		idx = 0
	}
	v := mockValidators[idx]

	dialogW := 52
	if dialogW > w-4 {
		dialogW = w - 4
	}

	actionNames := []string{"Delegate", "Redelegate", "Undelegate"}
	action := actionNames[m.delegateAction]

	var content strings.Builder
	content.WriteString(theme.DialogTitle.Render(action + " AKT"))
	content.WriteString("\n\n")
	content.WriteString(theme.KVKey.Width(14).Render("Validator") + theme.KVValueBold.Render(v.moniker) + "\n")

	if m.delegateAction == 2 {
		content.WriteString(theme.KVKey.Width(14).Render("Delegated") + theme.KVValue.Render("2,500 AKT") + "\n")
	}
	if m.delegateAction == 1 {
		content.WriteString(theme.KVKey.Width(14).Render("From") + theme.KVValue.Render(v.moniker) + "\n")
		content.WriteString(theme.KVKey.Width(14).Render("To") + theme.KVValueMuted.Render("(select validator)") + "\n")
	}

	content.WriteString("\n")
	content.WriteString(theme.KVKey.Width(14).Render("Amount") + theme.KVValueBold.Render("500 AKT") + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Available") + theme.KVValue.Render("148.52 AKT") + "\n")
	content.WriteString("\n")
	content.WriteString(theme.KVKey.Width(14).Render("Gas estimate") + theme.KVValue.Render("~250,000") + "\n")
	content.WriteString(theme.KVKey.Width(14).Render("Fee estimate") + theme.KVValue.Render("~0.025 AKT") + "\n")
	content.WriteString("\n")

	var cancelBtn, confirmBtn string
	if m.delegateStep == 0 {
		cancelBtn = theme.DialogButtonPrimary.Render(" Cancel (Esc) ")
		confirmBtn = theme.DialogButtonSecondary.Render(" Confirm (Enter) ")
	} else {
		cancelBtn = theme.DialogButtonSecondary.Render(" Cancel (Esc) ")
		confirmBtn = theme.DialogButtonPrimary.Render(" Confirm (Enter) ")
	}
	content.WriteString(cancelBtn + "  " + confirmBtn)

	box := theme.DialogBorder.
		Width(dialogW).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Log Viewer (l key from Deployment Detail)
// ────────────────────────────────────────────────────────────────────────────────

func mockLogLines() []string {
	return []string{
		"2026-04-13T10:00:01Z [info]  Starting container nginx:latest",
		"2026-04-13T10:00:02Z [info]  Pulling image nginx:latest from registry",
		"2026-04-13T10:00:05Z [info]  Image pulled successfully (3.2s)",
		"2026-04-13T10:00:05Z [info]  Creating container...",
		"2026-04-13T10:00:06Z [info]  Container created: abc123def456",
		"2026-04-13T10:00:06Z [info]  Starting container...",
		"2026-04-13T10:00:07Z [info]  Container started successfully",
		"2026-04-13T10:00:07Z [info]  nginx: [notice] using the \"epoll\" event method",
		"2026-04-13T10:00:07Z [info]  nginx: configuration file /etc/nginx/nginx.conf test passed",
		"2026-04-13T10:00:07Z [info]  nginx: [notice] start worker processes",
		"2026-04-13T10:00:07Z [info]  nginx: [notice] start worker process 29",
		"2026-04-13T10:00:07Z [info]  nginx: [notice] start worker process 30",
		"2026-04-13T10:00:12Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:00:22Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:00:32Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:00:35Z [info]  GET /api/v1/status 200 1.2ms",
		"2026-04-13T10:00:36Z [info]  GET /api/v1/health 200 0.8ms",
		"2026-04-13T10:00:42Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:01:01Z [warn]  High memory usage: 478/512Mi (93.4%)",
		"2026-04-13T10:01:05Z [info]  GET /api/v1/metrics 200 2.1ms",
		"2026-04-13T10:01:12Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:01:15Z [info]  POST /api/v1/data 201 12.3ms",
		"2026-04-13T10:01:22Z [info]  Health check passed (HTTP 200)",
		"2026-04-13T10:01:30Z [warn]  Connection pool near capacity: 95/100",
	}
}

func (m model) updateLogViewer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "l":
		m.logViewerOpen = false
	case "j", "down":
		if m.logScroll < len(m.logLines)-1 {
			m.logScroll++
		}
	case "k", "up":
		if m.logScroll > 0 {
			m.logScroll--
		}
	case "G":
		m.logScroll = len(m.logLines) - 1
	case "g":
		m.logScroll = 0
	}
	return m, nil
}

func (m model) overlayLogViewer(base string, w int) string {
	logW := w - 8
	if logW > 100 {
		logW = 100
	}
	if logW < 40 {
		logW = 40
	}

	visibleLines := m.height - 12
	if visibleLines < 5 {
		visibleLines = 5
	}

	filtered := m.filteredDeployments()
	idx := m.deplCursor
	if idx >= len(filtered) {
		idx = 0
	}
	dseq := 0
	if len(filtered) > 0 {
		dseq = filtered[idx].dseq
	}

	var content strings.Builder
	content.WriteString(theme.DialogTitle.Render(fmt.Sprintf("Logs · Deployment #%d", dseq)))
	content.WriteString("\n")
	content.WriteString(theme.HRule(logW - 6))
	content.WriteString("\n")

	start := m.logScroll
	end := start + visibleLines
	if end > len(m.logLines) {
		end = len(m.logLines)
	}

	for i := start; i < end; i++ {
		line := m.logLines[i]
		// Color [warn] lines differently
		if strings.Contains(line, "[warn]") {
			content.WriteString(lipgloss.NewStyle().Foreground(theme.Yellow).Render(line))
		} else if strings.Contains(line, "[error]") {
			content.WriteString(theme.ErrorLabel.Render(line))
		} else {
			content.WriteString(theme.Col.Render(line))
		}
		content.WriteString("\n")
	}

	content.WriteString(theme.HRule(logW - 6))
	content.WriteString("\n")
	content.WriteString(theme.ColMuted.Render(fmt.Sprintf("Line %d/%d", m.logScroll+1, len(m.logLines))) +
		"  " + theme.FooterKey.Render("j/k") + " " + theme.FooterDesc.Render("scroll") +
		"  " + theme.FooterKey.Render("g/G") + " " + theme.FooterDesc.Render("top/bottom") +
		"  " + theme.FooterKey.Render("Esc") + " " + theme.FooterDesc.Render("close"))

	box := theme.PaletteBorder.
		Width(logW).
		Padding(1, 1).
		Render(content.String())

	return overlayCenter(base, box, w, m.height)
}

// ────────────────────────────────────────────────────────────────────────────────
// Deploy Workflow (akt deploy <sdl>)
// ────────────────────────────────────────────────────────────────────────────────

var deploySteps = []struct {
	name string
	desc string
}{
	{"Create Deployment", "Broadcasting MsgCreateDeployment tx..."},
	{"Wait for Bids", "Listening for provider bids on deployment group..."},
	{"Select Bid", "Choose a provider bid to accept"},
	{"Create Lease", "Broadcasting MsgCreateLease tx..."},
	{"Send Manifest", "Uploading SDL manifest to provider..."},
	{"Wait Active", "Waiting for workload to become active..."},
	{"Show Endpoints", "Deployment is live!"},
}

var mockBids = []struct {
	provider string
	price    string
	gpu      string
	region   string
}{
	{"akash1q7spv...m6rx6", "12.5 uakt/blk", "H100 80GB", "US-East"},
	{"akash1f5k9...p3wn8", "14.2 uakt/blk", "A100 40GB", "EU-West"},
	{"akash1h8g2...k4mn9", "11.8 uakt/blk", "H100 80GB", "US-West"},
	{"akash1m3xr...j7tn2", "18.0 uakt/blk", "A100 80GB", "AP-SE"},
}

func (m model) updateDeployWorkflow(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.popView()
		m.deployStep = 0
		m.deployBidCur = 0
	case "enter":
		if m.deployStep == 2 {
			// Bid selected, advance
			m.deployStep++
		} else if m.deployStep < len(deploySteps)-1 {
			m.deployStep++
		} else {
			// Workflow complete — go to deployments
			m.deployStep = 0
			m.deployBidCur = 0
			m.switchPrimary(viewDeployments)
		}
	case "j", "down":
		if m.deployStep == 2 && m.deployBidCur < len(mockBids)-1 {
			m.deployBidCur++
		}
	case "k", "up":
		if m.deployStep == 2 && m.deployBidCur > 0 {
			m.deployBidCur--
		}
	}
	return m, nil
}

func (m model) renderDeployWorkflow(w int) string {
	var lines []string

	lines = append(lines, "")

	// Step progress bar
	var stepBar strings.Builder
	stepBar.WriteString("  ")
	for i := range deploySteps {
		if i < m.deployStep {
			// Completed
			stepBar.WriteString(lipgloss.NewStyle().Foreground(theme.Green).Render("✓"))
		} else if i == m.deployStep {
			// Current
			stepBar.WriteString(theme.ColAccent.Render("●"))
		} else {
			// Pending
			stepBar.WriteString(theme.ColMuted.Render("○"))
		}
		if i < len(deploySteps)-1 {
			if i < m.deployStep {
				stepBar.WriteString(lipgloss.NewStyle().Foreground(theme.GreenDim).Render("───"))
			} else {
				stepBar.WriteString(theme.ColMuted.Render("───"))
			}
		}
	}
	lines = append(lines, stepBar.String())
	lines = append(lines, "")

	// Step labels
	var labelBar strings.Builder
	labelBar.WriteString("  ")
	stepW := (w - 4) / len(deploySteps)
	if stepW < 12 {
		stepW = 12
	}
	for i, step := range deploySteps {
		name := step.name
		if len(name) > stepW-1 {
			name = name[:stepW-2] + "…"
		}
		padded := fmt.Sprintf("%-*s", stepW, name)
		if i == m.deployStep {
			labelBar.WriteString(theme.ColBold.Render(padded))
		} else if i < m.deployStep {
			labelBar.WriteString(lipgloss.NewStyle().Foreground(theme.Green).Render(padded))
		} else {
			labelBar.WriteString(theme.ColMuted.Render(padded))
		}
	}
	lines = append(lines, labelBar.String())
	lines = append(lines, "")
	lines = append(lines, " "+theme.HRule(w-2))
	lines = append(lines, "")

	// Current step content
	currentStep := deploySteps[m.deployStep]
	lines = append(lines, "  "+theme.SectionTitle.Render(
		fmt.Sprintf("Step %d of %d: %s", m.deployStep+1, len(deploySteps), currentStep.name)))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines, "")

	switch m.deployStep {
	case 0: // Create Deployment
		lines = append(lines,
			"    "+theme.KVKey.Render("SDL File")+theme.KVValue.Render("deploy.yaml"),
			"    "+theme.KVKey.Render("Image")+theme.KVValueBold.Render("llama-3:70b"),
			"    "+theme.KVKey.Render("CPU")+theme.KVValue.Render("8 cores"),
			"    "+theme.KVKey.Render("Memory")+theme.KVValue.Render("64Gi"),
			"    "+theme.KVKey.Render("GPU")+theme.KVValue.Render("1x A100"),
			"    "+theme.KVKey.Render("Deposit")+theme.KVValue.Render("20 AKT"),
			"",
			"    "+theme.SpinnerText.Render(currentStep.desc),
			"",
			"    "+theme.ColMuted.Render("Press Enter to continue"),
		)
	case 1: // Wait for Bids
		lines = append(lines,
			"    "+m.spinner.View()+" "+theme.SpinnerText.Render("Waiting for provider bids..."),
			"",
			"    "+theme.KVKey.Width(14).Render("Bids received")+theme.KVValueBold.Render(fmt.Sprintf("%d", len(mockBids))),
			"    "+theme.KVKey.Width(14).Render("Timeout")+theme.KVValue.Render("5m remaining"),
			"",
			"    "+theme.ColMuted.Render("Press Enter to review bids"),
		)
	case 2: // Select Bid
		lines = append(lines, "    "+theme.ColHeader.Render("Select a provider bid:"))
		lines = append(lines, "")
		lines = append(lines, "    "+
			col(theme.ColHeader, 22, "PROVIDER")+
			col(theme.ColHeader, 16, "PRICE")+
			col(theme.ColHeader, 14, "GPU")+
			theme.ColHeader.Render("REGION"))
		lines = append(lines, "   "+theme.HRule(w-6))

		for i, bid := range mockBids {
			cur := "    "
			if i == m.deployBidCur {
				cur = "  " + theme.TableCursor.Render("▸ ")
			}
			var row string
			if i == m.deployBidCur {
				row = cur +
					col(theme.ColBold, 22, bid.provider) +
					col(theme.ColBold, 16, bid.price) +
					col(theme.ColBold, 14, bid.gpu) +
					theme.ColBold.Render(bid.region)
				row = theme.TableRowSelected.Width(w).Render(row)
			} else {
				row = cur +
					col(theme.Col, 22, bid.provider) +
					col(theme.Col, 16, bid.price) +
					col(theme.Col, 14, bid.gpu) +
					theme.ColMuted.Render(bid.region)
			}
			lines = append(lines, row)
		}
		lines = append(lines, "")
		lines = append(lines, "    "+theme.ColMuted.Render("j/k to select · Enter to accept bid"))
	case 3: // Create Lease
		bid := mockBids[m.deployBidCur]
		lines = append(lines,
			"    "+theme.KVKey.Render("Provider")+theme.KVValueBold.Render(bid.provider),
			"    "+theme.KVKey.Render("Price")+theme.KVValue.Render(bid.price),
			"    "+theme.KVKey.Render("GPU")+theme.KVValue.Render(bid.gpu),
			"",
			"    "+theme.SpinnerText.Render("Broadcasting MsgCreateLease tx..."),
			"    "+theme.KVKey.Width(14).Render("Tx Hash")+theme.KVValueMuted.Render("E4F2A8B1C3D5..."),
			"",
			"    "+theme.ColMuted.Render("Press Enter to continue"),
		)
	case 4: // Send Manifest
		lines = append(lines,
			"    "+theme.SpinnerText.Render("Uploading SDL manifest to provider..."),
			"",
			"    "+theme.KVKey.Width(14).Render("Status")+theme.KVValue.Render("Manifest accepted"),
			"    "+theme.KVKey.Width(14).Render("Provider")+theme.KVValue.Render(mockBids[m.deployBidCur].provider),
			"",
			"    "+theme.ColMuted.Render("Press Enter to continue"),
		)
	case 5: // Wait Active
		lines = append(lines,
			"    "+m.spinner.View()+" "+theme.SpinnerText.Render("Waiting for workload to start..."),
			"",
			"    "+theme.KVKey.Width(14).Render("Status")+theme.KVValue.Render("Pulling image..."),
			"    "+theme.KVKey.Width(14).Render("Image")+theme.KVValue.Render("llama-3:70b"),
			"",
			"    "+theme.ColMuted.Render("Press Enter to continue"),
		)
	case 6: // Show Endpoints
		lines = append(lines,
			"    "+lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("✓ Deployment is live!"),
			"",
			"    "+theme.KVKey.Render("DSEQ")+theme.KVValueBold.Render("18543"),
			"    "+theme.KVKey.Render("Provider")+theme.KVValue.Render(mockBids[m.deployBidCur].provider),
			"    "+theme.KVKey.Render("Price")+theme.KVValue.Render(mockBids[m.deployBidCur].price),
			"",
			"    "+theme.SectionTitle.Render("Endpoints"),
			"    "+theme.HRule(w-8),
			"    "+theme.KVKey.Width(10).Render("inference")+theme.KVValueBold.Render("http://abc123.provider.akash.network:8000"),
			"    "+theme.KVKey.Width(10).Render("metrics")+theme.KVValue.Render("http://abc123.provider.akash.network:9090"),
			"",
			"    "+theme.ColMuted.Render("Press Enter to go to Deployments"),
		)
	}

	return strings.Join(lines, "\n")
}

// Deploy workflow breadcrumb helper
func deployStepName(step int) string {
	if step >= 0 && step < len(deploySteps) {
		return deploySteps[step].name
	}
	return "Deploy"
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--screenshot" {
		renderScreenshot()
		return
	}

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Ensure unused imports don't cause errors
var _ = time.Second
