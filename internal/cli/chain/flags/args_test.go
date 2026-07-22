package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
)

// ---------------------------------------------------------------------------
// DSeqFromArgs
// ---------------------------------------------------------------------------

func TestDSeqFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fallback uint64
		want     uint64
		wantErr  string
	}{
		{
			name:     "no args returns fallback",
			args:     nil,
			fallback: 42,
			want:     42,
		},
		{
			name:     "empty slice returns fallback",
			args:     []string{},
			fallback: 42,
			want:     42,
		},
		{
			name:     "positional dseq wins over fallback",
			args:     []string{"12345"},
			fallback: 42,
			want:     12345,
		},
		{
			name:     "positional dseq with zero fallback",
			args:     []string{"100"},
			fallback: 0,
			want:     100,
		},
		{
			name:    "invalid dseq errors",
			args:    []string{"notanumber"},
			wantErr: "invalid dseq",
		},
		{
			name:    "negative dseq errors",
			args:    []string{"-1"},
			wantErr: "invalid dseq",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DSeqFromArgs(tc.args, tc.fallback)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// GroupSeqsFromArgs
// ---------------------------------------------------------------------------

func TestGroupSeqsFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		dseq     uint64
		gseq     uint32
		wantDSeq uint64
		wantGSeq uint32
		wantErr  string
	}{
		{
			name:     "no args returns fallbacks",
			args:     nil,
			dseq:     42,
			gseq:     1,
			wantDSeq: 42,
			wantGSeq: 1,
		},
		{
			name:     "positional dseq only — gseq keeps fallback",
			args:     []string{"12345"},
			dseq:     42,
			gseq:     1,
			wantDSeq: 12345,
			wantGSeq: 1,
		},
		{
			name:     "positional dseq and gseq win over fallbacks",
			args:     []string{"12345", "2"},
			dseq:     42,
			gseq:     1,
			wantDSeq: 12345,
			wantGSeq: 2,
		},
		{
			name:    "invalid dseq errors",
			args:    []string{"abc"},
			wantErr: "invalid dseq",
		},
		{
			name:    "invalid gseq errors",
			args:    []string{"12345", "abc"},
			wantErr: "invalid gseq",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotDSeq, gotGSeq, err := GroupSeqsFromArgs(tc.args, tc.dseq, tc.gseq)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDSeq, gotDSeq)
			assert.Equal(t, tc.wantGSeq, gotGSeq)
		})
	}
}

// ---------------------------------------------------------------------------
// LeaseSeqsFromArgs
// ---------------------------------------------------------------------------

func TestLeaseSeqsFromArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		dseq         uint64
		provider     string
		wantDSeq     uint64
		wantProvider string
		wantErr      string
	}{
		{
			name:         "no args returns fallbacks",
			args:         nil,
			dseq:         42,
			provider:     testProvider,
			wantDSeq:     42,
			wantProvider: testProvider,
		},
		{
			name:         "positional dseq only — provider keeps fallback",
			args:         []string{"12345"},
			dseq:         42,
			provider:     testProvider,
			wantDSeq:     12345,
			wantProvider: testProvider,
		},
		{
			name:         "positional dseq and provider win over fallbacks",
			args:         []string{"12345", testProvider},
			dseq:         42,
			provider:     testOwner,
			wantDSeq:     12345,
			wantProvider: testProvider,
		},
		{
			name:         "positional values with empty fallbacks",
			args:         []string{"100", testProvider},
			wantDSeq:     100,
			wantProvider: testProvider,
		},
		{
			name:    "invalid dseq errors",
			args:    []string{"notanumber"},
			wantErr: "invalid dseq",
		},
		{
			name:    "invalid provider errors",
			args:    []string{"12345", "notanaddress"},
			wantErr: "invalid provider",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotDSeq, gotProvider, err := LeaseSeqsFromArgs(tc.args, tc.dseq, tc.provider)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDSeq, gotDSeq)
			assert.Equal(t, tc.wantProvider, gotProvider)
		})
	}
}

// ---------------------------------------------------------------------------
// ExprFromArgs
// ---------------------------------------------------------------------------

func TestExprFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fallback string
		want     string
	}{
		{
			name:     "no args returns fallback",
			args:     nil,
			fallback: "message.sender='akash1...'",
			want:     "message.sender='akash1...'",
		},
		{
			name:     "empty slice returns fallback",
			args:     []string{},
			fallback: "tx.height=5",
			want:     "tx.height=5",
		},
		{
			name:     "positional expression wins over fallback",
			args:     []string{"block.height > 7"},
			fallback: "tx.height=5",
			want:     "block.height > 7",
		},
		{
			name: "positional expression with empty fallback",
			args: []string{"message.action=withdraw"},
			want: "message.action=withdraw",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExprFromArgs(tc.args, tc.fallback))
		})
	}
}

// ---------------------------------------------------------------------------
// DeploymentIDFromArgs
// ---------------------------------------------------------------------------

func TestDeploymentIDFromArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		fallback     dv1.DeploymentID
		defaultOwner string
		want         dv1.DeploymentID
		wantErr      string
	}{
		{
			name:         "no args keeps flag fallback",
			args:         nil,
			fallback:     dv1.DeploymentID{Owner: testOwner, DSeq: 42},
			defaultOwner: testProvider,
			want:         dv1.DeploymentID{Owner: testOwner, DSeq: 42},
		},
		{
			name:         "no args and no flags defaults owner",
			args:         nil,
			defaultOwner: testOwner,
			want:         dv1.DeploymentID{Owner: testOwner},
		},
		{
			name:         "positional dseq wins, owner from default account",
			args:         []string{"12345"},
			fallback:     dv1.DeploymentID{DSeq: 42},
			defaultOwner: testOwner,
			want:         dv1.DeploymentID{Owner: testOwner, DSeq: 12345},
		},
		{
			name:     "positional owner/dseq wins over flag fallback",
			args:     []string{testProvider + "/100"},
			fallback: dv1.DeploymentID{Owner: testOwner, DSeq: 42},
			want:     dv1.DeploymentID{Owner: testProvider, DSeq: 100},
		},
		{
			name:    "state keyword is rejected",
			args:    []string{"active"},
			wantErr: "state keyword",
		},
		{
			name:    "invalid dseq errors",
			args:    []string{"notanumber"},
			wantErr: "not a valid address or dseq",
		},
		{
			name:    "dseq shorthand without default account errors",
			args:    []string{"12345"},
			wantErr: "no default account",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeploymentIDFromArgs(tc.args, tc.fallback, tc.defaultOwner)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
