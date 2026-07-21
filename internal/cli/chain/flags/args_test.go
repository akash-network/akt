package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
