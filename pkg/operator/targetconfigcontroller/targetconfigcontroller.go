package targetconfigcontroller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v410_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

const (
	workQueueKey              = "key"
	TargetPolicyConfigMapName = "policy-configmap"
)

type TargetConfigController struct {
	ctx                   context.Context
	targetImagePullSpec   string
	operatorImagePullSpec string
	operatorClient        v1helpers.StaticPodOperatorClient
	kubeClient            kubernetes.Interface
	eventRecorder         events.Recorder
	configMapLister       corev1listers.ConfigMapLister
	featureGateLister     configlistersv1.FeatureGateLister
	featureGateCacheSync  cache.InformerSynced
	// queue only ever has one item, but it has nice error handling backoff/retry semantics
	queue workqueue.RateLimitingInterface
}

func NewTargetConfigController(
	ctx context.Context,
	targetImagePullSpec, operatorImagePullSpec string,
	operatorConfigClient v1helpers.OperatorClient,
	namespacedKubeInformers informers.SharedInformerFactory,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	operatorClient v1helpers.StaticPodOperatorClient,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder,
) *TargetConfigController {
	c := &TargetConfigController{
		ctx:                   ctx,
		targetImagePullSpec:   targetImagePullSpec,
		operatorImagePullSpec: operatorImagePullSpec,
		kubeClient:            kubeClient,
		configMapLister:       kubeInformersForNamespaces.ConfigMapLister(),
		operatorClient:        operatorClient,
		eventRecorder:         eventRecorder,

		featureGateLister:    configInformer.Config().V1().FeatureGates().Lister(),
		featureGateCacheSync: configInformer.Config().V1().FeatureGates().Informer().HasSynced,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TargetConfigController"),
	}

	operatorConfigClient.Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().Secrets().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ServiceAccounts().Informer().AddEventHandler(c.eventHandler())

	configInformer.Config().V1().FeatureGates().Informer().AddEventHandler(c.eventHandler())
	// we react to some config changes
	kubeInformersForNamespaces.InformersFor(operatorclient.GlobalUserSpecifiedConfigNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())

	// we only watch some namespaces
	namespacedKubeInformers.Core().V1().Namespaces().Informer().AddEventHandler(c.namespaceEventHandler())

	return c
}

func (c TargetConfigController) sync() error {
	operatorSpec, _, _, err := c.operatorClient.GetStaticPodOperatorState()
	if err != nil {
		return err
	}

	switch operatorSpec.ManagementState {
	case operatorv1.Managed:
	case operatorv1.Unmanaged:
		return nil

	case operatorv1.Removed:
		// TODO probably just fail
		return nil
	default:
		c.eventRecorder.Warningf("ManagementStateUnknown", "Unrecognized operator management state %q", operatorSpec.ManagementState)
		return nil
	}
	requeue, err := createTargetConfigController_v311_00_to_latest(c, c.eventRecorder, operatorSpec)
	if err != nil {
		return err
	}
	if requeue {
		return fmt.Errorf("synthetic requeue request")
	}

	return nil
}

// Run starts the kube-scheduler and blocks until stopCh is closed.
func (c *TargetConfigController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting TargetConfigController")
	defer klog.Infof("Shutting down TargetConfigController")

	if !cache.WaitForCacheSync(stopCh, c.featureGateCacheSync) {
		utilruntime.HandleError(fmt.Errorf("caches did not sync"))
		return
	}

	// doesn't matter what workers say, only start one.
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *TargetConfigController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *TargetConfigController) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)

	err := c.sync()
	if err == nil {
		c.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	c.queue.AddRateLimited(dsKey)

	return true
}

// eventHandler queues the operator to check spec and status
func (c *TargetConfigController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(workQueueKey) },
	}
}

// this set of namespaces will include things like logging and metrics which are used to drive
var interestingNamespaces = sets.NewString(operatorclient.TargetNamespace)

