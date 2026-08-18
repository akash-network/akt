package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

// stubTransport answers requests from a URL-suffix -> response table without
// touching the network. The bootstrap fetchers build their URLs from package
// constants (the real github.com/akash-network/net endpoints), so intercepting
// at the RoundTripper is the only way to exercise them as written.
type stubTransport struct {
	// routes maps a URL suffix to the status and body to answer with.
	routes map[string]stubResponse

	// seen records every requested URL, in order.
	seen []string
}

type stubResponse struct {
	status int
	body   string
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.seen = append(s.seen, req.URL.String())

	for suffix, resp := range s.routes {
		if strings.HasSuffix(req.URL.String(), suffix) {
			return &http.Response{
				StatusCode: resp.status,
				Body:       io.NopCloser(strings.NewReader(resp.body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
	}

	return nil, fmt.Errorf("no stub route for %s", req.URL)
}

func newStubClient(routes map[string]stubResponse) (*http.Client, *stubTransport) {
	tr := &stubTransport{routes: routes}

	return &http.Client{Transport: tr}, tr
}

// installDefaultTransport swaps http.DefaultTransport for the duration of the
// test so fetchAllNetworks — which builds its own http.Client — can be driven
// offline. Restored via Cleanup; these tests must not run in parallel.
func installDefaultTransport(t *testing.T, routes map[string]stubResponse) *stubTransport {
	t.Helper()

	tr := &stubTransport{routes: routes}
	prev := http.DefaultTransport
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = prev })

	return tr
}

const mainnetMeta = `{
  "chain_name": "akashnet",
  "chain_id": "akashnet-2",
  "pretty_name": "Akash",
  "network_type": "mainnet",
  "status": "live",
  "fees": {"fee_tokens": [{
    "denom": "uakt",
    "average_gas_price": 0.0025,
    "high_gas_price": 0.025
  }]},
  "apis": {
    "rpc":  [{"address": "https://rpc.example.com:443"}, {"address": ""}],
    "rest": [{"address": "https://api.example.com"}],
    "grpc": [{"address": "grpc.example.com:9090"}]
  }
}`

// TestMetaToNetworkMapsEndpointsAndGasPrice pins the translation from the
// upstream meta.json into an akt Network. A wrong gas-price format here writes
// an unusable value into every generated context (money path: gas prices are
// what every subsequent tx pays).
func TestMetaToNetworkMapsEndpointsAndGasPrice(t *testing.T) {
	client, _ := newStubClient(map[string]stubResponse{
		"/mainnet/meta.json": {http.StatusOK, mainnetMeta},
	})

	meta, err := fetchMeta(client, "mainnet")
	if err != nil {
		t.Fatalf("fetchMeta: %v", err)
	}

	n := metaToNetwork("mainnet", meta)

	if n.Name != "mainnet" || n.ChainID != "akashnet-2" {
		t.Errorf("name/chain-id = %q/%q", n.Name, n.ChainID)
	}
	// The empty RPC address in the fixture must be dropped, not carried
	// through as a "" endpoint that later fails to dial.
	if len(n.Endpoints.RPC) != 1 || n.Endpoints.RPC[0] != "https://rpc.example.com:443" {
		t.Errorf("rpc endpoints = %v, want the single non-empty address", n.Endpoints.RPC)
	}
	if len(n.Endpoints.API) != 1 || n.Endpoints.API[0] != "https://api.example.com" {
		t.Errorf("api endpoints = %v", n.Endpoints.API)
	}
	if len(n.Endpoints.GRPC) != 1 || n.Endpoints.GRPC[0] != "grpc.example.com:9090" {
		t.Errorf("grpc endpoints = %v", n.Endpoints.GRPC)
	}
	if n.GasPrices != "0.025uakt" {
		t.Errorf("gas prices = %q, want registry high price 0.025uakt", n.GasPrices)
	}
	if n.GasAdjustment != "1.5" {
		t.Errorf("gas adjustment = %q, want 1.5", n.GasAdjustment)
	}
}

// TestMetaToNetworkOmitsUnusableGasPrice covers the two guards around the fee
// token: a zero price or a missing denom must leave GasPrices empty so the
// context falls back to its own default rather than writing "0" or a bare
// number that the SDK cannot parse.
func TestMetaToNetworkOmitsUnusableGasPrice(t *testing.T) {
	cases := map[string]*metaJSON{
		"no fee tokens": {ChainID: "x-1"},
		"zero price":    metaWithFee("uakt", 0),
		"empty denom":   metaWithFee("", 0.025),
	}

	for name, meta := range cases {
		if got := metaToNetwork("n", meta).GasPrices; got != "" {
			t.Errorf("%s: gas prices = %q, want empty", name, got)
		}
	}
}

func metaWithFee(denom string, price float64) *metaJSON {
	m := &metaJSON{ChainID: "x-1"}
	m.Fees.FeeTokens = append(m.Fees.FeeTokens, struct {
		Denom        string  `json:"denom"`
		HighGasPrice float64 `json:"high_gas_price"`
	}{denom, price})

	return m
}

// TestListRepoDirsKeepsOnlyDirectories covers the filter that separates
// network directories from repo-root files (LICENSE, README...). Without it
// the wizard would offer files as selectable networks.
func TestListRepoDirsKeepsOnlyDirectories(t *testing.T) {
	client, _ := newStubClient(map[string]stubResponse{
		"/contents": {http.StatusOK, `[
			{"name":"mainnet","type":"dir"},
			{"name":"testnet-02","type":"dir"},
			{"name":"README.md","type":"file"}
		]`},
	})

	dirs, err := listRepoDirs(client)
	if err != nil {
		t.Fatalf("listRepoDirs: %v", err)
	}

	if len(dirs) != 2 || dirs[0] != "mainnet" || dirs[1] != "testnet-02" {
		t.Errorf("dirs = %v, want [mainnet testnet-02]", dirs)
	}
}

// TestListRepoDirsSurfacesAPIFailures covers the non-200 and malformed-JSON
// branches. GitHub rate-limits unauthenticated API calls with a 403, which is
// exactly what a first-run user behind a shared NAT will hit; the wizard must
// report it rather than proceeding with an empty network list.
func TestListRepoDirsSurfacesAPIFailures(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		client, _ := newStubClient(map[string]stubResponse{
			"/contents": {http.StatusForbidden, `{"message":"API rate limit exceeded"}`},
		})

		if _, err := listRepoDirs(client); err == nil {
			t.Fatal("a 403 must be surfaced")
		} else if !strings.Contains(err.Error(), "403") {
			t.Errorf("error should name the status, got %q", err)
		}
	})

	t.Run("malformed body", func(t *testing.T) {
		client, _ := newStubClient(map[string]stubResponse{
			"/contents": {http.StatusOK, `not json`},
		})

		if _, err := listRepoDirs(client); err == nil {
			t.Fatal("a malformed listing must be surfaced")
		}
	})
}

