# Sidecar Deployment

Runs the exporter as a sidecar container alongside your GPU workload. Useful when you want per-workload monitoring without deploying a cluster-wide DaemonSet.

## When to use

- You want per-workload idle monitoring without a cluster-wide deployment
- You want metrics automatically labeled with the pod name and namespace
- You want to bundle monitoring into your application's pod spec

## Requirements

- NVIDIA driver >= 535.113.01 on the node
- Privileged container access (the sidecar needs to reach GPU devices via host volume mounts)

## Tradeoffs

**Same privilege level as other modes**: NVML requires access to `/dev/nvidiactl` and the NVIDIA shared libraries, which means the sidecar needs privileged mode and host volume mounts — the same as the DaemonSet and Deployment modes. The difference is scope: the sidecar runs per-workload rather than per-node.

**Process names show as "unknown"**: NVML returns host-level PIDs, but without `hostPID` the sidecar can't resolve them via `/proc/<pid>/cmdline`. The metrics still track per-process memory and utilization — you just won't see the process name. In sidecar mode this is usually fine since you already know what workload is running in the pod.

**Sees all GPU processes on the node, not just the pod's**: NVML queries the physical GPU, so the exporter reports all processes using that GPU. Use the `pod` and `namespace` labels (injected automatically via env vars) to identify which metrics belong to your workload in Prometheus.

## Quick start

Copy the sidecar container spec and volumes from `pod-example.yaml` into your existing Pod or Deployment:

```yaml
# Add to your pod spec:
spec:
  shareProcessNamespace: true
  containers:
  - name: your-workload
    # ... your existing container ...
  - name: gpu-idle-exporter
    # ... copy from pod-example.yaml ...
  volumes:
    # ... copy nvidia-lib and host-dev volumes from pod-example.yaml ...
```

Or deploy the full example:

```bash
kubectl apply -f pod-example.yaml
```

## Prometheus scraping

The pod includes annotation-based scrape config. Metrics automatically include `pod`, `namespace`, and `node` labels from the Downward API env vars.

## Customization

### NVIDIA library path

The host volume mount assumes GKE's path (`/home/kubernetes/bin/nvidia/lib64`). See the [DaemonSet README](../daemonset/README.md#nvidia-library-path) for common alternatives on other platforms.