func (c *TargetConfigController) namespaceEventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ns, ok := obj.(*corev1.Namespace)
			if !ok {
				c.queue.Add(workQueueKey)
			}
			if ns.Name == operatorclient.TargetNamespace {
				c.queue.Add(workQueueKey)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			ns, ok := old.(*corev1.Namespace)
			if !ok {
				c.queue.Add(workQueueKey)
			}
			if ns.Name == operatorclient.TargetNamespace {
				c.queue.Add(workQueueKey)
			}
		},
		DeleteFunc: func(obj interface{}) {
			ns, ok := obj.(*corev1.Namespace)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
					return
				}
				ns, ok = tombstone.Obj.(*corev1.Namespace)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Namespace %#v", obj))
					return
				}
			}
			if ns.Name == operatorclient.TargetNamespace {
				c.queue.Add(workQueueKey)
			}
		},
	}
}

// createTargetConfigController_v311_00_to_latest takes care of synchronizing (not upgrading) the thing we're managing.
// most of the time the sync method will be good for a large span of minor versions
func createTargetConfigController_v311_00_to_latest(c TargetConfigController, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec) (bool, error) {
	errors := []error{}

	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(c.configMapLister, c.kubeClient.CoreV1(), recorder, operatorSpec)
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

	condition := operatorv1.OperatorCondition{
		Type:   "TargetConfigControllerDegraded",
		Status: operatorv1.ConditionFalse,
	}
	if _, _, err := v1helpers.UpdateStaticPodStatus(c.operatorClient, v1helpers.UpdateStaticPodConditionFn(condition)); err != nil {
		return true, err
	}

	return false, nil
}

func manageKubeSchedulerConfigMap_v311_00_to_latest(lister corev1listers.ConfigMapLister, client corev1client.ConfigMapsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec) (*corev1.ConfigMap, bool, error) {
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

	// In order to transition from v1alpha1 componentconfig to v1beta1, we need a bridge hack, here.
	// (This is because v1alpha1 and v1beta1 are not available in the same version, so we can't simply transition)
	// This reads the componentconfig info and turns it into the corresponding kube-scheduler args
	// TODO(@damemi): This should only exist to land the 1.19 rebase
	schedulerComponentConfig, err := configMapsGetter.ConfigMaps(operatorclient.TargetNamespace).Get(ctx, "config", metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}
	flags := getFlagsFromComponentConfig(schedulerComponentConfig)
	required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, flags...)

	configMap := resourceread.ReadConfigMapV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/pod-cm.yaml"))
	configMap.Data["pod.yaml"] = resourceread.WritePodV1OrDie(required)
	configMap.Data["forceRedeploymentReason"] = operatorSpec.ForceRedeploymentReason
	configMap.Data["version"] = version.Get().String()
	return resourceapply.ApplyConfigMap(configMapsGetter, recorder, configMap)
}

