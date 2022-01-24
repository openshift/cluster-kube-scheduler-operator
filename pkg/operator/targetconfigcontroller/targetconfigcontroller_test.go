package targetconfigcontroller

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"
	"unsafe"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	fakeconfigv1client "github.com/openshift/client-go/config/clientset/versioned/fake"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/bindata"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
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
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var codec = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

var configLowNodeUtilization = &configv1.Scheduler{
	Spec: configv1.SchedulerSpec{Policy: configv1.ConfigMapNameReference{Name: ""},
		Profile: configv1.LowNodeUtilization,
	},
}

var configHighNodeUtilization = &configv1.Scheduler{
	Spec: configv1.SchedulerSpec{Policy: configv1.ConfigMapNameReference{Name: ""},
		Profile: configv1.HighNodeUtilization,
	},
}

var configNoScoring = &configv1.Scheduler{
	Spec: configv1.SchedulerSpec{Policy: configv1.ConfigMapNameReference{Name: ""},
		Profile: configv1.NoScoring,
	},
}

var configUnknown = &configv1.Scheduler{
	Spec: configv1.SchedulerSpec{Policy: configv1.ConfigMapNameReference{Name: ""},
		Profile: "unknown-config",
	},
}
var defaultConfig string = string(bindata.MustAsset("assets/config/defaultconfig.yaml"))
var schedConfigLowNodeUtilization string = string(bindata.MustAsset(
	"assets/config/defaultconfig-postbootstrap-lownodeutilization.yaml"))
var schedConfigHighNodeUtilization string = string(bindata.MustAsset(
	"assets/config/defaultconfig-postbootstrap-highnodeutilization.yaml"))
var schedConfigNoScoring string = string(bindata.MustAsset(
	"assets/config/defaultconfig-postbootstrap-noscoring.yaml"))

var configMapLowNodeUtilization = &corev1.ConfigMap{
	TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "config",
		Namespace: "openshift-kube-scheduler",
	},
	Data: map[string]string{"config.yaml": schedConfigLowNodeUtilization},
}

var configMapHighNodeUtilization = &corev1.ConfigMap{
	TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "config",
		Namespace: "openshift-kube-scheduler",
	},
	Data: map[string]string{"config.yaml": schedConfigHighNodeUtilization},
}

var configMapNoScoring = &corev1.ConfigMap{
	TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "config",
		Namespace: "openshift-kube-scheduler",
	},
	Data: map[string]string{"config.yaml": schedConfigNoScoring},
}

