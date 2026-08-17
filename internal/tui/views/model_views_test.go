package views

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

type recordingService struct {
	calls   []string
	tallied []*govv1.Proposal
}

func (s *recordingService) command(call string) tea.Cmd {
	s.calls = append(s.calls, call)
	return CmdFunc(call)
}

func (s *recordingService) LoadDeployments(owner string) tea.Cmd {
	return s.command("deployments:" + owner)
}

func (s *recordingService) LoadLeases(owner string) tea.Cmd {
	return s.command("leases:" + owner)
}

func (s *recordingService) LoadDeploymentLeases(owner string, dseq uint64) tea.Cmd {
	return s.command(fmt.Sprintf("deployment-leases:%s:%d", owner, dseq))
}

func (s *recordingService) LoadBids(owner string, dseq uint64) tea.Cmd {
	return s.command(fmt.Sprintf("bids:%s:%d", owner, dseq))
}

func (s *recordingService) LoadProviders() tea.Cmd { return s.command("providers") }
func (s *recordingService) LoadProposals() tea.Cmd { return s.command("proposals") }

func (s *recordingService) LoadTallies(proposals []*govv1.Proposal) tea.Cmd {
	s.tallied = append([]*govv1.Proposal(nil), proposals...)
	return s.command("tallies")
}

func (s *recordingService) LoadValidators() tea.Cmd  { return s.command("validators") }
func (s *recordingService) LoadStakingPool() tea.Cmd { return s.command("staking-pool") }
func (s *recordingService) LoadBalance(owner string) tea.Cmd {
	return s.command("balance:" + owner)
}
func (s *recordingService) LoadStoreStats() tea.Cmd { return s.command("store-stats") }
func (s *recordingService) LoadSyncState() tea.Cmd  { return s.command("sync-state") }

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

func hasCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}

func TestBaseViewsEnforceNavigationAndScrollBoundaries(t *testing.T) {
	t.Parallel()

	detail := NewBaseDetailView()
	detail.SetSize(80, 20)
	detail.Update(keyPress('j'))
	detail.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if detail.Scroll != 2 {
		t.Fatalf("detail scroll after two down keys = %d, want 2", detail.Scroll)
	}
	detail.Update(keyPress('k'))
	detail.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	detail.Update(keyPress('k'))
	if detail.Scroll != 0 {
		t.Fatalf("detail scroll crossed the top boundary: %d", detail.Scroll)
	}
	if got := detail.VisibleWindow(nil, 2); got != nil {
		t.Fatalf("VisibleWindow(nil) = %v, want nil", got)
	}
	if got := detail.VisibleWindow([]string{"a"}, 0); got != nil {
		t.Fatalf("VisibleWindow(height=0) = %v, want nil", got)
	}
	detail.Scroll = 100
	window := detail.VisibleWindow([]string{"a", "b", "c", "d"}, 2)
	if strings.Join(window, "") != "cd" || detail.Scroll != 2 {
		t.Fatalf("clamped detail window = %v at scroll %d, want [c d] at 2", window, detail.Scroll)
	}
	if got := detail.ScrollHint(2, 2); got != "" {
		t.Fatalf("non-overflow ScrollHint = %q, want empty", got)
	}
	if got := detail.ScrollHint(5, 2); !strings.Contains(got, "3/4") {
		t.Fatalf("overflow ScrollHint = %q, want current/maximum", got)
	}

	base := NewBaseListView(components.ResourceTableConfig{Columns: []components.TableColumn{
		{Header: "NAME", Width: 10},
	}}, keys.DefaultKeyMap())
	base.SetSize(40, 10)
	base.SetRows([]components.TableRow{
		{ID: "one", Cells: []string{"one"}},
		{ID: "two", Cells: []string{"two"}},
	})
	base.Update(keyPress('j'))
	if base.Cursor() != 1 || base.SelectedRow().ID != "two" {
		t.Fatalf("list down selection = %d/%v, want second row", base.Cursor(), base.SelectedRow())
	}
	base.Update(keyPress('k'))
	if base.Cursor() != 0 || !strings.Contains(ansi.Strip(base.View().Content), "one") {
		t.Fatalf("list up/render did not return to first row")
	}
}

