package operator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"

	"github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

const TargetPolicyConfigMapName = "policy-configmap"

// syncKubeScheduler_v311_00_to_latest takes care of synchronizing (not upgrading) the thing we're managing.
// most of the time the sync method will be good for a large span of minor versions
func createTargetConfigReconciler_v311_00_to_latest(c TargetConfigReconciler, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler) (bool, error) {
	operatorConfigOriginal := operatorConfig.DeepCopy()
	errors := []error{}

	directResourceResults := resourceapply.ApplyDirectly(c.kubeClient, c.eventRecorder, v311_00_assets.Asset,
		"v3.11.0/kube-scheduler/ns.yaml",
		"v3.11.0/kube-scheduler/kubeconfig-cm.yaml",
		"v3.11.0/kube-scheduler/leader-election-rolebinding.yaml",
		"v3.11.0/kube-scheduler/scheduler-clusterrolebinding.yaml",
		"v3.11.0/kube-scheduler/policyconfigmap-role.yaml",
		"v3.11.0/kube-scheduler/policyconfigmap-rolebinding.yaml",
		"v3.11.0/kube-scheduler/svc.yaml",
		"v3.11.0/kube-scheduler/sa.yaml",
	)
	for _, currResult := range directResourceResults {
		if currResult.Error != nil {
			errors = append(errors, fmt.Errorf("%q (%T): %v", currResult.File, currResult.Type, currResult.Error))
		}
	}
	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(c.configMapLister, c.kubeClient.CoreV1(), recorder, operatorConfig, c.SchedulerLister)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	_, _, err = manageServiceAccountCABundle(c.configMapLister, c.kubeClient.CoreV1(), recorder)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/serviceaccount-ca", err))
	}
	_, _, err = managePod_v311_00_to_latest(c.kubeClient.CoreV1(), c.kubeClient.CoreV1(), recorder, operatorConfig, c.targetImagePullSpec, c.featureGateLister)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/kube-scheduler-pod", err))
	}

	if len(errors) > 0 {
		message := ""
		for _, err := range errors {
			message = message + err.Error() + "\n"
		}
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1.OperatorCondition{
			Type:    "TargetConfigReconcilerDegraded",
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
		Type:   "TargetConfigReconcilerDegraded",
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

func manageKubeSchedulerConfigMap_v311_00_to_latest(lister corev1listers.ConfigMapLister, client coreclientv1.ConfigMapsGetter, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler, schedulerLister configlistersv1.SchedulerLister) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/cm.yaml"))
	var defaultConfig []byte
	observedpolicyConfigMap, err := schedulerLister.Get("cluster")
	if err != nil {
		klog.Infof("Error while listing configmap %v", err.Error())
	}
	var policyConfigMapName string
	if err == nil && observedpolicyConfigMap != nil && len(observedpolicyConfigMap.Spec.Policy.Name) > 0 {
		policyConfigMapName = observedpolicyConfigMap.Spec.Policy.Name
		policyConfigMap, err := lister.ConfigMaps(operatorclient.GlobalUserSpecifiedConfigNamespace).Get(policyConfigMapName)
		if err == nil {
			// Create a new Configmap within targetNamespace to be used.
			targetPolicyConfigMap := policyConfigMap.DeepCopy()
			targetPolicyConfigMap.Namespace = operatorclient.TargetNamespace
			// TODO: Switch to using config observer instead of doing it here.
			targetPolicyConfigMap.Name = TargetPolicyConfigMapName
			targetPolicyConfigMap.ResourceVersion = ""
			_, err := client.ConfigMaps(operatorclient.TargetNamespace).Create(targetPolicyConfigMap)
			if err == nil || apierrors.IsAlreadyExists(err) {
				klog.Infof("Custom policy config map to be used by scheduler is successfully created")
				defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap-with-policy.yaml")
			} else {
				// This means policyconfigmap could not be created, so let's default to postbootstrap only.
				klog.Infof("Policy configmap creation error %v and using default algorithm provider in kubernetes scheduler", err.Error())
				defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap.yaml")
			}
		} else {
			klog.Infof("Error while listing scheduler configmap from openshift-config namespace %v and using default algorithm provider in kubernetes scheduler", err.Error())
			defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap.yaml")
		}
	} else {
		msg := "unknown"
		switch {
		case err != nil:
			msg = err.Error()
		case len(observedpolicyConfigMap.Spec.Policy.Name) == 0:
			msg = "missing policy"
		}
		klog.Infof("Error while getting scheduler type %v and using default algorithm provider in kubernetes scheduler", msg)
		defaultConfig = v311_00_assets.MustAsset("v3.11.0/kube-scheduler/defaultconfig-postbootstrap.yaml")
	}
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, defaultConfig, operatorConfig.Spec.ObservedConfig.Raw, operatorConfig.Spec.UnsupportedConfigOverrides.Raw)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}

