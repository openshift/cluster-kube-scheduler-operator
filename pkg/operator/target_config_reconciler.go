package operator

import (
	"fmt"
	"time"

	"k8s.io/klog"

	operatorv1 "github.com/openshift/api/operator/v1"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	corev1 "k8s.io/api/core/v1"
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
)

type TargetConfigReconciler struct {
	targetImagePullSpec  string
	operatorConfigClient operatorv1client.KubeSchedulersGetter
	operatorClient       v1helpers.StaticPodOperatorClient
	kubeClient           kubernetes.Interface
	eventRecorder        events.Recorder
	configMapLister      corev1listers.ConfigMapLister
	configMapsGetter     corev1client.ConfigMapsGetter
	secretMapsGetter     corev1client.SecretsGetter
	featureGateLister    configlistersv1.FeatureGateLister
	featureGateCacheSync cache.InformerSynced
	// queue only ever has one item, but it has nice error handling backoff/retry semantics
	queue workqueue.RateLimitingInterface
}

func NewTargetConfigReconciler(
	operatorConfigClient v1helpers.OperatorClient,
	targetImagePullSpec string,
	namespacedKubeInformers informers.SharedInformerFactory,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	configInformer configinformers.SharedInformerFactory,
	legacyOperatorConfigClient operatorv1client.KubeSchedulersGetter,
	operatorClient v1helpers.StaticPodOperatorClient,
	kubeClient kubernetes.Interface,
	eventRecorder events.Recorder,
) *TargetConfigReconciler {
	c := &TargetConfigReconciler{
		targetImagePullSpec:  targetImagePullSpec,
		operatorConfigClient: legacyOperatorConfigClient,
		kubeClient:           kubeClient,
		configMapLister:      kubeInformersForNamespaces.ConfigMapLister(),
		configMapsGetter:     v1helpers.CachedConfigMapGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		secretMapsGetter:     v1helpers.CachedSecretGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		operatorClient:       operatorClient,
		eventRecorder:        eventRecorder,
		featureGateLister:    configInformer.Config().V1().FeatureGates().Lister(),
		featureGateCacheSync: configInformer.Config().V1().FeatureGates().Informer().HasSynced,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TargetConfigReconciler"),
	}

	operatorConfigClient.Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Rbac().V1().Roles().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Rbac().V1().RoleBindings().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().Secrets().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().ServiceAccounts().Informer().AddEventHandler(c.eventHandler())
	namespacedKubeInformers.Core().V1().Services().Informer().AddEventHandler(c.eventHandler())

	configInformer.Config().V1().FeatureGates().Informer().AddEventHandler(c.eventHandler())
	// we react to some config changes
	kubeInformersForNamespaces.InformersFor(operatorclient.GlobalUserSpecifiedConfigNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace).Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())

	// we only watch some namespaces
	namespacedKubeInformers.Core().V1().Namespaces().Informer().AddEventHandler(c.namespaceEventHandler())

	return c
}

func (c TargetConfigReconciler) sync() error {
	operatorConfig, err := c.operatorConfigClient.KubeSchedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		return err
	}

	switch operatorConfig.Spec.ManagementState {
	case operatorv1.Managed:
	case operatorv1.Unmanaged:
		return nil

	case operatorv1.Removed:
		// TODO probably just fail
		return nil
	default:
		c.eventRecorder.Warningf("ManagementStateUnknown", "Unrecognized operator management state %q", operatorConfig.Spec.ManagementState)
		return nil
	}
	requeue, err := createTargetConfigReconciler_v311_00_to_latest(c, c.eventRecorder, operatorConfig)
	if err != nil {
		return err
	}
	if requeue {
		return fmt.Errorf("synthetic requeue request")
	}

	return nil
}

// Run starts the kube-scheduler and blocks until stopCh is closed.
func (c *TargetConfigReconciler) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting TargetConfigReconciler")
	defer klog.Infof("Shutting down TargetConfigReconciler")

	if !cache.WaitForCacheSync(stopCh, c.featureGateCacheSync) {
		utilruntime.HandleError(fmt.Errorf("caches did not sync"))
		return
	}

	// doesn't matter what workers say, only start one.
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *TargetConfigReconciler) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *TargetConfigReconciler) processNextWorkItem() bool {
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
func (c *TargetConfigReconciler) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(workQueueKey) },
	}
}

// this set of namespaces will include things like logging and metrics which are used to drive
var interestingNamespaces = sets.NewString(operatorclient.TargetNamespace)

func (c *TargetConfigReconciler) namespaceEventHandler() cache.ResourceEventHandler {
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
