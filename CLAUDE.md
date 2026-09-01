# k8s-dra-driver-cache-partition

## Overview

Kubernetes DRA (Dynamic Resource Allocation) driver that exposes deterministic
L3 cache partitions using the Linux resctrl filesystem (Intel RDT, AMD QoS,
ARM MPAM). Cache ways are partitioned into non-overlapping slices, advertised
as ResourceSlices, and allocated to pods via ResourceClaims.

Cache partitioning is enforced at container creation via CDI hooks — CRI-O
executes a `createRuntime` hook that writes the container PID to the
resctrl group's `tasks` file.

## Project Layout

- `cmd/driver/` - DRA driver entrypoint (also CDI hook mode via `--cdi-hook`)
- `pkg/resctrl/` - resctrl filesystem operations (create/delete groups, assign PIDs)
- `pkg/cache/` - Cache partition model, partitioning logic, NUMA discovery
- `pkg/driver/` - DRA kubelet plugin (gRPC NodeServer), state, ResourceSlice publishing, CDI spec generation
- `deploy/helm/` - Helm chart
- `deployments/` - Raw Kubernetes manifests (DaemonSet, DeviceClass, RBAC, NFD rule, examples)

## Driver Name

`cache-partition.fabiendupont.io`

## Build

```bash
make build      # build binary
make test       # run tests
make image      # build container image
```

## Helm

```bash
helm install dra-cache-partition deploy/helm/dra-cache-partition/ -n dra-cache-partition --create-namespace
```

## Key Design Decisions

- Cache partitions are the atomic unit: one partition = one resctrl group = one DRA device
- Each partition is identified as `cache<N>-part<M>` (cache ID + partition index)
- Partitions within a cache ID share the same total ways; each gets contiguous non-overlapping ways
- Cache assignment is enforced via CDI `createRuntime` hooks that write the container PID to the resctrl group's `tasks` file — a plain file write with no kernel config restrictions
- Works on Intel (RDT/CAT), AMD (QoS), and ARM (MPAM) — all use the resctrl filesystem
- NFD NodeFeatureRule auto-detects nodes with L3 CAT support (cpu.cpuid.CAT_L3)
- NUMA node membership is discovered from sysfs and published as a device attribute for topology coordinator integration
