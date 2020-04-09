package operator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v410_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
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
func createTargetConfigReconciler_v311_00_to_latest(ctx context.Context, c TargetConfigReconciler, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec) (bool, error) {
	errors := []error{}
	ctx, span := global.TraceProvider().Tracer("target-config-reconciler").Start(ctx, "creatingTargetConfigReconciler")
	defer span.End()

	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(ctx, c.configMapLister, c.kubeClient.CoreV1(), recorder, operatorSpec)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	_, _, err = manageServiceAccountCABundle(c.configMapLister, c.kubeClient.CoreV1(), recorder)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/serviceaccount-ca", err))
	}
	err = ensureLocalhostRecoverySAToken(c.ctx, c.kubeClient.CoreV1(), recorder)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "serviceaccount/localhost-recovery-client", err))
	}
	_, _, err = managePod_v311_00_to_latest(c.ctx, c.kubeClient.CoreV1(), c.kubeClient.CoreV1(), recorder, operatorSpec, c.targetImagePullSpec, c.operatorImagePullSpec, c.featureGateLister)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/kube-scheduler-pod", err))
	}

	if len(errors) > 0 {
		condition := operatorv1.OperatorCondition{
			Type:    "TargetConfigControllerDegraded",
			Status:  operatorv1.ConditionTrue,
			Reason:  "SynchronizationError",
			Message: v1helpers.NewMultiLineAggregate(errors).Error(),
		}
		if _, _, err := v1helpers.UpdateStaticPodStatus(c.operatorClient, v1helpers.UpdateStaticPodConditionFn(condition)); err != nil {
			return true, err
		}
		return true, nil
	}

	span.AddEvent(ctx, "updated controller condition", core.KeyValue{Key: "TargetConfigControllerDegraded", Value: core.String(string(operatorv1.ConditionFalse))})
	condition := operatorv1.OperatorCondition{
		Type:   "TargetConfigControllerDegraded",
		Status: operatorv1.ConditionFalse,
	}
	if _, _, err := v1helpers.UpdateStaticPodStatus(c.operatorClient, v1helpers.UpdateStaticPodConditionFn(condition)); err != nil {
		return true, err
	}

	return false, nil
}

func manageKubeSchedulerConfigMap_v311_00_to_latest(ctx context.Context, lister corev1listers.ConfigMapLister, client corev1client.ConfigMapsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec) (*corev1.ConfigMap, bool, error) {
	_, span := global.TraceProvider().Tracer("target-config-reconciler").Start(ctx, "managing kubeschedulerconfigmap")
	defer span.End()
	configMap := resourceread.ReadConfigMapV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/cm.yaml"))
	defaultConfig := v410_00_assets.MustAsset("v4.1.0/config/defaultconfig-postbootstrap.yaml")
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, defaultConfig, operatorSpec.ObservedConfig.Raw, operatorSpec.UnsupportedConfigOverrides.Raw)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}

func managePod_v311_00_to_latest(ctx context.Context, configMapsGetter corev1client.ConfigMapsGetter, secretsGetter corev1client.SecretsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec, imagePullSpec, operatorImagePullSpec string, featureGateLister configlistersv1.FeatureGateLister) (*corev1.ConfigMap, bool, error) {
	required := resourceread.ReadPodV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/pod.yaml"))
	images := map[string]string{
		"${IMAGE}":          imagePullSpec,
		"${OPERATOR_IMAGE}": operatorImagePullSpec,
	}
	for i := range required.Spec.Containers {
		for pat, img := range images {
			if required.Spec.Containers[i].Image == pat {
				required.Spec.Containers[i].Image = img
				break
			}
		}
	}
	for i := range required.Spec.InitContainers {
		for pat, img := range images {
			if required.Spec.InitContainers[i].Image == pat {
				required.Spec.InitContainers[i].Image = img
				break
			}
		}
	}

	// check for feature gates from feature lister.
	featureGates := checkForFeatureGates(featureGateLister)
	sortedFeatureGates := getSortedFeatureGates(featureGates)
	allFeatureGates := getFeatureGateString(sortedFeatureGates, featureGates)
	required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("--feature-gates=%v", allFeatureGates))

	switch operatorSpec.LogLevel {
	case operatorv1.Normal:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	case operatorv1.Debug:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 4))
	case operatorv1.Trace:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 6))
	case operatorv1.TraceAll:
		// We use V(10) here because many critical debugging logs from the scheduler are set to loglevel 10 upstream,
		// such as node scores when running priority plugins. See https://github.com/openshift/cluster-kube-scheduler-operator/pull/232
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 10))
	default:
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", 2))
	}

	if _, err := secretsGetter.Secrets(required.Namespace).Get(ctx, "serving-cert", metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return nil, false, err
	} else if err == nil {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, "--tls-cert-file=/etc/kubernetes/static-pod-resources/secrets/serving-cert/tls.crt")
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, "--tls-private-key-file=/etc/kubernetes/static-pod-resources/secrets/serving-cert/tls.key")
	}

	configMap := resourceread.ReadConfigMapV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorSpec.ForceRedeploymentReason
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

func manageServiceAccountCABundle(lister corev1listers.ConfigMapLister, client corev1client.ConfigMapsGetter, recorder events.Recorder) (*corev1.ConfigMap, bool, error) {
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

func ensureLocalhostRecoverySAToken(ctx context.Context, client corev1client.CoreV1Interface, recorder events.Recorder) error {
	requiredSA := resourceread.ReadServiceAccountV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/localhost-recovery-sa.yaml"))
	requiredToken := resourceread.ReadSecretV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/localhost-recovery-token.yaml"))

	saClient := client.ServiceAccounts(operatorclient.TargetNamespace)
	serviceAccount, err := saClient.Get(ctx, requiredSA.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// The default token secrets get random names so we have created a custom secret
	// to be populated with SA token so we have a stable name.
	secretsClient := client.Secrets(operatorclient.TargetNamespace)
	token, err := secretsClient.Get(ctx, requiredToken.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Token creation / injection for a SA is asynchronous.
	// We will report and error if it's missing, go degraded and get re-queued when the SA token is updated.

	uid := token.Annotations[corev1.ServiceAccountUIDKey]
	if len(uid) == 0 {
		return fmt.Errorf("secret %s/%s hasn't been populated with SA token yet: missing SA UID", token.Namespace, token.Name)
	}

	if uid != string(serviceAccount.UID) {
		return fmt.Errorf("secret %s/%s hasn't been populated with current SA token yet: SA UID mismatch", token.Namespace, token.Name)
	}

	if len(token.Data) == 0 {
		return fmt.Errorf("secret %s/%s hasn't been populated with any data yet", token.Namespace, token.Name)
	}

	// Explicitly check that the fields we use are there, so we find out easily if some are removed or renamed.

	_, ok := token.Data["token"]
	if !ok {
		return fmt.Errorf("secret %s/%s hasn't been populated with current SA token yet", token.Namespace, token.Name)
	}

	_, ok = token.Data["ca.crt"]
	if !ok {
		return fmt.Errorf("secret %s/%s hasn't been populated with current SA token root CA yet", token.Namespace, token.Name)
	}

	return err
}
