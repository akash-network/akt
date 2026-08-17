package governance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAllParamsStartsWithIndependentModuleMap(t *testing.T) {
	first := NewAllParams()
	second := NewAllParams()
	require.NotNil(t, first.Modules)
	require.Empty(t, first.Modules)

	first.Modules["gov"] = &ModuleParams{Module: "gov"}
	require.Empty(t, second.Modules)
}

func TestGetModuleDisplayName(t *testing.T) {
	require.Equal(t, "Governance", GetModuleDisplayName("gov"))
	require.Equal(t, "custom-module", GetModuleDisplayName("custom-module"))
}
