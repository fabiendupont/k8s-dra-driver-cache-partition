package cache

// CachePartition represents a partitioned slice of an L3 (or L2) cache.
type CachePartition struct {
	// ID uniquely identifies this partition, formatted as "cache<C>-part<P>".
	ID string

	// CacheID is the resctrl cache domain index (typically maps to a socket).
	CacheID int

	// Index is the partition index within the cache domain (0-based).
	Index int

	// Ways is the number of cache ways assigned to this partition.
	Ways int

	// TotalWays is the total number of cache ways in the cache domain.
	TotalWays int

	// CBM is the Cache Bit Mask as a hex string (e.g., "f" for 4 ways).
	CBM string

	// SizeBytes is the approximate size of this partition in bytes.
	SizeBytes int64

	// Level is the cache level ("L3" or "L2").
	Level string

	// NUMANode is the NUMA node associated with this cache domain.
	// Set to -1 if NUMA information is not available.
	NUMANode int

	// ResctrlGroup is the name of the resctrl group for this partition.
	ResctrlGroup string

	// WaySizeBytes is the size of one cache way in bytes (total / numWays).
	// Zero when the total cache size is unknown.
	WaySizeBytes int64

	// MinCBMBits is the minimum number of contiguous cache ways per partition
	// as reported by the hardware (info/L3/min_cbm_bits).
	MinCBMBits int

	// MBAThrottle is the memory bandwidth throttle value for this partition.
	// 0 means MBA is not configured. The unit is determined by MBAMode.
	MBAThrottle int

	// MBAMode is "percent" when MBAThrottle is a percentage (0-100),
	// "mbps" when it is in MB/s. Empty when MBA is not configured.
	MBAMode string

	// CPUList is the kernel compact CPU range string for all CPUs sharing this
	// cache domain (e.g. "0-11" or "0-3,8-11"). Empty when not available.
	CPUList string

	// SMBAThrottle is the slow-memory bandwidth throttle value. 0 = not configured.
	SMBAThrottle int

	// SMBAMode is "percent" or "mbps". Empty when SMBA is not configured.
	SMBAMode string

	// L2Ways is the number of L2 cache ways registered for this partition's group.
	// 0 means L2 CAT is not configured.
	L2Ways int

	// L2TotalWays is the total L2 cache ways available.
	L2TotalWays int

	// L2CBM is the L2 Cache Bit Mask hex string for this partition.
	// Empty when L2 CAT is not configured.
	L2CBM string
}

// Utilization returns the fraction of the cache assigned to this partition (0.0 to 1.0).
func (p *CachePartition) Utilization() float64 {
	return float64(p.Ways) / float64(p.TotalWays)
}

// CacheDomain represents a single cache domain (typically per-socket) discovered from resctrl.
type CacheDomain struct {
	// CacheID is the resctrl cache domain index.
	CacheID int

	// Level is "L3" or "L2".
	Level string

	// TotalWays is the number of cache ways in this domain.
	TotalWays int

	// FullCBM is the full cache bit mask (all ways set).
	FullCBM uint64

	// MinCBMBits is the minimum number of contiguous bits per mask.
	MinCBMBits int

	// NumCLOSIDs is the maximum number of Classes of Service.
	NumCLOSIDs int

	// SharedCPUList is the list of CPUs sharing this cache domain.
	SharedCPUList string

	// NUMANode is the NUMA node associated with this domain.
	NUMANode int
}
