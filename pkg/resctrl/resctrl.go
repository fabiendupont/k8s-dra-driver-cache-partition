package resctrl

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultRoot = "/sys/fs/resctrl"

var root = DefaultRoot

func SetRoot(path string) { root = path }
func Root() string        { return root }

type CATInfo struct {
	NumCLOSIDs int
	CBMMask    uint64
	MinCBMBits int
	CacheIDs   []int
}

func (c *CATInfo) NumWays() int {
	n := 0
	m := c.CBMMask
	for m > 0 {
		n += int(m & 1)
		m >>= 1
	}
	return n
}

func IsAvailable() bool {
	info := filepath.Join(root, "info", "L3")
	fi, err := os.Stat(info)
	return err == nil && fi.IsDir()
}

func Info() (*CATInfo, error) {
	infoDir := filepath.Join(root, "info", "L3")
	if _, err := os.Stat(infoDir); err != nil {
		return nil, fmt.Errorf("L3 CAT not available: %w", err)
	}

	info := &CATInfo{}

	numCLOSIDs, err := readIntFile(filepath.Join(infoDir, "num_closids"))
	if err != nil {
		return nil, fmt.Errorf("reading num_closids: %w", err)
	}
	info.NumCLOSIDs = numCLOSIDs

	cbmMaskStr, err := readTrimmedFile(filepath.Join(infoDir, "cbm_mask"))
	if err != nil {
		return nil, fmt.Errorf("reading cbm_mask: %w", err)
	}
	cbm, err := strconv.ParseUint(cbmMaskStr, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing cbm_mask %q: %w", cbmMaskStr, err)
	}
	info.CBMMask = cbm

	minBits, err := readIntFile(filepath.Join(infoDir, "min_cbm_bits"))
	if err != nil {
		return nil, fmt.Errorf("reading min_cbm_bits: %w", err)
	}
	info.MinCBMBits = minBits

	cacheIDs, err := discoverCacheIDs()
	if err != nil {
		return nil, fmt.Errorf("discovering cache IDs: %w", err)
	}
	info.CacheIDs = cacheIDs

	return info, nil
}

func CreateGroup(name, schemata string) error {
	groupPath := filepath.Join(root, name)
	if err := os.MkdirAll(groupPath, 0755); err != nil {
		return fmt.Errorf("creating resctrl group %s: %w", name, err)
	}
	if schemata != "" {
		if err := os.WriteFile(filepath.Join(groupPath, "schemata"), []byte(schemata+"\n"), 0644); err != nil {
			_ = os.Remove(groupPath)
			return fmt.Errorf("writing schemata for %s: %w", name, err)
		}
	}
	return nil
}

func DeleteGroup(name string) error {
	groupPath := filepath.Join(root, name)
	// Drain tasks back to root group before removing the directory; the kernel
	// refuses to rmdir a resctrl group that still has assigned tasks.
	tasksPath := filepath.Join(groupPath, "tasks")
	if data, err := os.ReadFile(tasksPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = os.WriteFile(filepath.Join(root, "tasks"), data, 0644)
	}
	return os.Remove(groupPath)
}

func AssignPID(group string, pid int) error {
	tasksPath := filepath.Join(root, group, "tasks")
	return os.WriteFile(tasksPath, []byte(strconv.Itoa(pid)), 0644)
}

func ReadSchemata(group string) (string, error) {
	return readTrimmedFile(filepath.Join(root, group, "schemata"))
}

// MBAInfo describes the Memory Bandwidth Allocation capabilities of the platform.
type MBAInfo struct {
	NumCLOSIDs        int
	MinBandwidth      int  // minimum bandwidth value (percent or MBps)
	BandwidthGran     int  // step granularity
	CtrlInPercentages bool // true = percent mode, false = MBps mode
	DomainIDs         []int
}

// MBAMode returns "percent" or "mbps" based on the control mode.
func (m *MBAInfo) MBAMode() string {
	if m.CtrlInPercentages {
		return "percent"
	}
	return "mbps"
}

// IsMBAAvailable reports whether the resctrl MBA interface is present.
func IsMBAAvailable() bool {
	info := filepath.Join(root, "info", "MB")
	fi, err := os.Stat(info)
	return err == nil && fi.IsDir()
}

