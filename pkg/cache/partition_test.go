package cache

import (
	"strconv"
	"testing"

	"github.com/fabiendupont/k8s-dra-driver-cache-partition/pkg/resctrl"
)

func testCATInfo(numWays, numCLOSIDs int, cacheIDs []int) *resctrl.CATInfo {
	var cbm uint64
	for i := 0; i < numWays; i++ {
		cbm |= 1 << uint(i)
	}
	return &resctrl.CATInfo{
		NumCLOSIDs: numCLOSIDs,
		CBMMask:    cbm,
		MinCBMBits: 1,
		CacheIDs:   cacheIDs,
	}
}

func TestPartitionCache(t *testing.T) {
	tests := []struct {
		name      string
		numWays   int
		closids   int
		cacheIDs  []int
		count     int
		wantParts int
		wantWays  int
		wantErr   bool
	}{
		{
			name:      "20 ways into 4 partitions",
			numWays:   20,
			closids:   16,
			cacheIDs:  []int{0},
			count:     4,
			wantParts: 4,
			wantWays:  5,
		},
		{
			name:      "12 ways into 3 partitions, 2 caches",
			numWays:   12,
			closids:   16,
			cacheIDs:  []int{0, 1},
			count:     3,
			wantParts: 3,
			wantWays:  4,
		},
		{
			name:      "single partition",
			numWays:   20,
			closids:   16,
			cacheIDs:  []int{0},
			count:     1,
			wantParts: 1,
			wantWays:  20,
		},
		{
			name:     "too many partitions for CLOSIDs",
			numWays:  20,
			closids:  5,
			cacheIDs: []int{0},
			count:    5,
			wantErr:  true,
		},
		{
			name:     "zero partitions",
			numWays:  20,
			closids:  16,
			cacheIDs: []int{0},
			count:    0,
			wantErr:  true,
		},
		{
			name:     "more partitions than ways",
			numWays:  4,
			closids:  16,
			cacheIDs: []int{0},
			count:    8,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := testCATInfo(tt.numWays, tt.closids, tt.cacheIDs)
			perCache, err := PartitionCache(info, tt.count, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(perCache) != len(tt.cacheIDs) {
				t.Fatalf("got %d cache domains, want %d", len(perCache), len(tt.cacheIDs))
			}

			for _, parts := range perCache {
				if len(parts) != tt.wantParts {
					t.Fatalf("got %d partitions, want %d", len(parts), tt.wantParts)
				}
				for _, p := range parts {
					if p.Ways != tt.wantWays {
						t.Errorf("partition %s: ways = %d, want %d", p.ID, p.Ways, tt.wantWays)
					}
					if p.TotalWays != tt.numWays {
						t.Errorf("partition %s: totalWays = %d, want %d", p.ID, p.TotalWays, tt.numWays)
					}
				}
			}
		})
	}
}

func TestPartitionCacheNonOverlapping(t *testing.T) {
	info := testCATInfo(20, 16, []int{0})
	perCache, err := PartitionCache(info, 4, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := perCache[0]
	for i := 0; i < len(parts); i++ {
		cbmI, _ := strconv.ParseUint(parts[i].CBM, 16, 64)
		for j := i + 1; j < len(parts); j++ {
			cbmJ, _ := strconv.ParseUint(parts[j].CBM, 16, 64)
			if cbmI&cbmJ != 0 {
				t.Errorf("partitions %s (cbm=%s) and %s (cbm=%s) overlap",
					parts[i].ID, parts[i].CBM, parts[j].ID, parts[j].CBM)
			}
		}
	}

	var combined uint64
	for _, p := range parts {
		cbm, _ := strconv.ParseUint(p.CBM, 16, 64)
		combined |= cbm
	}
	if combined != info.CBMMask {
		t.Errorf("combined CBM %x does not cover full mask %x", combined, info.CBMMask)
	}
}

func TestFormatSchemata(t *testing.T) {
	tests := []struct {
		cacheID int
		cbm     string
		want    string
	}{
		{0, "1f", "L3:0=1f"},
		{1, "3e0", "L3:1=3e0"},
		{0, "fffff", "L3:0=fffff"},
	}

	for _, tt := range tests {
		got := FormatSchemata(tt.cacheID, tt.cbm)
		if got != tt.want {
			t.Errorf("FormatSchemata(%d, %q) = %q, want %q", tt.cacheID, tt.cbm, got, tt.want)
		}
	}
}

func TestUtilization(t *testing.T) {
	p := &CachePartition{Ways: 5, TotalWays: 20}
	u := p.Utilization()
	if u != 0.25 {
		t.Errorf("Utilization() = %f, want 0.25", u)
	}
}
