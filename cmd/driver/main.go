package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	drav1 "k8s.io/kubelet/pkg/apis/dra/v1"

	"github.com/fabiendupont/k8s-dra-driver-cache-partition/pkg/cache"
	"github.com/fabiendupont/k8s-dra-driver-cache-partition/pkg/driver"
	"github.com/fabiendupont/k8s-dra-driver-cache-partition/pkg/resctrl"
)

const driverName = "cache-partition.fabiendupont.io"

func main() {
	// CDI hook mode: invoked by CRI-O as an OCI hook to assign the container
	// PID to a resctrl group. Must run before flag.Parse().
	if len(os.Args) > 1 && os.Args[1] == "--cdi-hook" {
		if err := runCDIHook(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "cdi-hook: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	var (
		socketPath     string
		registryDir    string
		nodeName       string
		partitionCount int
		resctrlRoot    string
		healthPort     int
		cdiDir         string
	)

	flag.StringVar(&socketPath, "socket", "/var/lib/kubelet/plugins/cache-partition.fabiendupont.io/plugin.sock", "DRA plugin gRPC socket path")
	flag.StringVar(&registryDir, "registry-dir", "/var/lib/kubelet/plugins_registry", "Kubelet plugin registry directory")
	flag.StringVar(&nodeName, "node-name", "", "Kubernetes node name")
	flag.IntVar(&partitionCount, "partition-count", 4, "Number of cache partitions per cache domain")
	flag.StringVar(&resctrlRoot, "resctrl-root", "/sys/fs/resctrl", "Root path of the resctrl filesystem")
	flag.IntVar(&healthPort, "health-port", 8081, "Port for health check endpoints (/healthz, /readyz)")
	flag.StringVar(&cdiDir, "cdi-dir", "/var/run/cdi", "Directory for CDI spec files")

	klog.InitFlags(nil)
	flag.Parse()

	if nodeName == "" {
		nodeName = os.Getenv("NODE_NAME")
	}
	if nodeName == "" {
		klog.Fatal("--node-name or NODE_NAME required")
	}

	resctrl.SetRoot(resctrlRoot)

	if !resctrl.IsAvailable() {
		klog.Fatalf("resctrl/CAT not available at %s", resctrlRoot)
	}

	info, err := resctrl.Info()
	if err != nil {
		klog.Fatalf("Failed to read resctrl info: %v", err)
	}

	klog.InfoS("Detected cache allocation capabilities",
		"numCLOSIDs", info.NumCLOSIDs,
		"totalWays", info.NumWays(),
		"cbmMask", fmt.Sprintf("%x", info.CBMMask),
		"minCBMBits", info.MinCBMBits,
		"cacheIDs", info.CacheIDs,
	)

	cacheToNUMA, numaErr := cache.LookupCacheNUMA("", "")
	if numaErr != nil {
		klog.InfoS("NUMA lookup unavailable, cache partitions will have numaNode=-1", "error", numaErr)
	}

	perCache, err := cache.PartitionCache(info, partitionCount, cacheToNUMA)
	if err != nil {
		klog.Fatalf("Failed to partition cache: %v", err)
	}

	allPartitions := cache.AllPartitions(perCache)
	klog.InfoS("Partitioned cache domains",
		"cacheIDs", len(perCache),
		"partitionsPerCache", partitionCount,
		"totalPartitions", len(allPartitions),
	)

	// Create resctrl groups for each partition.
	for _, parts := range perCache {
		for _, p := range parts {
			schemata := cache.FormatSchemata(p.CacheID, p.CBM)
			if err := resctrl.CreateGroup(p.ResctrlGroup, schemata); err != nil {
				klog.Fatalf("Failed to create resctrl group %s: %v", p.ResctrlGroup, err)
			}
			klog.V(2).InfoS("Created resctrl group",
				"group", p.ResctrlGroup, "schemata", schemata)
		}
	}
	defer func() {
		for _, parts := range perCache {
			for _, p := range parts {
				_ = resctrl.DeleteGroup(p.ResctrlGroup)
			}
		}
	}()

	state := driver.NewAllocationState(perCache)

	// Install hook binary and write CDI specs.
	pluginDir := filepath.Dir(socketPath)
	if err := os.MkdirAll(pluginDir, 0750); err != nil {
		klog.Fatalf("Failed to create plugin directory: %v", err)
	}
	hookBinaryPath, err := driver.InstallHookBinary(pluginDir)
	if err != nil {
		klog.Fatalf("Failed to install hook binary: %v", err)
	}

	if err := driver.WriteCDISpecs(cdiDir, hookBinaryPath, perCache); err != nil {
		klog.Fatalf("Failed to write CDI specs: %v", err)
	}
	defer driver.CleanupCDISpecs(cdiDir)

	driver.RegisterMetrics()
	driver.PartitionsTotal.Set(float64(len(allPartitions)))

	kubeClient, err := buildKubeClient()
	if err != nil {
		klog.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	publisher := driver.NewSlicePublisher(kubeClient, driverName, nodeName)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	recovered, recoverErr := driver.RecoverAllocations(ctx, kubeClient, driverName, nodeName, state)
	if recoverErr != nil {
		klog.ErrorS(recoverErr, "Partial failure recovering allocations", "recovered", recovered)
	}
	if recovered > 0 {
		driver.PartitionsAllocated.Set(float64(recovered))
		klog.InfoS("Recovered partitions from previous allocations", "partitions", recovered)
	}

	if err := publisher.PublishSlices(ctx, perCache); err != nil {
		klog.Fatalf("Failed to publish ResourceSlices: %v", err)
	}

	drv := driver.NewDriver(driverName, nodeName, kubeClient, state, publisher)

	healthServer := driver.NewHealthServer(healthPort)
	go func() {
		if err := healthServer.Serve(ctx); err != nil {
			klog.Fatalf("Health server error: %v", err)
		}
	}()

	healthServer.MarkReady()

	registrar := driver.NewRegistrar(driverName, socketPath)
	go func() {
		if err := registrar.Serve(ctx, registryDir); err != nil {
			klog.Fatalf("Registration server error: %v", err)
		}
	}()

	if err := runGRPCServer(ctx, socketPath, drv); err != nil {
		klog.Fatalf("gRPC server error: %v", err)
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if err := publisher.DeleteSlice(cleanupCtx); err != nil {
		klog.ErrorS(err, "Failed to delete ResourceSlice during shutdown")
	}
}

// runCDIHook is the CDI hook entry point. It reads OCI state from stdin to
// get the container PID, then writes that PID to the resctrl group's tasks file.
func runCDIHook(args []string) error {
	var group string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--group=") {
			group = arg[len("--group="):]
		}
	}
	if group == "" {
		return fmt.Errorf("--group is required")
	}

	var state struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&state); err != nil {
		return fmt.Errorf("reading OCI state: %w", err)
	}
	if state.PID == 0 {
		return fmt.Errorf("OCI state has no PID")
	}

	if err := resctrl.AssignPID(group, state.PID); err != nil {
		return fmt.Errorf("assigning pid %d to resctrl group %s: %w", state.PID, group, err)
	}

	fmt.Fprintf(os.Stderr, "Assigned container pid %d to resctrl group %s\n", state.PID, group)
	return nil
}

func runGRPCServer(ctx context.Context, socketPath string, drv *driver.Driver) error {
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", socketPath, err)
	}

	server := grpc.NewServer()
	drav1.RegisterDRAPluginServer(server, drv)

	go func() {
		<-ctx.Done()
		klog.InfoS("Shutting down gRPC server")
		server.GracefulStop()
	}()

	klog.InfoS("gRPC server listening", "socket", socketPath)
	return server.Serve(listener)
}

func buildKubeClient() (kubernetes.Interface, error) {
	var client kubernetes.Interface
	var lastErr error

	for attempt := 0; attempt < 10; attempt++ {
		config, err := rest.InClusterConfig()
		if err != nil {
			lastErr = fmt.Errorf("building in-cluster config: %w", err)
			klog.InfoS("Waiting for in-cluster config", "attempt", attempt+1, "error", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		client, err = kubernetes.NewForConfig(config)
		if err != nil {
			lastErr = fmt.Errorf("creating kubernetes client: %w", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		_, err = client.Discovery().ServerVersion()
		if err != nil {
			lastErr = fmt.Errorf("verifying API server connectivity: %w", err)
			klog.InfoS("API server not reachable yet", "attempt", attempt+1, "error", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		return client, nil
	}
	return nil, lastErr
}