// MBA reads MBA capabilities from the resctrl info filesystem.
func MBA() (*MBAInfo, error) {
	infoDir := filepath.Join(root, "info", "MB")
	if _, err := os.Stat(infoDir); err != nil {
		return nil, fmt.Errorf("MBA not available: %w", err)
	}

	info := &MBAInfo{}

	numCLOSIDs, err := readIntFile(filepath.Join(infoDir, "num_closids"))
	if err != nil {
		return nil, fmt.Errorf("reading MBA num_closids: %w", err)
	}
	info.NumCLOSIDs = numCLOSIDs

	minBW, err := readIntFile(filepath.Join(infoDir, "min_bandwidth"))
	if err != nil {
		return nil, fmt.Errorf("reading min_bandwidth: %w", err)
	}
	info.MinBandwidth = minBW

	gran, err := readIntFile(filepath.Join(infoDir, "bandwidth_gran"))
	if err != nil {
		return nil, fmt.Errorf("reading bandwidth_gran: %w", err)
	}
	info.BandwidthGran = gran

	// ctrl_in_percentages: 1 = percent mode, 0 = MBps mode. Missing = percent (older kernels).
	ctrlPct, err := readIntFile(filepath.Join(infoDir, "ctrl_in_percentages"))
	if err != nil {
		info.CtrlInPercentages = true // default to percent on older kernels
	} else {
		info.CtrlInPercentages = ctrlPct != 0
	}

	domainIDs, err := discoverDomainIDs("MB")
	if err != nil {
		return nil, fmt.Errorf("discovering MBA domain IDs: %w", err)
	}
	info.DomainIDs = domainIDs

	return info, nil
}

// FormatMBASchemata formats a single-domain MBA schemata line.
func FormatMBASchemata(domainID, bandwidth int) string {
	return fmt.Sprintf("MB:%d=%d", domainID, bandwidth)
}

func discoverCacheIDs() ([]int, error) {
	return discoverDomainIDs("L3")
}

func discoverDomainIDs(prefix string) ([]int, error) {
	schemataPath := filepath.Join(root, "schemata")
	data, err := readTrimmedFile(schemataPath)
	if err != nil {
		return nil, fmt.Errorf("reading root schemata: %w", err)
	}
	linePrefix := prefix + ":"
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, linePrefix) {
			continue
		}
		parts := strings.TrimPrefix(line, linePrefix)
		var ids []int
		for _, segment := range strings.Split(parts, ";") {
			eqIdx := strings.Index(segment, "=")
			if eqIdx < 0 {
				continue
			}
			id, err := strconv.Atoi(segment[:eqIdx])
			if err != nil {
				continue
			}
			ids = append(ids, id)
		}
		return ids, nil
	}
	return []int{0}, nil
}

// ── L2 CAT ──────────────────────────────────────────────────────────────────

// IsL2Available reports whether the resctrl L2 CAT interface is present.
func IsL2Available() bool {
	fi, err := os.Stat(filepath.Join(root, "info", "L2"))
	return err == nil && fi.IsDir()
}

// InfoL2 reads L2 CAT capabilities from the resctrl info filesystem.
// The returned CATInfo uses the same structure as the L3 Info().
func InfoL2() (*CATInfo, error) {
	infoDir := filepath.Join(root, "info", "L2")
	if _, err := os.Stat(infoDir); err != nil {
		return nil, fmt.Errorf("L2 CAT not available: %w", err)
	}

	info := &CATInfo{}

	numCLOSIDs, err := readIntFile(filepath.Join(infoDir, "num_closids"))
	if err != nil {
		return nil, fmt.Errorf("reading L2 num_closids: %w", err)
	}
	info.NumCLOSIDs = numCLOSIDs

	cbmMaskStr, err := readTrimmedFile(filepath.Join(infoDir, "cbm_mask"))
	if err != nil {
		return nil, fmt.Errorf("reading L2 cbm_mask: %w", err)
	}
	cbm, err := strconv.ParseUint(cbmMaskStr, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing L2 cbm_mask %q: %w", cbmMaskStr, err)
	}
	info.CBMMask = cbm

	minBits, err := readIntFile(filepath.Join(infoDir, "min_cbm_bits"))
	if err != nil {
		return nil, fmt.Errorf("reading L2 min_cbm_bits: %w", err)
	}
	info.MinCBMBits = minBits

	cacheIDs, err := discoverDomainIDs("L2")
	if err != nil {
		// L2 domains may not appear in root schemata on all platforms; default to L3 IDs.
		cacheIDs, _ = discoverDomainIDs("L3")
	}
	info.CacheIDs = cacheIDs

	return info, nil
}

// ── SMBA ────────────────────────────────────────────────────────────────────

