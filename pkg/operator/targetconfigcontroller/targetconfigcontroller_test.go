package targetconfigcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/clock"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/bindata"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/configobservercontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	schedulerconfigv1 "k8s.io/kube-scheduler/config/v1"
	"k8s.io/utils/ptr"
)

var (
	codec = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

	configLowNodeUtilization  = newSchedulerConfig(configv1.LowNodeUtilization)
	configHighNodeUtilization = newSchedulerConfig(configv1.HighNodeUtilization)
	configNoScoring           = newSchedulerConfig(configv1.NoScoring)
	configUnknown             = newSchedulerConfig("unknown-config")

	defaultConfig                  = string(bindata.MustAsset("assets/config/defaultconfig.yaml"))
	schedConfigLowNodeUtilization  = string(bindata.MustAsset("assets/config/defaultconfig-postbootstrap-lownodeutilization.yaml"))
	schedConfigHighNodeUtilization = string(bindata.MustAsset("assets/config/defaultconfig-postbootstrap-highnodeutilization.yaml"))
	schedConfigNoScoring           = string(bindata.MustAsset("assets/config/defaultconfig-postbootstrap-noscoring.yaml"))

	configMapLowNodeUtilization  = newSchedulerConfigConfigMap(schedConfigLowNodeUtilization)
	configMapHighNodeUtilization = newSchedulerConfigConfigMap(schedConfigHighNodeUtilization)
	configMapNoScoring           = newSchedulerConfigConfigMap(schedConfigNoScoring)
	wantCM                       = newSchedulerConfigConfigMap("") // data are checked separately

	emptySchedProfiles = []schedulerconfigv1.KubeSchedulerProfile{}

	highNodeUtilizationSchedProfiles = []schedulerconfigv1.KubeSchedulerProfile{
		{
			SchedulerName: ptr.To[string]("default-scheduler"),
			PluginConfig: []schedulerconfigv1.PluginConfig{
				{
					Name: "NodeResourcesFit",
					Args: runtime.RawExtension{Raw: []uint8(`{"scoringStrategy":{"type":"MostAllocated"}}`)},
				},
			},
			Plugins: &schedulerconfigv1.Plugins{
				Score: schedulerconfigv1.PluginSet{
					Enabled: []schedulerconfigv1.Plugin{
						{Name: "NodeResourcesFit", Weight: ptr.To[int32](5)},
					},
					Disabled: []schedulerconfigv1.Plugin{
						{Name: "NodeResourcesBalancedAllocation"},
					},
				},
			},
		},
	}

	noScoringSchedProfiles = []schedulerconfigv1.KubeSchedulerProfile{
		{
			SchedulerName: ptr.To[string]("default-scheduler"),
			Plugins: &schedulerconfigv1.Plugins{
				PreScore: schedulerconfigv1.PluginSet{
					Disabled: []schedulerconfigv1.Plugin{
						{Name: "*"},
					},
				},
				Score: schedulerconfigv1.PluginSet{
					Disabled: []schedulerconfigv1.Plugin{
						{Name: "*"},
					},
				},
			},
		},
	}

	defaultKubeconfigData = `apiVersion: v1
clusters:
  - cluster:
      certificate-authority: /etc/kubernetes/static-pod-resources/configmaps/serviceaccount-ca/ca-bundle.crt
      server: https://127.0.0.1:443
    name: lb-int
contexts:
  - context:
      cluster: lb-int
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
`

	configMapKubeConfigCMDefault = newSchedulerKubeconfigConfigMap(defaultKubeconfigData)

	unsupportedConfigOverridesSchedulerArgJSON = []byte(`{"arguments":{"master":"https://localhost:1234"}}`)

	unsupportedConfigOverridesMultipleSchedulerArgsJSON = []byte(`{"arguments":{"master":"https://localhost:1234","unsupported-kube-api-over-localhost":"true"}}`)

	fakeUnsupportedConfigArgsJson = []byte(`{"arguments":{"fakeKey":["value1","value2"]}}`)

	unmarshalFakeUnsupportedConfigArgsJson = []byte(`{"arguments":{"fakeKey1","fakeKey2"}}`)
)

