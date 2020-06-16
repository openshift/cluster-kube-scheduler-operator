package scheduler

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
)

// observeSchedulerConfig lists the scheduler configuration and updates the name of the configmap that we want scheduler
// to use as policy config
func ObserveSchedulerConfig(genericListers configobserver.Listers, recorder events.Recorder, existingConfig map[string]interface{}) (map[string]interface{}, []error) {
	listers := genericListers.(configobservation.Listers)
	errs := []error{}
	prevObservedConfig := map[string]interface{}{}

	sourceTargetLocation := resourcesynccontroller.ResourceLocation{}
	observedConfig := map[string]interface{}{}
	schedulerConfig, err := listers.SchedulerLister.Get("cluster")
	if errors.IsNotFound(err) {
		klog.Warningf("schedulers.config.openshift.io/cluster: not found")
		// We don't have scheduler CR, so remove the policy configmap if it exists in openshift-kube-scheduler namespace
		err = listers.ResourceSyncer().SyncConfigMap(
			resourcesynccontroller.ResourceLocation{
				Namespace: operatorclient.TargetNamespace,
				Name:      "policy-configmap",
			},
			sourceTargetLocation,
		)
		return observedConfig, errs
	}
	if err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	configMapName := schedulerConfig.Spec.Policy.Name

	switch {
	case len(configMapName) == 0:
		sourceTargetLocation = resourcesynccontroller.ResourceLocation{}
	case len(configMapName) > 0:
		sourceTargetLocation = resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.GlobalUserSpecifiedConfigNamespace,
			Name:      configMapName,
		}
	}

	// Sync the configmap from openshift-config namespace to openshift-kube-scheduler namespace. If the configMapName
	// is empty string, it will mirror the deletion as well.
	err = listers.ResourceSyncer().SyncConfigMap(
		resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.TargetNamespace,
			Name:      "policy-configmap",
		},
		sourceTargetLocation,
	)
	if err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	return observedConfig, errs
}
