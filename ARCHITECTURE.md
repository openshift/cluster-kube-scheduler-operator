# Architecture

## Overview

The cluster-kube-scheduler-operator is a static pod operator that manages the `kube-scheduler` on OpenShift control plane nodes. It is deployed by the Cluster Version Operator (CVO) and uses the [library-go](https://github.com/openshift/library-go) static pod operator framework.

The operator's primary responsibilities:
- Observe cluster configuration from multiple sources and synthesize kube-scheduler config
- Manage kube-scheduler static pods across control plane nodes (install, revision, prune)
- Report status via the `ClusterOperator/kube-scheduler` resource

## Data Flow

```text
 config.openshift.io resources
 (Scheduler, APIServer)
              │
              ▼
   ┌───────────────────────────────────────────────────┐
   │              Config Observer Controllers          │
   │  (observe external state, produce observedConfig) │
   └──────────────────────┬────────────────────────────┘
                          │ observedConfig (sparse JSON)
                          ▼
   ┌───────────────────────────────────────────────────┐
   │           Target Config Controller                │
   │  (merge defaults + observedConfig + overrides     │
   │   → render ConfigMaps in target namespace)        │
   └──────────────────────┬────────────────────────────┘
                          │ ConfigMaps
                          ▼
   ┌───────────────────────────────────────────────────┐
   │          Static Pod Controllers (library-go)      │
   │  Installer → Revision Controller → Pruner         │
   │  (roll out new revisions to each control plane    │
   │   node as static pod manifests)                   │
   └──────────────────────┬────────────────────────────┘
                          │
                          ▼
              kube-scheduler static pods
              (one per control plane node)
```

## Operator Startup

Entry point: `cmd/cluster-kube-scheduler-operator/main.go` → `pkg/operator/starter.go:RunOperator()`.

Startup sequence:
1. Create clients (Kubernetes, config, operator)
2. Create informers for watched namespaces (see [Namespaces](#namespaces))
3. Initialize feature gates via `FeatureGateAccessor` and wait for observation (1-minute timeout)
4. Create and start all controllers concurrently
5. Block until context cancellation

## Namespaces

| Namespace | Constant | Purpose |
|-----------|----------|---------|
| `openshift-config` | `GlobalUserSpecifiedConfigNamespace` | User-provided configuration (policy configmaps) |
| `openshift-config-managed` | `GlobalMachineSpecifiedConfigNamespace` | Platform-managed configuration |
| `openshift-kube-scheduler-operator` | `OperatorNamespace` | Operator deployment and its resources |
| `openshift-kube-scheduler` | `TargetNamespace` | Operand: kube-scheduler pods and config |

The `ResourceSyncController` copies ConfigMaps and Secrets between these namespaces as needed (e.g. policy configmap from `openshift-config` to the target namespace).

## Static Pod Management

The operator uses library-go's `staticpod.NewBuilder()` to manage kube-scheduler static pods. This framework provides:

- **Installer controller** — creates new static pod revisions on each control plane node. Uses a custom installer command (`cluster-kube-scheduler-operator installer`).
- **Revision controller** — tracks revisions of ConfigMaps and Secrets. When any revisioned resource changes, a new revision is created. The first ConfigMap in the list (`kube-scheduler-pod`) contains the static pod manifest template.
- **Pruner** — removes old static pod revisions to free disk space.
- **PDB guard** — ensures availability during upgrades (only on multi-node clusters; disabled for single-node).

Resources are split into two categories:
- **Revisioned** — ConfigMaps and Secrets declared in `deploymentConfigMaps`/`deploymentSecrets` in `starter.go`. Any change to these triggers a new static pod revision. The first ConfigMap in the list contains the static pod manifest template.
- **Unrevisioned certs** — Secrets declared in `CertSecrets` in `starter.go`, managed by `WithUnrevisionedCerts`. These are updated in-place without triggering a new revision.

## Configuration Observers

Configuration observers watch external cluster resources and produce a sparse JSON config (`observedConfig`) that gets merged into the kube-scheduler configuration. Each observer function receives the existing config and returns `(observedConfig, errors)`.

Observers are registered in `pkg/operator/configobservation/configobservercontroller/observe_config_controller.go`:

| Package | Watches | Effect |
|---------|---------|--------|
| `scheduler/` | `Scheduler` CR | Syncs `policy-configmap` from `openshift-config` to `openshift-kube-scheduler` if `.spec.policy.name` is set |
| *(library-go)* `apiserver` | `APIServer` CR | TLS security profile |

## Target Config Controller

`pkg/operator/targetconfigcontroller/` takes the merged configuration (defaults + observedConfig + unsupportedConfigOverrides) and renders it into concrete resources in the target namespace:

- `config` ConfigMap — the main kube-scheduler configuration, built by merging `bindata/assets/config/defaultconfig.yaml` with the observed config and applying the scheduling profile
- `kube-scheduler-pod` ConfigMap — the static pod manifest template

The target config controller handles scheduling profiles (`LowNodeUtilization`, `HighNodeUtilization`, `NoScoring`) and passes cluster feature gates as `--feature-gates` flags to the scheduler binary.

## Render Command

`cmd/render/` is a bootstrap manifest renderer used during cluster installation. It takes installer-provided inputs and renders the initial set of manifests needed to bootstrap kube-scheduler before the operator is running. The templates live in `bindata/bootkube/`.

## Other Controllers

| Controller | Purpose |
|-----------|---------|
| `StaticResourceController` | Applies static manifests from `bindata/` (namespace, network policies, RBAC, service, ServiceAccount, kubeconfig) |
| `ClusterOperatorStatus` | Reports operator status, versions, and related objects to `ClusterOperator/kube-scheduler` |
| `ResourceSyncController` | Syncs ConfigMaps/Secrets between namespaces (e.g. policy configmap) |
| `ConfigMetrics` | Registers Prometheus metrics derived from the `Scheduler` CR (`cluster_master_schedulable`, `cluster_legacy_scheduler_policy`, `cluster_default_node_selector`) |