// newSchedulerConfig creates a Scheduler configuration with the specified profile
func newSchedulerConfig(profile configv1.SchedulerProfile) *configv1.Scheduler {
	return &configv1.Scheduler{
		Spec: configv1.SchedulerSpec{
			Policy:  configv1.ConfigMapNameReference{Name: ""},
			Profile: profile,
		},
	}
}

// newSchedulerConfigConfigMap creates a ConfigMap for the scheduler configuration
func newSchedulerConfigConfigMap(configData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: operatorclient.TargetNamespace,
		},
		Data: map[string]string{"config.yaml": configData},
	}
}

// newSchedulerKubeconfigConfigMap creates a ConfigMap for the scheduler kubeconfig
func newSchedulerKubeconfigConfigMap(kubeconfigData string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scheduler-kubeconfig",
			Namespace: operatorclient.TargetNamespace,
		},
		Data: map[string]string{"kubeconfig": kubeconfigData},
	}
}

// newFakeSchedConfigLister creates a fakeSchedConfigLister with a single scheduler config
func newFakeSchedConfigLister(name string, config *configv1.Scheduler) *fakeSchedConfigLister {
	return &fakeSchedConfigLister{
		Items: map[string]*configv1.Scheduler{name: config},
	}
}

func Test_manageKubeSchedulerConfigMap_v311_00_to_latest(t *testing.T) {

	tests := []struct {
		name                  string
		configSchedulerLister configlistersv1.SchedulerLister
		featureGates          featuregates.FeatureGate
		want                  *corev1.ConfigMap
		wantConfig            string
		wantSchedProfiles     []schedulerconfigv1.KubeSchedulerProfile
		want1                 bool
		wantErr               bool
	}{
		{
			name:                  "unknown-cluster",
			configSchedulerLister: newFakeSchedConfigLister("unknown", configLowNodeUtilization),
			wantSchedProfiles:     emptySchedProfiles,
			want1:                 false,
			wantErr:               true,
		},
		{
			name:                  "unknown-profile",
			configSchedulerLister: newFakeSchedConfigLister("cluster", configUnknown),
			wantSchedProfiles:     emptySchedProfiles,
			want1:                 false,
			wantErr:               true,
		},
		{
			name:                  "low-node-utilization",
			configSchedulerLister: newFakeSchedConfigLister("cluster", configLowNodeUtilization),
			wantSchedProfiles:     emptySchedProfiles,
			want1:                 true,
			wantErr:               false,
		},
		{
			name:                  "high-node-utilization",
			configSchedulerLister: newFakeSchedConfigLister("cluster", configHighNodeUtilization),
			wantSchedProfiles:     highNodeUtilizationSchedProfiles,
			want1:                 true,
			wantErr:               false,
		},
		{
			name:                  "no-scoring",
			configSchedulerLister: newFakeSchedConfigLister("cluster", configNoScoring),
			wantSchedProfiles:     noScoringSchedProfiles,
			want1:                 true,
			wantErr:               false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featureGates := tt.featureGates
			if featureGates == nil {
				// use a default feature gate where DynamicResourceAllocation is disabled
				featureGates = featuregates.NewFeatureGate(nil, []configv1.FeatureGateName{"DynamicResourceAllocation"})
			}
			// need a client for each test
			got, got1, err := manageKubeSchedulerConfigMap_v311_00_to_latest(context.TODO(), featureGates, fake.NewSimpleClientset().CoreV1(), NewFakeRecorder(1024), tt.configSchedulerLister)
			if (err != nil) != tt.wantErr {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got == nil {
				t.Errorf("expected non-nil CM, got nil")
				return
			}
			// check the CM (without data) is properly generated
			if _, exists := got.Data["config.yaml"]; !exists {
				t.Errorf("generated CM is missing 'config.yaml' data")
				return
			}
			gotData := got.Data["config.yaml"]
			got.Data["config.yaml"] = ""
			if !equality.Semantic.DeepEqual(got, wantCM) {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() diff: %v", cmp.Diff(got, wantCM))
			}

			// check the scheduler configuration/CM data
			gotConfig := &schedulerconfigv1.KubeSchedulerConfiguration{}
			if err := yaml.Unmarshal([]byte(gotData), gotConfig); err != nil {
				t.Errorf("unable to Unmarshal configuration: %v", err)
				return
			}

			if !equality.Semantic.DeepEqual(gotConfig.Profiles, tt.wantSchedProfiles) {
				if len(gotConfig.Profiles) != len(tt.wantSchedProfiles) {
					t.Errorf("the expected number of profiles (%v) is different from the retrieved one (%v)", len(tt.wantSchedProfiles), len(gotConfig.Profiles))
					return
				}
				if len(gotConfig.Profiles) > 0 {
					if diff := cmp.Diff(tt.wantSchedProfiles[0], gotConfig.Profiles[0]); diff != "" {
						t.Errorf("%v", diff)
					}
					return
				}
			}
			if got1 != tt.want1 {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestManageSchedulerKubeconfig(t *testing.T) {
	tests := []struct {
		name                string
		inputInfrastructure *configv1.Infrastructure
		expectedConfigMap   *corev1.ConfigMap
		expectedBool        bool
		expectedErr         error
	}{
		{
			name:                "default",
			inputInfrastructure: &configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Status: configv1.InfrastructureStatus{APIServerInternalURL: "https://127.0.0.1:443"}},
			expectedConfigMap:   configMapKubeConfigCMDefault,
			expectedBool:        true,
		},
		{
			name:                "missingAPIServerInternalURL",
			inputInfrastructure: &configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Status: configv1.InfrastructureStatus{APIServerInternalURL: ""}},
			expectedConfigMap:   nil,
			expectedBool:        false,
		},
		{
			name:                "missingCluster",
			inputInfrastructure: &configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "fakecluster"}, Status: configv1.InfrastructureStatus{APIServerInternalURL: "https://127.0.0.1:443"}},
			expectedConfigMap:   nil,
			expectedBool:        false,
		},
	}

	for _, tc := range tests {
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		indexer.Add(tc.inputInfrastructure)
		infrastructureLister := configlistersv1.NewInfrastructureLister(indexer)
		eventRecorder := events.NewInMemoryRecorder("", clock.RealClock{})
		fakeKubeClient := fake.NewSimpleClientset()
		gotCM, gotBool, _ := manageSchedulerKubeconfig(context.TODO(),
			fakeKubeClient.CoreV1(), infrastructureLister, eventRecorder)

		if !reflect.DeepEqual(gotCM, tc.expectedConfigMap) {
			t.Errorf("manageSchedulerKubeconfig() got = %v, want %v", gotCM, tc.expectedConfigMap)
		}
		if gotBool != tc.expectedBool {
			t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() got1 = %v, want %v", gotBool, tc.expectedBool)
		}
	}
}

func TestCheckForFeatureGates(t *testing.T) {
	tests := []struct {
		name           string
		featureGates   featuregates.FeatureGate
		expectedResult map[string]bool
	}{
		{
			name: "default",
			featureGates: featuregates.NewFeatureGate(
				[]configv1.FeatureGateName{"APIPriorityAndFairness", "RotateKubeletServerCertificate"},
				[]configv1.FeatureGateName{"RetroactiveDefaultStorageClass"},
			),
			expectedResult: map[string]bool{
				"APIPriorityAndFairness":         true,
				"RetroactiveDefaultStorageClass": false,
				"RotateKubeletServerCertificate": true,
			},
		},
		{
			name: "techpreview",
			featureGates: featuregates.NewFeatureGate(
				[]configv1.FeatureGateName{"APIPriorityAndFairness", "BuildCSIVolumes"},
				[]configv1.FeatureGateName{},
			),
			expectedResult: map[string]bool{
				"APIPriorityAndFairness": true,
				"BuildCSIVolumes":        true,
			},
		},

		{
			name: "custom",
			featureGates: featuregates.NewFeatureGate(
				[]configv1.FeatureGateName{"CSIMigration", "CSIMigrationAWS"},
				[]configv1.FeatureGateName{"CSIMigrationGCE"},
			),
			expectedResult: map[string]bool{
				"CSIMigration":    true,
				"CSIMigrationAWS": true,
				"CSIMigrationGCE": false,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualFeatureGates := checkForFeatureGates(tc.featureGates)
			if !reflect.DeepEqual(actualFeatureGates, tc.expectedResult) {
				expected := sets.StringKeySet(tc.expectedResult)
				actual := sets.StringKeySet(actualFeatureGates)
				t.Logf("missing in actual: %v", expected.Difference(actual))
				t.Logf("missing in expected: %v", actual.Difference(expected))
				t.Fatalf("Expected %v feature gates to be present but found %v", tc.expectedResult, actualFeatureGates)
			}
		})
	}
}

func TestGetSortedFeatureGates(t *testing.T) {
	featueGates := map[string]bool{
		"ExperimentalCriticalPodAnnotation": true,
		"RotateKubeletServerCertificate":    true,
		"CSIBlockVolume":                    true,
		"LocalStorageCapacityIsolation":     false,
	}
	expectedFeatureGateString := "CSIBlockVolume=true,ExperimentalCriticalPodAnnotation=true,LocalStorageCapacityIsolation=false,RotateKubeletServerCertificate=true"
	sortedFeatureGates := getSortedFeatureGates(featueGates)
	actualFeatureGateString := getFeatureGateString(sortedFeatureGates, featueGates)
	if expectedFeatureGateString != actualFeatureGateString {
		t.Fatalf("Expected %v as featuregate string but got %v", expectedFeatureGateString, actualFeatureGateString)
	}
}

// newKubeSchedulerOperator creates a KubeScheduler operator with optional unsupported config overrides
func newKubeSchedulerOperator(unsupportedConfigOverrides []byte) *operatorv1.KubeScheduler {
	return newKubeSchedulerOperatorWithObservedConfig(nil, unsupportedConfigOverrides)
}

// newKubeSchedulerOperatorWithObservedConfig creates a KubeScheduler operator with optional observed config and unsupported config overrides
func newKubeSchedulerOperatorWithObservedConfig(observedConfig, unsupportedConfigOverrides []byte) *operatorv1.KubeScheduler {
	operatorSpec := operatorv1.OperatorSpec{}
	if observedConfig != nil {
		operatorSpec.ObservedConfig = runtime.RawExtension{Raw: observedConfig}
	}
	if unsupportedConfigOverrides != nil {
		operatorSpec.UnsupportedConfigOverrides = runtime.RawExtension{Raw: unsupportedConfigOverrides}
	}
	return &operatorv1.KubeScheduler{
		Spec: operatorv1.KubeSchedulerSpec{
			StaticPodOperatorSpec: operatorv1.StaticPodOperatorSpec{
				OperatorSpec: operatorSpec,
			},
		},
	}
}

func TestManagePodToLatest(t *testing.T) {
	scenarios := []struct {
		name       string
		goldenFile string
		operator   *operatorv1.KubeScheduler
	}{

		// scenario 1
		{
			name:       "happy path: a pod with default values is created",
			goldenFile: "./testdata/ks_pod_scenario_1.yaml",
			operator:   newKubeSchedulerOperator(nil),
		},

		// scenario 2
		{
			name:       "an unsupported flag is passed directly to the kube scheduler",
			goldenFile: "./testdata/ks_pod_scenario_2.yaml",
			operator:   newKubeSchedulerOperator(unsupportedConfigOverridesSchedulerArgJSON),
		},

		// scenario 3
		{
			name:       "unsupported flags are passed directly to the kube scheduler",
			goldenFile: "./testdata/ks_pod_scenario_3.yaml",
			operator:   newKubeSchedulerOperator(unsupportedConfigOverridesMultipleSchedulerArgsJSON),
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// test data
			eventRecorder := events.NewInMemoryRecorder("", clock.RealClock{})
			fakeKubeClient := fake.NewSimpleClientset()
			configSchedulerIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			configSchedulerIndexer.Add(&configv1.Scheduler{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.SchedulerSpec{}})
			configSchedulerLister := configlistersv1.NewSchedulerLister(configSchedulerIndexer)

			// act
			actualConfigMap, _, err := managePod_v311_00_to_latest(
				context.TODO(),
				featuregates.NewFeatureGate(
					[]configv1.FeatureGateName{
						"APIPriorityAndFairness",
						"DownwardAPIHugePages",
						"OpenShiftPodSecurityAdmission",
						"RotateKubeletServerCertificate",
					},
					[]configv1.FeatureGateName{
						"RetroactiveDefaultStorageClass",
					}),
				fakeKubeClient.CoreV1(),
				fakeKubeClient.CoreV1(),
				eventRecorder,
				&scenario.operator.Spec.StaticPodOperatorSpec,
				"CaptainAmerica",
				"Piper",
				configSchedulerLister)

			// validate
			if err != nil {
				t.Fatal(err)
			}

			rawSchedulerPod, exist := actualConfigMap.Data["pod.yaml"]
			if !exist {
				t.Fatal("didn't find pod.yaml")
			}

			actualSchedulerPod := &corev1.Pod{}
			if err := runtime.DecodeInto(codec, []byte(rawSchedulerPod), actualSchedulerPod); err != nil {
				t.Fatal(err)
			}

			data := readBytesFromFile(t, scenario.goldenFile)
			goldenSchedulerPod := &corev1.Pod{}
			if err := runtime.DecodeInto(codec, data, goldenSchedulerPod); err != nil {
				t.Fatal(err)
			}

			if !equality.Semantic.DeepEqual(actualSchedulerPod, goldenSchedulerPod) {
				t.Errorf("created Scheduler Pod is different from the expected one (form a golden file) : %s", diff.Diff(actualSchedulerPod, goldenSchedulerPod))
			}
		})
	}
}

