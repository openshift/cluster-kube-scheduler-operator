package operator

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/apis/kubescheduler/v1alpha1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1alpha1helpers"
)

// syncKubeScheduler_v311_00_to_latest takes care of synchronizing (not upgrading) the thing we're managing.
// most of the time the sync method will be good for a large span of minor versions
func createTargetConfigReconciler_v311_00_to_latest(c TargetConfigReconciler, operatorConfig *v1alpha1.KubeSchedulerOperatorConfig) (bool, error) {
	operatorConfigOriginal := operatorConfig.DeepCopy()
	errors := []error{}

	directResourceResults := resourceapply.ApplyDirectly(c.kubeClient, v311_00_assets.Asset,
		"v3.11.0/kube-scheduler/ns.yaml",
		"v3.11.0/kube-scheduler/public-info-role.yaml",
		"v3.11.0/kube-scheduler/public-info-rolebinding.yaml",
		"v3.11.0/kube-scheduler/scheduler-clusterrolebinding.yaml",
		"v3.11.0/kube-scheduler/svc.yaml",
		"v3.11.0/kube-scheduler/sa.yaml",
		"v3.11.0/kube-scheduler/installer-sa.yaml",
		"v3.11.0/kube-scheduler/installer-cluster-rolebinding.yaml",
	)
	for _, currResult := range directResourceResults {
		if currResult.Error != nil {
			errors = append(errors, fmt.Errorf("%q (%T): %v", currResult.File, currResult.Type, currResult.Error))
		}
	}

	schedulerConfig, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(c.kubeClient.CoreV1(), operatorConfig)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	_, _, err = managePod_v311_00_to_latest(c.kubeClient.CoreV1(), operatorConfig)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/kube-scheduler-pod", err))
	}
	configData := ""
	if schedulerConfig != nil {
		configData = schedulerConfig.Data["config.yaml"]
	}
	_, _, err = manageKubeSchedulerPublicConfigMap_v311_00_to_latest(c.kubeClient.CoreV1(), configData, operatorConfig)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/public-info", err))
	}

	if len(errors) > 0 {
		message := ""
		for _, err := range errors {
			message = message + err.Error() + "\n"
		}
		v1alpha1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1alpha1.OperatorCondition{
			Type:    "TargetConfigReconcilerFailing",
			Status:  operatorv1alpha1.ConditionTrue,
			Reason:  "SynchronizationError",
			Message: message,
		})
		if !reflect.DeepEqual(operatorConfigOriginal, operatorConfig) {
			_, updateError := c.operatorConfigClient.KubeSchedulerOperatorConfigs().UpdateStatus(operatorConfig)
			return true, updateError
		}
		return true, nil
	}

	v1alpha1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1alpha1.OperatorCondition{
		Type:   "TargetConfigReconcilerFailing",
		Status: operatorv1alpha1.ConditionFalse,
	})
	if !reflect.DeepEqual(operatorConfigOriginal, operatorConfig) {
		_, updateError := c.operatorConfigClient.KubeSchedulerOperatorConfigs().UpdateStatus(operatorConfig)
		if updateError != nil {
			return true, updateError
		}
	}

	return false, nil
}

func manageKubeSchedulerConfigMap_v311_00_to_latest(client coreclientv1.ConfigMapsGetter, operatorConfig *v1alpha1.KubeSchedulerOperatorConfig) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/cm.yaml"))
	defaultConfig := v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig.yaml")
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, defaultConfig, operatorConfig.Spec.UserConfig.Raw, operatorConfig.Spec.ObservedConfig.Raw)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, requiredConfigMap)
}

func managePod_v311_00_to_latest(client coreclientv1.ConfigMapsGetter, operatorConfig *v1alpha1.KubeSchedulerOperatorConfig) (*corev1.ConfigMap, bool, error) {
	required := resourceread.ReadPodV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod.yaml"))
	switch corev1.PullPolicy(operatorConfig.Spec.ImagePullPolicy) {
	case corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever:
		required.Spec.Containers[0].ImagePullPolicy = corev1.PullPolicy(operatorConfig.Spec.ImagePullPolicy)
	case "":
	default:
		return nil, false, fmt.Errorf("invalid imagePullPolicy specified: %v", operatorConfig.Spec.ImagePullPolicy)
	}
	required.Spec.Containers[0].Image = operatorConfig.Spec.ImagePullSpec
	required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", operatorConfig.Spec.Logging.Level))

	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorConfig.Spec.ForceRedeploymentReason
	configMap.Data["version"] = version.Get().String()
	return resourceapply.ApplyConfigMap(client, configMap)
}

func manageKubeSchedulerPublicConfigMap_v311_00_to_latest(client coreclientv1.ConfigMapsGetter, schedulerConfigString string, operatorConfig *v1alpha1.KubeSchedulerOperatorConfig) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/public-info.yaml"))
	if operatorConfig.Status.CurrentAvailability != nil {
		configMap.Data["version"] = operatorConfig.Status.CurrentAvailability.Version
	} else {
		configMap.Data["version"] = ""
	}

	return resourceapply.ApplyConfigMap(client, configMap)
}
