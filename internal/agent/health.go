package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemResources holds dynamic system resource info.
type SystemResources struct {
	CPUCores       int32
	TotalMemoryMB  int64
	UsedMemoryMB   int64
	ActiveSessions int32
	MaxSessions    int32
}

// CollectResources gathers current system resource metrics.
func CollectResources(activeSessions, maxSessions int32) SystemResources {
	totalMB, usedMB := systemMemoryMB()
	return SystemResources{
		CPUCores:       int32(runtime.NumCPU()),
		TotalMemoryMB:  totalMB,
		UsedMemoryMB:   usedMB,
		ActiveSessions: activeSessions,
		MaxSessions:    maxSessions,
	}
}

// systemMemoryMB returns total and used system memory in MB.
// Uses sysctl/vm_stat on macOS and /proc/meminfo on Linux.
// Returns (0, 0) if unable to determine.
func systemMemoryMB() (total, used int64) {
	switch runtime.GOOS {
	case "darwin":
		return darwinMemoryMB()
	case "linux":
		return linuxMemoryMB()
	default:
		return 0, 0
	}
}

func darwinMemoryMB() (total, used int64) {
	// Total memory via sysctl
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0
	}
	totalBytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, 0
	}
	total = totalBytes / (1024 * 1024)

	// Used memory via vm_stat (pages active + wired)
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, 0
	}
	lines := strings.Split(string(vmOut), "\n")
	var activePages, wiredPages int64
	for _, line := range lines {
		if strings.Contains(line, "Pages active") {
			activePages = parseVMStatValue(line)
		} else if strings.Contains(line, "Pages wired") {
			wiredPages = parseVMStatValue(line)
		}
	}
	// Page size is typically 4096 on macOS
	usedBytes := (activePages + wiredPages) * 4096
	used = usedBytes / (1024 * 1024)
	return total, used
}

func parseVMStatValue(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	valStr := strings.TrimSpace(parts[1])
	valStr = strings.TrimSuffix(valStr, ".")
	val, _ := strconv.ParseInt(valStr, 10, 64)
	return val
}

func linuxMemoryMB() (total, used int64) {
	out, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var totalKB, availableKB int64
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			totalKB = parseMeminfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			availableKB = parseMeminfoKB(line)
		}
	}
	total = totalKB / 1024
	used = (totalKB - availableKB) / 1024
	return total, used
}

func parseMeminfoKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	val, _ := strconv.ParseInt(fields[1], 10, 64)
	return val
}
