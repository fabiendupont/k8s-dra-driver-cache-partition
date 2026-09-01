# Cache Partition DRA Driver

A Kubernetes [Dynamic Resource Allocation (DRA)](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/) driver that exposes deterministic L3 cache partitions using the Linux `resctrl` filesystem.

Each L3 cache is partitioned into non-overlapping slices of cache ways. These slices are advertised as DRA devices, allocated to pods via ResourceClaims, and enforced by assigning container processes to resctrl groups.

## How It Works

```
L3 Cache (cache ID 0), 20 ways, 4 partitions:

|---part0---|---part1---|---part2---|---part3---|
  ways 0-4    ways 5-9   ways 10-14  ways 15-19
  CBM=0x1f   CBM=0x3e0  CBM=0x7c00  CBM=0xf8000
  ~15MB       ~15MB       ~15MB       ~15MB
```

1. **Discover** — On startup, the driver reads `/sys/fs/resctrl/info/L3/` to discover the number of cache ways, CLOSIDs (max partitions), and cache IDs.

2. **Partition** — Cache ways are divided evenly across the configured partition count. Each partition gets a contiguous, non-overlapping set of ways expressed as a Cache Bit Mask (CBM). A resctrl group is created for each partition.

3. **Advertise** — Partitions are published as DRA devices in a ResourceSlice. Each device exposes attributes like `cacheWays`, `cacheSizeBytes`, `numaNode`, and `cacheLevel`.

4. **Allocate** — The Kubernetes scheduler picks a partition for each ResourceClaim. When the pod starts, kubelet calls `NodePrepareResources` and the driver records the allocation, returning CDI device IDs.

5. **Enforce** — At container creation, CRI-O reads the CDI spec and executes a `createRuntime` hook. The hook writes the container's init PID to the resctrl group's `tasks` file (`echo $PID > /sys/fs/resctrl/<group>/tasks`). This is a plain file write — no special syscalls, no kernel config restrictions.

6. **Release** — When the pod is deleted, `NodeUnprepareResources` frees the partition for reuse.

## Prerequisites