// IsSMBAAvailable reports whether the resctrl SMBA (Slow Memory Bandwidth
// Allocation) interface is present. SMBA is available on Intel Xeon systems
// with on-package HBM (e.g., Sapphire Rapids HBM).
func IsSMBAAvailable() bool {
	fi, err := os.Stat(filepath.Join(root, "info", "SMBA"))
	return err == nil && fi.IsDir()
}

// SMBA reads SMBA capabilities. It reuses MBAInfo since the sysfs layout is
// identical to MBA; the only difference is the schemata keyword (SMBA vs MB).
func SMBA() (*MBAInfo, error) {
	infoDir := filepath.Join(root, "info", "SMBA")
	if _, err := os.Stat(infoDir); err != nil {
		return nil, fmt.Errorf("SMBA not available: %w", err)
	}

	info := &MBAInfo{}

	numCLOSIDs, err := readIntFile(filepath.Join(infoDir, "num_closids"))
	if err != nil {
		return nil, fmt.Errorf("reading SMBA num_closids: %w", err)
	}
	info.NumCLOSIDs = numCLOSIDs

	minBW, err := readIntFile(filepath.Join(infoDir, "min_bandwidth"))
	if err != nil {
		return nil, fmt.Errorf("reading SMBA min_bandwidth: %w", err)
	}
	info.MinBandwidth = minBW

	gran, err := readIntFile(filepath.Join(infoDir, "bandwidth_gran"))
	if err != nil {
		return nil, fmt.Errorf("reading SMBA bandwidth_gran: %w", err)
	}
	info.BandwidthGran = gran

	ctrlPct, err := readIntFile(filepath.Join(infoDir, "ctrl_in_percentages"))
	if err != nil {
		info.CtrlInPercentages = true
	} else {
		info.CtrlInPercentages = ctrlPct != 0
	}

	domainIDs, err := discoverDomainIDs("SMBA")
	if err != nil {
		domainIDs, _ = discoverDomainIDs("L3")
	}
	info.DomainIDs = domainIDs

	return info, nil
}

// FormatSMBASchemata formats a single-domain SMBA schemata line.
func FormatSMBASchemata(domainID, bandwidth int) string {
	return fmt.Sprintf("SMBA:%d=%d", domainID, bandwidth)
}

// ── CMT/MBM Monitoring ──────────────────────────────────────────────────────

// MonData holds the monitoring counters for one resctrl group / cache domain.
type MonData struct {
	LLCOccupancy  uint64 // current L3 occupancy in bytes (gauge)
	MBMLocalBytes uint64 // accumulated local DRAM bytes since kernel boot (counter)
	MBMTotalBytes uint64 // accumulated total bandwidth bytes since kernel boot (counter)
}

// IsMonitoringAvailable reports whether the resctrl L3 monitoring interface
// (CMT / MBM) is present on this platform.
func IsMonitoringAvailable() bool {
	fi, err := os.Stat(filepath.Join(root, "info", "L3_MON"))
	return err == nil && fi.IsDir()
}

// MonitoringFeatures returns the list of monitoring features supported by the
// hardware (e.g. ["llc_occupancy", "mbm_local_bytes", "mbm_total_bytes"]).
func MonitoringFeatures() []string {
	path := filepath.Join(root, "info", "L3_MON", "mon_features")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var features []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			features = append(features, line)
		}
	}
	return features
}

// ReadMonData reads the monitoring counters for a named resctrl group and a
// specific cache domain. Files that are absent or unreadable produce a zero
// value for that counter rather than an error.
func ReadMonData(group string, cacheID int) (*MonData, error) {
	monDir := filepath.Join(root, group, "mon_data", fmt.Sprintf("mon_L3_%d", cacheID))
	if _, err := os.Stat(monDir); err != nil {
		return nil, fmt.Errorf("mon_data dir for group %s cacheID %d not found: %w", group, cacheID, err)
	}

	d := &MonData{}
	if v, err := readUint64File(filepath.Join(monDir, "llc_occupancy")); err == nil {
		d.LLCOccupancy = v
	}
	if v, err := readUint64File(filepath.Join(monDir, "mbm_local_bytes")); err == nil {
		d.MBMLocalBytes = v
	}
	if v, err := readUint64File(filepath.Join(monDir, "mbm_total_bytes")); err == nil {
		d.MBMTotalBytes = v
	}
	return d, nil
}

func readUint64File(path string) (uint64, error) {
	s, err := readTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readIntFile(path string) (int, error) {
	s, err := readTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(s)
}
