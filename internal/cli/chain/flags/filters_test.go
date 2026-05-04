package flags

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	_ "pkg.akt.dev/go/sdkutil" // configures akash bech32 prefix
)

// testOwner and testProvider are valid akash bech32 addresses for deterministic
// testing. Generated from sequential byte slices using the "akash" prefix.
const (
	testOwner    = "akash1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnwduagr"
	testProvider = "akash1v3jkvemgd94xkmrddehhqutjwd682anh9zw2p2"
)

// ---------------------------------------------------------------------------
// DepFiltersFromArg
// ---------------------------------------------------------------------------

func TestDepFiltersFromArg(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		defaultOwner string
		wantOwner    string
		wantDSeq     uint64
		wantErr      string
	}{
		{
			name:    "empty arg errors",
			arg:     "",
			wantErr: "argument is required",
		},
		{
			name:      "owner only — lists all deployments",
			arg:       testOwner,
			wantOwner: testOwner,
			wantDSeq:  0,
		},
		{
			name:      "owner/dseq",
			arg:       testOwner + "/100",
			wantOwner: testOwner,
			wantDSeq:  100,
		},
		{
			name:         "dseq only — uses default owner",
			arg:          "42",
			defaultOwner: testOwner,
			wantOwner:    testOwner,
			wantDSeq:     42,
		},
		{
			name:    "dseq only — no default owner errors",
			arg:     "42",
			wantErr: "no default account set",
		},
		{
			name:    "invalid address and not a number",
			arg:     "notanaddress",
			wantErr: "is not a valid address or dseq number",
		},
		{
			name:    "owner/dseq/extra errors",
			arg:     testOwner + "/100/extra",
			wantErr: "too many parts",
		},
		{
			name:    "owner/invalid-dseq errors",
			arg:     testOwner + "/notanumber",
			wantErr: "invalid dseq",
		},
		{
			name:         "dseq/extra with default owner errors — too many parts",
			arg:          "42/extra",
			defaultOwner: testOwner,
			wantErr:      "too many parts",
		},
		{
			name:    "dseq/extra without default owner errors — no default account",
			arg:     "42/extra",
			wantErr: "no default account set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := DepFiltersFromArg(tc.arg, tc.defaultOwner)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, f.Owner)
			assert.Equal(t, tc.wantDSeq, f.DSeq)
		})
	}
}

// ---------------------------------------------------------------------------
// GroupIDFromArg
// ---------------------------------------------------------------------------

func TestGroupIDFromArg(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		defaultOwner   string
		wantOwner      string
		wantDSeq       uint64
		wantGSeq       uint32
		wantFullySpec  bool
		wantErr        string
	}{
		{
			name:    "empty arg errors",
			arg:     "",
			wantErr: "argument is required",
		},
		{
			name:    "owner alone errors — needs dseq",
			arg:     testOwner,
			wantErr: "expected at least owner/dseq",
		},
		{
			name:          "owner/dseq — gseq omitted",
			arg:           testOwner + "/100",
			wantOwner:     testOwner,
			wantDSeq:      100,
			wantGSeq:      0,
			wantFullySpec: false,
		},
		{
			name:          "owner/dseq/gseq — fully specified",
			arg:           testOwner + "/100/1",
			wantOwner:     testOwner,
			wantDSeq:      100,
			wantGSeq:      1,
			wantFullySpec: true,
		},
		{
			name:          "dseq only — uses default owner",
			arg:           "100",
			defaultOwner:  testOwner,
			wantOwner:     testOwner,
			wantDSeq:      100,
			wantGSeq:      0,
			wantFullySpec: false,
		},
		{
			name:          "dseq/gseq — uses default owner",
			arg:           "100/2",
			defaultOwner:  testOwner,
			wantOwner:     testOwner,
			wantDSeq:      100,
			wantGSeq:      2,
			wantFullySpec: true,
		},
		{
			name:    "dseq — no default owner errors",
			arg:     "100",
			wantErr: "no default account set",
		},
		{
			name:    "too many parts errors",
			arg:     testOwner + "/100/1/extra",
			wantErr: "too many parts",
		},
		{
			name:    "invalid dseq errors",
			arg:     testOwner + "/notanumber",
			wantErr: "invalid dseq",
		},
		{
			name:    "invalid gseq errors",
			arg:     testOwner + "/100/notanumber",
			wantErr: "invalid gseq",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, fullySpec, err := GroupIDFromArg(tc.arg, tc.defaultOwner)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, id.Owner)
			assert.Equal(t, tc.wantDSeq, id.DSeq)
			assert.Equal(t, tc.wantGSeq, id.GSeq)
			assert.Equal(t, tc.wantFullySpec, fullySpec)
		})
	}
}