func TestViewUtilitiesCoverIdentityPreservingBoundaries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		max   int
		want  string
	}{
		{input: "short", max: 8, want: "short"},
		{input: "abcdef", max: 3, want: "abc"},
		{input: "abcdefgh", max: 6, want: "abc..."},
	} {
		if got := Truncate(tc.input, tc.max); got != tc.want {
			t.Fatalf("Truncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
	for input, want := range map[int64]string{
		0: "0", 999: "999", 1_000: "1,000", 1_234_567: "1,234,567", -1_234: "-1,234",
	} {
		if got := CommaGroup(input); got != want {
			t.Fatalf("CommaGroup(%d) = %q, want %q", input, got, want)
		}
	}
	if got := CmdFunc(messages.PopViewMsg{})(); fmt.Sprintf("%T", got) != "messages.PopViewMsg" {
		t.Fatalf("CmdFunc returned %T, want messages.PopViewMsg", got)
	}
	if got := formatStoredCoins("not-a-coin"); got != "not-a-coin" {
		t.Fatalf("invalid stored coin = %q, want preserved input", got)
	}
	if got := formatStoredCoins(""); got != "" {
		t.Fatalf("empty stored coin = %q, want empty", got)
	}
}

func TestDeploymentsViewBehaviorAndActions(t *testing.T) {
	svc := &recordingService{}
	view := NewDeploymentsView(svc, keys.DefaultKeyMap(), "akash1owner")
	view.SetSize(240, 20)
	if got := view.Init()(); got != "deployments:akash1owner" {
		t.Fatalf("Init message = %v, want owner-specific deployment load", got)
	}
	if view.Breadcrumb() != "Deployments" || len(view.ShortHelp()) != 6 {
		t.Fatalf("unexpected deployments chrome: %q %#v", view.Breadcrumb(), view.ShortHelp())
	}

	now := time.Now()
	active := &store.DeploymentRecord{
		Owner: "akash1owner", DSeq: 7, State: "active", SDLPath: "/tmp/web.yaml",
		Labels:    map[string]string{"cpu": "2", "memory": "4Gi", "gpu": "a100", "provider": "akash1providerfull"},
		CreatedAt: now.Add(-2 * time.Hour).Unix(), Deposit: "5000000uakt", EscrowBalance: "2500000uakt",
	}
	closed := &store.DeploymentRecord{Owner: "akash1owner", DSeq: 8, State: "closed"}
	view.Update(messages.DeploymentsLoadedMsg{Deployments: []*store.DeploymentRecord{active, closed}})
	plain := ansi.Strip(view.View().Content)
	for _, want := range []string{"web.yaml", "akash1providerfull", "2.5 AKT", "5 AKT", "2 items"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("deployment table missing %q:\n%s", want, plain)
		}
	}

	_, selectCmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	push, ok := selectCmd().(messages.PushViewMsg)
	if !ok {
		t.Fatalf("select returned %T, want PushViewMsg", selectCmd())
	}
	if detail, ok := push.View.(*DeploymentDetailView); !ok || detail.Deployment() != active {
		t.Fatalf("select pushed %#v, want active deployment detail", push.View)
	}

	_, logsCmd := view.Update(keyPress('l'))
	logs, ok := logsCmd().(messages.StartLogStreamMsg)
	if !ok || logs.Owner != active.Owner || logs.DSeq != active.DSeq {
		t.Fatalf("logs command = %#v, want active deployment identity", logsCmd())
	}
	_, closeCmd := view.Update(keyPress('d'))
	confirm, ok := closeCmd().(messages.ShowConfirmMsg)
	if !ok || confirm.Kind != components.ConfirmClose || !confirm.Data.Danger || !strings.Contains(confirm.Data.Body, "7") {
		t.Fatalf("close command = %#v, want irreversible deployment confirmation", closeCmd())
	}

	for step, wantCount := range []int{1, 1, 2} {
		_, cmd := view.Update(keyPress('f'))
		if cmd == nil || view.Table.FilteredCount() != wantCount {
			t.Fatalf("filter step %d count/cmd = %d/%v, want %d/non-nil", step, view.Table.FilteredCount(), cmd, wantCount)
		}
	}
	if len(view.ShortHelp()) != 6 {
		t.Fatalf("all-state filter should remove filter badge: %#v", view.ShortHelp())
	}
	if got := view.Refresh()(); got != "deployments:akash1owner" {
		t.Fatalf("Refresh message = %v", got)
	}

	before := view.Table.FilteredCount()
	view.Update(messages.DeploymentsLoadedMsg{Err: errors.New("store unavailable")})
	if view.Table.FilteredCount() != before {
		t.Fatalf("failed reload replaced last known deployment rows")
	}

	empty := NewDeploymentsView(svc, keys.DefaultKeyMap(), "akash1owner")
	for _, code := range []rune{'l', 'd'} {
		if _, cmd := empty.Update(keyPress(code)); cmd != nil {
			t.Fatalf("empty deployments key %q returned command", code)
		}
	}
}

func TestDeploymentCellHelpersCoverMissingAndTimeUnits(t *testing.T) {
	t.Parallel()

	if len(deploymentsColumns()) != 10 {
		t.Fatalf("deployment column count changed without row mapping update")
	}
	if got := labelOrDash(nil, "cpu"); got != "—" {
		t.Fatalf("nil label = %q, want dash", got)
	}
	if got := labelOrDash(map[string]string{"cpu": ""}, "cpu"); got != "—" {
		t.Fatalf("empty label = %q, want dash", got)
	}
	if got := deploymentCells(&store.DeploymentRecord{DSeq: 9}); len(got) != 10 || got[1] != "—" || got[7] != "—" {
		t.Fatalf("sparse deployment cells = %#v", got)
	}

	now := time.Now()
	for _, tc := range []struct {
		then   time.Time
		suffix string
	}{
		{then: now.Add(-10 * time.Second), suffix: "s"},
		{then: now.Add(-2 * time.Minute), suffix: "m"},
		{then: now.Add(-2 * time.Hour), suffix: "h"},
		{then: now.Add(-48 * time.Hour), suffix: "d"},
	} {
		if got := relativeTime(tc.then.Unix()); !strings.HasSuffix(got, tc.suffix) {
			t.Fatalf("relativeTime(%v) = %q, want suffix %q", tc.then, got, tc.suffix)
		}
	}
}

func TestLeasesViewFiltersAndActions(t *testing.T) {
	svc := &recordingService{}
	view := NewLeasesView(svc, keys.DefaultKeyMap(), "akash1owner")
	view.SetSize(240, 20)
	if got := view.Init()(); got != "leases:akash1owner" {
		t.Fatalf("Init message = %v", got)
	}
	if NewLeasesView(nil, keys.DefaultKeyMap(), "").Init() != nil {
		t.Fatal("nil-service leases Init should be inert")
	}

	active := &store.LeaseRecord{
		ID:    store.LeaseID{Owner: "akash1owner", DSeq: 42, GSeq: 1, OSeq: 2, Provider: "akash1providerfull"},
		State: "active", Price: "2500uakt", CreatedAt: time.Now().Add(-time.Hour).Unix(),
	}
	closed := &store.LeaseRecord{ID: store.LeaseID{Owner: "akash1owner", DSeq: 43, Provider: "akash1other"}, State: "closed"}
	view.Update(messages.LeasesLoadedMsg{Leases: []*store.LeaseRecord{active, closed}})
	plain := ansi.Strip(view.View().Content)
	for _, want := range []string{"akash1providerfull", "2.5 mAKT", "2 items"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("leases table missing %q:\n%s", want, plain)
		}
	}

	_, selectCmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	push, ok := selectCmd().(messages.PushViewMsg)
	if !ok {
		t.Fatalf("lease select returned %T", selectCmd())
	}
	if detail, ok := push.View.(*LeaseDetailView); !ok || detail.Lease() != active {
		t.Fatalf("lease select pushed %#v, want active lease", push.View)
	}
	_, logsCmd := view.Update(keyPress('l'))
	logs, ok := logsCmd().(messages.StartLogStreamMsg)
	if !ok || logs.Owner != active.ID.Owner || logs.DSeq != active.ID.DSeq {
		t.Fatalf("lease logs = %#v, want selected identity", logsCmd())
	}

	for step, wantCount := range []int{1, 1, 2} {
		_, cmd := view.Update(keyPress('f'))
		if cmd == nil || view.Table.FilteredCount() != wantCount {
			t.Fatalf("lease filter step %d = %d rows, want %d", step, view.Table.FilteredCount(), wantCount)
		}
	}
	if got := view.Refresh()(); got != "leases:akash1owner" {
		t.Fatalf("Refresh message = %v", got)
	}
	if view.Breadcrumb() != "Leases" || len(view.ShortHelp()) != 5 {
		t.Fatalf("unexpected leases chrome")
	}
	view.Update(messages.LeasesLoadedMsg{Err: errors.New("store unavailable")})
	if view.Table.FilteredCount() != 2 {
		t.Fatalf("failed lease reload replaced last known rows")
	}
}