func TestGetUnsupportedFlagsFromConfig(t *testing.T) {
	tests := []struct {
		name                   string
		inputUnsupportedConfig []byte
		expectedResult         []string
	}{
		{
			name:                   "unsupportedFlagsinJson",
			inputUnsupportedConfig: unsupportedConfigOverridesMultipleSchedulerArgsJSON,
			expectedResult:         []string{"--master=https://localhost:1234", "--unsupported-kube-api-over-localhost=true"},
		},
		{
			name:                   "unsupportedFakeFlagsinJsonwithStringList",
			inputUnsupportedConfig: fakeUnsupportedConfigArgsJson,
			expectedResult:         []string{"--fakeKey=value1", "--fakeKey=value2"},
		},
		{
			name:                   "unmashalUnsupportedFakeFlagsinJson",
			inputUnsupportedConfig: unmarshalFakeUnsupportedConfigArgsJson,
			expectedResult:         nil,
		},
		{
			name:                   "emptyUnsupportedFlags",
			inputUnsupportedConfig: []byte(``),
			expectedResult:         nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := getUnsupportedFlagsFromConfig(tc.inputUnsupportedConfig)

			if !reflect.DeepEqual(got, tc.expectedResult) {
				t.Errorf("getUnsupportedFlagsFromConfig() got = %v, want %v", got, tc.expectedResult)
			}
		})
	}
}

