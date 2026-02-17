package targetconfigcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ghodss/yaml"

	v1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/bindata"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/certrotation"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/management"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	schedulerconfigv1 "k8s.io/kube-scheduler/config/v1"
)

type TargetConfigController struct {
	targetImagePullSpec   string
	operatorImagePullSpec string
	operatorImageVersion  string
	featureGates          featuregates.FeatureGate
	operatorClient        v1helpers.StaticPodOperatorClient
	kubeClient            kubernetes.Interface
	configMapLister       corev1listers.ConfigMapLister
	infrastuctureLister   configlistersv1.InfrastructureLister
	configSchedulerLister configlistersv1.SchedulerLister
}

func NewTargetConfigController(
	targetImagePullSpec, operatorImagePullSpec, operatorImageVersion string,
	featureGates featuregates.FeatureGate,
	operatorConfigClient v1helpers.OperatorClient,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	operatorClient v1helpers.StaticPodOperatorClient,
	kubeClient kubernetes.Interface,
	recorder events.Recorder,
) factory.Controller {
	c := &TargetConfigController{
		targetImagePullSpec:   targetImagePullSpec,
		operatorImagePullSpec: operatorImagePullSpec,
		operatorImageVersion:  operatorImageVersion,
		featureGates:          featureGates,
		kubeClient:            kubeClient,
		configMapLister:       kubeInformersForNamespaces.ConfigMapLister(),
		infrastuctureLister:   configInformer.Config().V1().Infrastructures().Lister(),
		operatorClient:        operatorClient,
		configSchedulerLister: configInformer.Config().V1().Schedulers().Lister(),
	}

	return factory.New().WithInformers(
		// these are for watching our outputs in case someone changes them
		kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace).Core().V1().ConfigMaps().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace).Core().V1().Secrets().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace).Core().V1().ServiceAccounts().Informer(),

		// for configmaps and secrets from our inputs
		kubeInformersForNamespaces.InformersFor(operatorclient.GlobalUserSpecifiedConfigNamespace).Core().V1().ConfigMaps().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.GlobalUserSpecifiedConfigNamespace).Core().V1().Secrets().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.GlobalMachineSpecifiedConfigNamespace).Core().V1().ConfigMaps().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.GlobalMachineSpecifiedConfigNamespace).Core().V1().Secrets().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.OperatorNamespace).Core().V1().ConfigMaps().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorclient.OperatorNamespace).Core().V1().Secrets().Informer(),

		operatorClient.Informer(),
		operatorConfigClient.Informer(),

		configInformer.Config().V1().Schedulers().Informer(),
		configInformer.Config().V1().FeatureGates().Informer(),
		configInformer.Config().V1().Infrastructures().Informer(),
	).WithNamespaceInformer(
		// we only watch our output namespace
		kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace).Core().V1().Namespaces().Informer(), operatorclient.TargetNamespace,
	).ResyncEvery(time.Minute).WithSync(c.sync).ToController("TargetConfigController", recorder)
}

func (c TargetConfigController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	operatorSpec, _, _, err := c.operatorClient.GetStaticPodOperatorState()
	if err != nil {
		return err
	}

	if !management.IsOperatorManaged(operatorSpec.ManagementState) {
		return nil
	}

	requeue, err := createTargetConfigController_v311_00_to_latest(ctx, syncCtx, c, operatorSpec)
	if err != nil {
		return err
	}
	if requeue {
		return fmt.Errorf("synthetic requeue request")
	}

	return nil
}

