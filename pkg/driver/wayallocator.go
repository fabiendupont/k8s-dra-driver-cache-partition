package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// WayAllocator tracks which cache ways are in use and allocates consecutive
// free bits on demand. Used in structured-parameters mode where resctrl groups
// are created per-claim rather than pre-partitioned at startup.
type WayAllocator struct {
	mu        sync.Mutex
	totalWays map[int]int    // cacheID → total way count
	allocated map[int]uint64 // cacheID → bitmask of ways currently in use
}

// NewWayAllocator creates an allocator with the given total ways per cache domain.
func NewWayAllocator(totalWays map[int]int) *WayAllocator {
	a := &WayAllocator{
		totalWays: make(map[int]int),
		allocated: make(map[int]uint64),
	}
	for id, n := range totalWays {
		a.totalWays[id] = n
		a.allocated[id] = 0
	}
	return a
}

// Allocate finds and reserves `ways` consecutive free bits in cache domain cacheID.
// Returns the CBM bitmask for the allocated ways.
func (a *WayAllocator) Allocate(cacheID, ways int) (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	total, ok := a.totalWays[cacheID]
	if !ok {
		return 0, fmt.Errorf("unknown cache domain %d", cacheID)
	}
	if ways <= 0 || ways > total {
		return 0, fmt.Errorf("requested %d ways, domain %d has %d total", ways, cacheID, total)
	}

	used := a.allocated[cacheID]
	// First-fit search for `ways` consecutive free bits.
	for start := 0; start+ways <= total; start++ {
		candidate := buildWayMask(start, ways)
		if used&candidate == 0 {
			a.allocated[cacheID] |= candidate
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no %d consecutive free ways in cache domain %d (used: %b)", ways, cacheID, used)
}

// Release frees the ways represented by cbm in cache domain cacheID.
func (a *WayAllocator) Release(cacheID int, cbm uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allocated[cacheID] &^= cbm
}

// FreeWays returns the number of unallocated ways in a cache domain.
func (a *WayAllocator) FreeWays(cacheID int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	used := a.allocated[cacheID]
	free := 0
	for i := 0; i < a.totalWays[cacheID]; i++ {
		if used>>uint(i)&1 == 0 {
			free++
		}
	}
	return free
}

// RebuildFromResctrl scans resctrl group directories matching groupPrefix
// and reconstructs the allocated bitmask from their schemata files.
// Call at startup for crash recovery.
func (a *WayAllocator) RebuildFromResctrl(resctrlRoot, groupPrefix string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	entries, err := os.ReadDir(resctrlRoot)
	if err != nil {
		return fmt.Errorf("reading resctrl root %s: %w", resctrlRoot, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), groupPrefix) {
			continue
		}
		schemataPath := filepath.Join(resctrlRoot, entry.Name(), "schemata")
		data, err := os.ReadFile(schemataPath)
		if err != nil {
			klog.V(4).InfoS("Skipping resctrl group (no schemata)", "group", entry.Name(), "error", err)
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "L3:") {
				continue
			}
			// Parse "L3:0=1f;1=3e0"
			for _, seg := range strings.Split(strings.TrimPrefix(line, "L3:"), ";") {
				eqIdx := strings.Index(seg, "=")
				if eqIdx < 0 {
					continue
				}
				cacheID, err := strconv.Atoi(seg[:eqIdx])
				if err != nil {
					continue
				}
				cbm, err := strconv.ParseUint(seg[eqIdx+1:], 16, 64)
				if err != nil {
					continue
				}
				a.allocated[cacheID] |= cbm
			}
		}
		klog.V(4).InfoS("Recovered CBM from resctrl group", "group", entry.Name())
	}
	return nil
}

func buildWayMask(start, count int) uint64 {
	var mask uint64
	for i := start; i < start+count; i++ {
		mask |= 1 << uint(i)
	}
	return mask
}
