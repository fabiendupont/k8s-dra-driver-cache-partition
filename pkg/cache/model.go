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
