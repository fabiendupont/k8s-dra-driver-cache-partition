package driver

import "github.com/prometheus/client_golang/prometheus"

const metricsNamespace = "dra_cache_partition"

var (
	PartitionsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "partitions_total",
		Help:      "Total number of cache partitions advertised by the driver.",
	})

	PartitionsAllocated = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "partitions_allocated",
		Help:      "Number of currently allocated cache partitions.",
	})

	PrepareTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "prepare_total",
		Help:      "Total number of NodePrepareResources calls.",
	}, []string{"result"})

	UnprepareTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "unprepare_total",
		Help:      "Total number of NodeUnprepareResources calls.",
	}, []string{"result"})
)

func RegisterMetrics() {
	prometheus.MustRegister(
		PartitionsTotal,
		PartitionsAllocated,
		PrepareTotal,
		UnprepareTotal,
	)
}
