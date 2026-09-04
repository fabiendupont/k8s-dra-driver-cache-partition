package driver

import (
	"testing"
)

func newTestAllocator() *WayAllocator {
	return NewWayAllocator(map[int]int{0: 20, 1: 20})
}

func TestWayAllocator_Basic(t *testing.T) {
	a := newTestAllocator()

	cbm, err := a.Allocate(0, 5)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if cbm == 0 {
		t.Fatal("Allocate returned zero CBM")
	}
	// Bits 0-4 should be set (first fit).
	wantCBM := uint64(0x1f)
	if cbm != wantCBM {
		t.Errorf("first Allocate(0,5) = %b, want %b", cbm, wantCBM)
	}
	if a.FreeWays(0) != 15 {
		t.Errorf("FreeWays after allocating 5 = %d, want 15", a.FreeWays(0))
	}

	// Second allocation should use bits 5-9.
	cbm2, err := a.Allocate(0, 5)
	if err != nil {
		t.Fatalf("second Allocate: %v", err)
	}
	if cbm2&cbm != 0 {
		t.Errorf("second allocation overlaps first: %b & %b = %b", cbm2, cbm, cbm2&cbm)
	}

	a.Release(0, cbm)
	if a.FreeWays(0) != 15 {
		t.Errorf("FreeWays after release = %d, want 15", a.FreeWays(0))
	}
}

func TestWayAllocator_Exhaustion(t *testing.T) {
	a := NewWayAllocator(map[int]int{0: 4})

	_, err := a.Allocate(0, 3)
	if err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	// Only 1 way left, requesting 2 should fail.
	_, err = a.Allocate(0, 2)
	if err == nil {
		t.Fatal("expected error for exhausted ways, got nil")
	}
}

func TestWayAllocator_UnknownDomain(t *testing.T) {
	a := newTestAllocator()
	_, err := a.Allocate(99, 1)
	if err == nil {
		t.Fatal("expected error for unknown domain, got nil")
	}
}

func TestWayAllocator_ReleaseAndReallocate(t *testing.T) {
	a := NewWayAllocator(map[int]int{0: 4})

	cbm1, _ := a.Allocate(0, 2) // bits 0-1
	cbm2, _ := a.Allocate(0, 2) // bits 2-3
	a.Release(0, cbm1)           // free bits 0-1

	// Should reallocate bits 0-1.
	cbm3, err := a.Allocate(0, 2)
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if cbm3&cbm2 != 0 {
		t.Errorf("reallocated bits overlap with still-allocated region")
	}
}

func TestParseCacheWayDeviceName(t *testing.T) {
	tests := []struct {
		name    string
		cacheID int
		ways    int
		wantErr bool
	}{
		{"cache0-5way", 0, 5, false},
		{"cache1-20way", 1, 20, false},
		{"cache0-part0", 0, 0, true},
		{"notadevice", 0, 0, true},
	}
	for _, tt := range tests {
		cacheID, ways, err := parseCacheWayDeviceName(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseCacheWayDeviceName(%q): expected error", tt.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCacheWayDeviceName(%q): %v", tt.name, err)
			continue
		}
		if cacheID != tt.cacheID || ways != tt.ways {
			t.Errorf("parseCacheWayDeviceName(%q) = (%d,%d), want (%d,%d)",
				tt.name, cacheID, ways, tt.cacheID, tt.ways)
		}
	}
}