// createTargetConfigController_v311_00_to_latest takes care of synchronizing (not upgrading) the thing we're managing.
// most of the time the sync method will be good for a large span of minor versions
func createTargetConfigController_v311_00_to_latest(ctx context.Context, syncCtx factory.SyncContext, c TargetConfigController, operatorSpec *operatorv1.StaticPodOperatorSpec) (bool, error) {
	errors := []error{}

	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(ctx, c.featureGates, c.kubeClient.CoreV1(), syncCtx.Recorder(), c.configSchedulerLister)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	_, _, err = manageServiceAccountCABundle(ctx, c.configMapLister, c.kubeClient.CoreV1(), syncCtx.Recorder())
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/serviceaccount-ca", err))
	}
	err = ensureLocalhostRecoverySAToken(ctx, c.kubeClient.CoreV1(), syncCtx.Recorder())
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "serviceaccount/localhost-recovery-client", err))
	}
	_, _, err = manageSchedulerKubeconfig(ctx, c.kubeClient.CoreV1(), c.infrastuctureLister, syncCtx.Recorder())
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/scheduler-kubeconfig", err))
	}
	_, _, err = managePod_v311_00_to_latest(ctx, c.featureGates, c.kubeClient.CoreV1(), c.kubeClient.CoreV1(), syncCtx.Recorder(), operatorSpec, c.targetImagePullSpec, c.operatorImagePullSpec, c.operatorImageVersion, c.configSchedulerLister)
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
		if _, _, err := v1helpers.UpdateStaticPodStatus(ctx, c.operatorClient, v1helpers.UpdateStaticPodConditionFn(condition)); err != nil {
			return true, err
		}
		return true, nil
	}

	condition := operatorv1.OperatorCondition{
		Type:   "TargetConfigControllerDegraded",
		Status: operatorv1.ConditionFalse,
	}
	if _, _, err := v1helpers.UpdateStaticPodStatus(ctx, c.operatorClient, v1helpers.UpdateStaticPodConditionFn(condition)); err != nil {
		return true, err
	}

	return false, nil
}

func manageKubeSchedulerConfigMap_v311_00_to_latest(ctx context.Context, featureGates featuregates.FeatureGate, client corev1client.ConfigMapsGetter, recorder events.Recorder, configSchedulerLister configlistersv1.SchedulerLister) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(bindata.MustAsset("assets/kube-scheduler/cm.yaml"))

	var kubeSchedulerConfiguration []byte

	config, err := configSchedulerLister.Get("cluster")
	if err != nil {
		return nil, false, err
	}
	// Scheduler Policy API has been removed in k8s 1.23
	// Still keep the CRD field for compatibility, but it does nothing now
	if len(config.Spec.Policy.Name) > 0 {
		return nil, false, fmt.Errorf("scheduler Policy config has been removed upstream, this field remains for CRD compatibility but does nothing now. Please use a Profile instead (defaulting to LowNodeUtilization).")
	} else {
		switch config.Spec.Profile {
		case v1.LowNodeUtilization, "":
			kubeSchedulerConfiguration = bindata.MustAsset("assets/config/defaultconfig-postbootstrap-lownodeutilization.yaml")
		case v1.HighNodeUtilization:
			kubeSchedulerConfiguration = bindata.MustAsset("assets/config/defaultconfig-postbootstrap-highnodeutilization.yaml")
		case v1.NoScoring:
			kubeSchedulerConfiguration = bindata.MustAsset("assets/config/defaultconfig-postbootstrap-noscoring.yaml")
		default:
			return nil, false, fmt.Errorf("profile %q not recognized", config.Spec.Profile)
		}
	}

	schedulerConfiguration := &schedulerconfigv1.KubeSchedulerConfiguration{}
	if err := yaml.Unmarshal(kubeSchedulerConfiguration, schedulerConfiguration); err != nil {
		return nil, false, err
	}

	var enableDRAPlugin bool
	switch config.Spec.ProfileCustomizations.DynamicResourceAllocation {
	case v1.DRAEnablementEnabled:
		enableDRAPlugin = true
	case "", v1.DRAEnablementDisabled:
		// no-op
	default:
		return nil, false, fmt.Errorf("dynamicResourceAllocation customization %q not recognized", config.Spec.ProfileCustomizations.DynamicResourceAllocation)
	}
	// if the feature gate DynamicResourceAllocation is enabled, we will enable the plugin
	if !enableDRAPlugin {
		if featureGates.Enabled("DynamicResourceAllocation") {
			enableDRAPlugin = true
		}
	}
	if enableDRAPlugin {
		if len(schedulerConfiguration.Profiles) == 0 {
			schedulerConfiguration.Profiles = []schedulerconfigv1.KubeSchedulerProfile{{}}
		}
		if schedulerConfiguration.Profiles[0].Plugins == nil {
			schedulerConfiguration.Profiles[0].Plugins = &schedulerconfigv1.Plugins{}
		}
		schedulerConfiguration.Profiles[0].Plugins.MultiPoint.Enabled = append(schedulerConfiguration.Profiles[0].Plugins.MultiPoint.Enabled, schedulerconfigv1.Plugin{Name: "DynamicResources"})
	}

	schedulerConfigurationBytes, err := yaml.Marshal(schedulerConfiguration)
	if err != nil {
		return nil, false, err
	}

	defaultConfig := bindata.MustAsset("assets/config/defaultconfig.yaml")
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, schedulerConfigurationBytes, defaultConfig)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(ctx, client, recorder, requiredConfigMap)
}

