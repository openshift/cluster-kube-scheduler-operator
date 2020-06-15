// Code generated for package v410_00_assets by go-bindata DO NOT EDIT. (@generated)
// sources:
// bindata/v4.1.0/config/defaultconfig-postbootstrap.yaml
// bindata/v4.1.0/config/defaultconfig.yaml
// bindata/v4.1.0/kube-scheduler/cm.yaml
// bindata/v4.1.0/kube-scheduler/kubeconfig-cert-syncer.yaml
// bindata/v4.1.0/kube-scheduler/kubeconfig-cm.yaml
// bindata/v4.1.0/kube-scheduler/leader-election-rolebinding.yaml
// bindata/v4.1.0/kube-scheduler/localhost-recovery-client-crb.yaml
// bindata/v4.1.0/kube-scheduler/localhost-recovery-sa.yaml
// bindata/v4.1.0/kube-scheduler/localhost-recovery-token.yaml
// bindata/v4.1.0/kube-scheduler/ns.yaml
// bindata/v4.1.0/kube-scheduler/pod-cm.yaml
// bindata/v4.1.0/kube-scheduler/pod.yaml
// bindata/v4.1.0/kube-scheduler/policyconfigmap-role.yaml
// bindata/v4.1.0/kube-scheduler/policyconfigmap-rolebinding.yaml
// bindata/v4.1.0/kube-scheduler/sa.yaml
// bindata/v4.1.0/kube-scheduler/scheduler-clusterrolebinding.yaml
// bindata/v4.1.0/kube-scheduler/svc.yaml
package v410_00_assets

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _v410ConfigDefaultconfigPostbootstrapYaml = []byte(`apiVersion: kubescheduler.config.k8s.io/v1alpha1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/static-pod-resources/configmaps/scheduler-kubeconfig/kubeconfig
leaderElection:
  leaderElect: true
  lockObjectNamespace: "openshift-kube-scheduler"
  resourceLock: "configmaps"
`)

func v410ConfigDefaultconfigPostbootstrapYamlBytes() ([]byte, error) {
	return _v410ConfigDefaultconfigPostbootstrapYaml, nil
}

func v410ConfigDefaultconfigPostbootstrapYaml() (*asset, error) {
	bytes, err := v410ConfigDefaultconfigPostbootstrapYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/config/defaultconfig-postbootstrap.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410ConfigDefaultconfigYaml = []byte(`apiVersion: kubescheduler.config.k8s.io/v1alpha1
kind: KubeSchedulerConfiguration
`)

func v410ConfigDefaultconfigYamlBytes() ([]byte, error) {
	return _v410ConfigDefaultconfigYaml, nil
}

func v410ConfigDefaultconfigYaml() (*asset, error) {
	bytes, err := v410ConfigDefaultconfigYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/config/defaultconfig.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerCmYaml = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  namespace: openshift-kube-scheduler
  name: config
data:
  config.yaml:
`)

func v410KubeSchedulerCmYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerCmYaml, nil
}

func v410KubeSchedulerCmYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerCmYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/cm.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerKubeconfigCertSyncerYaml = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-scheduler-cert-syncer-kubeconfig
  namespace: openshift-kube-scheduler
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
      - cluster:
          certificate-authority: /etc/kubernetes/static-pod-resources/secrets/localhost-recovery-client-token/ca.crt
          server: https://localhost:6443
          tls-server-name: localhost-recovery
        name: loopback
    contexts:
      - context:
          cluster: loopback
          user: kube-scheduler
        name: kube-scheduler
    current-context: kube-scheduler
    kind: Config
    preferences: {}
    users:
      - name: kube-scheduler
        user:
          tokenFile: /etc/kubernetes/static-pod-resources/secrets/localhost-recovery-client-token/token
`)

func v410KubeSchedulerKubeconfigCertSyncerYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerKubeconfigCertSyncerYaml, nil
}

func v410KubeSchedulerKubeconfigCertSyncerYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerKubeconfigCertSyncerYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/kubeconfig-cert-syncer.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerKubeconfigCmYaml = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: scheduler-kubeconfig
  namespace: openshift-kube-scheduler
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
      - cluster:
          certificate-authority: /etc/kubernetes/static-pod-resources/configmaps/serviceaccount-ca/ca-bundle.crt
          server: https://localhost:6443
        name: loopback
    contexts:
      - context:
          cluster: loopback
          user: kube-scheduler
        name: kube-scheduler
    current-context: kube-scheduler
    kind: Config
    preferences: {}
    users:
      - name: kube-scheduler
        user:
          client-certificate: /etc/kubernetes/static-pod-certs/secrets/kube-scheduler-client-cert-key/tls.crt
          client-key: /etc/kubernetes/static-pod-certs/secrets/kube-scheduler-client-cert-key/tls.key
