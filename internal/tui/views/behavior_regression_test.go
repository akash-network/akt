package views_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/tui/views"
)

type resizeRecorder struct {
	heights []int
}

func (m *resizeRecorder) Init() tea.Cmd { return nil }

func (m *resizeRecorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.heights = append(m.heights, size.Height)
	}
	return m, nil
}

func (m *resizeRecorder) View() tea.View { return tea.NewView("monitor") }

func TestLeasesViewSelectsCompleteLeaseIdentity(t *testing.T) {
	t.Parallel()

	first := &store.LeaseRecord{ID: store.LeaseID{
		Owner: "akash1owner", DSeq: 42, GSeq: 1, OSeq: 1,
		Provider: "akash1providerfirst",
	}}
	second := &store.LeaseRecord{ID: store.LeaseID{
		Owner: "akash1owner", DSeq: 42, GSeq: 2, OSeq: 1,
		Provider: "akash1providersecond",
	}}
	view := views.NewLeasesView(nil, keys.DefaultKeyMap(), first.ID.Owner)
	view.Update(messages.LeasesLoadedMsg{Leases: []*store.LeaseRecord{first, second}})
	view.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	_, cmd := view.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selecting the second lease returned no navigation command")
	}
	push, ok := cmd().(messages.PushViewMsg)
	if !ok {
		t.Fatalf("select command returned %T, want messages.PushViewMsg", cmd())
	}
	detail, ok := push.View.(*views.LeaseDetailView)
	if !ok {
		t.Fatalf("pushed view has type %T, want *views.LeaseDetailView", push.View)
	}
	if got := detail.Lease(); got != second {
		t.Fatalf("selected lease = %#v, want the second row %#v", got, second)
	}
}

func TestLeaseDetailNeverTruncatesProviderAddress(t *testing.T) {
	t.Parallel()

	provider := "akash1provider000000000000000000000000000000000000000"
	view := views.NewLeaseDetailView(keys.DefaultKeyMap(), &store.LeaseRecord{
		ID: store.LeaseID{DSeq: 42, GSeq: 1, OSeq: 1, Provider: provider},
	})
	view.SetSize(160, 80)
	plain := ansi.Strip(view.View().Content)

	if got := strings.Count(plain, provider); got < 2 {
		t.Fatalf("provider address appears %d times, want full address in Provider and Bid ID fields:\n%s", got, plain)
	}
	if strings.Contains(plain, provider[:12]+"...") {
		t.Fatalf("detail view contains truncated provider address:\n%s", plain)
	}
}

func TestLogViewerAppendLinesDiscoversServiceFilters(t *testing.T) {
	t.Parallel()

	viewer := views.NewLogViewer()
	viewer.Open("deployment", "42", "")
	viewer.AppendLines([]views.LogLine{
		{Scope: "api", Message: "api ready"},
		{Scope: "web", Message: "web ready"},
		{Scope: "api", Message: "duplicate service"},
	})

	viewer.CycleServiceFilter()
	if got := viewer.ServiceFilter(); got != "api" {
		t.Fatalf("first discovered service = %q, want api", got)
	}
	viewer.CycleServiceFilter()
	if got := viewer.ServiceFilter(); got != "web" {
		t.Fatalf("second discovered service = %q, want web", got)
	}
	viewer.CycleServiceFilter()
	if got := viewer.ServiceFilter(); got != "" {
		t.Fatalf("filter after final service = %q, want all services", got)
	}
}

func TestMonitorAdapterPassesExactShellContentHeight(t *testing.T) {
	t.Parallel()

	recorder := &resizeRecorder{}
	adapter := views.NewMonitorAdapter(recorder)
	adapter.SetSize(100, 23)

	if len(recorder.heights) != 1 || recorder.heights[0] != 23 {
		t.Fatalf("monitor resize heights = %v, want [23]", recorder.heights)
	}
}