// TestFetchMetaSurfacesFailures covers the error branches of the per-network
// meta fetch: a directory without a meta.json (404) and a corrupt document.
func TestFetchMetaSurfacesFailures(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		client, _ := newStubClient(map[string]stubResponse{
			"/meta.json": {http.StatusNotFound, ""},
		})

		if _, err := fetchMeta(client, "ghost"); err == nil {
			t.Fatal("a 404 meta.json must be an error")
		} else if !strings.Contains(err.Error(), "404") {
			t.Errorf("error should name the status, got %q", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		client, _ := newStubClient(map[string]stubResponse{
			"/meta.json": {http.StatusOK, `{"chain_id":`},
		})

		if _, err := fetchMeta(client, "broken"); err == nil {
			t.Fatal("a malformed meta.json must be an error")
		}
	})
}

// TestFetchAllNetworksSkipsUnusableEntries is the important one: a single bad
// directory in the upstream repo must not abort first-run setup. Entries whose
// meta.json is missing, or that carry no chain_id, are skipped; the rest still
// reach the user's config.
func TestFetchAllNetworksSkipsUnusableEntries(t *testing.T) {
	installDefaultTransport(t, map[string]stubResponse{
		"/contents": {http.StatusOK, `[
			{"name":"mainnet","type":"dir"},
			{"name":"broken","type":"dir"},
			{"name":"nochain","type":"dir"},
			{"name":"LICENSE","type":"file"}
		]`},
		"/mainnet/meta.json": {http.StatusOK, mainnetMeta},
		"/broken/meta.json":  {http.StatusInternalServerError, ""},
		"/nochain/meta.json": {http.StatusOK, `{"chain_name":"x","apis":{}}`},
	})

	networks, err := fetchAllNetworks()
	if err != nil {
		t.Fatalf("fetchAllNetworks: %v", err)
	}

	if len(networks) != 1 {
		t.Fatalf("networks = %+v, want only the usable mainnet entry", networks)
	}
	if networks[0].Name != "mainnet" || networks[0].ChainID != "akashnet-2" {
		t.Errorf("network = %+v", networks[0])
	}
}