func TestLeaseDetailLifecycleAndSparseRendering(t *testing.T) {
	t.Parallel()

	km := keys.DefaultKeyMap()
	if plain := ansi.Strip(NewLeaseDetailView(km, nil).View().Content); !strings.Contains(plain, "No lease selected") {
		t.Fatalf("nil lease detail = %q", plain)
	}
	lease := &store.LeaseRecord{
		ID:    store.LeaseID{DSeq: 42, GSeq: 1, OSeq: 2, Provider: "akash1providerfulladdress"},
		State: "active", Price: "5000000uakt", CreatedAt: time.Now().Add(-time.Hour).Unix(),
		Endpoints: []store.LeaseEndpoint{{Service: "web", URI: "https://web.example", ExternalPort: 443}},
	}
	detail := NewLeaseDetailView(km, lease)
	detail.SetSize(120, 12)
	if detail.Init() != nil || detail.Refresh() != nil || detail.Breadcrumb() != "Detail" || len(detail.ShortHelp()) != 2 {
		t.Fatalf("unexpected lease detail lifecycle/chrome")
	}
	plain := ansi.Strip(detail.View().Content)
	for _, want := range []string{"akash1providerfulladdress", "5 AKT/block", "j/k: scroll"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("lease detail missing %q:\n%s", want, plain)
		}
	}
	detail.Update(keyPress('j'))
	if detail.Scroll == 0 {
		t.Fatal("lease detail did not delegate scroll")
	}
	_, cmd := detail.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if _, ok := cmd().(messages.PopViewMsg); !ok {
		t.Fatalf("lease back returned %T, want PopViewMsg", cmd())
	}
}

func TestModelKeyBindingsRemainConfigurable(t *testing.T) {
	t.Parallel()

	km := keys.DefaultKeyMap()
	km.Select = key.NewBinding(key.WithKeys("x"))
	view := NewLeasesView(nil, km, "")
	record := &store.LeaseRecord{ID: store.LeaseID{DSeq: 1}}
	view.Update(messages.LeasesLoadedMsg{Leases: []*store.LeaseRecord{record}})
	if _, cmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("hard-coded enter bypassed custom select binding")
	}
	if _, cmd := view.Update(keyPress('x')); cmd == nil {
		t.Fatal("custom select binding did not trigger selection")
	}
}