// setupFakeClients creates fake Kubernetes and config clients with all required test resources
func setupFakeClients(t *testing.T, apiServer *configv1.APIServer) (
	kubernetes.Interface,
	v1helpers.KubeInformersForNamespaces,
	configinformers.SharedInformerFactory,
) {
	// Create required resources in the target namespace
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localhost-recovery-client",
			Namespace: operatorclient.TargetNamespace,
			UID:       "test-uid",
		},
	}
	token := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localhost-recovery-client-token",
			Namespace: operatorclient.TargetNamespace,
			Annotations: map[string]string{
				corev1.ServiceAccountUIDKey: "test-uid",
			},
		},
		Data: map[string][]byte{
			"token":  []byte("test-token"),
			"ca.crt": []byte("test-ca"),
		},
	}

	// Create required ConfigMaps for manageServiceAccountCABundle
	// Generate valid test CA certificates
	testCA, err := crypto.MakeSelfSignedCAConfig("test-ca", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create test CA: %v", err)
	}
	testCACertPEM, _, err := testCA.GetPEMBytes()
	if err != nil {
		t.Fatalf("failed to get CA PEM bytes: %v", err)
	}

	kubeAPIServerServerCA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver-server-ca",
			Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": string(testCACertPEM),
		},
	}
	defaultIngressCert := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-ingress-cert",
			Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": string(testCACertPEM),
		},
	}

	// setup kube client with required resources
	fakeKubeClient := fake.NewSimpleClientset(sa, token, kubeAPIServerServerCA, defaultIngressCert)
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(
		fakeKubeClient,
		operatorclient.GlobalUserSpecifiedConfigNamespace,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.TargetNamespace,
		operatorclient.OperatorNamespace,
	)

	// Create Infrastructure object for manageSchedulerKubeconfig
	infrastructure := &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status: configv1.InfrastructureStatus{
			APIServerInternalURL: "https://127.0.0.1:443",
		},
	}

	// Create Scheduler object
	scheduler := &configv1.Scheduler{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       configv1.SchedulerSpec{},
	}

	// Build list of objects to pre-populate the fake config client
	configObjects := []runtime.Object{infrastructure, scheduler}
	if apiServer != nil {
		configObjects = append(configObjects, apiServer)
	}
	fakeConfigClient := configfake.NewSimpleClientset(configObjects...)
	configInformers := configinformers.NewSharedInformerFactory(fakeConfigClient, 0)

	// Populate required informer caches
	configInformers.Config().V1().Schedulers().Informer().GetIndexer().Add(scheduler)
	configInformers.Config().V1().Infrastructures().Informer().GetIndexer().Add(infrastructure)
	if apiServer != nil {
		configInformers.Config().V1().APIServers().Informer().GetIndexer().Add(apiServer)
	}

	return fakeKubeClient, kubeInformersForNamespaces, configInformers
}