`)

func v410KubeSchedulerKubeconfigCmYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerKubeconfigCmYaml, nil
}

func v410KubeSchedulerKubeconfigCmYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerKubeconfigCmYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/kubeconfig-cm.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerLeaderElectionRolebindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  namespace: kube-system
  name: system:openshift:leader-locking-kube-scheduler
roleRef:
  kind: Role
  name: system::leader-locking-kube-scheduler
subjects:
- kind: User
  name: system:kube-scheduler
`)

func v410KubeSchedulerLeaderElectionRolebindingYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerLeaderElectionRolebindingYaml, nil
}

func v410KubeSchedulerLeaderElectionRolebindingYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerLeaderElectionRolebindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/leader-election-rolebinding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerLocalhostRecoveryClientCrbYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:openshift:operator:kube-scheduler-recovery
roleRef:
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: localhost-recovery-client
  namespace: openshift-kube-scheduler
`)

func v410KubeSchedulerLocalhostRecoveryClientCrbYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerLocalhostRecoveryClientCrbYaml, nil
}

func v410KubeSchedulerLocalhostRecoveryClientCrbYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerLocalhostRecoveryClientCrbYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/localhost-recovery-client-crb.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerLocalhostRecoverySaYaml = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: localhost-recovery-client
  namespace: openshift-kube-scheduler
`)

func v410KubeSchedulerLocalhostRecoverySaYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerLocalhostRecoverySaYaml, nil
}

func v410KubeSchedulerLocalhostRecoverySaYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerLocalhostRecoverySaYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/localhost-recovery-sa.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerLocalhostRecoveryTokenYaml = []byte(`apiVersion: v1
kind: Secret
metadata:
  name: localhost-recovery-client-token
  namespace: openshift-kube-scheduler
  annotations:
    kubernetes.io/service-account.name: localhost-recovery-client
type: kubernetes.io/service-account-token
`)

func v410KubeSchedulerLocalhostRecoveryTokenYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerLocalhostRecoveryTokenYaml, nil
}

func v410KubeSchedulerLocalhostRecoveryTokenYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerLocalhostRecoveryTokenYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/localhost-recovery-token.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerNsYaml = []byte(`apiVersion: v1
kind: Namespace
metadata:
  annotations:
    openshift.io/node-selector: ""
  name: openshift-kube-scheduler
  labels:
    openshift.io/run-level: "0"
    openshift.io/cluster-monitoring: "true"
`)

func v410KubeSchedulerNsYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerNsYaml, nil
}

func v410KubeSchedulerNsYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerNsYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/ns.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerPodCmYaml = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  namespace: openshift-kube-scheduler
  name: kube-scheduler-pod
data:
  pod.yaml:
  forceRedeploymentReason:
  version:
`)

func v410KubeSchedulerPodCmYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerPodCmYaml, nil
}

func v410KubeSchedulerPodCmYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerPodCmYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/pod-cm.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerPodYaml = []byte(`apiVersion: v1
kind: Pod
metadata:
  name: openshift-kube-scheduler
  namespace: openshift-kube-scheduler
  annotations:
    kubectl.kubernetes.io/default-logs-container: kube-scheduler
  labels:
    app: openshift-kube-scheduler
    scheduler: "true"
    revision: "REVISION"