func managePod_v311_00_to_latest(configMapsGetter coreclientv1.ConfigMapsGetter, secretsGetter coreclientv1.SecretsGetter, recorder events.Recorder, operatorConfig *operatorv1.KubeScheduler, imagePullSpec string, featureGateLister configlistersv1.FeatureGateLister) (*corev1.ConfigMap, bool, error) {
	required := resourceread.ReadPodV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod.yaml"))
	if len(imagePullSpec) > 0 {
		required.Spec.Containers[0].Image = imagePullSpec
		if len(required.Spec.InitContainers) > 0 {
			required.Spec.InitContainers[0].Image = imagePullSpec
		}
	}

	// check for feature gates from feature lister.
	featureGates := checkForFeatureGates(featureGateLister)
	sortedFeatureGates := getSortedFeatureGates(featureGates)
	allFeatureGates := getFeatureGateString(sortedFeatureGates, featureGates)
	required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("--feature-gates=%v", allFeatureGates))

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

	if _, err := secretsGetter.Secrets(required.Namespace).Get("serving-cert", metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return nil, false, err
	} else if err == nil {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, "--tls-cert-file=/etc/kubernetes/static-pod-resources/secrets/serving-cert/tls.crt")
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, "--tls-private-key-file=/etc/kubernetes/static-pod-resources/secrets/serving-cert/tls.key")
	}

	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorConfig.Spec.ForceRedeploymentReason
	configMap.Data["version"] = version.Get().String()
	return resourceapply.ApplyConfigMap(configMapsGetter, recorder, configMap)
}

func getSortedFeatureGates(featureGates map[string]bool) []string {
	var sortedFeatureGates []string
	for featureGateName := range featureGates {
		sortedFeatureGates = append(sortedFeatureGates, featureGateName)
	}
	sort.Strings(sortedFeatureGates)
	return sortedFeatureGates
}

func getFeatureGateString(sortedFeatureGates []string, featureGates map[string]bool) string {
	allFeatureGates := ""
	for _, featureGateName := range sortedFeatureGates {
		allFeatureGates = allFeatureGates + "," + fmt.Sprintf("%v=%v", featureGateName, featureGates[featureGateName])
	}
	return strings.TrimPrefix(allFeatureGates, ",")
}
func checkForFeatureGates(featureGateLister configlistersv1.FeatureGateLister) map[string]bool {
	featureGateListConfig, err := featureGateLister.Get("cluster")
	var enabledFeatureSets, disabledFeatureSets []string
	var featureGates = make(map[string]bool)
	if err != nil {
		klog.Infof("Error while listing features.config.openshift.io/cluster with %v: so return default feature gates", err.Error())
		if featureSet, ok := v1.FeatureSets[v1.Default]; ok {
			enabledFeatureSets = featureSet.Enabled
			disabledFeatureSets = featureSet.Disabled
		}
		return generateFeatureGates(enabledFeatureSets, disabledFeatureSets, featureGates)
	}

	currentFeatureSetConfig := featureGateListConfig.Spec.FeatureSet
	if featureSet, ok := v1.FeatureSets[currentFeatureSetConfig]; ok {
		enabledFeatureSets = featureSet.Enabled
		disabledFeatureSets = featureSet.Disabled
	} else {
		klog.Infof("Invalid feature set config found in features.config.openshift.io/cluster %v. Please look at allowed features", currentFeatureSetConfig)
	}
	return generateFeatureGates(enabledFeatureSets, disabledFeatureSets, featureGates)
}

func generateFeatureGates(enabledFeatureGates, disabledFeatureGates []string, featureGates map[string]bool) map[string]bool {
	for _, enabledFeatureGate := range enabledFeatureGates {
		featureGates[enabledFeatureGate] = true
	}
	for _, disabledFeatureGate := range disabledFeatureGates {
		featureGates[disabledFeatureGate] = false
	}
	return featureGates
}

func manageServiceAccountCABundle(lister corev1listers.ConfigMapLister, client coreclientv1.ConfigMapsGetter, recorder events.Recorder) (*corev1.ConfigMap, bool, error) {
	requiredConfigMap, err := resourcesynccontroller.CombineCABundleConfigMaps(
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.TargetNamespace, Name: "serviceaccount-ca"},
		lister,
		// include the ca bundle needed to recognize the server
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "kube-apiserver-server-ca"},
		// include the ca bundle needed to recognize default
		// certificates generated by cluster-ingress-operator
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "router-ca"},
	)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}
