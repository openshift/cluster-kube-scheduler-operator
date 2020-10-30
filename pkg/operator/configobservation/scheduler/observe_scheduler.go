package scheduler

import (
	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/configobserver"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
)

// observeSchedulerConfig syncs the scheduler config from the openshift-config namespace to the kube-scheduler, if set
// TODO(@damemi): This currently handles either a Policy or a Profiles config. When Policy is removed, those handlers should also be
func ObserveSchedulerConfig(genericListers configobserver.Listers, recorder events.Recorder, existingConfig map[string]interface{}) (map[string]interface{}, []error) {
	listers := genericListers.(configobservation.Listers)
	errs := []error{}
	prevObservedConfig := map[string]interface{}{}
	observedConfig := map[string]interface{}{}

	sourceTargetLocation := resourcesynccontroller.ResourceLocation{}
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

	// Deprecated handling for Policy config
	// TODO(@damemi): Remove this when Policy API is removed
	if len(schedulerConfig.Spec.Policy.Name) > 0 {
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

	currentProfiles, _, err := unstructured.NestedSlice(existingConfig, "profiles")
	if err != nil {
		return prevObservedConfig, append(errs, err)
	}
	if len(currentProfiles) > 0 {
		if err := unstructured.SetNestedField(prevObservedConfig, currentProfiles, "profiles"); err != nil {
			errs = append(errs, err)
		}
	}

	if len(schedulerConfig.Spec.Profiles.Name) == 0 {
		if len(currentProfiles) > 0 {
			recorder.Eventf("ObservedConfigMapNameChanged", "scheduler profiles configmap removed")
			unstructured.RemoveNestedField(prevObservedConfig, "profiles")
		}
		return prevObservedConfig, errs
	}
	profileConfig, err := listers.ConfigmapLister.ConfigMaps(operatorclient.GlobalUserSpecifiedConfigNamespace).Get(schedulerConfig.Spec.Profiles.Name)
	if err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	config := profileConfig.Data["profiles.cfg"]
	if len(config) == 0 && len(currentProfiles) > 0 {
		recorder.Eventf("ObservedProfilesChanged", "scheduler profiles removed")
		return prevObservedConfig, errs
	}
	profiles := map[string][]interface{}{}
	if err := yaml.Unmarshal([]byte(config), &profiles); err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	if err := unstructured.SetNestedSlice(observedConfig, profiles["profiles"], "profiles"); err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	return observedConfig, errs
}
