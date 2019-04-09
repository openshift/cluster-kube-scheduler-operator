package operator

import (
	configv1 "github.com/openshift/api/config/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"reflect"
	"testing"
)

func TestCheckForFeatureGates(t *testing.T) {
	tests := []struct {
		name           string
		configValue    configv1.FeatureSet
		expectedResult map[string]bool
	}{
		{
			name:        "default",
			configValue: configv1.Default,
			expectedResult: map[string]bool{
				"ExperimentalCriticalPodAnnotation": true,
				"RotateKubeletServerCertificate":    true,
				"SupportPodPidsLimit":               true,
				"LocalStorageCapacityIsolation":     false,
			},
		},
		{
			name:        "techpreview",
			configValue: configv1.TechPreviewNoUpgrade,
			expectedResult: map[string]bool{
				"ExperimentalCriticalPodAnnotation": true,
				"RotateKubeletServerCertificate":    true,
				"SupportPodPidsLimit":               true,
				"CSIBlockVolume":                    true,
				"LocalStorageCapacityIsolation":     false,
			},
		},
	}
	for _, tc := range tests {
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		indexer.Add(&configv1.FeatureGate{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
			Spec: configv1.FeatureGateSpec{
				FeatureSet: tc.configValue,
			},
		})
		featureGateLister := configlistersv1.NewFeatureGateLister(indexer)
		actualFeatureGates, _ := checkForFeatureGates(featureGateLister)
		if !reflect.DeepEqual(actualFeatureGates, tc.expectedResult) {
			t.Fatalf("Expected %v feature gates to be present but found %v", tc.expectedResult, actualFeatureGates)
		}
	}
}
