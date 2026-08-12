package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/resource"
	inventoryv1 "pkg.akt.dev/go/inventory/v1"
	providerv1 "pkg.akt.dev/go/provider/v1"
)

func TestExtractNodesWithGPUCalculatesAvailableResources(t *testing.T) {
	status := &providerv1.Status{
		Cluster: &providerv1.ClusterStatus{
			Inventory: providerv1.Inventory{
				Cluster: inventoryv1.Cluster{
					Nodes: inventoryv1.Nodes{
						{
							Name: "balanced",
							Resources: inventoryv1.NodeResources{
								CPU: inventoryv1.CPU{Quantity: inventoryv1.ResourcePair{
									Allocatable: testQuantity("8"),
									Allocated:   testQuantity("2500m"),
								}},
								Memory: inventoryv1.Memory{Quantity: inventoryv1.ResourcePair{
									Allocatable: testQuantity("64Gi"),
									Allocated:   testQuantity("16Gi"),
								}},
								GPU: inventoryv1.GPU{
									Quantity: inventoryv1.ResourcePair{
										Allocatable: testQuantity("2"),
										Allocated:   testQuantity("1"),
									},
									Info: inventoryv1.GPUInfoS{{
										Vendor:     "nvidia",
										VendorID:   "10de",
										Name:       "A100",
										ModelID:    "20b0",
										Interface:  "PCIe",
										MemorySize: "80Gi",
									}},
								},
							},
						},
						{
							Name: "overcommitted",
							Resources: inventoryv1.NodeResources{
								CPU: inventoryv1.CPU{Quantity: inventoryv1.ResourcePair{
									Allocatable: testQuantity("2"),
									Allocated:   testQuantity("3"),
								}},
								Memory: inventoryv1.Memory{Quantity: inventoryv1.ResourcePair{
									Allocatable: testQuantity("1Ki"),
								}},
								GPU: inventoryv1.GPU{Quantity: inventoryv1.ResourcePair{
									Allocatable: testQuantity("3"),
									Allocated:   testQuantity("4"),
								}},
							},
						},
						{
							Name: "allocated-only",
							Resources: inventoryv1.NodeResources{
								CPU:    inventoryv1.CPU{Quantity: inventoryv1.ResourcePair{Allocated: testQuantity("1")}},
								Memory: inventoryv1.Memory{Quantity: inventoryv1.ResourcePair{Allocated: testQuantity("1Ki")}},
								GPU:    inventoryv1.GPU{Quantity: inventoryv1.ResourcePair{Allocated: testQuantity("1")}},
							},
						},
					},
				},
			},
		},
	}

	require.Equal(t, []ProviderNodeWithGPU{
		{
			Name:           "balanced",
			CPUAllocatable: 8000,
			CPUAvailable:   5500,
			MemAllocatable: 64 * 1024 * 1024 * 1024,
			MemAvailable:   48 * 1024 * 1024 * 1024,
			GPUAllocatable: 2,
			GPUAvailable:   1,
			GPUs: []GPUInfo{{
				Vendor:     "nvidia",
				VendorID:   "10de",
				Name:       "A100",
				ModelID:    "20b0",
				Interface:  "PCIe",
				MemorySize: "80Gi",
			}},
		},
		{
			Name:           "overcommitted",
			CPUAllocatable: 2000,
			CPUAvailable:   2000,
			MemAllocatable: 1024,
			MemAvailable:   1024,
			GPUAllocatable: 3,
			GPUAvailable:   3,
		},
		{Name: "allocated-only"},
	}, extractNodesWithGPU(status))
}

func TestProviderGRPCHelpersHandleEmptyAndInvalidInputs(t *testing.T) {
	require.Nil(t, extractNodesWithGPU(nil))
	require.Nil(t, extractNodesWithGPU(&providerv1.Status{}))
	require.Nil(t, extractGPUInfo(nil))

	for _, tc := range []struct {
		name string
		uri  string
		want string
	}{
		{name: "hostname", uri: "https://provider.example.com:8443/path", want: "provider.example.com:8444"},
		{name: "IPv4", uri: "http://192.0.2.10", want: "192.0.2.10:8444"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := convertToGRPCEndpoint(tc.uri)
			require.NoError(t, err)
			require.Equal(t, tc.want, endpoint)
		})
	}

	for _, uri := range []string{"provider.example.com:8443", "https:///missing-host", "%"} {
		_, err := convertToGRPCEndpoint(uri)
		require.Error(t, err, "URI %q", uri)
	}

	_, err := QueryProviderStatusGRPC(context.Background(), "provider.example.com:8443", false)
	require.ErrorContains(t, err, "invalid host URI")
}

func TestQueryProviderStatusGRPCRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := QueryProviderStatusGRPC(ctx, "https://127.0.0.1", true)
	require.ErrorContains(t, err, "failed to get provider status")
	require.Equal(t, codes.Canceled, status.Code(err))
}

func testQuantity(value string) *resource.Quantity {
	quantity := resource.MustParse(value)
	return &quantity
}