func managePod_v311_00_to_latest(ctx context.Context, featureGates featuregates.FeatureGate, configMapsGetter corev1client.ConfigMapsGetter, secretsGetter corev1client.SecretsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec, imagePullSpec, operatorImagePullSpec, operatorImageVersion string, configSchedulerLister configlistersv1.SchedulerLister) (*corev1.ConfigMap, bool, error) {
	required := resourceread.ReadPodV1OrDie(bindata.MustAsset("assets/kube-scheduler/pod.yaml"))
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
	featureGateMap := checkForFeatureGates(featureGates)
	sortedFeatureGates := getSortedFeatureGates(featureGateMap)
	allFeatureGates := getFeatureGateString(sortedFeatureGates, featureGateMap)
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

	config, err := configSchedulerLister.Get("cluster")
	if err != nil {
		return nil, false, err
	}

	// there's no need to convert UnsupportedConfigOverrides.Raw from yaml to json as it always comes as json.
	// passing no data to Unmarshal has unexpected behaviour and could be avoided by examining the length of the input.
	// see for more details: https://play.golang.org/p/dsWkiPrzZoL
	var observedConfig map[string]interface{}
	if len(operatorSpec.ObservedConfig.Raw) > 0 {
		if err := json.Unmarshal(operatorSpec.ObservedConfig.Raw, &observedConfig); err != nil {
			return nil, false, fmt.Errorf("failed to unmarshal the observedConfig: %v", err)
		}
	}

	cipherSuites, cipherSuitesFound, err := unstructured.NestedStringSlice(observedConfig, "servingInfo", "cipherSuites")
	if err != nil {
		return nil, false, fmt.Errorf("couldn't get the servingInfo.cipherSuites config from observedConfig: %v", err)
	}

	minTLSVersion, minTLSVersionFound, err := unstructured.NestedString(observedConfig, "servingInfo", "minTLSVersion")
	if err != nil {
		return nil, false, fmt.Errorf("couldn't get the servingInfo.minTLSVersion config from observedConfig: %v", err)
	}

	if cipherSuitesFound && len(cipherSuites) > 0 {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("--tls-cipher-suites=%s", strings.Join(cipherSuites, ",")))
	}

	if minTLSVersionFound && len(minTLSVersion) > 0 {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("--tls-min-version=%s", minTLSVersion))
	}

	// for now only unsupported config is supported
	// it can be easily extended in the future - not that the both configs would have to be merged
	unsupportedArgs, err := getUnsupportedFlagsFromConfig(operatorSpec.UnsupportedConfigOverrides.Raw)
	if err != nil {
		klog.Warningf("failed on getting arguments from UnsupportedConfigOverrides config due to %v", err)
	} else if len(unsupportedArgs) > 0 {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, unsupportedArgs...)
	}

	// Set operator image version. This is currently only used to always deploy the pod during upgrade,
	// causing Progressing=True to be shown as expected.
	for i, container := range required.Spec.Containers {
		if container.Name != "kube-scheduler-recovery-controller" {
			continue
		}
		required.Spec.Containers[i].Env = append(container.Env, corev1.EnvVar{
			Name:  "OPERATOR_IMAGE_VERSION",
			Value: operatorImageVersion,
		})
	}

	configMap := resourceread.ReadConfigMapV1OrDie(bindata.MustAsset("assets/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorSpec.ForceRedeploymentReason
	configMap.Data["version"] = version.Get().String()
	appliedConfigMap, changed, err := resourceapply.ApplyConfigMap(ctx, configMapsGetter, recorder, configMap)
	if changed && len(config.Spec.Policy.Name) > 0 {
		klog.Warning("Setting .spec.policy is no longer supported. Please use .spec.profile instead.")
	}
	return appliedConfigMap, changed, err
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
func checkForFeatureGates(featureGates featuregates.FeatureGate) map[string]bool {
	var featureGateMap = make(map[string]bool)

	for _, featureGate := range featureGates.KnownFeatures() {
		if featureGates.Enabled(featureGate) {
			featureGateMap[string(featureGate)] = true
		} else {
			featureGateMap[string(featureGate)] = false
		}
	}

	return featureGateMap
}

func manageServiceAccountCABundle(ctx context.Context, lister corev1listers.ConfigMapLister, client corev1client.ConfigMapsGetter, recorder events.Recorder) (*corev1.ConfigMap, bool, error) {
	requiredConfigMap, err := resourcesynccontroller.CombineCABundleConfigMaps(
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.TargetNamespace, Name: "serviceaccount-ca"},
		lister,
		certrotation.AdditionalAnnotations{
			JiraComponent: "kube-scheduler",
		},
		// include the ca bundle needed to recognize the server
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "kube-apiserver-server-ca"},
		// include the ca bundle needed to recognize default
		// certificates generated by cluster-ingress-operator
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "default-ingress-cert"},
	)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(ctx, client, recorder, requiredConfigMap)
}

