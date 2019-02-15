package operator

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"reflect"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

// syncKubeScheduler_v311_00_to_latest takes care of synchronizing (not upgrading) the thing we're managing.
// most of the time the sync method will be good for a large span of minor versions
func createTargetConfigReconciler_v311_00_to_latest(c TargetConfigReconciler, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler) (bool, error) {
	operatorConfigOriginal := operatorConfig.DeepCopy()
	errors := []error{}

	directResourceResults := resourceapply.ApplyDirectly(c.kubeClient, c.eventRecorder, v311_00_assets.Asset,
		"v3.11.0/kube-scheduler/ns.yaml",
		"v3.11.0/kube-scheduler/scheduler-clusterrolebinding.yaml",
		"v3.11.0/kube-scheduler/svc.yaml",
		"v3.11.0/kube-scheduler/sa.yaml",
	)
	for _, currResult := range directResourceResults {
		if currResult.Error != nil {
			errors = append(errors, fmt.Errorf("%q (%T): %v", currResult.File, currResult.Type, currResult.Error))
		}
	}
	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(c.configMapLister, c.kubeClient.CoreV1(), recorder, operatorConfig)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	_, _, err = managePod_v311_00_to_latest(c.kubeClient.CoreV1(), recorder, operatorConfig, c.targetImagePullSpec)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/kube-scheduler-pod", err))
	}

	if len(errors) > 0 {
		message := ""
		for _, err := range errors {
			message = message + err.Error() + "\n"
		}
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1.OperatorCondition{
			Type:    "TargetConfigReconcilerFailing",
			Status:  operatorv1.ConditionTrue,
			Reason:  "SynchronizationError",
			Message: message,
		})
		if !reflect.DeepEqual(operatorConfigOriginal, operatorConfig) {
			_, updateError := c.operatorConfigClient.KubeSchedulers().UpdateStatus(operatorConfig)
			return true, updateError
		}
		return true, nil
	}

	v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1.OperatorCondition{
		Type:   "TargetConfigReconcilerFailing",
		Status: operatorv1.ConditionFalse,
	})
	if !reflect.DeepEqual(operatorConfigOriginal, operatorConfig) {
		_, updateError := c.operatorConfigClient.KubeSchedulers().UpdateStatus(operatorConfig)
		if updateError != nil {
			return true, updateError
		}
	}

	return false, nil
}

func manageKubeSchedulerConfigMap_v311_00_to_latest(lister corev1listers.ConfigMapLister, client coreclientv1.ConfigMapsGetter, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/cm.yaml"))
	var defaultConfig []byte
	policyConfigMap, err := lister.ConfigMaps(operatorclient.GlobalUserSpecifiedConfigNamespace).Get("policy-configmap")
	if err == nil {
		// Create a new Configmap within targetNamespace to be used.
		targetPolicyConfigMap := policyConfigMap.DeepCopy()
		targetPolicyConfigMap.Namespace = operatorclient.TargetNamespace
		targetPolicyConfigMap.ResourceVersion = ""
		_, err := client.ConfigMaps(operatorclient.TargetNamespace).Create(targetPolicyConfigMap)
		if err == nil || apierrors.IsAlreadyExists(err) {
			defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap-with-policy.yaml")
		} else {
			// This means policyconfigmap could not be created, so let's default to postbootstrap only.
			glog.Infof("Policy configmap creation error %v", err.Error())
			defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap.yaml")
		}
	} else {
		defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap.yaml")
	}
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, defaultConfig, operatorConfig.Spec.ObservedConfig.Raw, operatorConfig.Spec.UnsupportedConfigOverrides.Raw)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}

func managePod_v311_00_to_latest(client coreclientv1.ConfigMapsGetter, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler, imagePullSpec string) (*corev1.ConfigMap, bool, error) {
	required := resourceread.ReadPodV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod.yaml"))
	required.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
	if len(imagePullSpec) > 0 {
		required.Spec.Containers[0].Image = imagePullSpec
	}
	switch operatorConfig.Spec.LogLevel {
	case operatorv1.Normal:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	case operatorv1.Debug:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 4))
	case operatorv1.Trace:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 6))
	case operatorv1.TraceAll:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 8))
	default:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	}

	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorConfig.Spec.ForceRedeploymentReason
	configMap.Data["version"] = version.Get().String()
	return resourceapply.ApplyConfigMap(client, recorder, configMap)
}
