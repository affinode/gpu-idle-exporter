# DaemonSet Deployment

Runs the exporter on every GPU node in the cluster. This is the recommended deployment mode for cluster-wide GPU monitoring.

## When to use

- You want visibility into **all GPU processes across all nodes**
- You have cluster-admin access (or can deploy privileged DaemonSets)
- You want automatic scheduling on new GPU nodes as they join the cluster

## Requirements

- NVIDIA driver >= 535.113.01 (for per-process SM utilization)
- Cluster-admin privileges (the pod runs as privileged with `hostPID: true`)
- GPU nodes with the NVIDIA driver and libraries installed

## What you get

- Per-process metrics with full process names (PID resolution via host `/proc`)
- Device-level utilization, memory, power, and temperature metrics
- Idle process tracking with duration and memory breakdown
- Automatic `node` label on all metrics (via Downward API)

## Quick start

```bash
# Review and edit daemonset.yaml for your environment first
kubectl apply -f daemonset.yaml
```

## Prometheus scraping

The DaemonSet includes annotation-based autodiscovery:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "9835"
prometheus.io/path: "/metrics"
```

No additional ServiceMonitor or scrape config is needed if your Prometheus is configured for annotation-based discovery.

## Customization

### Node affinity

The manifest ships with a GKE-specific node selector (`cloud.google.com/gke-accelerator`). Update the `nodeAffinity` section for your environment:

| Provider | Label |
|----------|-------|
| GKE | `cloud.google.com/gke-accelerator: Exists` (default) |
| EKS | `k8s.amazonaws.com/accelerator: nvidia-gpu` |
| Generic | `nvidia.com/gpu.present: "true"` |

### NVIDIA library path

The host volume mount assumes GKE's NVIDIA library path (`/home/kubernetes/bin/nvidia/lib64`). Common alternatives:

- Ubuntu/Debian: `/usr/lib/x86_64-linux-gnu/`
- CUDA base images: `/usr/local/nvidia/lib64/`

Update both the `hostPath` and the `LD_LIBRARY_PATH` env var if your path differs.