func ensureLocalhostRecoverySAToken(ctx context.Context, client corev1client.CoreV1Interface, recorder events.Recorder) error {
	requiredSA := resourceread.ReadServiceAccountV1OrDie(bindata.MustAsset("assets/kube-scheduler/localhost-recovery-sa.yaml"))
	requiredToken := resourceread.ReadSecretV1OrDie(bindata.MustAsset("assets/kube-scheduler/localhost-recovery-token.yaml"))

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

func manageSchedulerKubeconfig(ctx context.Context, client corev1client.CoreV1Interface, infrastructureLister configlistersv1.InfrastructureLister, recorder events.Recorder) (*corev1.ConfigMap, bool, error) {
	cmString := string(bindata.MustAsset("assets/kube-scheduler/kubeconfig-cm.yaml"))

	infrastructure, err := infrastructureLister.Get("cluster")
	if err != nil {
		return nil, false, err
	}
	apiServerInternalURL := infrastructure.Status.APIServerInternalURL
	if len(apiServerInternalURL) == 0 {
		return nil, false, fmt.Errorf("infrastucture/cluster: missing APIServerInternalURL")
	}

	for pattern, value := range map[string]string{
		"$LB_INT_URL": apiServerInternalURL,
	} {
		cmString = strings.ReplaceAll(cmString, pattern, value)
	}

	requiredCM := resourceread.ReadConfigMapV1OrDie([]byte(cmString))
	return resourceapply.ApplyConfigMap(ctx, client, recorder, requiredCM)
}

// getUnsupportedFlagsFromConfig reads and parses flags stored in the "arguments" filed in the unsupported config
func getUnsupportedFlagsFromConfig(unsupportedConfig []byte) ([]string, error) {
	if len(unsupportedConfig) == 0 {
		return nil, nil
	}
	type unstructuredSchedulerConfig struct {
		Arguments map[string]interface{} `json:"arguments"`
	}

	rawUnsupportedConfig := &unstructuredSchedulerConfig{}
	if err := json.Unmarshal(unsupportedConfig, &rawUnsupportedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the unsupportedConfig: %v", err)
	}

	shellEscapedArgs, err := v1helpers.FlagsFromUnstructured(rawUnsupportedConfig.Arguments)
	if err != nil {
		return nil, err
	}

	return v1helpers.ToFlagSlice(shellEscapedArgs), nil
}