// getFlagsFromComponentConfig turns CC args into their matching (though possibly deprecated) kube-scheduler args
// Because v1alpha1 and v1beta1 aren't available in the same version, we can't parse the fields automatically and
// we need to manually go through each possible config option and add a flag if necessary.
// This goes one-by-one through every possible CC field found in KubeSchedulerConfiguration: https://github.com/kubernetes/kube-scheduler/blob/release-1.18/config/v1alpha1/types.go
// TODO(@damemi): Remove this immediately.
func getFlagsFromComponentConfig(cm *corev1.ConfigMap) []string {
	args := make([]string, 0)

	data := cm.Data["config.yaml"]
	existingConfig := map[string]interface{}{}
	if err := json.NewDecoder(bytes.NewBuffer([]byte(data))).Decode(&existingConfig); err != nil {
		klog.V(4).Infof("decode of existing config failed with error: %v", err)
	}

	// Get SchedulerName
	if schedulerName, ok, err := unstructured.NestedString(existingConfig, "schedulerName"); ok && err == nil {
		args = append(args, fmt.Sprintf("--scheduler-name=%s", schedulerName))
	}

	// Get Policy config args
	// We only allow configmap policy source, and it is always synced to the same location in the TargetNamespace
	// So if algorithmSource is set, we know that we have the same configmap. See observe_scheduler.go
	if _, ok, err := unstructured.NestedStringMap(existingConfig, "algorithmSource"); ok && err == nil {
		args = append(args, "--policy-configmap=policy-configmap")
		args = append(args, fmt.Sprintf("--policy-configmap-namespace=%s", operatorclient.TargetNamespace))
	}

	// Get HardPodAffinitySymmetricWeight
	if hardPodAffinitySymmetricWeight, ok, err := unstructured.NestedString(existingConfig, "hardPodAffinitySymmetricWeight"); ok && err == nil {
		args = append(args, fmt.Sprintf("--hard-pod-affinity-symmetric-weight=%s", hardPodAffinitySymmetricWeight))
	}

	// Get leaderelection args
	if leaderElection, ok, err := unstructured.NestedStringMap(existingConfig, "leaderElection"); ok && err == nil {
		if leaderElect, ok := leaderElection["leaderElect"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect=%s", leaderElect))
		}
		if leaseDuration, ok := leaderElection["leaseDuration"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-lease-duration=%s", leaseDuration))
		}
		if renewDeadline, ok := leaderElection["renewDeadline"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-renew-deadline=%s", renewDeadline))
		}
		if retryPeriod, ok := leaderElection["retryPeriod"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-retry-period=%s", retryPeriod))
		}
		if resourceLock, ok := leaderElection["resourceLock"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-resource-lock=%s", resourceLock))
		}

		// lockObjectNamespace and lockObjectName are replaced in v1beta1 by ResourceName and ResourceNamespace
		if lockObjectNamespace, ok := leaderElection["lockObjectNamespace"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-resource-namespace=%s", lockObjectNamespace))
		}
		if lockObjectName, ok := leaderElection["lockObjectName"]; ok {
			args = append(args, fmt.Sprintf("--leader-elect-resource-name=%s", lockObjectName))
		}
	}

	// Get clientconnection args
	if clientConnection, ok, err := unstructured.NestedStringMap(existingConfig, "clientConnection"); ok && err == nil {
		if kubeconfig, ok := clientConnection["kubeconfig"]; ok {
			args = append(args, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
		}
		if contentType, ok := clientConnection["contentType"]; ok {
			args = append(args, fmt.Sprintf("--kube-api-content-type=%s", contentType))
		}
		if qps, ok := clientConnection["qps"]; ok {
			args = append(args, fmt.Sprintf("--kube-api-qps=%s", qps))
		}
		if burst, ok := clientConnection["burst"]; ok {
			args = append(args, fmt.Sprintf("--kube-api-burst=%s", burst))
		}
		// TODO: Missing AcceptContentTypes flag
	}

	// Get healthzbindaddress
	// TODO?

	// Get metricsbindaddress
	// TODO?

	// Get debugging config args
	if enableProfiling, ok, err := unstructured.NestedString(existingConfig, "enableProfiling"); ok && err == nil {
		args = append(args, fmt.Sprintf("--profiling=%s", enableProfiling))
	}
	if enableContentionProfiling, ok, err := unstructured.NestedString(existingConfig, "enableContentionProfiling"); ok && err == nil {
		args = append(args, fmt.Sprintf("--contention-profiling=%s", enableContentionProfiling))
	}

	// Get DisablePreemption
	// TODO?

	// Get PercentageOfNodesToScore
	// TODO?

	// Get BindTimeoutSeconds
	// TODO?

	// Get PodInitialBackoffSeconds
	// TODO?

	// Get PodMaxBackoffSeconds
	// TODO?

	return args
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
		resourcesynccontroller.ResourceLocation{Namespace: operatorclient.GlobalMachineSpecifiedConfigNamespace, Name: "default-ingress-cert"},
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