// TestFetchAllNetworksPropagatesListingFailure covers the other side: if the
// directory listing itself fails there is nothing to select from, and Run must
// see an error rather than an empty slice it would mistake for "no networks".
func TestFetchAllNetworksPropagatesListingFailure(t *testing.T) {
	installDefaultTransport(t, map[string]stubResponse{
		"/contents": {http.StatusServiceUnavailable, ""},
	})

	if _, err := fetchAllNetworks(); err == nil {
		t.Fatal("a failed repo listing must propagate")
	}
}

// TestAllSelected covers the "Select all" checkbox state used by the wizard's
// multi-select: it is checked only when every network is checked.
func TestAllSelected(t *testing.T) {
	cases := []struct {
		checked []bool
		want    bool
	}{
		{nil, true}, // vacuously true: nothing is unchecked
		{[]bool{true, true, true}, true},
		{[]bool{true, false, true}, false},
		{[]bool{false}, false},
	}

	for _, c := range cases {
		if got := allSelected(c.checked); got != c.want {
			t.Errorf("allSelected(%v) = %v, want %v", c.checked, got, c.want)
		}
	}
}

// TestInteractiveSelectorsFallBackWithoutATerminal covers the term.MakeRaw
// failure arms of the two wizard selectors. When stdin is not a terminal (a
// pipe, a CI runner) raw mode cannot be entered, and the selectors must return
// a safe default instead of blocking on a read that will never be answered:
// every network selected, and the first keyring backend this host can actually
// provide.
//
// The keyring default is host-dependent on purpose. "os" is an alias for the
// platform credential store, and a headless machine has none — writing "os"
// into the config there produced a context that claimed the system keyring
// while the SDK quietly used an encrypted file (SPEC §1.5, §3.9.3).
func TestInteractiveSelectorsFallBackWithoutATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = r.Close()
	}()

	prev := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = prev }()

	networks := []aktctx.Network{
		{Name: "mainnet", ChainID: "akashnet-2"},
		{Name: "sandbox", ChainID: "sandbox-2"},
	}

	got := multiSelect(networks)
	if len(got) != len(networks) {
		t.Errorf("multiSelect fallback = %+v, want every network", got)
	}

	want := "file"
	if aktkeyring.SystemKeyringAvailable() {
		want = "os"
	}

	if backend := selectKeyringBackend(); backend != want {
		t.Errorf("keyring backend fallback = %q, want %q", backend, want)
	}
}

// TestKeyringBackendFallbackIsAlwaysUsable is the property the previous test
// asserts per host, stated directly: whatever the wizard picks without a
// terminal, opening it must succeed. A backend that resolves to nothing is
// exactly the silent substitution this guards against.
func TestKeyringBackendFallbackIsAlwaysUsable(t *testing.T) {
	root := t.TempDir()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = r.Close()
	}()

	prev := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = prev }()

	backend := selectKeyringBackend()

	if _, ok := aktkeyring.EffectiveBackend(root, aktctx.Keyring{Name: "default", Backend: backend}); !ok {
		t.Errorf("wizard selected backend %q, which this host cannot provide", backend)
	}
}