// ---------------------------------------------------------------------------
// OrderFiltersFromArg
// ---------------------------------------------------------------------------

func TestOrderFiltersFromArg(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		defaultOwner string
		wantOwner    string
		wantDSeq     uint64
		wantGSeq     uint32
		wantOSeq     uint32
		wantErr      string
	}{
		{
			name:    "empty arg errors",
			arg:     "",
			wantErr: "argument is required",
		},
		{
			name:      "owner/dseq/gseq/oseq — full path",
			arg:       testOwner + "/100/1/2",
			wantOwner: testOwner,
			wantDSeq:  100,
			wantGSeq:  1,
			wantOSeq:  2,
		},
		{
			name:      "owner/dseq — partial",
			arg:       testOwner + "/100",
			wantOwner: testOwner,
			wantDSeq:  100,
		},
		{
			name:         "dseq only — uses default owner",
			arg:          "100",
			defaultOwner: testOwner,
			wantOwner:    testOwner,
			wantDSeq:     100,
		},
		{
			name:    "dseq — no default owner errors",
			arg:     "100",
			wantErr: "no default account set",
		},
		{
			name:      "owner only — dseq/gseq/oseq all zero",
			arg:       testOwner,
			wantOwner: testOwner,
		},
		{
			name:    "too many parts errors",
			arg:     testOwner + "/100/1/2/extra",
			wantErr: "too many parts",
		},
		{
			name:    "invalid dseq errors",
			arg:     testOwner + "/abc",
			wantErr: "invalid dseq",
		},
		{
			name:    "invalid gseq errors",
			arg:     testOwner + "/100/abc",
			wantErr: "invalid gseq",
		},
		{
			name:    "invalid oseq errors",
			arg:     testOwner + "/100/1/abc",
			wantErr: "invalid oseq",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := OrderFiltersFromArg(tc.arg, tc.defaultOwner)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, f.Owner)
			assert.Equal(t, tc.wantDSeq, f.DSeq)
			assert.Equal(t, tc.wantGSeq, f.GSeq)
			assert.Equal(t, tc.wantOSeq, f.OSeq)
		})
	}
}

// ---------------------------------------------------------------------------
// BidFiltersFromArg
// ---------------------------------------------------------------------------

func TestBidFiltersFromArg(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		defaultOwner string
		byProvider   bool
		wantOwner    string
		wantProvider string
		wantDSeq     uint64
		wantGSeq     uint32
		wantOSeq     uint32
		wantErr      string
	}{
		{
			name:    "empty arg errors",
			arg:     "",
			wantErr: "argument is required",
		},
		// --- Owner perspective (byProvider=false) ---
		{
			name:      "owner/dseq/gseq/oseq/provider — full path",
			arg:       testOwner + "/100/1/2/" + testProvider,
			wantOwner: testOwner,
			wantDSeq:  100,
			wantGSeq:  1,
			wantOSeq:  2,
			wantProvider: testProvider,
		},
		{
			name:      "owner/dseq — partial",
			arg:       testOwner + "/100",
			wantOwner: testOwner,
			wantDSeq:  100,
		},
		{
			name:         "dseq only — uses default owner",
			arg:          "100",
			defaultOwner: testOwner,
			wantOwner:    testOwner,
			wantDSeq:     100,
		},
		{
			name:    "dseq — no default owner errors",
			arg:     "100",
			wantErr: "no default account set",
		},
		{
			name:    "invalid first component errors",
			arg:     "notvalid",
			wantErr: "is not a valid address or dseq number",
		},
		{
			name:    "too many parts errors",
			arg:     testOwner + "/100/1/2/" + testProvider + "/extra",
			wantErr: "too many parts",
		},
		{
			name:    "invalid trailing address errors",
			arg:     testOwner + "/100/1/2/notanaddress",
			wantErr: "invalid address",
		},
		// --- Provider perspective (byProvider=true) ---
		{
			name:         "by-provider: provider/dseq/gseq/oseq/owner",
			arg:          testProvider + "/100/1/2/" + testOwner,
			byProvider:   true,
			wantProvider: testProvider,
			wantDSeq:     100,
			wantGSeq:     1,
			wantOSeq:     2,
			wantOwner:    testOwner,
		},
		{
			name:         "by-provider: provider/dseq — partial",
			arg:          testProvider + "/100",
			byProvider:   true,
			wantProvider: testProvider,
			wantDSeq:     100,
		},
		{
			name:       "by-provider: dseq only errors — provider required",
			arg:        "100",
			byProvider: true,
			wantErr:    "provider address is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := BidFiltersFromArg(tc.arg, tc.defaultOwner, tc.byProvider)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOwner, f.Owner)
			assert.Equal(t, tc.wantProvider, f.Provider)
			assert.Equal(t, tc.wantDSeq, f.DSeq)
			assert.Equal(t, tc.wantGSeq, f.GSeq)
			assert.Equal(t, tc.wantOSeq, f.OSeq)
		})
	}
}

