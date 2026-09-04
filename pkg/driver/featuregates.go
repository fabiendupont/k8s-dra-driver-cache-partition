package driver

import (
	"context"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// DRACapabilities reports which optional DRA feature gates are active on this cluster.
type DRACapabilities struct {
	// PartitionableDevices: SharedCounters + ConsumesCounters + AllowMultipleAllocations.
	PartitionableDevices bool
	// ConsumableCapacity: Device.Capacity with RequestPolicy (ValidRange/ValidValues).
	ConsumableCapacity bool
}

// ProbeFeatureGates detects which DRA feature gates are enabled by issuing
// dry-run ResourceSlice creates and checking whether gated fields are preserved.
func ProbeFeatureGates(ctx context.Context, client kubernetes.Interface, driverName, nodeName string) DRACapabilities {
	caps := DRACapabilities{}
	caps.PartitionableDevices = probePartitionableDevices(ctx, client, driverName, nodeName)
	caps.ConsumableCapacity = probeConsumableCapacity(ctx, client, driverName, nodeName)
	klog.InfoS("DRA feature gate probe complete",
		"partitionableDevices", caps.PartitionableDevices,
		"consumableCapacity", caps.ConsumableCapacity,
	)
	return caps
}

func probePartitionableDevices(ctx context.Context, client kubernetes.Interface, driverName, nodeName string) bool {
	probe := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "dra-probe-partitionable-" + nodeName},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   driverName,
			NodeName: &nodeName,
			Pool:     resourceapi.ResourcePool{Name: "probe", ResourceSliceCount: 1},
			SharedCounters: []resourceapi.CounterSet{
				{
					Name: "probe",
					Counters: map[string]resourceapi.Counter{
						"ways": {Value: resource.MustParse("1")},
					},
				},
			},
		},
	}
	result, err := client.ResourceV1().ResourceSlices().Create(ctx, probe, metav1.CreateOptions{
		DryRun: []string{metav1.DryRunAll},
	})
	if err != nil {
		klog.V(4).InfoS("DRAPartitionableDevices probe failed", "error", err)
		return false
	}
	enabled := len(result.Spec.SharedCounters) > 0
	klog.V(4).InfoS("DRAPartitionableDevices probe", "enabled", enabled)
	return enabled
}

func probeConsumableCapacity(ctx context.Context, client kubernetes.Interface, driverName, nodeName string) bool {
	rp := &resourceapi.CapacityRequestPolicy{
		Default: func() *resource.Quantity { q := resource.MustParse("1"); return &q }(),
	}
	probe := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "dra-probe-consumable-" + nodeName},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   driverName,
			NodeName: &nodeName,
			Pool:     resourceapi.ResourcePool{Name: "probe", ResourceSliceCount: 1},
			Devices: []resourceapi.Device{
				{
					Name: "probe",
					Capacity: map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{
						"ways": {
							Value:         resource.MustParse("20"),
							RequestPolicy: rp,
						},
					},
				},
			},
		},
	}
	result, err := client.ResourceV1().ResourceSlices().Create(ctx, probe, metav1.CreateOptions{
		DryRun: []string{metav1.DryRunAll},
	})
	if err != nil {
		klog.V(4).InfoS("DRAConsumableCapacity probe failed", "error", err)
		return false
	}
	enabled := len(result.Spec.Devices) > 0 &&
		result.Spec.Devices[0].Capacity["ways"].RequestPolicy != nil
	klog.V(4).InfoS("DRAConsumableCapacity probe", "enabled", enabled)
	return enabled
}