// fakeResourceSyncer implements resourcesynccontroller.ResourceSyncer for testing
type fakeResourceSyncer struct{}

func (f *fakeResourceSyncer) SyncConfigMap(destination, source resourcesynccontroller.ResourceLocation) error {
	return nil
}

func (f *fakeResourceSyncer) SyncSecret(destination, source resourcesynccontroller.ResourceLocation) error {
	return nil
}

// fakeSyncContext implements factory.SyncContext for testing
type fakeSyncContext struct {
	recorder events.Recorder
}

func (f *fakeSyncContext) Queue() workqueue.RateLimitingInterface {
	return nil
}

func (f *fakeSyncContext) QueueKey() string {
	return ""
}

func (f *fakeSyncContext) Recorder() events.Recorder {
	return f.recorder
}

func TestManagePod_TLSConfiguration(t *testing.T) {
	// Get the default Intermediate TLS profile
	intermediateProfile := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	intermediateCiphers := crypto.OpenSSLToIANACipherSuites(intermediateProfile.Ciphers)

	tests := []struct {
		name                 string
		apiServer            *configv1.APIServer
		expectedCipherSuites string
		expectedMinTLSVer    string
	}{
		{
			name:                 "no APIServer config",
			apiServer:            nil,
			expectedCipherSuites: fmt.Sprintf("--tls-cipher-suites=%s", strings.Join(intermediateCiphers, ",")),
			expectedMinTLSVer:    fmt.Sprintf("--tls-min-version=%s", intermediateProfile.MinTLSVersion),
		},
		{
			name: "APIServer with TLS security profile",
			apiServer: &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers: []string{
									"ECDHE-ECDSA-AES128-GCM-SHA256",
									"ECDHE-RSA-AES128-GCM-SHA256",
								},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					},
				},
			},
			expectedCipherSuites: "--tls-cipher-suites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expectedMinTLSVer:    "--tls-min-version=VersionTLS12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			// setup operator client with wrapper to support UpdateOperatorSpec
			fakeOperatorClient := &fakeOperatorClientWrapper{
				StaticPodOperatorClient: v1helpers.NewFakeStaticPodOperatorClient(
					&operatorv1.StaticPodOperatorSpec{
						OperatorSpec: operatorv1.OperatorSpec{},
					},
					&operatorv1.StaticPodOperatorStatus{
						OperatorStatus: operatorv1.OperatorStatus{},
					},
					nil,
					nil,
				),
			}

			// setup fake clients with all required resources
			fakeKubeClient, kubeInformersForNamespaces, configInformers := setupFakeClients(t, tt.apiServer)

			// create event recorder
			eventRecorder := events.NewInMemoryRecorder("", clock.RealClock{})

			// Create config observer - this registers event handlers with informers
			configObserver := configobservercontroller.NewConfigObserver(
				fakeOperatorClient,
				kubeInformersForNamespaces,
				configInformers,
				&fakeResourceSyncer{},
				eventRecorder,
			)

			// Create target config controller - this registers event handlers with informers
			targetConfigController := NewTargetConfigController(
				"test-image",
				"test-operator-image",
				featuregates.NewFeatureGate(nil, nil),
				fakeOperatorClient,
				kubeInformersForNamespaces,
				configInformers,
				fakeOperatorClient,
				fakeKubeClient,
				eventRecorder,
			)

			// Start informers after controllers have registered their event handlers
			kubeInformersForNamespaces.Start(ctx.Done())
			configInformers.Start(ctx.Done())

			// Validate that operator spec doesn't have observed config before running config observer
			if specBefore, _, _, err := fakeOperatorClient.GetStaticPodOperatorState(); err != nil {
				t.Fatalf("failed to get operator spec before config observer sync: %v", err)
			} else if len(specBefore.ObservedConfig.Raw) > 0 || specBefore.ObservedConfig.Object != nil {
				t.Fatalf("operator spec should not have ObservedConfig before config observer sync, got Raw=%v Object=%v",
					len(specBefore.ObservedConfig.Raw), specBefore.ObservedConfig.Object)
			}

			// Run config observer sync to update observed config in operator spec
			// This will call apiserver.ObserveTLSSecurityProfile internally
			if err := configObserver.Sync(ctx, &fakeSyncContext{recorder: eventRecorder}); err != nil {
				t.Logf("WARNING: config observer sync returned error: %v", err)
			}

			// Validate that observed config was injected by config observer
			if specAfter, _, _, err := fakeOperatorClient.GetStaticPodOperatorState(); err != nil {
				t.Fatalf("failed to get operator spec after config observer sync: %v", err)
			} else if len(specAfter.ObservedConfig.Raw) == 0 {
				t.Fatalf("operator spec should have ObservedConfig.Raw populated after config observer sync")
			}

			// Run target config controller sync to trigger the full production code path
			if err := targetConfigController.Sync(ctx, &fakeSyncContext{recorder: eventRecorder}); err != nil {
				t.Fatalf("targetConfigController.Sync failed: %v", err)
			}

			// Check operator status for any degraded conditions
			if _, statusCheck, _, _ := fakeOperatorClient.GetStaticPodOperatorState(); statusCheck != nil {
				t.Logf("Status has %d conditions", len(statusCheck.Conditions))
				for _, condition := range statusCheck.Conditions {
					t.Logf("Condition: %s = %s (reason: %s, message: %s)", condition.Type, condition.Status, condition.Reason, condition.Message)
				}
			}

			// Read the generated ConfigMap from the fake kube client
			actualConfigMap, err := fakeKubeClient.CoreV1().ConfigMaps(operatorclient.TargetNamespace).Get(ctx, "kube-scheduler-pod", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("failed to get kube-scheduler-pod ConfigMap: %v", err)
			}

			rawSchedulerPod, exist := actualConfigMap.Data["pod.yaml"]
			if !exist {
				t.Fatal("didn't find pod.yaml")
			}

			actualSchedulerPod := &corev1.Pod{}
			if err := runtime.DecodeInto(codec, []byte(rawSchedulerPod), actualSchedulerPod); err != nil {
				t.Fatal(err)
			}

			// Check container args for TLS settings
			foundCipherSuites := false
			foundMinTLSVersion := false

			for _, arg := range actualSchedulerPod.Spec.Containers[0].Args {
				if strings.HasPrefix(arg, "--tls-cipher-suites=") {
					foundCipherSuites = true
					if arg != tt.expectedCipherSuites {
						t.Errorf("Expected cipher suites arg %q, got %q", tt.expectedCipherSuites, arg)
					}
				}
				if strings.HasPrefix(arg, "--tls-min-version=") {
					foundMinTLSVersion = true
					if arg != tt.expectedMinTLSVer {
						t.Errorf("Expected min TLS version arg %q, got %q", tt.expectedMinTLSVer, arg)
					}
				}
			}

			if !foundCipherSuites {
				t.Errorf("Expected to find --tls-cipher-suites arg but didn't")
			}
			if !foundMinTLSVersion {
				t.Errorf("Expected to find --tls-min-version arg but didn't")
			}
		})
	}
}

