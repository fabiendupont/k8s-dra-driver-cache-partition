package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultSysCPUPath = "/sys/devices/system/cpu"
const defaultSysNodePath = "/sys/devices/system/node"

func LookupCacheNUMA(sysCPUPath, sysNodePath string) (map[int]int, error) {
	if sysCPUPath == "" {
		sysCPUPath = defaultSysCPUPath
	}
	if sysNodePath == "" {
		sysNodePath = defaultSysNodePath
	}

	cpuToNUMA, err := buildCPUToNUMA(sysNodePath)
	if err != nil {
		return nil, fmt.Errorf("building CPU-to-NUMA map: %w", err)
	}

	cacheToNUMA := make(map[int]int)
	entries, err := os.ReadDir(sysCPUPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysCPUPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cpu") {
			continue
		}
		cpuStr := strings.TrimPrefix(entry.Name(), "cpu")
		cpuID, err := strconv.Atoi(cpuStr)
		if err != nil {
			continue
		}

		l3IDPath := findL3CacheID(filepath.Join(sysCPUPath, entry.Name(), "cache"))
		if l3IDPath == "" {
			continue
		}

		data, err := os.ReadFile(l3IDPath)
		if err != nil {
			continue
		}
		cacheID, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}

		if _, exists := cacheToNUMA[cacheID]; exists {
			continue
		}

		if numaNode, ok := cpuToNUMA[cpuID]; ok {
			cacheToNUMA[cacheID] = numaNode
		}
	}

	if len(cacheToNUMA) == 0 {
		return nil, fmt.Errorf("no L3 cache-to-NUMA mappings found")
	}

	return cacheToNUMA, nil
}

func findL3CacheIndexDir(cachePath string) string {
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "index") {
			continue
		}
		levelPath := filepath.Join(cachePath, entry.Name(), "level")
		data, err := os.ReadFile(levelPath)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == "3" {
			return filepath.Join(cachePath, entry.Name())
		}
	}
	return ""
}

func findL3CacheID(cachePath string) string {
	dir := findL3CacheIndexDir(cachePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "id")
}

// LookupCacheCPUList returns a map from L3 cache domain ID to the shared_cpu_list string (e.g. "0-11").
func LookupCacheCPUList(sysCPUPath string) (map[int]string, error) {
	if sysCPUPath == "" {
		sysCPUPath = defaultSysCPUPath
	}
	cacheToCPU := make(map[int]string)
	entries, err := os.ReadDir(sysCPUPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysCPUPath, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cpu") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(entry.Name(), "cpu")); err != nil {
			continue
		}
		indexDir := findL3CacheIndexDir(filepath.Join(sysCPUPath, entry.Name(), "cache"))
		if indexDir == "" {
			continue
		}
		idData, err := os.ReadFile(filepath.Join(indexDir, "id"))
		if err != nil {
			continue
		}
		cacheID, err := strconv.Atoi(strings.TrimSpace(string(idData)))
		if err != nil {
			continue
		}
		if _, exists := cacheToCPU[cacheID]; exists {
			continue
		}
		cpuListData, err := os.ReadFile(filepath.Join(indexDir, "shared_cpu_list"))
		if err != nil {
			continue
		}
		cacheToCPU[cacheID] = strings.TrimSpace(string(cpuListData))
	}
	if len(cacheToCPU) == 0 {
		return nil, fmt.Errorf("no L3 CPU lists found in %s", sysCPUPath)
	}
	return cacheToCPU, nil
}

// LookupCacheSizes returns a map from L3 cache domain ID to total size in bytes.
func LookupCacheSizes(sysCPUPath string) (map[int]int64, error) {
	if sysCPUPath == "" {
		sysCPUPath = defaultSysCPUPath
	}
	cacheToSize := make(map[int]int64)
	entries, err := os.ReadDir(sysCPUPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysCPUPath, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cpu") {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(entry.Name(), "cpu")); err != nil {
			continue
		}
		indexDir := findL3CacheIndexDir(filepath.Join(sysCPUPath, entry.Name(), "cache"))
		if indexDir == "" {
			continue
		}
		idData, err := os.ReadFile(filepath.Join(indexDir, "id"))
		if err != nil {
			continue
		}
		cacheID, err := strconv.Atoi(strings.TrimSpace(string(idData)))
		if err != nil {
			continue
		}
		if _, exists := cacheToSize[cacheID]; exists {
			continue
		}
		sizeData, err := os.ReadFile(filepath.Join(indexDir, "size"))
		if err != nil {
			continue
		}
		sizeBytes, err := parseCacheSize(strings.TrimSpace(string(sizeData)))
		if err != nil {
			continue
		}
		cacheToSize[cacheID] = sizeBytes
	}
	if len(cacheToSize) == 0 {
		return nil, fmt.Errorf("no L3 cache sizes found in %s", sysCPUPath)
	}
	return cacheToSize, nil
}

func parseCacheSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	multiplier := int64(1)
	switch strings.ToUpper(s[len(s)-1:]) {
	case "K":
		multiplier = 1024
		s = s[:len(s)-1]
	case "M":
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case "G":
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return v * multiplier, nil
}

func buildCPUToNUMA(sysNodePath string) (map[int]int, error) {
	cpuToNUMA := make(map[int]int)
	entries, err := os.ReadDir(sysNodePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "node") {
			continue
		}
		nodeIDStr := strings.TrimPrefix(entry.Name(), "node")
		nodeID, err := strconv.Atoi(nodeIDStr)
		if err != nil {
			continue
		}

		cpulistPath := filepath.Join(sysNodePath, entry.Name(), "cpulist")
		data, err := os.ReadFile(cpulistPath)
		if err != nil {
			continue
		}

		cpus, err := parseCPUList(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}
		for _, cpu := range cpus {
			cpuToNUMA[cpu] = nodeID
		}
	}

	return cpuToNUMA, nil
}

func parseCPUList(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	var cpus []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if dashIdx := strings.Index(part, "-"); dashIdx >= 0 {
			start, err := strconv.Atoi(part[:dashIdx])
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(part[dashIdx+1:])
			if err != nil {
				return nil, err
			}
			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		} else {
			cpu, err := strconv.Atoi(part)
			if err != nil {
				return nil, err
			}
			cpus = append(cpus, cpu)
		}
	}
	return cpus, nil
}
