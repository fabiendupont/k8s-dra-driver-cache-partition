package resctrl

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestResctrl(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	SetRoot(tmpDir)
	t.Cleanup(func() { SetRoot(DefaultRoot) })

	infoDir := filepath.Join(tmpDir, "info", "L3")
	_ = os.MkdirAll(infoDir, 0755)
	_ = os.WriteFile(filepath.Join(infoDir, "num_closids"), []byte("16\n"), 0644)
	_ = os.WriteFile(filepath.Join(infoDir, "cbm_mask"), []byte("fffff\n"), 0644)
	_ = os.WriteFile(filepath.Join(infoDir, "min_cbm_bits"), []byte("1\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schemata"), []byte("L3:0=fffff;1=fffff\n"), 0644)

	return tmpDir
}

func TestIsAvailable(t *testing.T) {
	setupTestResctrl(t)
	if !IsAvailable() {
		t.Fatal("expected IsAvailable() = true")
	}
}

func TestIsAvailableNoResctrl(t *testing.T) {
	SetRoot(t.TempDir())
	defer SetRoot(DefaultRoot)

	if IsAvailable() {
		t.Fatal("expected IsAvailable() = false")
	}
}

func TestInfo(t *testing.T) {
	setupTestResctrl(t)

	info, err := Info()
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}

	if info.NumCLOSIDs != 16 {
		t.Errorf("NumCLOSIDs = %d, want 16", info.NumCLOSIDs)
	}
	if info.CBMMask != 0xfffff {
		t.Errorf("CBMMask = %x, want fffff", info.CBMMask)
	}
	if info.MinCBMBits != 1 {
		t.Errorf("MinCBMBits = %d, want 1", info.MinCBMBits)
	}
	if info.NumWays() != 20 {
		t.Errorf("NumWays() = %d, want 20", info.NumWays())
	}
	if len(info.CacheIDs) != 2 || info.CacheIDs[0] != 0 || info.CacheIDs[1] != 1 {
		t.Errorf("CacheIDs = %v, want [0 1]", info.CacheIDs)
	}
}

func TestCreateGroup(t *testing.T) {
	tmpDir := setupTestResctrl(t)

	err := CreateGroup("test-group", "L3:0=1f;1=1f")
	if err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}

	groupPath := filepath.Join(tmpDir, "test-group")
	if _, err := os.Stat(groupPath); err != nil {
		t.Fatalf("group directory not created: %v", err)
	}

	schemata, err := ReadSchemata("test-group")
	if err != nil {
		t.Fatalf("ReadSchemata error: %v", err)
	}
	if schemata != "L3:0=1f;1=1f" {
		t.Errorf("schemata = %q, want %q", schemata, "L3:0=1f;1=1f")
	}
}

func TestDeleteGroup(t *testing.T) {
	tmpDir := setupTestResctrl(t)

	_ = os.MkdirAll(filepath.Join(tmpDir, "to-delete"), 0755)

	err := DeleteGroup("to-delete")
	if err != nil {
		t.Fatalf("DeleteGroup error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "to-delete")); !os.IsNotExist(err) {
		t.Fatal("group directory still exists after delete")
	}
}

func TestAssignPID(t *testing.T) {
	tmpDir := setupTestResctrl(t)

	groupPath := filepath.Join(tmpDir, "pid-group")
	_ = os.MkdirAll(groupPath, 0755)

	err := AssignPID("pid-group", 12345)
	if err != nil {
		t.Fatalf("AssignPID error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(groupPath, "tasks"))
	if string(data) != "12345" {
		t.Errorf("tasks file = %q, want %q", string(data), "12345")
	}
}
