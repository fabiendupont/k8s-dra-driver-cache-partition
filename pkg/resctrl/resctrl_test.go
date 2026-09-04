package resctrl

import (
	"fmt"
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

func setupMBA(t *testing.T, tmpDir string, pctMode bool) {
	t.Helper()
	mbaDirPath := filepath.Join(tmpDir, "info", "MB")
	_ = os.MkdirAll(mbaDirPath, 0755)
	_ = os.WriteFile(filepath.Join(mbaDirPath, "num_closids"), []byte("16\n"), 0644)
	_ = os.WriteFile(filepath.Join(mbaDirPath, "min_bandwidth"), []byte("10\n"), 0644)
	_ = os.WriteFile(filepath.Join(mbaDirPath, "bandwidth_gran"), []byte("10\n"), 0644)
	ctrl := "0"
	if pctMode {
		ctrl = "1"
	}
	_ = os.WriteFile(filepath.Join(mbaDirPath, "ctrl_in_percentages"), []byte(ctrl+"\n"), 0644)
	// Extend root schemata with MB line.
	_ = os.WriteFile(filepath.Join(tmpDir, "schemata"), []byte("L3:0=fffff;1=fffff\nMB:0=100;1=100\n"), 0644)
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

func TestIsMBAAvailable(t *testing.T) {
	tmpDir := setupTestResctrl(t)

	if IsMBAAvailable() {
		t.Fatal("expected IsMBAAvailable() = false before MBA dir exists")
	}

	setupMBA(t, tmpDir, true)
	if !IsMBAAvailable() {
		t.Fatal("expected IsMBAAvailable() = true after MBA dir created")
	}
}

func TestMBA_PercentMode(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	setupMBA(t, tmpDir, true)

	info, err := MBA()
	if err != nil {
		t.Fatalf("MBA(): %v", err)
	}
	if info.NumCLOSIDs != 16 {
		t.Errorf("NumCLOSIDs = %d, want 16", info.NumCLOSIDs)
	}
	if info.MinBandwidth != 10 {
		t.Errorf("MinBandwidth = %d, want 10", info.MinBandwidth)
	}
	if info.BandwidthGran != 10 {
		t.Errorf("BandwidthGran = %d, want 10", info.BandwidthGran)
	}
	if !info.CtrlInPercentages {
		t.Error("CtrlInPercentages = false, want true")
	}
	if info.MBAMode() != "percent" {
		t.Errorf("MBAMode() = %q, want percent", info.MBAMode())
	}
	if len(info.DomainIDs) != 2 || info.DomainIDs[0] != 0 || info.DomainIDs[1] != 1 {
		t.Errorf("DomainIDs = %v, want [0 1]", info.DomainIDs)
	}
}

func TestMBA_MBpsMode(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	setupMBA(t, tmpDir, false)

	info, err := MBA()
	if err != nil {
		t.Fatalf("MBA(): %v", err)
	}
	if info.CtrlInPercentages {
		t.Error("CtrlInPercentages = true, want false")
	}
	if info.MBAMode() != "mbps" {
		t.Errorf("MBAMode() = %q, want mbps", info.MBAMode())
	}
}

func TestFormatMBASchemata(t *testing.T) {
	got := FormatMBASchemata(0, 70)
	want := "MB:0=70"
	if got != want {
		t.Errorf("FormatMBASchemata(0, 70) = %q, want %q", got, want)
	}
}

// ── L2 CAT tests ────────────────────────────────────────────────────────────

func setupL2(t *testing.T, tmpDir string) {
	t.Helper()
	l2Dir := filepath.Join(tmpDir, "info", "L2")
	_ = os.MkdirAll(l2Dir, 0755)
	_ = os.WriteFile(filepath.Join(l2Dir, "num_closids"), []byte("16\n"), 0644)
	_ = os.WriteFile(filepath.Join(l2Dir, "cbm_mask"), []byte("ff\n"), 0644)
	_ = os.WriteFile(filepath.Join(l2Dir, "min_cbm_bits"), []byte("1\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schemata"), []byte("L3:0=fffff;1=fffff\nL2:0=ff;1=ff\n"), 0644)
}

func TestIsL2Available(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	if IsL2Available() {
		t.Fatal("expected IsL2Available() = false before L2 dir exists")
	}
	setupL2(t, tmpDir)
	if !IsL2Available() {
		t.Fatal("expected IsL2Available() = true after L2 dir created")
	}
}

func TestInfoL2(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	setupL2(t, tmpDir)

	info, err := InfoL2()
	if err != nil {
		t.Fatalf("InfoL2(): %v", err)
	}
	if info.CBMMask != 0xff {
		t.Errorf("CBMMask = %x, want ff", info.CBMMask)
	}
	if info.NumWays() != 8 {
		t.Errorf("NumWays() = %d, want 8", info.NumWays())
	}
	if info.NumCLOSIDs != 16 {
		t.Errorf("NumCLOSIDs = %d, want 16", info.NumCLOSIDs)
	}
}

// ── SMBA tests ───────────────────────────────────────────────────────────────

func setupSMBA(t *testing.T, tmpDir string) {
	t.Helper()
	smbaDir := filepath.Join(tmpDir, "info", "SMBA")
	_ = os.MkdirAll(smbaDir, 0755)
	_ = os.WriteFile(filepath.Join(smbaDir, "num_closids"), []byte("16\n"), 0644)
	_ = os.WriteFile(filepath.Join(smbaDir, "min_bandwidth"), []byte("10\n"), 0644)
	_ = os.WriteFile(filepath.Join(smbaDir, "bandwidth_gran"), []byte("10\n"), 0644)
	_ = os.WriteFile(filepath.Join(smbaDir, "ctrl_in_percentages"), []byte("1\n"), 0644)
}

func TestIsSMBAAvailable(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	if IsSMBAAvailable() {
		t.Fatal("expected IsSMBAAvailable() = false before SMBA dir exists")
	}
	setupSMBA(t, tmpDir)
	if !IsSMBAAvailable() {
		t.Fatal("expected IsSMBAAvailable() = true after SMBA dir created")
	}
}

func TestSMBA(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	setupSMBA(t, tmpDir)

	info, err := SMBA()
	if err != nil {
		t.Fatalf("SMBA(): %v", err)
	}
	if info.NumCLOSIDs != 16 {
		t.Errorf("NumCLOSIDs = %d, want 16", info.NumCLOSIDs)
	}
	if !info.CtrlInPercentages {
		t.Error("CtrlInPercentages = false, want true")
	}
	if info.MBAMode() != "percent" {
		t.Errorf("MBAMode() = %q, want percent", info.MBAMode())
	}
}

func TestFormatSMBASchemata(t *testing.T) {
	got := FormatSMBASchemata(1, 50)
	want := "SMBA:1=50"
	if got != want {
		t.Errorf("FormatSMBASchemata(1, 50) = %q, want %q", got, want)
	}
}

// ── Monitoring tests ─────────────────────────────────────────────────────────

func setupMonitoring(t *testing.T, tmpDir string) {
	t.Helper()
	monDir := filepath.Join(tmpDir, "info", "L3_MON")
	_ = os.MkdirAll(monDir, 0755)
	_ = os.WriteFile(filepath.Join(monDir, "mon_features"),
		[]byte("llc_occupancy\nmbm_local_bytes\nmbm_total_bytes\n"), 0644)
}

func makeMonData(t *testing.T, tmpDir, group string, cacheID int, llc, local, total uint64) {
	t.Helper()
	dir := filepath.Join(tmpDir, group, "mon_data", fmt.Sprintf("mon_L3_%d", cacheID))
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "llc_occupancy"), []byte(fmt.Sprintf("%d\n", llc)), 0644)
	_ = os.WriteFile(filepath.Join(dir, "mbm_local_bytes"), []byte(fmt.Sprintf("%d\n", local)), 0644)
	_ = os.WriteFile(filepath.Join(dir, "mbm_total_bytes"), []byte(fmt.Sprintf("%d\n", total)), 0644)
}

