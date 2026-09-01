package cache

import (
	"fmt"

	"github.com/fabiendupont/k8s-dra-driver-cache-partition/pkg/resctrl"
)

func PartitionCache(info *resctrl.CATInfo, count int) ([][]*CachePartition, error) {
	if count <= 0 {
		return nil, fmt.Errorf("partition count must be positive, got %d", count)
	}
	if count > info.NumCLOSIDs-1 {
		return nil, fmt.Errorf("partition count %d exceeds available CLOSIDs %d (one reserved for default)", count, info.NumCLOSIDs-1)
	}

	totalWays := info.NumWays()
	waysPerPartition := totalWays / count
	if waysPerPartition == 0 {
		return nil, fmt.Errorf("cannot divide %d cache ways into %d partitions", totalWays, count)
	}
	if waysPerPartition < info.MinCBMBits {
		return nil, fmt.Errorf("partition size %d ways is below minimum %d contiguous bits", waysPerPartition, info.MinCBMBits)
	}

	var allPartitions [][]*CachePartition
	for _, cacheID := range info.CacheIDs {
		var partitions []*CachePartition
		for i := 0; i < count; i++ {
			startWay := i * waysPerPartition
			cbm := buildCBM(startWay, waysPerPartition)

			partitions = append(partitions, &CachePartition{
				ID:           fmt.Sprintf("cache%d-part%d", cacheID, i),
				CacheID:      cacheID,
				Index:        i,
				Ways:         waysPerPartition,
				TotalWays:    totalWays,
				CBM:          fmt.Sprintf("%x", cbm),
				Level:        "L3",
				NUMANode:     -1,
				ResctrlGroup: fmt.Sprintf("dra-cache%d-part%d", cacheID, i),
			})
		}
		allPartitions = append(allPartitions, partitions)
	}

	return allPartitions, nil
}

func FormatSchemata(cacheID int, cbm string) string {
	return fmt.Sprintf("L3:%d=%s", cacheID, cbm)
}

func AllPartitions(perCache [][]*CachePartition) []*CachePartition {
	var all []*CachePartition
	for _, parts := range perCache {
		all = append(all, parts...)
	}
	return all
}

func buildCBM(startWay, numWays int) uint64 {
	var cbm uint64
	for i := startWay; i < startWay+numWays; i++ {
		cbm |= 1 << uint(i)
	}
	return cbm
}