func readBytesFromFile(t *testing.T, filename string) []byte {
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

// A scheduler configuration lister
type fakeSchedConfigLister struct {
	Err   error
	Items map[string]*configv1.Scheduler
}

func (lister *fakeSchedConfigLister) List(selector labels.Selector) ([]*configv1.Scheduler, error) {
	itemsList := make([]*configv1.Scheduler, 0)
	for _, v := range lister.Items {
		itemsList = append(itemsList, v)
	}
	return itemsList, lister.Err
}

func (lister *fakeSchedConfigLister) Get(name string) (*configv1.Scheduler, error) {
	if lister.Err != nil {
		return nil, lister.Err
	}
	item := lister.Items[name]
	if item == nil {
		return nil, errors.NewNotFound(schema.GroupResource{}, name)
	}
	return item, nil
}

// An events recorder
type fakeRecorder struct {
	Events chan string
}

func (f *fakeRecorder) Event(reason, note string) {
	if f.Events != nil {
		f.Events <- fmt.Sprintf("%s %s", reason, note)
	}
}

func (f *fakeRecorder) Eventf(reason, note string, args ...interface{}) {
	if f.Events != nil {
		msg := fmt.Sprintf("%s %s", reason, note)
		if len(args) > 0 {
			msg += " " + fmt.Sprint(args...)
		}
		f.Events <- msg
	}
}

func (f *fakeRecorder) Warning(reason, note string) {
	if f.Events != nil {
		f.Events <- fmt.Sprintf("%s %s", reason, note)
	}
}

func (f *fakeRecorder) Warningf(reason, note string, args ...interface{}) {
	if f.Events != nil {
		msg := fmt.Sprintf("%s %s", reason, note)
		if len(args) > 0 {
			msg += " " + fmt.Sprint(args...)
		}
		f.Events <- msg
	}
}

func (f *fakeRecorder) ForComponent(componentName string) events.Recorder {
	return *(*(events.Recorder))(unsafe.Pointer(f))
}

func (f *fakeRecorder) WithComponentSuffix(componentNameSuffix string) events.Recorder {
	return *(*(events.Recorder))(unsafe.Pointer(f))
}

func (f *fakeRecorder) WithContext(ctx context.Context) events.Recorder {
	return *(*(events.Recorder))(unsafe.Pointer(f))
}

func (f *fakeRecorder) ComponentName() string {
	return ""
}

func (f *fakeRecorder) Shutdown() {
}

var _ events.Recorder = &fakeRecorder{}

func NewFakeRecorder(bufferSize int) *fakeRecorder {
	return &fakeRecorder{
		Events: make(chan string, bufferSize),
	}
}

// fakeWatcher implements watch.Interface for testing
type fakeWatcher struct {
	result chan watch.Event
}

func (f *fakeWatcher) Stop() {
	close(f.result)
}

func (f *fakeWatcher) ResultChan() <-chan watch.Event {
	return f.result
}

// fakeOperatorClientWrapper wraps the fake operator client to support UpdateOperatorSpec
// It delegates most operations to the embedded client but intercepts spec updates to convert Object to Raw
type fakeOperatorClientWrapper struct {
	v1helpers.StaticPodOperatorClient
}

func (w *fakeOperatorClientWrapper) UpdateOperatorSpec(ctx context.Context, resourceVersion string, spec *operatorv1.OperatorSpec) (*operatorv1.OperatorSpec, string, error) {
	// Simulate kube-apiserver behavior: convert Object to Raw if Raw is nil
	if spec.ObservedConfig.Object != nil && len(spec.ObservedConfig.Raw) == 0 {
		rawBytes, err := json.Marshal(spec.ObservedConfig.Object)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal ObservedConfig.Object to Raw: %v", err)
		}
		spec.ObservedConfig.Raw = rawBytes
	}

	// The library-go fake doesn't support UpdateOperatorSpec, so we call UpdateStaticPodOperatorSpec instead
	// First, get the current static pod spec to preserve non-OperatorSpec fields
	currentSpec, _, _, err := w.StaticPodOperatorClient.GetStaticPodOperatorState()
	if err != nil {
		return nil, "", err
	}

	// Update only the OperatorSpec portion
	currentSpec.OperatorSpec = *spec

	// Call UpdateStaticPodOperatorSpec with the modified spec
	updatedSpec, newRV, err := w.StaticPodOperatorClient.UpdateStaticPodOperatorSpec(ctx, resourceVersion, currentSpec)
	if err != nil {
		return nil, "", err
	}

	return &updatedSpec.OperatorSpec, newRV, nil
}
