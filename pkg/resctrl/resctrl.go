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
	return os.Remove(filepath.Join(root, name))
}

func AssignPID(group string, pid int) error {
	tasksPath := filepath.Join(root, group, "tasks")
	return os.WriteFile(tasksPath, []byte(strconv.Itoa(pid)), 0644)
}

func ReadSchemata(group string) (string, error) {
	return readTrimmedFile(filepath.Join(root, group, "schemata"))
}

func discoverCacheIDs() ([]int, error) {
	schemataPath := filepath.Join(root, "schemata")
	data, err := readTrimmedFile(schemataPath)
	if err != nil {
		return nil, fmt.Errorf("reading root schemata: %w", err)
	}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "L3:") {
			continue
		}
		parts := strings.TrimPrefix(line, "L3:")
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