spec:
  initContainers:
  - name: wait-for-host-port
    image: ${IMAGE}
    imagePullPolicy: IfNotPresent
    terminationMessagePolicy: FallbackToLogsOnError
    command: ['/usr/bin/timeout', '30', '/bin/bash', '-c']
    args:
    - |
      echo -n "Waiting for port :10259 and :10251 to be released."
      while [ -n "$(lsof -ni :10251)" -o -n "$(lsof -i :10259)" ]; do
        echo -n "."
        sleep 1
      done
  containers:
  - name: kube-scheduler
    image: ${IMAGE}
    imagePullPolicy: IfNotPresent
    terminationMessagePolicy: FallbackToLogsOnError
    command: ["/bin/bash", "-ec"]
    args:
    - |
      echo -n "Waiting kube-apiserver to respond."
      tries=0
      until curl --output /dev/null --silent -k https://localhost:6443/version; do
        echo -n "."
        sleep 1
        (( tries += 1 ))
        if [[ "${tries}" -gt 180 ]]; then
          echo "timed out waiting for kube-apiserver to respond."
          exit 1
        fi
      done
      echo

      exec hyperkube kube-scheduler
        --config=/etc/kubernetes/static-pod-resources/configmaps/config/config.yaml \
        --cert-dir=/var/run/kubernetes \
        --port=0 \
        --authentication-kubeconfig=/etc/kubernetes/static-pod-resources/configmaps/scheduler-kubeconfig/kubeconfig \
        --authorization-kubeconfig=/etc/kubernetes/static-pod-resources/configmaps/scheduler-kubeconfig/kubeconfig
    resources:
      requests:
        memory: 50Mi
        cpu: 15m
    ports:
    - containerPort: 10259
    volumeMounts:
    - mountPath: /etc/kubernetes/static-pod-resources
      name: resource-dir
    - mountPath: /etc/kubernetes/static-pod-certs
      name: cert-dir
    livenessProbe:
      httpGet:
        scheme: HTTP
        port: 10251
        path: healthz
      initialDelaySeconds: 45
      timeOutSeconds: 10
    readinessProbe:
      httpGet:
        scheme: HTTP
        port: 10251
        path: healthz
      initialDelaySeconds: 45
      timeOutSeconds: 10
  - name: kube-scheduler-cert-syncer
    env:
    - name: POD_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    - name: POD_NAMESPACE
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace
    image: ${OPERATOR_IMAGE}
    imagePullPolicy: IfNotPresent
    terminationMessagePolicy: FallbackToLogsOnError
    command: ["cluster-kube-scheduler-operator", "cert-syncer"]
    args:
      - --kubeconfig=/etc/kubernetes/static-pod-resources/configmaps/kube-scheduler-cert-syncer-kubeconfig/kubeconfig
      - --namespace=$(POD_NAMESPACE)
      - --destination-dir=/etc/kubernetes/static-pod-certs
    resources:
      requests:
        memory: 50Mi
        cpu: 5m
    volumeMounts:
      - mountPath: /etc/kubernetes/static-pod-resources
        name: resource-dir
      - mountPath: /etc/kubernetes/static-pod-certs
        name: cert-dir
  hostNetwork: true
  priorityClassName: system-node-critical
  tolerations:
  - operator: "Exists"
  volumes:
  - hostPath:
      path: /etc/kubernetes/static-pod-resources/kube-scheduler-pod-REVISION
    name: resource-dir
  - hostPath:
      path: /etc/kubernetes/static-pod-resources/kube-scheduler-certs
    name: cert-dir
`)

func v410KubeSchedulerPodYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerPodYaml, nil
}

func v410KubeSchedulerPodYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerPodYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/pod.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerPolicyconfigmapRoleYaml = []byte(`# As of now, system:kube-scheduler role cannot list, create or update configmaps from openshift-kube-scheduler namespace. So, creating a role.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: system:openshift:sa-listing-configmaps
  namespace: openshift-kube-scheduler
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - create
  - update
`)

func v410KubeSchedulerPolicyconfigmapRoleYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerPolicyconfigmapRoleYaml, nil
}

func v410KubeSchedulerPolicyconfigmapRoleYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerPolicyconfigmapRoleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/policyconfigmap-role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerPolicyconfigmapRolebindingYaml = []byte(`# As of now, system:kube-scheduler role cannot list configmaps from openshift-kube-scheduler namespace. So, creating a role.
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  namespace: openshift-kube-scheduler
  name: system:openshift:sa-listing-configmaps
roleRef:
  kind: Role
  name: system:openshift:sa-listing-configmaps
subjects:
- kind: User
  name: system:kube-scheduler
`)

func v410KubeSchedulerPolicyconfigmapRolebindingYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerPolicyconfigmapRolebindingYaml, nil
}

func v410KubeSchedulerPolicyconfigmapRolebindingYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerPolicyconfigmapRolebindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/policyconfigmap-rolebinding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerSaYaml = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  namespace: openshift-kube-scheduler
  name: openshift-kube-scheduler-sa
`)

func v410KubeSchedulerSaYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerSaYaml, nil
}

func v410KubeSchedulerSaYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerSaYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/sa.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerSchedulerClusterrolebindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:openshift:operator:kube-scheduler:public-2
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:kube-scheduler
subjects:
- kind: ServiceAccount
  name: openshift-kube-scheduler-sa
  namespace: openshift-kube-scheduler
`)

func v410KubeSchedulerSchedulerClusterrolebindingYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerSchedulerClusterrolebindingYaml, nil
}

func v410KubeSchedulerSchedulerClusterrolebindingYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerSchedulerClusterrolebindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/scheduler-clusterrolebinding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _v410KubeSchedulerSvcYaml = []byte(`apiVersion: v1
kind: Service
metadata:
  namespace: openshift-kube-scheduler
  name: scheduler
  annotations:
    service.alpha.openshift.io/serving-cert-secret-name: serving-cert
    prometheus.io/scrape: "true"
    prometheus.io/scheme: https
spec:
  selector:
    scheduler: "true"
  ports:
  - name: https
    port: 443
    targetPort: 10259
