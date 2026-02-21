# gpu-idle-exporter

Prometheus exporter that detects idle GPU processes — processes holding GPU memory without doing any compute work. Useful for identifying wasted GPU resources in shared Kubernetes clusters.

## How it works

The exporter polls NVIDIA GPUs via [NVML](https://developer.nvidia.com/nvidia-management-library-nvml) every 5 seconds (configurable) and tracks per-process compute utilization:

1. **Collect**: Queries each GPU for running processes, their memory usage, and SM (streaming multiprocessor) utilization
2. **Track**: Maintains per-process state across polls. A process is marked idle when it holds GPU memory but has 0% SM utilization for two consecutive polls (avoiding false positives from newly started processes)
3. **Export**: Publishes Prometheus metrics with per-process and per-device breakdowns

Stale processes (disappeared from NVML results for 30s) are automatically cleaned up.

## Metrics

### Per-process metrics

Labels: `gpu` (index), `pid`, `process` (name)

| Metric | Description |
|--------|-------------|
| `gpu_idle_process_compute_utilization_percent` | SM utilization percentage for this process |
| `gpu_idle_process_memory_used_bytes` | GPU memory held by this process |
| `gpu_idle_process_idle_seconds` | How long this process has been idle (0 when active) |
| `gpu_idle_process_idle_memory_bytes` | Memory held while idle (0 when active) |

### Device-level metrics

Labels: `gpu` (index), `model`, `uuid`

| Metric | Description |
|--------|-------------|
| `gpu_idle_device_utilization_percent` | Device-level compute utilization |
| `gpu_idle_device_memory_used_bytes` | Total memory in use on this GPU |
| `gpu_idle_device_memory_total_bytes` | Total memory capacity |
| `gpu_idle_device_power_watts` | Current power draw |
| `gpu_idle_device_temperature_celsius` | Core temperature |

### Aggregate metrics

Labels: `gpu` (index)

| Metric | Description |
|--------|-------------|
| `gpu_idle_memory_total_bytes` | Total memory held by all idle processes on this GPU |

## Requirements

- NVIDIA driver >= 535.113.01 (for per-process utilization via `nvmlDeviceGetProcessUtilization`)
- Kubernetes with GPU nodes (for Kubernetes deployment)

## Quick start

### Build from source

```bash
make build
```

### Docker

```bash
make docker
# Produces: ghcr.io/affinode/gpu-idle-exporter:latest
```

On Apple Silicon Macs, build with `--platform linux/amd64` since the binary targets amd64:

```bash
docker build --platform linux/amd64 -t ghcr.io/affinode/gpu-idle-exporter:latest -f deployments/docker/Dockerfile .
```

### Deploy to Kubernetes

The exporter can be deployed in different configurations depending on your cluster setup and monitoring needs. The [`examples/`](examples/) directory contains ready-to-use manifests for each mode:

- **[DaemonSet](examples/daemonset/)** — One pod per GPU node, cluster-wide visibility. Auto-schedules on new GPU nodes.
- **[Deployment](examples/deployment/)** — Like DaemonSet but with manual replica control. Useful when DaemonSets are restricted by policy.
- **[Sidecar](examples/sidecar/)** — Runs alongside your GPU workload. Per-workload scoped, no cluster-wide deployment needed.

Each example directory includes a README with requirements, tradeoffs, and customization instructions.

## Configuration

| Environment variable | Default | Description |
|---------------------|---------|-------------|
| `POLL_INTERVAL` | `5s` | How often to poll NVML (Go duration format) |
| `HTTP_PORT` | `9835` | Port for the `/metrics` and `/healthz` endpoints |
| `NODE_NAME` | _(unset)_ | If set, adds a `node` constant label to all metrics |
| `POD_NAME` | _(unset)_ | If set, adds a `pod` constant label to all metrics |
| `POD_NAMESPACE` | _(unset)_ | If set, adds a `namespace` constant label to all metrics |

## Example Prometheus queries

```promql
# Total idle GPU memory across all GPUs (bytes)
sum(gpu_idle_memory_total_bytes)

# Processes idle for more than 10 minutes
gpu_idle_process_idle_seconds > 600

# Top idle memory consumers
topk(10, gpu_idle_process_idle_memory_bytes > 0)

# Percentage of GPU memory wasted by idle processes
sum(gpu_idle_memory_total_bytes) by (gpu)
  / sum(gpu_idle_device_memory_total_bytes) by (gpu) * 100

# Alert: any process idle for over 1 hour holding more than 1 GiB
gpu_idle_process_idle_seconds > 3600 and gpu_idle_process_idle_memory_bytes > 1e9
```

## License

MIT
