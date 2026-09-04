package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// buildSysCPU creates a fake /sys/devices/system/cpu tree under dir.
// cpuCacheMap: cpu index -> (cacheID, totalSizeKB)
func buildSysCPU(t *testing.T, cpuCacheMap map[int][2]int) string {
	t.Helper()
	dir := t.TempDir()
	for cpuID, cc := range cpuCacheMap {
		cacheID, sizeKB := cc[0], cc[1]
		indexDir := filepath.Join(dir, fmt.Sprintf("cpu%d", cpuID), "cache", "index3")
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(indexDir, "level"), "3")
		writeFile(t, filepath.Join(indexDir, "id"), fmt.Sprintf("%d", cacheID))
		writeFile(t, filepath.Join(indexDir, "size"), fmt.Sprintf("%dK", sizeKB))
	}
	return dir
}

// buildSysNode creates a fake /sys/devices/system/node tree under dir.
// nodeMap: node index -> cpu list string (e.g. "0-3")
func buildSysNode(t *testing.T, nodeMap map[int]string) string {
	t.Helper()
	dir := t.TempDir()
	for nodeID, cpulist := range nodeMap {
		nodeDir := filepath.Join(dir, fmt.Sprintf("node%d", nodeID))
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(nodeDir, "cpulist"), cpulist)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseCPUList(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"0-3", []int{0, 1, 2, 3}},
		{"0,2,4", []int{0, 2, 4}},
		{"0-1,4-5", []int{0, 1, 4, 5}},
		{"", nil},
		{"7", []int{7}},
	}
	for _, tt := range tests {
		got, err := parseCPUList(tt.input)
		if err != nil {
			t.Errorf("parseCPUList(%q): %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseCPUList(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseCPUList(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseCPUList_Error(t *testing.T) {
	if _, err := parseCPUList("0-abc"); err == nil {
		t.Error("expected error for invalid range end, got nil")
	}
}

func TestParseCacheSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"20480K", 20 * 1024 * 1024},
		{"32M", 32 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"4096", 4096},
	}
	for _, tt := range tests {
		got, err := parseCacheSize(tt.input)
		if err != nil {
			t.Errorf("parseCacheSize(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseCacheSize(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
	if _, err := parseCacheSize(""); err == nil {
		t.Error("expected error for empty string")
	}
}

func TestLookupCacheNUMA_SingleSocket(t *testing.T) {
	// cpu0 and cpu1 share cache 0, both on node 0.
	cpuDir := buildSysCPU(t, map[int][2]int{0: {0, 20480}, 1: {0, 20480}})
	nodeDir := buildSysNode(t, map[int]string{0: "0-1"})

	m, err := LookupCacheNUMA(cpuDir, nodeDir)
	if err != nil {
		t.Fatalf("LookupCacheNUMA: %v", err)
	}
	if m[0] != 0 {
		t.Errorf("cache 0 -> NUMA %d, want 0", m[0])
	}
}

func TestLookupCacheNUMA_DualSocket(t *testing.T) {
	// cpu0 on cache 0 / node 0; cpu4 on cache 1 / node 1.
	cpuDir := buildSysCPU(t, map[int][2]int{0: {0, 20480}, 4: {1, 20480}})
	nodeDir := buildSysNode(t, map[int]string{0: "0-3", 1: "4-7"})

	m, err := LookupCacheNUMA(cpuDir, nodeDir)
	if err != nil {
		t.Fatalf("LookupCacheNUMA: %v", err)
	}
	if m[0] != 0 {
		t.Errorf("cache 0 -> NUMA %d, want 0", m[0])
	}
	if m[1] != 1 {
		t.Errorf("cache 1 -> NUMA %d, want 1", m[1])
	}
}

func TestLookupCacheSizes(t *testing.T) {
	cpuDir := buildSysCPU(t, map[int][2]int{0: {0, 20480}, 4: {1, 32768}})

	m, err := LookupCacheSizes(cpuDir)
	if err != nil {
		t.Fatalf("LookupCacheSizes: %v", err)
	}
	wantSize0 := int64(20480 * 1024)
	wantSize1 := int64(32768 * 1024)
	if m[0] != wantSize0 {
		t.Errorf("cache 0 size = %d, want %d", m[0], wantSize0)
	}
	if m[1] != wantSize1 {
		t.Errorf("cache 1 size = %d, want %d", m[1], wantSize1)
	}
}

func TestLookupCacheSizes_NoData(t *testing.T) {
	_, err := LookupCacheSizes(t.TempDir())
	if err == nil {
		t.Error("expected error for empty sysfs tree, got nil")
	}
}

func TestLookupCacheCPUList(t *testing.T) {
	dir := t.TempDir()
	// cpu0 on cache 0, cpu4 on cache 1 — write shared_cpu_list
	for cpuID, cc := range map[int][2]string{0: {"0", "0-3"}, 4: {"1", "4-7"}} {
		indexDir := filepath.Join(dir, fmt.Sprintf("cpu%d", cpuID), "cache", "index3")
		_ = os.MkdirAll(indexDir, 0755)
		writeFile(t, filepath.Join(indexDir, "level"), "3")
		writeFile(t, filepath.Join(indexDir, "id"), cc[0])
		writeFile(t, filepath.Join(indexDir, "shared_cpu_list"), cc[1])
	}

	m, err := LookupCacheCPUList(dir)
	if err != nil {
		t.Fatalf("LookupCacheCPUList: %v", err)
	}
	if m[0] != "0-3" {
		t.Errorf("cache 0 cpuList = %q, want %q", m[0], "0-3")
	}
	if m[1] != "4-7" {
		t.Errorf("cache 1 cpuList = %q, want %q", m[1], "4-7")
	}
}

func TestPartitionCacheWithCPUList(t *testing.T) {
	info := testCATInfo(20, 16, []int{0})
	cacheToCPU := map[int]string{0: "0-11"}
	perCache, err := PartitionCache(info, SpecsFromCount(4, info.NumWays()), nil, nil, cacheToCPU)
	if err != nil {
		t.Fatalf("PartitionCache: %v", err)
	}
	for _, parts := range perCache {
		for _, p := range parts {
			if p.CPUList != "0-11" {
				t.Errorf("partition %s CPUList = %q, want %q", p.ID, p.CPUList, "0-11")
			}
		}
	}
}

func TestPartitionCacheWithSize(t *testing.T) {
	cpuDir := buildSysCPU(t, map[int][2]int{0: {0, 20480}})

	cacheToSize, err := LookupCacheSizes(cpuDir)
	if err != nil {
		t.Fatalf("LookupCacheSizes: %v", err)
	}

	info := testCATInfo(20, 16, []int{0})
	perCache, err := PartitionCache(info, SpecsFromCount(4, info.NumWays()), nil, cacheToSize, nil)
	if err != nil {
		t.Fatalf("PartitionCache: %v", err)
	}

	totalSizeBytes := int64(20480 * 1024)
	expectedPartitionSize := totalSizeBytes / 4

	for _, parts := range perCache {
		for _, p := range parts {
			if p.SizeBytes != expectedPartitionSize {
				t.Errorf("partition %s SizeBytes = %d, want %d", p.ID, p.SizeBytes, expectedPartitionSize)
			}
		}
	}
}
