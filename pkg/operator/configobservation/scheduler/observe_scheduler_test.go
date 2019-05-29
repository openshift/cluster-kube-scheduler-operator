package scheduler

import (
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"fmt"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
)

func TestObserveSchedulerConfig(t *testing.T) {
	configMapName := "policy-configmap"
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(&configv1.Scheduler{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.SchedulerSpec{
			Policy: configv1.ConfigMapNameReference{Name: "policy-configmap"},
		},
	}); err != nil {
		t.Fatal(err.Error())
	}
	synced := map[string]string{}
	listers := configobservation.Listers{
		SchedulerLister: configlistersv1.NewSchedulerLister(indexer),
		ResourceSync:    &mockResourceSyncer{t: t, synced: synced},
	}
	result, errors := ObserveSchedulerConfig(listers, events.NewInMemoryRecorder("scheduler"), map[string]interface{}{})
	if len(errors) > 0 {
		t.Fatalf("expected len(errors) == 0")
	}
	observedConfigMapName, _, err := unstructured.NestedString(result, "algorithmSource", "policy", "configMap", "name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	observedConfigMapNamespace, _, err := unstructured.NestedString(result, "algorithmSource", "policy", "configMap", "namespace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if observedConfigMapName != configMapName {
		t.Fatalf("expected configmap to be %v but got %v", observedConfigMapName, configMapName)
	}
	if observedConfigMapNamespace != operatorclient.TargetNamespace {
		t.Fatalf("expected target namespace to be %v but got %v", observedConfigMapName, configMapName)
	}
}

type mockResourceSyncer struct {
	t      *testing.T
	synced map[string]string
}

func (rs *mockResourceSyncer) SyncConfigMap(destination, source resourcesynccontroller.ResourceLocation) error {
	if (source == resourcesynccontroller.ResourceLocation{}) {
		rs.synced[fmt.Sprintf("configmap/%v.%v", destination.Name, destination.Namespace)] = "DELETE"
	} else {
		rs.synced[fmt.Sprintf("configmap/%v.%v", destination.Name, destination.Namespace)] = fmt.Sprintf("configmap/%v.%v", source.Name, source.Namespace)
	}
	return nil
}

func (rs *mockResourceSyncer) SyncSecret(destination, source resourcesynccontroller.ResourceLocation) error {
	if (source == resourcesynccontroller.ResourceLocation{}) {
		rs.synced[fmt.Sprintf("secret/%v.%v", destination.Name, destination.Namespace)] = "DELETE"
	} else {
		rs.synced[fmt.Sprintf("secret/%v.%v", destination.Name, destination.Namespace)] = fmt.Sprintf("secret/%v.%v", source.Name, source.Namespace)
	}
	return nil
}
