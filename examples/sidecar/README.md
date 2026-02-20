# Sidecar Deployment

Runs the exporter as a sidecar container alongside your GPU workload. No cluster-admin privileges required.

## When to use

- You want per-workload idle monitoring without cluster-wide access
- Your team doesn't have permission to deploy DaemonSets or privileged pods
- You want metrics scoped to a specific application

## Requirements

- NVIDIA driver >= 535.113.01 on the node
- [nvidia-container-runtime](https://github.com/NVIDIA/nvidia-container-toolkit) installed on nodes (standard on GKE, EKS with GPU AMIs, and most managed Kubernetes GPU offerings)

## Tradeoffs

**Process names show as "unknown"**: NVML returns host-level PIDs, but without `hostPID` the sidecar can't resolve them via `/proc/<pid>/cmdline`. The metrics still track per-process memory and utilization — you just won't see the process name. In sidecar mode this is usually fine since you already know what workload is running in the pod.

**Sees all GPU processes on the node, not just the pod's**: NVML queries the physical GPU, so the exporter reports all processes using that GPU. Use the `pod` and `namespace` labels (injected automatically via env vars) to identify which metrics belong to your workload in Prometheus.

## Quick start

Copy the sidecar container spec from `pod-example.yaml` into your existing Pod or Deployment:

```yaml
# Add to your pod spec:
spec:
  shareProcessNamespace: true  # Required
  containers:
  - name: your-workload
    # ... your existing container ...
  - name: gpu-idle-exporter
    # ... copy from pod-example.yaml ...
```

Or deploy the full example:

```bash
kubectl apply -f pod-example.yaml
```

## How it works

The sidecar relies on the `nvidia-container-runtime` to inject NVIDIA libraries and device access into the container. The key env vars are:

```yaml
NVIDIA_VISIBLE_DEVICES: "all"
NVIDIA_DRIVER_CAPABILITIES: "utility"
```

These tell the runtime to expose GPU devices and the NVML utility library without needing privileged mode.

## Prometheus scraping

The pod includes annotation-based scrape config. Metrics automatically include `pod`, `namespace`, and `node` labels from the Downward API env vars.

## Fallback: manual volume mounts

If your cluster doesn't have the nvidia-container-runtime, you'll need to:

1. Run the sidecar as privileged (`securityContext.privileged: true`)
2. Mount the host NVIDIA libraries and `/dev` as volumes
3. Remove the `NVIDIA_VISIBLE_DEVICES` and `NVIDIA_DRIVER_CAPABILITIES` env vars

See the commented-out section at the bottom of `pod-example.yaml` for the exact configuration.
