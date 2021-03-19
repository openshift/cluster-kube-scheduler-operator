package targetconfigcontroller

import (
	"context"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	operatorv1 "github.com/openshift/api/operator/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/operator/events"
)

var codec = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

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
				"LegacyNodeRoleBehavior":         false,
				"NodeDisruptionExclusion":        true,
				"RotateKubeletServerCertificate": true,
				"DownwardAPIHugePages":           true,
				"ServiceNodeExclusion":           true,
				"SupportPodPidsLimit":            true,
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
			actualConfigMap, _, err := managePod_v311_00_to_latest(
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
