# Deployment Mode

Runs the exporter as a standard Deployment with topology spread constraints to place one pod per GPU node. Same privileges as the DaemonSet, but with manual replica management.

## When to use

- You want node-wide GPU monitoring but **can't or don't want to use a DaemonSet** (e.g., DaemonSets are restricted by policy, or you want more control over rollout strategy)
- You prefer managing replicas explicitly rather than relying on DaemonSet auto-scheduling

## Requirements

- NVIDIA driver >= 535.113.01 (for per-process SM utilization)
- Cluster-admin privileges (the pod runs as privileged with `hostPID: true`)
- GPU nodes with the NVIDIA driver and libraries installed

## How it differs from DaemonSet

| | DaemonSet | Deployment |
|---|-----------|------------|
| Auto-schedules on new nodes | Yes | No |
| Replica management | Automatic (one per matching node) | Manual (set `replicas` count) |
| Rollout strategy | `RollingUpdate` or `OnDelete` | Full Deployment rollout options |
| Pod distribution | Guaranteed one per node | Best-effort via `topologySpreadConstraints` |

## Quick start

```bash
# Set replicas to match your GPU node count
kubectl apply -f deployment.yaml
```

## Scaling

Update the `replicas` field to match the number of GPU nodes in your cluster. The `topologySpreadConstraints` with `maxSkew: 1` and `whenUnsatisfiable: DoNotSchedule` ensures pods spread across nodes rather than stacking on one.

## Customization

See the [DaemonSet README](../daemonset/README.md) for node affinity and NVIDIA library path customization — the same options apply here.
