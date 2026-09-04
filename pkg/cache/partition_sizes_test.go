package cache

import "testing"

func TestParsePartitionSizes(t *testing.T) {
	tests := []struct {
		input   string
		want    []PartitionSpec
		wantErr bool
	}{
		{"1:8,2:4,4:2", []PartitionSpec{{1, 8}, {2, 4}, {4, 2}}, false},
		{"5:4", []PartitionSpec{{5, 4}}, false},
		{"", nil, true},
		{"bad", nil, true},
		{"1:x", nil, true},
		{"0:4", nil, true},
		{"4:0", nil, true},
	}
	for _, tt := range tests {
		got, err := ParsePartitionSizes(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParsePartitionSizes(%q): expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePartitionSizes(%q): %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParsePartitionSizes(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParsePartitionSizes(%q)[%d] = %v, want %v", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestPartitionCacheMultiSize(t *testing.T) {
	// 4 small (2-way) + 2 large (4-way) = 16 ways on a 20-way cache.
	info := testCATInfo(20, 16, []int{0})
	specs := []PartitionSpec{{Ways: 2, Count: 4}, {Ways: 4, Count: 2}}

	perCache, err := PartitionCache(info, specs, nil, nil, nil)
	if err != nil {
		t.Fatalf("PartitionCache: %v", err)
	}
	parts := perCache[0]
	if len(parts) != 6 {
		t.Fatalf("got %d partitions, want 6", len(parts))
	}
	// First 4 should be 2-way, last 2 should be 4-way.
	for i, p := range parts[:4] {
		if p.Ways != 2 {
			t.Errorf("part %d: ways = %d, want 2", i, p.Ways)
		}
	}
	for i, p := range parts[4:] {
		if p.Ways != 4 {
			t.Errorf("part %d: ways = %d, want 4", i+4, p.Ways)
		}
	}
	// Indices should be global (0-5).
	for i, p := range parts {
		if p.Index != i {
			t.Errorf("part %d: Index = %d, want %d", i, p.Index, i)
		}
	}
	// CBMs must not overlap and must fit within 20 ways.
	var combined uint64
	for _, p := range parts {
		var cbm uint64
		for _, c := range p.CBM {
			cbm <<= 4
			switch {
			case c >= '0' && c <= '9':
				cbm |= uint64(c - '0')
			case c >= 'a' && c <= 'f':
				cbm |= uint64(c-'a') + 10
			}
		}
		if combined&cbm != 0 {
			t.Errorf("partition %s CBM %s overlaps with previous partitions", p.ID, p.CBM)
		}
		combined |= cbm
	}
}

func TestPartitionCacheMultiSize_TooManyWays(t *testing.T) {
	info := testCATInfo(20, 16, []int{0})
	// 10*3 = 30 ways > 20 total.
	specs := []PartitionSpec{{Ways: 10, Count: 3}}
	_, err := PartitionCache(info, specs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for ways exceeding total, got nil")
	}
}

func TestSpecsFromCount(t *testing.T) {
	specs := SpecsFromCount(4, 20)
	total := 0
	for _, s := range specs {
		total += s.Ways * s.Count
	}
	if total != 20 {
		t.Errorf("SpecsFromCount(4,20): total ways = %d, want 20", total)
	}

	// count=0 should return nil.
	if SpecsFromCount(0, 20) != nil {
		t.Error("SpecsFromCount(0,20): expected nil")
	}
}