// ---------------------------------------------------------------------------
// LeaseFiltersFromArg — delegates to BidFiltersFromArg, so we test the
// conversion wrapper rather than re-testing all parse paths.
// ---------------------------------------------------------------------------

func TestLeaseFiltersFromArg(t *testing.T) {
	t.Run("delegates to BidFiltersFromArg", func(t *testing.T) {
		lf, err := LeaseFiltersFromArg(testOwner+"/100/1/2/"+testProvider, "", false)
		require.NoError(t, err)
		assert.Equal(t, testOwner, lf.Owner)
		assert.Equal(t, testProvider, lf.Provider)
		assert.Equal(t, uint64(100), lf.DSeq)
		assert.Equal(t, uint32(1), lf.GSeq)
		assert.Equal(t, uint32(2), lf.OSeq)
	})

	t.Run("propagates errors", func(t *testing.T) {
		_, err := LeaseFiltersFromArg("", "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "argument is required")
	})
}

// ---------------------------------------------------------------------------
// IsID helpers
// ---------------------------------------------------------------------------

func TestDepFiltersIsID(t *testing.T) {
	tests := []struct {
		name   string
		owner  string
		dseq   uint64
		wantID bool
	}{
		{"both set", testOwner, 100, true},
		{"owner empty", "", 100, false},
		{"dseq zero", testOwner, 0, false},
		{"both empty", "", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := DepFiltersFromArg(tc.owner+"/"+formatUint(tc.dseq), "")
			if err != nil {
				// For cases where parse fails, construct manually.
				f.Owner = tc.owner
				f.DSeq = tc.dseq
			}
			assert.Equal(t, tc.wantID, DepFiltersIsID(f))
		})
	}
}

func TestOrderFiltersIsID(t *testing.T) {
	tests := []struct {
		name   string
		owner  string
		dseq   uint64
		gseq   uint32
		oseq   uint32
		wantID bool
	}{
		{"all set", testOwner, 100, 1, 2, true},
		{"owner empty", "", 100, 1, 2, false},
		{"dseq zero", testOwner, 0, 1, 2, false},
		{"gseq zero", testOwner, 100, 0, 2, false},
		{"oseq zero", testOwner, 100, 1, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := mvbetaOrderFilters(tc.owner, tc.dseq, tc.gseq, tc.oseq)
			assert.Equal(t, tc.wantID, OrderFiltersIsID(f))
		})
	}
}

func TestBidFiltersIsID(t *testing.T) {
	tests := []struct {
		name     string
		owner    string
		dseq     uint64
		gseq     uint32
		oseq     uint32
		provider string
		wantID   bool
	}{
		{"all set", testOwner, 100, 1, 2, testProvider, true},
		{"provider empty", testOwner, 100, 1, 2, "", false},
		{"owner empty", "", 100, 1, 2, testProvider, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := mvbetaBidFilters(tc.owner, tc.dseq, tc.gseq, tc.oseq, tc.provider)
			assert.Equal(t, tc.wantID, BidFiltersIsID(f))
		})
	}
}

func TestLeaseFiltersIsID(t *testing.T) {
	t.Run("delegates to BidFiltersIsID", func(t *testing.T) {
		lf, err := LeaseFiltersFromArg(testOwner+"/100/1/2/"+testProvider, "", false)
		require.NoError(t, err)
		assert.True(t, LeaseFiltersIsID(lf))
	})

	t.Run("not fully specified", func(t *testing.T) {
		lf, err := LeaseFiltersFromArg(testOwner+"/100", "", false)
		require.NoError(t, err)
		assert.False(t, LeaseFiltersIsID(lf))
	})
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func formatUint(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func mvbetaOrderFilters(owner string, dseq uint64, gseq, oseq uint32) mvbeta.OrderFilters {
	return mvbeta.OrderFilters{Owner: owner, DSeq: dseq, GSeq: gseq, OSeq: oseq}
}

func mvbetaBidFilters(owner string, dseq uint64, gseq, oseq uint32, provider string) mvbeta.BidFilters {
	return mvbeta.BidFilters{Owner: owner, DSeq: dseq, GSeq: gseq, OSeq: oseq, Provider: provider}
}

// Ensure mv1.LeaseFilters is used to avoid unused import.
var _ mv1.LeaseFilters
