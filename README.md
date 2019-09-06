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

* Kubernetes Scheduler Operator

## Configuration


The configuration for the Kubernetes Scheduler is the result of merging:

* a [default config](https://github.com/openshift/cluster-kube-scheduler-operator/blob/master/bindata/v4.1.0/kube-scheduler/defaultconfig-postbootstrap.yaml)
* observed config (compare observed values above) from the spec `schedulers.config.openshift.io`.

All of these are sparse configurations, i.e. unvalidated json snippets which are merged in order to form a valid configuration at the end.

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
```

The current operator status is reported using the `ClusterOperator` resource. To get the current status you can run follow command:

```
$ oc get clusteroperator/kube-scheduler
```



