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

	PrepareDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Name:      "prepare_duration_seconds",
		Help:      "Latency of NodePrepareResources calls.",
		Buckets:   prometheus.DefBuckets,
	})

	UnprepareDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Name:      "unprepare_duration_seconds",
		Help:      "Latency of NodeUnprepareResources calls.",
		Buckets:   prometheus.DefBuckets,
	})

	ResctrlGroupCreateTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "resctrl_group_create_total",
		Help:      "Total number of resctrl group creation attempts.",
	}, []string{"result"})

	ResctrlGroupDeleteTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "resctrl_group_delete_total",
		Help:      "Total number of resctrl group deletion attempts.",
	}, []string{"result"})

	ResctrlTaskAssignTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "resctrl_task_assign_total",
		Help:      "Total number of resctrl task assignment attempts (CDI hook).",
	}, []string{"result"})

	CacheOccupancyBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "cache_occupancy_bytes",
		Help:      "Current L3 cache occupancy in bytes per partition (CMT).",
	}, []string{"partition", "cache_group_id"})

	MemBandwidthLocalBytesPerSec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "memory_bandwidth_local_bytes_per_second",
		Help:      "Local memory bandwidth in bytes/second per partition (MBM).",
	}, []string{"partition", "cache_group_id"})

	MemBandwidthTotalBytesPerSec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "memory_bandwidth_total_bytes_per_second",
		Help:      "Total memory bandwidth in bytes/second per partition (MBM).",
	}, []string{"partition", "cache_group_id"})
)

func RegisterMetrics() {
	prometheus.MustRegister(
		PartitionsTotal,
		PartitionsAllocated,
		PrepareTotal,
		UnprepareTotal,
		PrepareDuration,
		UnprepareDuration,
		ResctrlGroupCreateTotal,
		ResctrlGroupDeleteTotal,
		ResctrlTaskAssignTotal,
		CacheOccupancyBytes,
		MemBandwidthLocalBytesPerSec,
		MemBandwidthTotalBytesPerSec,
	)
}
