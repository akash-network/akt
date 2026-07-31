package store

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusPrettyOutputStripsANSIForNonTTY(t *testing.T) {
	home := t.TempDir()
	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Store")
	require.NotContains(t, stdout.String(), "\x1b[")
}

func TestStatusPrettyOutputHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	home := t.TempDir()
	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Store")
	require.NotContains(t, stdout.String(), "\x1b[")
}
