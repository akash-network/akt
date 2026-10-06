package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	wf "pkg.akt.dev/akt/internal/workflow"
)

func TestPreparedDeploymentRejectsMisplacedBaseAndResolvesRedeploy(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	rc := &aktctx.Context{AuthMethod: aktctx.AuthMethodKeyring}
	require.ErrorContains(t, validatePreparedDeployment(cmd, "update", map[string]any{"base-sdl": "base.yaml"}, rc), "requires --patch")
	require.ErrorContains(t, validatePreparedDeployment(cmd, "update", map[string]any{"base-sdl": "base.yaml", "patch": true}, &aktctx.Context{AuthMethod: aktctx.AuthMethodConsoleAPI}), "chain rail")
	require.NoError(t, validatePreparedDeployment(cmd, "redeploy", map[string]any{"dseq": 42, "sdl-file": writeValidWorkflowSDL(t), "deposit": "5000000uact"}, rc))
}

func TestWorkflowInputFailuresDoNotConsumeSecrets(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	input := &unreadSecretInput{}
	cmd.SetIn(input)
	malformed := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(malformed, []byte("services: ["), 0600))
	for _, tc := range []struct {
		params map[string]any
		want   string
	}{
		{map[string]any{"secrets-file": "-"}, "Console context"},
		{map[string]any{"sdl-file": "/missing-input.yaml"}, "read SDL"},
		{map[string]any{"sdl-file": malformed}, "valid YAML"},
	} {
		_, err := resolveWorkflowParams(cmd, tc.params, func() *aktctx.Manager { return m }, func() string { return "chain" })
		require.ErrorContains(t, err, tc.want)
	}
	require.False(t, input.read)
	for _, value := range []any{false, "/missing-patch.yaml", t.TempDir(), malformed} {
		require.Error(t, validatePatchFile(value))
	}
}

func TestWorkflowPersistsActualRedeploySource(t *testing.T) {
	actual := writeSDL(t)
	state := wf.NewRunState("redeploy", "redeploy", persistOwner, map[string]any{})
	state.SetStepResult("create-deployment", &wf.StepResult{Status: "success", Output: map[string]any{"sdl_path": actual}})
	rec := &sstore.DeploymentRecord{}
	applySDL(rec, state)
	require.Equal(t, actual, rec.SDLPath)
	require.NotEmpty(t, rec.SDLHash)
}

func TestPatchReadFailureAfterOpeningRegularFile(t *testing.T) {
	// Linux exposes this process's address space as a regular readable file.
	// Reading unmapped offset zero fails with EIO, independently of scheduling.
	if _, err := os.Stat("/proc/self/mem"); os.IsNotExist(err) {
		t.Skip("Linux process memory is unavailable")
	}
	require.ErrorContains(t, validatePatchFile("/proc/self/mem"), "read patch file")
}
