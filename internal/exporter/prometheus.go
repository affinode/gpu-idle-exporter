package exporter

import (
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/affinode/gpu-idle-exporter/internal/collector"
	"github.com/affinode/gpu-idle-exporter/internal/idle"
)

var (
	processLabels = []string{"gpu", "pid", "process"}
	deviceLabels  = []string{"gpu", "model", "uuid"}
	gpuOnlyLabel  = []string{"gpu"}
)

// Exporter manages Prometheus metric registration and updates.
type Exporter struct {
	// Per-process gauges
	processComputeUtil *prometheus.GaugeVec
	processMemUsed     *prometheus.GaugeVec
	processIdleSecs    *prometheus.GaugeVec
	processIdleMem     *prometheus.GaugeVec

	// Device-level gauges
	deviceUtil     *prometheus.GaugeVec
	deviceMemUsed  *prometheus.GaugeVec
	deviceMemTotal *prometheus.GaugeVec
	devicePower    *prometheus.GaugeVec
	deviceTemp     *prometheus.GaugeVec

	// Aggregate gauges
	idleMemTotal *prometheus.GaugeVec

	// Track which label sets we emitted last cycle for stale series cleanup
	prevProcessKeys map[string]bool
}

// New creates a new Exporter with all Prometheus metrics defined.
func New() *Exporter {
	return &Exporter{
		processComputeUtil: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_process_compute_utilization_percent",
			Help: "GPU compute (SM) utilization percentage for this process.",
		}, processLabels),
		processMemUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_process_memory_used_bytes",
			Help: "GPU memory held by this process in bytes.",
		}, processLabels),
		processIdleSecs: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_process_idle_seconds",
			Help: "Duration in seconds this process has been idle (0%% compute while holding memory). 0 when active.",
		}, processLabels),
		processIdleMem: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_process_idle_memory_bytes",
			Help: "GPU memory in bytes held by this process while idle. 0 when active.",
		}, processLabels),

		deviceUtil: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_device_utilization_percent",
			Help: "GPU compute utilization percentage (device-level).",
		}, deviceLabels),
		deviceMemUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_device_memory_used_bytes",
			Help: "GPU memory currently used in bytes (device-level).",
		}, deviceLabels),
		deviceMemTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_device_memory_total_bytes",
			Help: "GPU total memory in bytes (device-level).",
		}, deviceLabels),
		devicePower: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_device_power_watts",
			Help: "GPU current power draw in watts.",
		}, deviceLabels),
		deviceTemp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_device_temperature_celsius",
			Help: "GPU core temperature in Celsius.",
		}, deviceLabels),

		idleMemTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpu_idle_memory_total_bytes",
			Help: "Total GPU memory in bytes held by all idle processes on this GPU.",
		}, gpuOnlyLabel),

		prevProcessKeys: make(map[string]bool),
	}
}

// Register registers all metrics with the default Prometheus registry.
func (e *Exporter) Register() {
	prometheus.MustRegister(
		e.processComputeUtil,
		e.processMemUsed,
		e.processIdleSecs,
		e.processIdleMem,
		e.deviceUtil,
		e.deviceMemUsed,
		e.deviceMemTotal,
		e.devicePower,
		e.deviceTemp,
		e.idleMemTotal,
	)
}

// UpdateMetrics sets all Prometheus gauges from the latest snapshot and idle states.
func (e *Exporter) UpdateMetrics(snap *collector.Snapshot, states []idle.ProcessIdleState) {
	// --- Device-level metrics ---
	for _, d := range snap.Devices {
		gpuStr := strconv.Itoa(d.Index)
		labels := prometheus.Labels{"gpu": gpuStr, "model": d.Name, "uuid": d.UUID}

		e.deviceUtil.With(labels).Set(float64(d.Utilization))
		e.deviceMemUsed.With(labels).Set(float64(d.MemoryUsed))
		e.deviceMemTotal.With(labels).Set(float64(d.MemoryTotal))
		e.devicePower.With(labels).Set(d.PowerWatts)
		e.deviceTemp.With(labels).Set(float64(d.TempCelsius))
	}

	// --- Per-process metrics + aggregate idle memory ---
	currentKeys := make(map[string]bool, len(states))
	idleMemByGPU := make(map[int]uint64)

	for _, ps := range states {
		gpuStr := strconv.Itoa(ps.GPU)
		pidStr := strconv.FormatUint(uint64(ps.PID), 10)
		labels := prometheus.Labels{"gpu": gpuStr, "pid": pidStr, "process": ps.ProcessName}
		key := gpuStr + "\x00" + pidStr + "\x00" + ps.ProcessName
		currentKeys[key] = true

		e.processComputeUtil.With(labels).Set(float64(ps.SmUtil))
		e.processMemUsed.With(labels).Set(float64(ps.UsedMemory))
		e.processIdleSecs.With(labels).Set(ps.IdleDuration.Seconds())
		e.processIdleMem.With(labels).Set(float64(ps.IdleMemory))

		idleMemByGPU[ps.GPU] += ps.IdleMemory
	}

	// Aggregate idle memory per GPU
	for _, d := range snap.Devices {
		gpuStr := strconv.Itoa(d.Index)
		e.idleMemTotal.With(prometheus.Labels{"gpu": gpuStr}).Set(float64(idleMemByGPU[d.Index]))
	}

	// --- Stale series cleanup ---
	for prevKey := range e.prevProcessKeys {
		if !currentKeys[prevKey] {
			parts := strings.SplitN(prevKey, "\x00", 3)
			if len(parts) == 3 {
				labels := prometheus.Labels{"gpu": parts[0], "pid": parts[1], "process": parts[2]}
				e.processComputeUtil.Delete(labels)
				e.processMemUsed.Delete(labels)
				e.processIdleSecs.Delete(labels)
				e.processIdleMem.Delete(labels)
			}
		}
	}
	e.prevProcessKeys = currentKeys
}
