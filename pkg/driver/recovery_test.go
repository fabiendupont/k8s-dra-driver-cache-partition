package driver

import (
	"context"
	"testing"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRecoverAllocations(t *testing.T) {
	client := fake.NewClientset()
	s := makeState("cache0-part0", "cache0-part1", "cache0-part2")

	// Create a ResourceClaim allocated on testNode with two partitions.
	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-claim",
			Namespace: "default",
			UID:       types.UID("uid-1"),
		},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{Driver: testDriver, Pool: testNode, Device: "cache0-part0", Request: "req"},
						{Driver: testDriver, Pool: testNode, Device: "cache0-part1", Request: "req"},
						{Driver: "other-driver", Pool: testNode, Device: "cache0-part2", Request: "req"},
					},
				},
			},
		},
	}

	if _, err := client.ResourceV1().ResourceClaims("default").Create(context.Background(), claim, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating claim: %v", err)
	}

	recovered, err := RecoverAllocations(context.Background(), client, testDriver, testNode, s)
	if err != nil {
		t.Fatalf("RecoverAllocations: %v", err)
	}
	if recovered != 2 {
		t.Errorf("recovered = %d, want 2", recovered)
	}
	if s.AllocatedCount() != 2 {
		t.Errorf("AllocatedCount = %d, want 2", s.AllocatedCount())
	}
}

func TestRecoverAllocations_NoAllocation(t *testing.T) {
	client := fake.NewClientset()
	s := makeState("cache0-part0")

	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-claim", Namespace: "default"},
	}
	if _, err := client.ResourceV1().ResourceClaims("default").Create(context.Background(), claim, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating claim: %v", err)
	}

	recovered, err := RecoverAllocations(context.Background(), client, testDriver, testNode, s)
	if err != nil {
		t.Fatalf("RecoverAllocations: %v", err)
	}
	if recovered != 0 {
		t.Errorf("recovered = %d, want 0 for unallocated claim", recovered)
	}
}
