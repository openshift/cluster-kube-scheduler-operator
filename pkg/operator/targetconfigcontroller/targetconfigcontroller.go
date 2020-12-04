package targetconfigcontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v410_00_assets"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/version"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/resourcesynccontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
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
	"k8s.io/klog/v2"
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
	infrastuctureLister   configv1listers.InfrastructureLister
	featureGateLister     configlistersv1.FeatureGateLister
	configInformer        configinformers.SharedInformerFactory
	cachesSync            []cache.InformerSynced
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
		infrastuctureLister:   configInformer.Config().V1().Infrastructures().Lister(),
		operatorClient:        operatorClient,
		eventRecorder:         eventRecorder,
		configInformer:        configInformer,

		featureGateLister: configInformer.Config().V1().FeatureGates().Lister(),
		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TargetConfigController"),
	}

	operatorConfigClient.Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().Secrets().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ServiceAccounts().Informer().AddEventHandler(c.eventHandler())

	// We use infrastuctureInformer for observing load balancer URL
	configInformer.Config().V1().Infrastructures().Informer().AddEventHandler(c.eventHandler())
	c.cachesSync = append(c.cachesSync, configInformer.Config().V1().Infrastructures().Informer().HasSynced)

	configInformer.Config().V1().FeatureGates().Informer().AddEventHandler(c.eventHandler())
	c.cachesSync = append(c.cachesSync, configInformer.Config().V1().FeatureGates().Informer().HasSynced)
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

	if !cache.WaitForCacheSync(stopCh, c.cachesSync...) {
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

	_, _, err := manageKubeSchedulerConfigMap_v311_00_to_latest(c.configMapLister, c.kubeClient.CoreV1(), recorder, operatorSpec, c.configInformer)
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
	_, _, err = manageSchedulerKubeconfig(c.ctx, c.kubeClient.CoreV1(), c.infrastuctureLister, recorder)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap/scheduler-kubeconfig", err))
	}
	_, _, err = managePod_v311_00_to_latest(c.ctx, c.kubeClient.CoreV1(), c.kubeClient.CoreV1(), recorder, operatorSpec, c.targetImagePullSpec, c.operatorImagePullSpec, c.featureGateLister, c.configInformer)
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

func manageKubeSchedulerConfigMap_v311_00_to_latest(lister corev1listers.ConfigMapLister, client corev1client.ConfigMapsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec, configInformer configinformers.SharedInformerFactory) (*corev1.ConfigMap, bool, error) {
	configMap := resourceread.ReadConfigMapV1OrDie(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/cm.yaml"))

	var kubeSchedulerConfiguration []byte

	config, err := configInformer.Config().V1().Schedulers().Lister().Get("cluster")
	if err != nil {
		return nil, false, err
	}
	// TOOD(jchaloup): remove the condition once the policy API is removed from the code
	// Until that, ignore profiles if the policy API is used.
	if len(config.Spec.Policy.Name) > 0 {
		kubeSchedulerConfiguration = v410_00_assets.MustAsset("v4.1.0/config/defaultconfig-postbootstrap-lownodeutilization.yaml")
		klog.Warningf("Setting .spec.policy is deprecated and will be removed eventually. Please use .spec.profile instead.")
	} else {
		switch config.Spec.Profile {
		case v1.LowNodeUtilization, "":
			kubeSchedulerConfiguration = v410_00_assets.MustAsset("v4.1.0/config/defaultconfig-postbootstrap-lownodeutilization.yaml")
		case v1.HighNodeUtililzation:
			kubeSchedulerConfiguration = v410_00_assets.MustAsset("v4.1.0/config/defaultconfig-postbootstrap-highnodeutilization.yaml")
		case v1.NoScoring:
			kubeSchedulerConfiguration = v410_00_assets.MustAsset("v4.1.0/config/defaultconfig-postbootstrap-noscoring.yaml")
		default:
			return nil, false, fmt.Errorf("profile %q not recognized", config.Spec.Profile)
		}
	}

	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, kubeSchedulerConfiguration, operatorSpec.ObservedConfig.Raw, operatorSpec.UnsupportedConfigOverrides.Raw)
	if err != nil {
		return nil, false, err
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}

func managePod_v311_00_to_latest(ctx context.Context, configMapsGetter corev1client.ConfigMapsGetter, secretsGetter corev1client.SecretsGetter, recorder events.Recorder, operatorSpec *operatorv1.StaticPodOperatorSpec, imagePullSpec, operatorImagePullSpec string, featureGateLister configlistersv1.FeatureGateLister, configInformer configinformers.SharedInformerFactory) (*corev1.ConfigMap, bool, error) {
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

	config, err := configInformer.Config().V1().Schedulers().Lister().Get("cluster")
	if err != nil {
		return nil, false, err
	}
	if len(config.Spec.Policy.Name) > 0 {
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, "--policy-configmap=policy-configmap")
		required.Spec.Containers[0].Args = append(required.Spec.Containers[0].Args, fmt.Sprintf("--policy-configmap-namespace=%s", operatorclient.TargetNamespace))
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

	if featureGateListConfig.Spec.FeatureSet == v1.CustomNoUpgrade {
		if featureGateListConfig.Spec.FeatureGateSelection.CustomNoUpgrade != nil {
			enabledFeatureSets = featureGateListConfig.Spec.FeatureGateSelection.CustomNoUpgrade.Enabled
			disabledFeatureSets = featureGateListConfig.Spec.FeatureGateSelection.CustomNoUpgrade.Disabled
		}
	} else if featureSet, ok := v1.FeatureSets[featureGateListConfig.Spec.FeatureSet]; ok {
		enabledFeatureSets = featureSet.Enabled
		disabledFeatureSets = featureSet.Disabled
	} else {
		klog.Infof("Invalid feature set config found in features.config.openshift.io/cluster %v. Please look at allowed features", featureGateListConfig.Spec.FeatureSet)
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

func manageSchedulerKubeconfig(ctx context.Context, client corev1client.CoreV1Interface, infrastructureLister configv1listers.InfrastructureLister, recorder events.Recorder) (*corev1.ConfigMap, bool, error) {
	cmString := string(v410_00_assets.MustAsset("v4.1.0/kube-scheduler/kubeconfig-cm.yaml"))

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
	return resourceapply.ApplyConfigMap(client, recorder, requiredCM)
}