func TestIsMonitoringAvailable(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	if IsMonitoringAvailable() {
		t.Fatal("expected false before L3_MON dir exists")
	}
	setupMonitoring(t, tmpDir)
	if !IsMonitoringAvailable() {
		t.Fatal("expected true after L3_MON dir created")
	}
}

func TestMonitoringFeatures(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	setupMonitoring(t, tmpDir)

	features := MonitoringFeatures()
	if len(features) != 3 {
		t.Fatalf("got %d features, want 3: %v", len(features), features)
	}
}

func TestReadMonData(t *testing.T) {
	tmpDir := setupTestResctrl(t)
	_ = os.MkdirAll(filepath.Join(tmpDir, "my-group"), 0755)
	makeMonData(t, tmpDir, "my-group", 0, 1048576, 2000000, 3000000)

	data, err := ReadMonData("my-group", 0)
	if err != nil {
		t.Fatalf("ReadMonData: %v", err)
	}
	if data.LLCOccupancy != 1048576 {
		t.Errorf("LLCOccupancy = %d, want 1048576", data.LLCOccupancy)
	}
	if data.MBMLocalBytes != 2000000 {
		t.Errorf("MBMLocalBytes = %d, want 2000000", data.MBMLocalBytes)
	}
	if data.MBMTotalBytes != 3000000 {
		t.Errorf("MBMTotalBytes = %d, want 3000000", data.MBMTotalBytes)
	}
}

func TestReadMonData_MissingDir(t *testing.T) {
	setupTestResctrl(t)
	_, err := ReadMonData("nonexistent-group", 0)
	if err == nil {
		t.Fatal("expected error for missing mon_data dir, got nil")
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
