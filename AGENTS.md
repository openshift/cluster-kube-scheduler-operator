# Cluster Kube Scheduler Operator

A static pod operator that manages the lifecycle of `kube-scheduler` on OpenShift control plane nodes. Built on the [library-go](https://github.com/openshift/library-go) static pod operator framework, it observes cluster configuration and reconciles the target kube-scheduler config into static pod manifests. Installed by the [Cluster Version Operator](https://github.com/openshift/cluster-version-operator) (CVO).

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full design and data flow.

## Build and Test

```bash
make build                       # Build all binaries (operator + OTE test runner)
make test                        # Unit tests (./pkg/... ./cmd/... ./bindata/...)
make verify                      # Formatting, vetting, golang version checks
make e2e                         # E2E operator tests (1h timeout)
make test-e2e-preferred-host     # E2E preferred-host tests (1h timeout)
```

Go version: see `go.mod`.

## Project Structure

| Directory | Purpose |
|-----------|---------|
| `bindata/` | Embedded assets: default config, static pod template, alerts, RBAC, bootstrap manifests |
| `cmd/cluster-kube-scheduler-operator/` | Operator binary entry point (operator, installer, pruner) |
| `cmd/cluster-kube-scheduler-operator-tests-ext/` | OpenShift Tests Extension (OTE) test runner entry point |
| `cmd/render/` | Bootstrap manifest renderer for cluster installation |
| `manifests/` | CVO deployment manifests (namespace, deployment, RBAC, ServiceMonitors) |
| `pkg/operator/configmetrics/` | Prometheus metrics derived from the Scheduler CR |
| `pkg/operator/configobservation/` | Configuration observers — watch cluster state to produce operand config |
| `pkg/operator/operatorclient/` | Namespace constants and operator client interfaces |
| `pkg/operator/resourcesynccontroller/` | Syncs ConfigMaps/Secrets between namespaces |
| `pkg/operator/starter.go` | Operator initialization — creates clients, informers, and starts all controllers |
| `pkg/operator/targetconfigcontroller/` | Renders observed config + defaults into kube-scheduler ConfigMaps |
| `test/e2e/` | E2E test suite (operator behavior) |
| `test/e2e-preferred-host/` | E2E test suite (preferred-host kube-apiserver communication) |
| `test/library/` | Shared test utilities |

## Controller Pattern

Controllers use the library-go `factory.Controller` base. Each controller has a `sync(ctx, syncContext)` method called by the framework on informer events or periodic resyncs. The operator wires them in `pkg/operator/starter.go` via `RunOperator()`.

Config observers follow a specific pattern: each observer function receives the existing config and returns `(observedConfig, errors)`. Observers are registered in `pkg/operator/configobservation/configobservercontroller/observe_config_controller.go`.

## Key Conventions

- **Namespaces:** `openshift-kube-scheduler-operator` (operator), `openshift-kube-scheduler` (operand), `openshift-config` (user config), `openshift-config-managed` (platform config). Constants in `pkg/operator/operatorclient/interfaces.go`.
- **Logging:** `k8s.io/klog/v2` with verbosity levels
- **Error handling:** wrap with `fmt.Errorf("context: %w", err)`
- **Feature gates:** controllers that depend on feature gates use `FeatureGateAccessor` from library-go; wait for gates before starting

## What Not To Do

These are specific mistakes that are easy to make in this codebase — read before changing anything:

- **Don't modify `vendor/` directly** — use `go mod tidy && go mod vendor`. Manual edits to `vendor/` will be overwritten and break reproducibility.

- **Don't edit `bindata/assets.go`** — this file uses Go's `embed` directive and is auto-generated. Update the asset files under `bindata/assets/` and `bindata/bootkube/`; never edit the embed wrapper itself.

- **Don't add a config observer without registering it** — new observer functions must be added to the observer list in `pkg/operator/configobservation/configobservercontroller/observe_config_controller.go`. A function that is never registered will silently do nothing.

- **Don't bypass the static pod upgrade protocol** — the installer/revision/pruner controllers in library-go manage rollout ordering and availability guarantees. Do not apply static pod changes directly to nodes or skip the revision mechanism.

- **Don't use direct API calls in config observers** — use the listers available in `configobservation.Listers`. Direct `client.Get()` or `client.List()` calls in observer functions add latency and load, and break the informer-driven design.

- **Don't fix library-go bugs here** — if a bug is in library-go, fix it upstream in [library-go](https://github.com/openshift/library-go) and vendor the fix. Workarounds in this repo create divergence that makes future rebases harder.

- **Don't rename namespace constants** without a full grep of all usages — `OperatorNamespace`, `TargetNamespace`, etc. in `pkg/operator/operatorclient/interfaces.go` are used throughout the codebase and in static asset templates under `bindata/`.

- **Don't assume feature gates are available at startup** — `featureGateAccessor.InitialFeatureGatesObserved()` must be awaited (with a timeout) before accessing feature gates. The operator already does this in `starter.go`; don't bypass or reorder it.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for full guidelines. Key rules:

- Do not modify files under `vendor/`. Use `go mod tidy && go mod vendor`.
- `bindata/assets.go` uses Go's `embed` directive to embed asset files — update the embedded files, not this file.
- Write unit tests for every change. E2E tests for significant features.
- Backwards compatibility matters — deprecate before removing.
- Before modifying the operator API, ensure there is a corresponding enhancement proposal in [openshift/enhancements](https://github.com/openshift/enhancements). API changes require design review and approval.

## Testing

- **Unit tests:** co-located `*_test.go` files, table-driven, `go test ./pkg/... ./cmd/... ./bindata/...`
- **E2E tests:** suites under `test/e2e/` and `test/e2e-preferred-host/`, each with its own Makefile target. Tests use the Ginkgo v2 framework via the OTE framework.
- **OTE framework:** `cluster-kube-scheduler-operator-tests-ext` binary. See [CONTRIBUTING.md](CONTRIBUTING.md#openshift-tests-extension-ote) for usage.
