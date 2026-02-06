package monitor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// SystemStats holds a snapshot of system metrics.
type SystemStats struct {
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
	TempC       float64
	UptimeDays  float64
}

// Monitor reads system metrics periodically and stores them atomically.
type Monitor struct {
	stats    atomic.Pointer[SystemStats]
	interval time.Duration
	onUpdate func(SystemStats) // callback when stats are updated

	// CPU delta tracking
	prevIdle  uint64
	prevTotal uint64
}

// New creates a Monitor. onUpdate is called each time stats are refreshed.
func New(interval time.Duration, onUpdate func(SystemStats)) *Monitor {
	m := &Monitor{
		interval: interval,
		onUpdate: onUpdate,
	}
	m.stats.Store(&SystemStats{})
	return m
}

// Stats returns the latest stats without blocking.
func (m *Monitor) Stats() SystemStats {
	return *m.stats.Load()
}

// Run polls system metrics until the context is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	// Immediate first read
	m.refresh()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refresh()
		}
	}
}

func (m *Monitor) refresh() {
	s := &SystemStats{
		CPUPercent:  m.readCPU(),
		MemPercent:  readMemPercent(),
		DiskPercent: readDiskPercent(),
		TempC:       readTemp(),
		UptimeDays:  readUptime(),
	}
	m.stats.Store(s)
	if m.onUpdate != nil {
		m.onUpdate(*s)
	}
}

// --- CPU (Linux: /proc/stat) ---

func (m *Monitor) readCPU() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	f, err := os.Open("/proc/stat")
	if err != nil {
		slog.Debug("monitor: cannot read /proc/stat", "err", err)
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}

	// First line: cpu  user nice system idle iowait irq softirq steal ...
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var total, idle uint64
	for i, field := range fields[1:] {
		val, _ := strconv.ParseUint(field, 10, 64)
		total += val
		if i == 3 { // idle is the 4th value (index 3)
			idle = val
		}
	}

	// Calculate delta
	if m.prevTotal == 0 {
		m.prevIdle = idle
		m.prevTotal = total
		return 0
	}

	deltaTotal := total - m.prevTotal
	deltaIdle := idle - m.prevIdle
	m.prevIdle = idle
	m.prevTotal = total

	if deltaTotal == 0 {
		return 0
	}

	return float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
}

// --- Memory (Linux: /proc/meminfo) ---

func readMemPercent() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	var total, available uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseMeminfoKB(line)
		}
	}

	if total == 0 {
		return 0
	}
	return float64(total-available) / float64(total) * 100
}

func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	val, _ := strconv.ParseUint(fields[1], 10, 64)
	return val
}

// --- Disk (syscall.Statfs — works on Linux and macOS) ---

func readDiskPercent() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		slog.Debug("monitor: statfs failed", "err", err)
		return 0
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0
	}
	return float64(total-free) / float64(total) * 100
}

// --- Temperature (Linux: /sys/class/thermal) ---

func readTemp() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	// Try thermal_zone0 first (common on Raspberry Pi)
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}

	milliC, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}

	return float64(milliC) / 1000.0
}

// --- Uptime (Linux: /proc/uptime) ---

func readUptime() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}

	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return seconds / 86400.0
}

// FormatStats returns a human-readable stats summary.
func FormatStats(s SystemStats) string {
	return fmt.Sprintf("CPU: %.1f%% | Mem: %.1f%% | Disk: %.1f%% | Temp: %.1f°C | Up: %.1fd",
		s.CPUPercent, s.MemPercent, s.DiskPercent, s.TempC, s.UptimeDays)
}
