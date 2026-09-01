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

func findL3CacheID(cachePath string) string {
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
			return filepath.Join(cachePath, entry.Name(), "id")
		}
	}
	return ""
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