`)

func v410KubeSchedulerSvcYamlBytes() ([]byte, error) {
	return _v410KubeSchedulerSvcYaml, nil
}

func v410KubeSchedulerSvcYaml() (*asset, error) {
	bytes, err := v410KubeSchedulerSvcYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "v4.1.0/kube-scheduler/svc.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"v4.1.0/config/defaultconfig-postbootstrap.yaml":           v410ConfigDefaultconfigPostbootstrapYaml,
	"v4.1.0/config/defaultconfig.yaml":                         v410ConfigDefaultconfigYaml,
	"v4.1.0/kube-scheduler/cm.yaml":                            v410KubeSchedulerCmYaml,
	"v4.1.0/kube-scheduler/kubeconfig-cert-syncer.yaml":        v410KubeSchedulerKubeconfigCertSyncerYaml,
	"v4.1.0/kube-scheduler/kubeconfig-cm.yaml":                 v410KubeSchedulerKubeconfigCmYaml,
	"v4.1.0/kube-scheduler/leader-election-rolebinding.yaml":   v410KubeSchedulerLeaderElectionRolebindingYaml,
	"v4.1.0/kube-scheduler/localhost-recovery-client-crb.yaml": v410KubeSchedulerLocalhostRecoveryClientCrbYaml,
	"v4.1.0/kube-scheduler/localhost-recovery-sa.yaml":         v410KubeSchedulerLocalhostRecoverySaYaml,
	"v4.1.0/kube-scheduler/localhost-recovery-token.yaml":      v410KubeSchedulerLocalhostRecoveryTokenYaml,
	"v4.1.0/kube-scheduler/ns.yaml":                            v410KubeSchedulerNsYaml,
	"v4.1.0/kube-scheduler/pod-cm.yaml":                        v410KubeSchedulerPodCmYaml,
	"v4.1.0/kube-scheduler/pod.yaml":                           v410KubeSchedulerPodYaml,
	"v4.1.0/kube-scheduler/policyconfigmap-role.yaml":          v410KubeSchedulerPolicyconfigmapRoleYaml,
	"v4.1.0/kube-scheduler/policyconfigmap-rolebinding.yaml":   v410KubeSchedulerPolicyconfigmapRolebindingYaml,
	"v4.1.0/kube-scheduler/sa.yaml":                            v410KubeSchedulerSaYaml,
	"v4.1.0/kube-scheduler/scheduler-clusterrolebinding.yaml":  v410KubeSchedulerSchedulerClusterrolebindingYaml,
	"v4.1.0/kube-scheduler/svc.yaml":                           v410KubeSchedulerSvcYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"v4.1.0": {nil, map[string]*bintree{
		"config": {nil, map[string]*bintree{
			"defaultconfig-postbootstrap.yaml": {v410ConfigDefaultconfigPostbootstrapYaml, map[string]*bintree{}},
			"defaultconfig.yaml":               {v410ConfigDefaultconfigYaml, map[string]*bintree{}},
		}},
		"kube-scheduler": {nil, map[string]*bintree{
			"cm.yaml":                            {v410KubeSchedulerCmYaml, map[string]*bintree{}},
			"kubeconfig-cert-syncer.yaml":        {v410KubeSchedulerKubeconfigCertSyncerYaml, map[string]*bintree{}},
			"kubeconfig-cm.yaml":                 {v410KubeSchedulerKubeconfigCmYaml, map[string]*bintree{}},
			"leader-election-rolebinding.yaml":   {v410KubeSchedulerLeaderElectionRolebindingYaml, map[string]*bintree{}},
			"localhost-recovery-client-crb.yaml": {v410KubeSchedulerLocalhostRecoveryClientCrbYaml, map[string]*bintree{}},
			"localhost-recovery-sa.yaml":         {v410KubeSchedulerLocalhostRecoverySaYaml, map[string]*bintree{}},
			"localhost-recovery-token.yaml":      {v410KubeSchedulerLocalhostRecoveryTokenYaml, map[string]*bintree{}},
			"ns.yaml":                            {v410KubeSchedulerNsYaml, map[string]*bintree{}},
			"pod-cm.yaml":                        {v410KubeSchedulerPodCmYaml, map[string]*bintree{}},
			"pod.yaml":                           {v410KubeSchedulerPodYaml, map[string]*bintree{}},
			"policyconfigmap-role.yaml":          {v410KubeSchedulerPolicyconfigmapRoleYaml, map[string]*bintree{}},
			"policyconfigmap-rolebinding.yaml":   {v410KubeSchedulerPolicyconfigmapRolebindingYaml, map[string]*bintree{}},
			"sa.yaml":                            {v410KubeSchedulerSaYaml, map[string]*bintree{}},
			"scheduler-clusterrolebinding.yaml":  {v410KubeSchedulerSchedulerClusterrolebindingYaml, map[string]*bintree{}},
			"svc.yaml":                           {v410KubeSchedulerSvcYaml, map[string]*bintree{}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
