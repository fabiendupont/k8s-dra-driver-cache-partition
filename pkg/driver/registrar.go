package driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"

	drav1 "k8s.io/kubelet/pkg/apis/dra/v1"
)

// Registrar implements the kubelet plugin registration gRPC service.
type Registrar struct {
	registerapi.UnimplementedRegistrationServer
	driverName string
	endpoint   string
}

// NewRegistrar creates a plugin registrar.
func NewRegistrar(driverName, endpoint string) *Registrar {
	return &Registrar{
		driverName: driverName,
		endpoint:   endpoint,
	}
}

// GetInfo is called by the kubelet plugin watcher.
func (r *Registrar) GetInfo(ctx context.Context, req *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	klog.InfoS("GetInfo called by kubelet plugin watcher")
	return &registerapi.PluginInfo{
		Type:              registerapi.DRAPlugin,
		Name:              r.driverName,
		Endpoint:          r.endpoint,
		SupportedVersions: []string{drav1.DRAPluginService},
	}, nil
}

// NotifyRegistrationStatus is called by the kubelet to inform the plugin
// whether registration was successful.
func (r *Registrar) NotifyRegistrationStatus(ctx context.Context, status *registerapi.RegistrationStatus) (*registerapi.RegistrationStatusResponse, error) {
	if status.PluginRegistered {
		klog.InfoS("Plugin registered with kubelet successfully",
			"driver", r.driverName)
	} else {
		klog.ErrorS(nil, "Plugin registration failed",
			"driver", r.driverName, "error", status.Error)
	}
	return &registerapi.RegistrationStatusResponse{}, nil
}

// Serve starts the registration gRPC server. It blocks until ctx is cancelled.
func (r *Registrar) Serve(ctx context.Context, registryDir string) error {
	socketPath := filepath.Join(registryDir, r.driverName+"-reg.sock")

	if err := os.MkdirAll(registryDir, 0750); err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", socketPath, err)
	}

	server := grpc.NewServer()
	registerapi.RegisterRegistrationServer(server, r)

	go func() {
		<-ctx.Done()
		klog.InfoS("Shutting down registration server")
		server.GracefulStop()
		_ = os.Remove(socketPath)
	}()

	klog.InfoS("Registration server listening",
		"socket", socketPath, "driver", r.driverName)
	return server.Serve(listener)
}
