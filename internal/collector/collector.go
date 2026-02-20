package collector

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// DeviceInfo holds device-level metrics for a single GPU.
type DeviceInfo struct {
	Index       int
	UUID        string
	Name        string
	MemoryUsed  uint64  // bytes
	MemoryTotal uint64  // bytes
	Utilization uint32  // percent 0-100
	PowerWatts  float64 // watts
	TempCelsius uint32  // degrees C
}

// ProcessSample holds per-process data from NVML for a single GPU.
type ProcessSample struct {
	GPU        int
	PID        uint32
	UsedMemory uint64 // bytes
	SmUtil     uint32 // percent 0-100
}

// Snapshot is the result of a single collection cycle.
type Snapshot struct {
	Timestamp    time.Time
	Devices      []DeviceInfo
	Processes    []ProcessSample
	ProcessNames map[uint32]string // pid -> process name from /proc/<pid>/comm
}

// Collector handles NVML device and process metrics collection.
type Collector struct {
	// lastSampleTime tracks the last timestamp per device index for
	// nvmlDeviceGetProcessUtilization, which returns samples since a given timestamp.
	lastSampleTime map[int]uint64
}

// New creates a new Collector.
func New() *Collector {
	return &Collector{
		lastSampleTime: make(map[int]uint64),
	}
}

// Collect queries NVML for all GPU device and per-process metrics.
func (c *Collector) Collect() (*Snapshot, error) {
	snap := &Snapshot{
		Timestamp:    time.Now(),
		ProcessNames: make(map[uint32]string),
	}

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("DeviceGetCount: %v", nvml.ErrorString(ret))
	}

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			log.Printf("collector: DeviceGetHandleByIndex(%d): %v", i, nvml.ErrorString(ret))
			continue
		}

		di := c.collectDevice(i, device)
		snap.Devices = append(snap.Devices, di)

		procs := c.collectProcesses(i, device)
		snap.Processes = append(snap.Processes, procs...)
	}

	// Read process names from /proc/<pid>/comm
	for _, p := range snap.Processes {
		if _, exists := snap.ProcessNames[p.PID]; !exists {
			snap.ProcessNames[p.PID] = readProcessName(p.PID)
		}
	}

	return snap, nil
}

// collectDevice gathers device-level metrics for a single GPU.
func (c *Collector) collectDevice(index int, device nvml.Device) DeviceInfo {
	di := DeviceInfo{Index: index}

	if name, ret := device.GetName(); ret == nvml.SUCCESS {
		di.Name = name
	}
	if uuid, ret := device.GetUUID(); ret == nvml.SUCCESS {
		di.UUID = uuid
	}

	if memInfo, ret := device.GetMemoryInfo(); ret == nvml.SUCCESS {
		di.MemoryUsed = memInfo.Used
		di.MemoryTotal = memInfo.Total
	}

	if utilRates, ret := device.GetUtilizationRates(); ret == nvml.SUCCESS {
		di.Utilization = utilRates.Gpu
	}

	// GetPowerUsage returns milliwatts
	if power, ret := device.GetPowerUsage(); ret == nvml.SUCCESS {
		di.PowerWatts = float64(power) / 1000.0
	}

	if temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
		di.TempCelsius = temp
	}

	return di
}

// collectProcesses gathers per-process metrics for a single GPU.
func (c *Collector) collectProcesses(gpuIndex int, device nvml.Device) []ProcessSample {
	// Get processes holding GPU memory
	procs, ret := device.GetComputeRunningProcesses()
	if ret != nvml.SUCCESS {
		log.Printf("collector: GetComputeRunningProcesses(GPU %d): %v", gpuIndex, nvml.ErrorString(ret))
		return nil
	}
	if len(procs) == 0 {
		return nil
	}

	// Get per-process utilization samples since last poll
	lastTS := c.lastSampleTime[gpuIndex]
	utilSamples, ret := device.GetProcessUtilization(lastTS)
	if ret != nvml.SUCCESS && ret != nvml.ERROR_NOT_FOUND {
		// NOT_FOUND is returned when no samples are available (all processes idle) — not an error
		log.Printf("collector: GetProcessUtilization(GPU %d): %v", gpuIndex, nvml.ErrorString(ret))
	}

	// Update lastSampleTime to the max timestamp from results
	if len(utilSamples) > 0 {
		maxTS := lastTS
		for _, s := range utilSamples {
			if s.TimeStamp > maxTS {
				maxTS = s.TimeStamp
			}
		}
		c.lastSampleTime[gpuIndex] = maxTS
	}

	// Build PID -> max SmUtil map from utilization samples
	utilMap := make(map[uint32]uint32, len(utilSamples))
	for _, s := range utilSamples {
		if s.SmUtil > utilMap[s.Pid] {
			utilMap[s.Pid] = s.SmUtil
		}
	}

	// Merge: for each process with memory allocated, look up its utilization.
	// Processes absent from utilSamples default to SmUtil=0 (idle).
	samples := make([]ProcessSample, 0, len(procs))
	for _, p := range procs {
		samples = append(samples, ProcessSample{
			GPU:        gpuIndex,
			PID:        p.Pid,
			UsedMemory: p.UsedGpuMemory,
			SmUtil:     utilMap[p.Pid],
		})
	}

	return samples
}

// readProcessName reads the process name from /proc/<pid>/comm.
// The result is sanitized: control characters and null bytes are stripped
// (null bytes would break the stale-key delimiter in the exporter), and
// the name is truncated to 64 characters.
func readProcessName(pid uint32) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "unknown"
	}
	name := strings.TrimSpace(string(data))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f { // strip control characters including \x00
			return -1
		}
		return r
	}, name)
	if len(name) > 64 {
		name = name[:64]
	}
	if name == "" {
		return "unknown"
	}
	return name
}