func Test_manageKubeSchedulerConfigMap_v311_00_to_latest(t *testing.T) {

	fakeRecorder := NewFakeRecorder(1024)

	type args struct {
		recorder              events.Recorder
		configSchedulerLister configlistersv1.SchedulerLister
	}
	tests := []struct {
		name       string
		args       args
		want       *corev1.ConfigMap
		wantConfig string
		want1      bool
		wantErr    bool
	}{
		{
			name: "unknown-cluster",
			args: args{
				recorder: fakeRecorder,
				configSchedulerLister: &fakeSchedConfigLister{
					Items: map[string]*configv1.Scheduler{"unknown": configLowNodeUtilization},
				},
			},
			want:    nil,
			want1:   false,
			wantErr: true,
		},
		{
			name: "unknown-profile",
			args: args{
				recorder: fakeRecorder,
				configSchedulerLister: &fakeSchedConfigLister{
					Items: map[string]*configv1.Scheduler{"cluster": configUnknown},
				},
			},
			want:    nil,
			want1:   false,
			wantErr: true,
		},
		{
			name: "low-node-utilization",
			args: args{
				recorder: fakeRecorder,
				configSchedulerLister: &fakeSchedConfigLister{
					Items: map[string]*configv1.Scheduler{"cluster": configLowNodeUtilization},
				},
			},
			want:       configMapLowNodeUtilization,
			wantConfig: schedConfigLowNodeUtilization,
			want1:      true,
			wantErr:    false,
		},
		{
			name: "high-node-utilization",
			args: args{
				recorder: fakeRecorder,
				configSchedulerLister: &fakeSchedConfigLister{
					Items: map[string]*configv1.Scheduler{"cluster": configHighNodeUtilization},
				},
			},
			want:       configMapHighNodeUtilization,
			wantConfig: schedConfigHighNodeUtilization,
			want1:      true,
			wantErr:    false,
		},
		{
			name: "no-scoring",
			args: args{
				recorder: fakeRecorder,
				configSchedulerLister: &fakeSchedConfigLister{
					Items: map[string]*configv1.Scheduler{"cluster": configNoScoring},
				},
			},
			want:       configMapNoScoring,
			wantConfig: schedConfigNoScoring,
			want1:      true,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// need a client for each test
			got, got1, err := manageKubeSchedulerConfigMap_v311_00_to_latest(context.TODO(), fake.NewSimpleClientset().CoreV1(), tt.args.recorder, tt.args.configSchedulerLister)
			if (err != nil) != tt.wantErr {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			configMap := resourceread.ReadConfigMapV1OrDie(bindata.MustAsset("assets/kube-scheduler/cm.yaml"))
			requiredConfigMap, _, _ := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, []byte(tt.wantConfig), []byte(defaultConfig))

			if !equality.Semantic.DeepEqual(got, requiredConfigMap) {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("manageKubeSchedulerConfigMap_v311_00_to_latest() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

var defaultKubeconfigData = `apiVersion: v1
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

var configMapKubeConfigCMDefault = &corev1.ConfigMap{
	TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "scheduler-kubeconfig",
		Namespace: "openshift-kube-scheduler",
	},
	Data: map[string]string{"kubeconfig": defaultKubeconfigData},
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
		eventRecorder := events.NewInMemoryRecorder("")
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
		name                    string
		configValue             configv1.FeatureSet
		inputCustomFeatureGates *configv1.CustomFeatureGates
		expectedResult          map[string]bool
	}{
		{
			name:        "default",
			configValue: configv1.Default,
			expectedResult: map[string]bool{
				// as copied from vendor/github.com/openshift/api/config/v1/types_feature.go
				"APIPriorityAndFairness":         true,
				"LegacyNodeRoleBehavior":         false,
				"NodeDisruptionExclusion":        true,
				"RotateKubeletServerCertificate": true,
				"DownwardAPIHugePages":           true,
				"ServiceNodeExclusion":           true,
				"SupportPodPidsLimit":            true,
			},
		},
		{
			name:        "techpreview",
			configValue: configv1.TechPreviewNoUpgrade,
			expectedResult: map[string]bool{
				// as copied from vendor/github.com/openshift/api/config/v1/types_feature.go
				"APIPriorityAndFairness":         true,
				"CSIDriverAzureDisk":             true,
				"CSIDriverVSphere":               true,
				"CSIMigrationAWS":                true,
				"CSIMigrationOpenStack":          true,
				"LegacyNodeRoleBehavior":         false,
				"NodeDisruptionExclusion":        true,
				"RotateKubeletServerCertificate": true,
				"DownwardAPIHugePages":           true,
				"ServiceNodeExclusion":           true,
				"SupportPodPidsLimit":            true,
				"CSIMigrationAzureDisk":          true,
				"CSIMigrationGCE":                true,
				"ExternalCloudProvider":          true,
			},
		},
		{
			name:        "custom",
			configValue: configv1.CustomNoUpgrade,
			inputCustomFeatureGates: &configv1.CustomFeatureGates{
				Enabled:  []string{"CSIMigration", "CSIMigrationAWS"},
				Disabled: []string{"CSIMigrationGCE"},
			},
			expectedResult: map[string]bool{
				"CSIMigration":    true,
				"CSIMigrationAWS": true,
				"CSIMigrationGCE": false,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			indexer.Add(&configv1.FeatureGate{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.FeatureGateSpec{
					FeatureGateSelection: configv1.FeatureGateSelection{
						FeatureSet:      tc.configValue,
						CustomNoUpgrade: tc.inputCustomFeatureGates,
					},
				},
			})
			featureGateLister := configlistersv1.NewFeatureGateLister(indexer)
			actualFeatureGates := checkForFeatureGates(featureGateLister)
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
		"SupportPodPidsLimit":               true,
		"CSIBlockVolume":                    true,
		"LocalStorageCapacityIsolation":     false,
	}
	expectedFeatureGateString := "CSIBlockVolume=true,ExperimentalCriticalPodAnnotation=true,LocalStorageCapacityIsolation=false,RotateKubeletServerCertificate=true,SupportPodPidsLimit=true"
	sortedFeatureGates := getSortedFeatureGates(featueGates)
	actualFeatureGateString := getFeatureGateString(sortedFeatureGates, featueGates)
	if expectedFeatureGateString != actualFeatureGateString {
		t.Fatalf("Expected %v as featuregate string but got %v", expectedFeatureGateString, actualFeatureGateString)
	}
}

var unsupportedConfigOverridesSchedulerArgJSON = `
{
  "arguments": {
      "master": "https://localhost:1234"
  }
}
`

var unsupportedConfigOverridesMultipleSchedulerArgsJSON = `
{
  "arguments": {
      "master": "https://localhost:1234",
      "unsupported-kube-api-over-localhost": "true"
  }
}
`

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
			operator:   &operatorv1.KubeScheduler{Spec: operatorv1.KubeSchedulerSpec{StaticPodOperatorSpec: operatorv1.StaticPodOperatorSpec{OperatorSpec: operatorv1.OperatorSpec{}}}},
		},

		// scenario 2
		{
			name:       "an unsupported flag is passed directly to the kube scheduler",
			goldenFile: "./testdata/ks_pod_scenario_2.yaml",
			operator: &operatorv1.KubeScheduler{Spec: operatorv1.KubeSchedulerSpec{StaticPodOperatorSpec: operatorv1.StaticPodOperatorSpec{OperatorSpec: operatorv1.OperatorSpec{
				UnsupportedConfigOverrides: runtime.RawExtension{Raw: []byte(unsupportedConfigOverridesSchedulerArgJSON)},
			}}}},
		},

		// scenario 3
		{
			name:       "unsupported flags are passed directly to the kube scheduler",
			goldenFile: "./testdata/ks_pod_scenario_3.yaml",
			operator: &operatorv1.KubeScheduler{Spec: operatorv1.KubeSchedulerSpec{StaticPodOperatorSpec: operatorv1.StaticPodOperatorSpec{OperatorSpec: operatorv1.OperatorSpec{
				UnsupportedConfigOverrides: runtime.RawExtension{Raw: []byte(unsupportedConfigOverridesMultipleSchedulerArgsJSON)},
			}}}},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// test data
			eventRecorder := events.NewInMemoryRecorder("")
			fakeKubeClient := fake.NewSimpleClientset()
			featureGateLister := configlistersv1.NewFeatureGateLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{}))
			configSchedulerIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			configSchedulerIndexer.Add(&configv1.Scheduler{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.SchedulerSpec{}})
			configSchedulerLister := configlistersv1.NewSchedulerLister(configSchedulerIndexer)

			// act
			actualConfigMap, _, _, err := managePod_v311_00_to_latest(
				context.TODO(),
				fakeKubeClient.CoreV1(),
				fakeKubeClient.CoreV1(),
				eventRecorder,
				&scenario.operator.Spec.StaticPodOperatorSpec,
				"CaptainAmerica",
				"Piper",
				featureGateLister,
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
				t.Errorf("created Scheduler Pod is different from the expected one (form a golden file) : %s", diff.ObjectDiff(actualSchedulerPod, goldenSchedulerPod))
			}
		})
	}
}

type FakeSyncContext struct {
	recorder events.Recorder
}

func (f FakeSyncContext) Queue() workqueue.RateLimitingInterface {
	return nil
}

func (f FakeSyncContext) QueueKey() string {
	return ""
}

func (f FakeSyncContext) Recorder() events.Recorder {
	return f.recorder
}

func TestPolicyUpgradeable(t *testing.T) {
	tests := []struct {
		name         string
		policyCMName string
		upgradable   bool
		status       *operatorv1.StaticPodOperatorStatus
	}{
		{
			name:         "PolicyUpgradeable is true",
			policyCMName: "",
			upgradable:   true,
			status:       &operatorv1.StaticPodOperatorStatus{},
		},
		{
			name:         "PolicyUpgradeable is false",
			policyCMName: "custompolicy",
			upgradable:   false,
			status:       &operatorv1.StaticPodOperatorStatus{},
		},
		{
			name:         "PolicyUpgradeable is cleared",
			policyCMName: "",
			upgradable:   true,
			status: &operatorv1.StaticPodOperatorStatus{
				OperatorStatus: operatorv1.OperatorStatus{
					Conditions: []operatorv1.OperatorCondition{
						{
							Type:    "PolicyUpgradeable",
							Status:  operatorv1.ConditionFalse,
							Reason:  "PolicyFieldSpecified",
							Message: fmt.Sprintf("deprecated scheduler.policy field is set, and it is to be removed in the next release"),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			schedulerConfig := &configv1.Scheduler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: configv1.SchedulerSpec{Policy: configv1.ConfigMapNameReference{Name: test.policyCMName},
					Profile: configv1.LowNodeUtilization,
				},
			}

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "localhost-recovery-client",
					Namespace: "openshift-kube-scheduler",
					UID:       "da5deb00-fdcd-412e-b8e9-80ab943772bf",
				},
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "localhost-recovery-client-token",
					Namespace: "openshift-kube-scheduler",
					Annotations: map[string]string{
						"kubernetes.io/service-account.name": "localhost-recovery-client",
						corev1.ServiceAccountUIDKey:          "da5deb00-fdcd-412e-b8e9-80ab943772bf",
					},
				},
				Type: corev1.SecretTypeServiceAccountToken,
				Data: map[string][]byte{
					"token":  []byte("XXXX"),
					"ca.crt": []byte("XXXX"),
				},
			}

			infra := &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: configv1.InfrastructureStatus{
					APIServerInternalURL: "https://127.0.0.1:443"},
			}

			fg := &configv1.FeatureGate{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.FeatureGateSpec{
					FeatureGateSelection: configv1.FeatureGateSelection{
						FeatureSet: configv1.Default,
					},
				},
			}

			kubeClient := fake.NewSimpleClientset(sa, secret)
			configClient := fakeconfigv1client.NewSimpleClientset(schedulerConfig, infra, fg)
			configInformers := configv1informers.NewSharedInformerFactory(configClient, 10*time.Minute)
			kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
				"",
				operatorclient.GlobalUserSpecifiedConfigNamespace,
				operatorclient.GlobalMachineSpecifiedConfigNamespace,
				operatorclient.OperatorNamespace,
				operatorclient.TargetNamespace,
				"kube-system",
			)

			operatorClient := v1helpers.NewFakeStaticPodOperatorClient(
				&operatorv1.StaticPodOperatorSpec{
					OperatorSpec: operatorv1.OperatorSpec{
						ManagementState: operatorv1.Managed,
					},
				},
				test.status,
				nil,
				nil,
			)

			eventRecorder := events.NewRecorder(kubeClient.CoreV1().Events("test"), "test-operator", &corev1.ObjectReference{})

			targetConfigController := NewTargetConfigController(
				"targetImagePullSpec",
				"operatorImagePullSpec",
				operatorClient,
				kubeInformersForNamespaces,
				configInformers,
				operatorClient,
				kubeClient,
				eventRecorder,
			)

			ctx := context.TODO()

			configInformers.Start(ctx.Done())
			configInformers.WaitForCacheSync(ctx.Done())

			err := targetConfigController.Sync(ctx, FakeSyncContext{recorder: eventRecorder})
			if err != nil {
				t.Fatalf("Unexpected error returned by TargetConfigController.Sync: %v", err)
			}
			_, status, _, err := operatorClient.GetOperatorState()
			if err != nil {
				t.Fatalf("Unexpected error returned operatorClient.GetOperatorState(): %v", err)
			}

			// TargetConfigControllerDegraded is expected to be always set
			targetConfigControllerDegradedCondition := operatorv1.OperatorCondition{}
			var policyUpgradeableCondition *operatorv1.OperatorCondition
			for _, condition := range status.Conditions {
				if condition.Type == "TargetConfigControllerDegraded" {
					targetConfigControllerDegradedCondition = condition
				} else if condition.Type == "PolicyUpgradeable" {
					c := condition
					policyUpgradeableCondition = &c
				}
			}

			if targetConfigControllerDegradedCondition.Status != operatorv1.ConditionFalse {
				t.Errorf("TargetConfigControllerDegraded is expected to be %v, got %v instead", operatorv1.ConditionFalse, targetConfigControllerDegradedCondition.Status)
			}
			if test.upgradable {
				if policyUpgradeableCondition != nil {
					if policyUpgradeableCondition.Status != operatorv1.ConditionTrue {
						t.Errorf("PolicyUpgradeable condition expected to be missing or its status set to %v, got %v instead", operatorv1.ConditionTrue, policyUpgradeableCondition.Status)
					}
				}
			} else {
				if policyUpgradeableCondition == nil || policyUpgradeableCondition.Status != operatorv1.ConditionFalse {
					t.Errorf("PolicyUpgradeable condition expected to be set and its status set to %v, got %v instead", operatorv1.ConditionFalse, policyUpgradeableCondition.Status)
				}
			}
		})
	}
}

// Added unit test for getUnsupportedFlagsFromConfig
var fakeUnsupportedConfigArgsJson = `
{
  "arguments": {
      "fakeKey": [
			"value1",
			"value2"
	  ]
  }
}
`
var unmarshalFakeUnsupportedConfigArgsJson = `
{
  "arguments": {"fakeKey1", "fakeKey2"}
}
`

func TestGetUnsupportedFlagsFromConfig(t *testing.T) {
	tests := []struct {
		name                   string
		inputUnsupportedConfig []byte
		expectedResult         []string
	}{
		{
			name:                   "unsupportedFlagsinJson",
			inputUnsupportedConfig: []byte(unsupportedConfigOverridesMultipleSchedulerArgsJSON),
			expectedResult:         []string{"--master=https://localhost:1234", "--unsupported-kube-api-over-localhost=true"},
		},
		{
			name:                   "unsupportedFakeFlagsinJsonwithStringList",
			inputUnsupportedConfig: []byte(fakeUnsupportedConfigArgsJson),
			expectedResult:         []string{"--fakeKey=value1", "--fakeKey=value2"},
		},
		{
			name:                   "unmashalUnsupportedFakeFlagsinJson",
			inputUnsupportedConfig: []byte(unmarshalFakeUnsupportedConfigArgsJson),
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
		f.Events <- fmt.Sprintf(reason + " " + note)
	}
}

func (f *fakeRecorder) Eventf(reason, note string, args ...interface{}) {
	if f.Events != nil {
		f.Events <- fmt.Sprintf(reason+" "+note, args...)
	}
}

func (f *fakeRecorder) Warning(reason, note string) {
	if f.Events != nil {
		f.Events <- fmt.Sprintf(reason + " " + note)
	}
}

func (f *fakeRecorder) Warningf(reason, note string, args ...interface{}) {
	if f.Events != nil {
		f.Events <- fmt.Sprintf(reason+" "+note, args...)
	}
}

func (f *fakeRecorder) ForComponent(componentName string) events.Recorder {
	return *(*(events.Recorder))(unsafe.Pointer(f))
}

func (f *fakeRecorder) WithComponentSuffix(componentNameSuffix string) events.Recorder {
	return *(*(events.Recorder))(unsafe.Pointer(f))
}

func (f *fakeRecorder) ComponentName() string {
	return ""
}

func (f *fakeRecorder) Shutdown() {
}

func NewFakeRecorder(bufferSize int) *fakeRecorder {
	return &fakeRecorder{
		Events: make(chan string, bufferSize),
	}
}