- **Kubernetes 1.34+** (DRA is GA since 1.34; the driver uses the `resource.k8s.io/v1` API)
- **CPU with L3 Cache Allocation Technology** (see [Hardware Compatibility](#hardware-compatibility))
- **resctrl filesystem mounted** (`mount -t resctrl resctrl /sys/fs/resctrl`)
- **Go 1.26+** for building from source

### Hardware Compatibility

The driver uses the `resctrl` filesystem, which is the unified Linux interface for cache partitioning across all CPU vendors:

| Platform | Technology | resctrl support |
|----------|-----------|----------------|
| Intel Xeon (v4+) | Resource Director Technology (RDT) / CAT | Yes |
| AMD EPYC | QoS Extensions / L3 CAT | Yes |
| ARM (Neoverse+) | MPAM (Memory Partitioning and Monitoring) | Yes (kernel 6.x+) |

Use [Node Feature Discovery (NFD)](#node-feature-discovery) to auto-detect compatible nodes.

## Project Layout

```
cmd/driver/            Main entrypoint (DRA plugin + CDI hook mode)
pkg/resctrl/           resctrl filesystem operations
pkg/cache/             Cache partition model, partitioning logic, NUMA discovery
pkg/driver/            DRA plugin, state, ResourceSlice publishing, CDI spec generation
deploy/helm/           Helm chart
deployments/           Raw Kubernetes manifests
test/e2e/              End-to-end tests
```

## Build

```bash
make build      # build binary to bin/dra-cache-partition
make test       # run unit tests
make image      # build container image with podman
```

## Deploy

### With Helm

```bash
helm install dra-cache-partition deploy/helm/dra-cache-partition/ \
  -n dra-cache-partition --create-namespace \
  --set driver.partitionCount=4
```

### With raw manifests

```bash
kubectl apply -f deployments/rbac.yaml
kubectl apply -f deployments/device-class.yaml
kubectl apply -f deployments/node-feature-rule.yaml   # optional, requires NFD
kubectl apply -f deployments/daemonset.yaml
```

### Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--partition-count` | `4` | Number of cache partitions per cache ID |
| `--resctrl-root` | `/sys/fs/resctrl` | Path to the resctrl filesystem mount |
| `--socket` | `/var/lib/kubelet/plugins/cache-partition.fabiendupont.io/plugin.sock` | DRA plugin socket path |
| `--health-port` | `8081` | Port for health (`/healthz`, `/readyz`) and metrics (`/metrics`) endpoints |
| `--cdi-dir` | `/var/run/cdi` | Directory for CDI spec files |
| `--registry-dir` | `/var/lib/kubelet/plugins_registry` | Kubelet plugin registry directory |

## Usage

### Create a ResourceClaim

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: my-cache-partition
spec:
  devices:
    requests:
      - name: cache
        exactly:
          deviceClassName: cache-partition-slices
          selectors:
            - cel:
                expression: >-
                  device.attributes['cache-partition.fabiendupont.io'].cacheLevel == 'L3'
```

### Available device attributes

All attributes are in the `cache-partition.fabiendupont.io` domain.

| Attribute | Type | Description |
|-----------|------|-------------|
| `cacheID` | int | Hardware cache ID (typically maps to socket) |
| `cacheLevel` | string | Cache level (`L3`) |
| `cacheWays` | int | Number of cache ways in this partition |
| `cacheTotalWays` | int | Total cache ways available on this cache ID |
| `cacheSizeBytes` | int | Approximate size of this partition in bytes |
| `cbmHex` | string | Cache Bit Mask in hexadecimal |
| `numaNode` | int | NUMA node ID (-1 if unavailable) |

### Reference the claim from a Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cache-workload
spec:
  containers:
    - name: workload
      image: my-app:latest
      command: ["/my-app"]
      resources:
        claims:
          - name: cache
  resourceClaims:
    - name: cache
      resourceClaimName: my-cache-partition
```

Once the pod is running, the container's processes are assigned to an exclusive L3 cache partition via the resctrl group.

## Architecture

```
                         ┌─────────────────────────────┐
                         │        kube-scheduler        │
                         │  allocates partitions from   │
                         │  ResourceSlice devices       │
                         └──────────────┬──────────────┘
                                        │
┌───────────────────────────────────────┼───────────────────────────────────┐
│ Node                                  │                                   │
│                                       │                                   │
│  ┌────────────────────────────────────▼──────────────────────────────┐    │
│  │                          kubelet                                  │    │
│  │  NodePrepareResources ──► driver records allocation               │    │
│  │                           returns CDI device IDs                  │    │
│  │                                                                   │    │
│  │  Container creation ──► CRI-O reads CDI spec                      │    │
│  │                          executes createRuntime hook              │    │
│  │                          ──► hook writes PID to                   │    │
│  │                               /sys/fs/resctrl/<group>/tasks       │    │
│  │                                                                   │    │
│  │  NodeUnprepareResources ► driver releases partition               │    │
│  └───────────────────────────────────────────────────────────────────┘    │
│                                                                           │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐                  │
│  │ Allocation   │   │    Slice     │   │  CDI Specs   │                  │
│  │   State      │   │  Publisher   │   │  (/var/run/  │                  │
│  │              │   │              │   │   cdi/)      │                  │
│  │ partition →  │   │ ResourceSlice│   │              │                  │
│  │  claim       │   │  with all    │   │ createRuntime│                  │
│  │  mapping     │   │  devices     │   │  hooks per   │                  │
│  │              │   │  + numaNode  │   │  partition   │                  │
│  └──────────────┘   └──────────────┘   └──────────────┘                  │
│                                                                           │
│  ┌──────────────────────────────────────────────────────┐                │
│  │                   resctrl filesystem                  │                │
│  │  /sys/fs/resctrl/                                     │                │
│  │    ├── info/L3/ (capabilities)                        │                │
│  │    ├── cache0-part0/ (CBM=0x1f, tasks=<PIDs>)        │                │
│  │    ├── cache0-part1/ (CBM=0x3e0, tasks=<PIDs>)       │                │
│  │    └── ...                                            │                │
│  └──────────────────────────────────────────────────────┘                │
└───────────────────────────────────────────────────────────────────────────┘
```

### CDI Hook Enforcement

The `dra-cache-partition` binary doubles as the CDI hook entry point. When invoked with `--cdi-hook --group=<name>`, it reads the container PID from the OCI state on stdin and writes it to `/sys/fs/resctrl/<group>/tasks`.

Unlike CPU scheduling policies (`SCHED_DEADLINE`), resctrl group assignment is a plain file write with no kernel config restrictions — it works on any kernel with resctrl support.

### Recovery

If the driver pod restarts, it recovers by listing all ResourceClaims from the API server and re-populating its allocation state for claims that belong to its driver and node. The resctrl groups persist across driver restarts.

## Node Feature Discovery

The driver includes a [NodeFeatureRule](deployments/node-feature-rule.yaml) that labels nodes with L3 CAT support:

```yaml
cache-partition.fabiendupont.io/resctrl-cat: "true"
```

The DaemonSet uses this label as a `nodeSelector`. Install the rule alongside NFD:

```bash
kubectl apply -f deployments/node-feature-rule.yaml
```

## Topology Coordinator Integration

The driver publishes a `numaNode` attribute per device, enabling integration with the [Node Partition Topology Coordinator](https://github.com/fabiendupont/k8s-dra-topology-coordinator). A pod can claim NUMA-aligned cache partitions alongside CPU time slots and GPUs:

```bash
kubectl apply -f deployments/topology-rule.yaml
```

## Metrics

The driver exposes Prometheus metrics on port 8081 at `/metrics`.

| Metric | Type | Description |
|--------|------|-------------|
| `dra_cache_partition_partitions_total` | gauge | Total number of cache partitions advertised |
| `dra_cache_partition_partitions_allocated` | gauge | Number of currently allocated partitions |
| `dra_cache_partition_prepare_total` | counter | NodePrepareResources calls (labels: `result=success\|error`) |
| `dra_cache_partition_unprepare_total` | counter | NodeUnprepareResources calls (labels: `result=success\|error`) |

## Configuration Examples

### Fewer partitions, more cache per partition

4 partitions — each gets 25% of the L3 cache:

```yaml
args:
  - --partition-count=4
```

### More partitions, finer granularity

8 partitions — each gets ~12.5% of the L3 cache:

```yaml
args:
  - --partition-count=8
```

### Hardware limits

The maximum partition count is limited by the number of CLOSIDs (typically 8–16). Check your hardware:

```bash
cat /sys/fs/resctrl/info/L3/num_closids
```

## Troubleshooting

### Pod stuck in `CreateContainerError`

The CDI hook failed to assign the PID to the resctrl group. Check:

1. **resctrl not mounted** — Verify `/sys/fs/resctrl` is mounted on the node:
   ```bash
   mount | grep resctrl
   ```

2. **resctrl group missing** — The driver creates groups at startup. Check the driver logs:
   ```bash
   kubectl -n dra-cache-partition logs -l app=dra-cache-partition
   ```

3. **Hook binary missing** — The driver copies itself to the plugin directory at startup:
   ```bash
   ls /var/lib/kubelet/plugins/cache-partition.fabiendupont.io/dra-cache-partition-hook
   ```

### Pod stuck in Pending

Check that the ResourceClaim is allocated:

```bash
kubectl get resourceclaim my-cache-partition -o yaml
```

Look for `status.allocation`. If missing, check that:
- The DeviceClass exists
- The driver DaemonSet is running and the ResourceSlice is published
- The node has the `cache-partition.fabiendupont.io/resctrl-cat=true` label

### Driver pod not starting

The DaemonSet requires the node label `cache-partition.fabiendupont.io/resctrl-cat=true`. Either:
- Install NFD and apply the NodeFeatureRule, or
- Label nodes manually: `kubectl label node <name> cache-partition.fabiendupont.io/resctrl-cat=true`

## License

See [LICENSE](LICENSE) for details.
