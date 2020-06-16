package scheduler

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	configv1 "github.com/openshift/api/config/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
)

// TODO(@damemi): Update this to test to re-include checks that policy config is properly merged into ComponentConfig
//  (Once we support ComponentConfig/Plugins and deprecate Policy config)
//  Re: https://github.com/openshift/cluster-kube-scheduler-operator/pull/255
func TestObserveSchedulerConfig(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

	tests := []struct {
		description   string
		configMapName string
		updateName    bool
	}{
		{
			description:   "A different configmap name but still we need to set the policy configmap to hardcoded value",
			configMapName: "test-abc",
			updateName:    false,
		},
		{
			description:   "A configmap with same name as policy-configmap but still we need to set the policy configmap to hardcoded value",
			configMapName: "policy-configmap",
			updateName:    false,
		},
		{
			description:   "An empty configmap name should clear anything currently set in the observed config",
			configMapName: "policy-configmap",
			updateName:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			if err := indexer.Add(&configv1.Scheduler{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.SchedulerSpec{
					Policy: configv1.ConfigMapNameReference{Name: test.configMapName},
				},
			}); err != nil {
				t.Fatal(err.Error())
			}
			synced := map[string]string{}
			listers := configobservation.Listers{
				SchedulerLister: configlistersv1.NewSchedulerLister(indexer),
				ResourceSync:    &mockResourceSyncer{t: t, synced: synced},
			}
			_, errors := ObserveSchedulerConfig(listers, events.NewInMemoryRecorder("scheduler"), map[string]interface{}{})
			if len(errors) > 0 {
				t.Fatalf("expected len(errors) == 0")
			}
			if test.updateName {
				// clear the configmap name in scheduler config to test that this also carries to the observed config
				if err := indexer.Update(&configv1.Scheduler{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec: configv1.SchedulerSpec{
						Policy: configv1.ConfigMapNameReference{Name: ""},
					},
				}); err != nil {
					t.Fatal(err.Error())
				}
				_, errors = ObserveSchedulerConfig(listers, events.NewInMemoryRecorder("scheduler"), map[string]interface{}{})
				if len(errors) > 0 {
					t.Fatalf("expected len(errors) == 0")
				}
			}
		})
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
