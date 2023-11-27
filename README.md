# Kubernetes Scheduler operator

The Kubernetes Scheduler operator manages and updates the [Kubernetes Scheduler](https://github.com/kubernetes/kubernetes) deployed on top of
[OpenShift](https://openshift.io). The operator is based on OpenShift [library-go](https://github.com/openshift/library-go) framework and it
is installed via [Cluster Version Operator](https://github.com/openshift/cluster-version-operator) (CVO).

It contains the following components:

* Operator
* Bootstrap manifest renderer
* Installer based on static pods
* Configuration observer

By default, the operator exposes [Prometheus](https://prometheus.io) metrics via `metrics` service.
The metrics are collected from following components:

* Kubernetes Scheduler operator


## Configuration

The configuration for the Kubernetes Scheduler is the result of merging:

* a [default config](https://github.com/openshift/cluster-kube-scheduler-operator/blob/master/bindata/assets/config/defaultconfig.yaml)
* an observed config (compare observed values above) from the spec `schedulers.config.openshift.io`.

All of these are sparse configurations, i.e. unvalidated json snippets which are merged in order to form a valid configuration at the end.

## Scheduling profiles

The following profiles are currently provided:
* [`HighNodeUtilization`](#HighNodeUtilization)
* [`LowNodeUtilization`](#LowNodeUtilization)
* [`NoScoring`](#NoScoring)

Each of these enables cluster-wide scheduling.
Configured via [`Scheduler`](https://github.com/openshift/api/blob/master/config/v1/types_scheduling.go#L11) custom resource:

```
$ oc get scheduler cluster -o yaml
```

```yaml
apiVersion: config.openshift.io/v1
kind: Scheduler
metadata:
  name: cluster
spec:
  mastersSchedulable: false
  policy:
    name: ""
  profile: LowNodeUtilization
  ...
```

### HighNodeUtilization

This profile disables `NodeResourcesBalancedAllocation` and `NodeResourcesFit` plugin with (`LeastAllocated` type)
and enables `NodeResourcesFit` plugin (with `MostAllocated` type).
Favoring nodes that have a high allocation of resources.
In the past the profile corresponded to disabling `NodeResourcesLeastAllocated` and `NodeResourcesBalancedAllocation` plugins
and enabling `NodeResourcesMostAllocated` plugin.

### LowNodeUtilization

The default list of scheduling profiles as provided by the kube-scheduler.

### NoScoring

This profiles disabled all scoring plugins.

## Debugging

Operator also expose events that can help debugging issues. To get operator events, run following command:

```
$ oc get events -n  openshift-cluster-kube-scheduler-operator
```

This operator is configured via [`KubeScheduler`](https://github.com/openshift/api/blob/master/operator/v1/types_scheduler.go#L12) custom resource:

```
$ oc describe kubescheduler
```
```yaml
apiVersion: operator.openshift.io/v1
kind: KubeScheduler
metadata:
  name: cluster
spec:
  managementState: Managed
  ...
```

The log level of individual kube-scheduler instances can be increased by setting `.spec.logLevel` field:
```
$ oc explain kubescheduler.spec.logLevel
KIND:     KubeScheduler
VERSION:  operator.openshift.io/v1

FIELD:    logLevel <string>

DESCRIPTION:
     logLevel is an intent based logging for an overall component. It does not
     give fine grained control, but it is a simple way to manage coarse grained
     logging choices that operators have to interpret for their operands. Valid
     values are: "Normal", "Debug", "Trace", "TraceAll". Defaults to "Normal".
```

For example:
```yaml
apiVersion: operator.openshift.io/v1
kind: KubeScheduler
metadata:
  name: cluster
spec:
  logLevel: Debug
  ...
```

More about the individual configuration options can be learnt by invoking `oc explain`:

```
$ oc explain kubescheduler
```


The current operator status is reported using the `ClusterOperator` resource. To get the current status you can run follow command:

```
$ oc get clusteroperator/kube-scheduler
```

## Developing and debugging the operator

In the running cluster [cluster-version-operator](https://github.com/openshift/cluster-version-operator/) is responsible
for maintaining functioning and non-altered elements.  In that case to be able to use custom operator image one has to
perform one of these operations:

1. Set your operator in umanaged state, see [here](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusterversion.md) for details, in short:

```
oc patch clusterversion/version --type='merge' -p "$(cat <<- EOF
spec:
  overrides:
  - group: apps
    kind: Deployment
    name: kube-scheduler-operator
    namespace: openshift-kube-scheduler-operator
    unmanaged: true
EOF
)"
```

2. Scale down cluster-version-operator:

```
oc scale --replicas=0 deploy/cluster-version-operator -n openshift-cluster-version
```

IMPORTANT: This apprach disables cluster-version-operator completly, whereas previous only tells it to not manage a kube-scheduler-operator!

After doing this you can now change the image of the operator to the desired one:

```
oc patch pod/openshift-kube-scheduler-operator-<rand_digits> -n openshift-kube-scheduler-operator -p '{"spec":{"containers":[{"name":"kube-scheduler-operator-container","image":"<user>/cluster-kube-scheduler-operator"}]}}'
```

## Developing and debugging the bootkube bootstrap phase

The operator image version used by the [installer](https://github.com/openshift/installer/blob/master/pkg/asset/ignition/bootstrap/) bootstrap phase can be overridden by creating a custom origin-release image pointing to the developer's operator `:latest` image:

```
$ IMAGE_ORG=<user> make images
$ docker push <user>/origin-cluster-kube-scheduler-operator

$ cd ../cluster-kube-apiserver-operator
$ IMAGES=cluster-kube-scheduler-operator IMAGE_ORG=<user> make origin-release
$ docker push <user>/origin-release:latest

$ cd ../installer
$ OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE=docker.io/<user>/origin-release:latest bin/openshift-install cluster ...
```

## Profiling with pprof

### Enable profiling

By default the kube-scheduler profiling is disabled. The profiling can be enabled manually by editing `config.yaml` files under each master node.

Warning: the configuration gets undone after the new revision gets performed and the steps need to be repeated.

**Steps**:

1. access every master node (e.g. `ssh` or with `oc debug`)
   1. edit `/etc/kubernetes/static-pod-resources/kube-scheduler-pod-$REV/configmaps/config/config.yaml` (where `$REV` corresponds to the latest revision) and set `enableProfiling` field to `True`.
   1. make a benign change to `/etc/kubernetes/manifests/kube-scheduler-pod.yaml`, e.g. updating "Waiting for port" to "Waiting for port" (adding one blank space to the string). Wait for the updated pod manifest to be picked up and a new kube-scheduler instance running and ready.
1. `oc port-forward pod/$KUBE_SCHEDULER_POD_NAME 10259:10259` in a separate terminal/window (where `$KUBE_SCHEDULER_POD_NAME` corresponds to a running kube-scheduler pod instance)
1. apply the following manifests to allow anonymous access:

   ```
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRole
   metadata:
    name: kubescheduler-anonymous-access
   rules:
   - nonResourceURLs: ["/debug", "/debug/*"]
     verbs:
     - get
     - list
   ---
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRoleBinding
   metadata:
    name: kubescheduler-anonymous-access
   roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: ClusterRole
    name: kubescheduler-anonymous-access
   subjects:
   - apiGroup: rbac.authorization.k8s.io
     kind: User
     name: system:anonymous
   ```
1. access https://localhost:10259/debug/pprof/

### heap profiling

The tool requires to pull the heap file and the kube-scheduler binary.

**Steps**:

1. Pull the heap data by accessing https://localhost:10259/debug/pprof/heap
1. Extract the kube-scheduler binary from the corresponding image (by checking the kube-scheduler pod manifest):
   ```sh
   $ podman pull --authfile $AUTHFILE $KUBE_SCHEDULER_IMAGE
   $ podman cp $(podman create --name kube-scheduler $KUBE_SCHEDULER_IMAGE):/usr/bin/kube-scheduler ./kube-scheduler
   ```
   - `$AUTHFILE` corresponds to your authentication file if not already located in the known paths
   - `$KUBE_SCHEDULER_IMAGE` corresponds to the kube-scheduler image found in a kube-scheduler pod manifest
1. Run `go tool pprof kube-scheduler heap`
