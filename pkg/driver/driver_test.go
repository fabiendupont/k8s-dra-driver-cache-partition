package driver

import (
	"context"
	"testing"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	drav1 "k8s.io/kubelet/pkg/apis/dra/v1"
)

func newTestDriver(t *testing.T, client *fake.Clientset) *Driver {
	t.Helper()
	s := makeState("cache0-part0", "cache0-part1")
	publisher := NewSlicePublisher(client, testDriver, testNode, DRACapabilities{})
	return NewDriver(testDriver, testNode, client, s, publisher, DRACapabilities{}, nil, nil, "", "")
}

func makeAllocatedClaim(client *fake.Clientset, ns, name, uid, device string) {
	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid)},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{Driver: testDriver, Pool: testNode, Device: device, Request: "my-req"},
					},
				},
			},
		},
	}
	if _, err := client.ResourceV1().ResourceClaims(ns).Create(context.Background(), claim, metav1.CreateOptions{}); err != nil {
		panic(err)
	}
}

func TestNodePrepareResources_HappyPath(t *testing.T) {
	client := fake.NewClientset()
	drv := newTestDriver(t, client)
	makeAllocatedClaim(client, "default", "my-claim", "uid-1", "cache0-part0")

	req := &drav1.NodePrepareResourcesRequest{
		Claims: []*drav1.Claim{{Uid: "uid-1", Name: "my-claim", Namespace: "default"}},
	}
	resp, err := drv.NodePrepareResources(context.Background(), req)
	if err != nil {
		t.Fatalf("NodePrepareResources: %v", err)
	}

	claimResp := resp.Claims["uid-1"]
	if claimResp == nil {
		t.Fatal("no response for uid-1")
	}
	if claimResp.Error != "" {
		t.Fatalf("claim error: %s", claimResp.Error)
	}
	if len(claimResp.Devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(claimResp.Devices))
	}
	if claimResp.Devices[0].DeviceName != "cache0-part0" {
		t.Errorf("device name = %q, want cache0-part0", claimResp.Devices[0].DeviceName)
	}
}

func TestNodePrepareResources_ClaimNotFound(t *testing.T) {
	client := fake.NewClientset()
	drv := newTestDriver(t, client)

	req := &drav1.NodePrepareResourcesRequest{
		Claims: []*drav1.Claim{{Uid: "uid-x", Name: "no-claim", Namespace: "default"}},
	}
	resp, err := drv.NodePrepareResources(context.Background(), req)
	if err != nil {
		t.Fatalf("NodePrepareResources returned gRPC error: %v", err)
	}

	claimResp := resp.Claims["uid-x"]
	if claimResp == nil || claimResp.Error == "" {
		t.Fatal("expected error response for missing claim")
	}
}

func TestNodePrepareResources_WrongDriver(t *testing.T) {
	client := fake.NewClientset()
	drv := newTestDriver(t, client)

	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "foreign-claim", Namespace: "default", UID: "uid-2"},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{Driver: "other-driver", Pool: testNode, Device: "cache0-part0", Request: "r"},
					},
				},
			},
		},
	}
	if _, err := client.ResourceV1().ResourceClaims("default").Create(context.Background(), claim, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating claim: %v", err)
	}

	req := &drav1.NodePrepareResourcesRequest{
		Claims: []*drav1.Claim{{Uid: "uid-2", Name: "foreign-claim", Namespace: "default"}},
	}
	resp, _ := drv.NodePrepareResources(context.Background(), req)
	claimResp := resp.Claims["uid-2"]
	if claimResp == nil || claimResp.Error == "" {
		t.Fatal("expected error response for claim with wrong driver")
	}
}

func TestNodeUnprepareResources(t *testing.T) {
	client := fake.NewClientset()
	drv := newTestDriver(t, client)
	makeAllocatedClaim(client, "default", "my-claim", "uid-1", "cache0-part0")

	prepReq := &drav1.NodePrepareResourcesRequest{
		Claims: []*drav1.Claim{{Uid: "uid-1", Name: "my-claim", Namespace: "default"}},
	}
	if _, err := drv.NodePrepareResources(context.Background(), prepReq); err != nil {
		t.Fatalf("NodePrepareResources: %v", err)
	}
	if drv.state.AllocatedCount() != 1 {
		t.Fatalf("expected 1 allocated partition after prepare, got %d", drv.state.AllocatedCount())
	}

	unprepReq := &drav1.NodeUnprepareResourcesRequest{
		Claims: []*drav1.Claim{{Uid: "uid-1", Name: "my-claim", Namespace: "default"}},
	}
	resp, err := drv.NodeUnprepareResources(context.Background(), unprepReq)
	if err != nil {
		t.Fatalf("NodeUnprepareResources: %v", err)
	}
	claimResp := resp.Claims["uid-1"]
	if claimResp == nil {
		t.Fatal("no response for uid-1 in unprepare")
	}
	if claimResp.Error != "" {
		t.Fatalf("unprepare error: %s", claimResp.Error)
	}
	if drv.state.AllocatedCount() != 0 {
		t.Fatalf("expected 0 allocated partitions after unprepare, got %d", drv.state.AllocatedCount())
	}
}
