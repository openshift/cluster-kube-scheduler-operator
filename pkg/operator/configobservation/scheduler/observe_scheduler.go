package scheduler

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	policyConfigMapRootPath := []string{"algorithmSource", "policy", "configMap"}
	policyConfigMapNamePath := append(policyConfigMapRootPath, "name")
	policyConfigMapNamespacePath := append(policyConfigMapRootPath, "namespace")
	currentPolicyConfigMapName, _, err := unstructured.NestedString(existingConfig, policyConfigMapNamePath...)
	if err != nil {
		return prevObservedConfig, append(errs, err)
	}
	if len(currentPolicyConfigMapName) > 0 {
		if err := unstructured.SetNestedField(prevObservedConfig, currentPolicyConfigMapName, policyConfigMapNamePath...); err != nil {
			errs = append(errs, err)
		}
	}
	currentPolicyConfigMapNamespace, _, err := unstructured.NestedString(existingConfig, policyConfigMapNamespacePath...)
	if err != nil {
		return prevObservedConfig, append(errs, err)
	}
	if len(currentPolicyConfigMapNamespace) > 0 {
		if err := unstructured.SetNestedField(prevObservedConfig, currentPolicyConfigMapNamespace, policyConfigMapNamespacePath...); err != nil {
			errs = append(errs, err)
		}
	}

	observedConfig := map[string]interface{}{}
	schedulerConfig, err := listers.SchedulerLister.Get("cluster")
	if errors.IsNotFound(err) {
		klog.Warningf("schedulers.config.openshift.io/cluster: not found")
		return observedConfig, errs
	}
	if err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	configMapName := schedulerConfig.Spec.Policy.Name

	if len(configMapName) == 0 {
		return observedConfig, errs
	}

	err = listers.ResourceSyncer().SyncConfigMap(
		resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.TargetNamespace,
			Name:      configMapName,
		},
		resourcesynccontroller.ResourceLocation{
			Namespace: operatorclient.GlobalUserSpecifiedConfigNamespace,
			Name:      configMapName,
		},
	)
	if err != nil {
		errs = append(errs, err)
		return prevObservedConfig, errs
	}
	if err := unstructured.SetNestedField(observedConfig, configMapName, policyConfigMapNamePath...); err != nil {
		errs = append(errs, err)
	}
	if configMapName != currentPolicyConfigMapName {
		recorder.Eventf("ObservedConfigMapNameChanged", "scheduler configmap changed to %q", configMapName)
	}
	if err := unstructured.SetNestedField(observedConfig, operatorclient.TargetNamespace, policyConfigMapNamespacePath...); err != nil {
		errs = append(errs, err)
	}
	return observedConfig, errs
}
