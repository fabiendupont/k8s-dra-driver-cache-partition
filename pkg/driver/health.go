package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// HealthServer exposes HTTP health endpoints for liveness and readiness probes.
type HealthServer struct {
	port       int
	ready      atomic.Bool
	resctrlDir string
	cdiSpecDir string
}

// NewHealthServer creates a health server on the given port.
// resctrlDir and cdiSpecDir are used for liveness checks.
func NewHealthServer(port int) *HealthServer {
	return &HealthServer{
		port:       port,
		resctrlDir: "/sys/fs/resctrl",
		cdiSpecDir: "/var/run/cdi",
	}
}

// MarkReady signals that the driver is ready to serve requests.
// Call this after ResourceSlice publication and CDI spec installation.
func (h *HealthServer) MarkReady() {
	h.ready.Store(true)
	klog.InfoS("Health server: marked ready")
}

type healthCheckResult struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Serve starts the health HTTP server. It blocks until ctx is cancelled.
func (h *HealthServer) Serve(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}
		healthy := true

		// Check resctrl mount is accessible.
		if _, err := os.Stat(h.resctrlDir); err != nil {
			checks["resctrl"] = fmt.Sprintf("unavailable: %v", err)
			healthy = false
		} else {
			checks["resctrl"] = "ok"
		}

		// Check CDI spec file exists and is non-empty.
		cdiSpecPath := h.cdiSpecDir + "/" + cdiVendor + "-" + cdiClass + ".json"
		if fi, err := os.Stat(cdiSpecPath); err != nil || fi.Size() == 0 {
			if err != nil {
				checks["cdi_spec"] = fmt.Sprintf("missing: %v", err)
			} else {
				checks["cdi_spec"] = "empty"
			}
			healthy = false
		} else {
			checks["cdi_spec"] = "ok"
		}

		result := healthCheckResult{Checks: checks}
		if healthy {
			result.Status = "healthy"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		} else {
			result.Status = "unhealthy"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		data, _ := json.Marshal(result)
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", h.port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutCtx); err != nil {
			klog.ErrorS(err, "Health server shutdown error")
		}
	}()

	klog.InfoS("Health server listening", "port", h.port)
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
