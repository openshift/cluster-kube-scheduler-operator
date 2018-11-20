package operator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/imdario/mergo"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corelistersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorconfigclientv1alpha1 "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/clientset/versioned/typed/kubescheduler/v1alpha1"
	operatorconfiginformerv1alpha1 "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions/kubescheduler/v1alpha1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

const configObservationErrorConditionReason = "ConfigObservationError"

// Not used
type Listers struct {
	configmapLister corelistersv1.ConfigMapLister
}

// Not used
// observeConfigFunc observes configuration and returns the observedConfig. This function should not return an
// observedConfig that would cause the service being managed by the operator to crash. For example, if a required
// configuration key cannot be observed, consider reusing the configuration key's previous value. Errors that occur
// while attempting to generate the observedConfig should be returned in the errs slice.
type observeConfigFunc func(listers Listers, existingConfig map[string]interface{}) (observedConfig map[string]interface{}, errs []error)

type ConfigObserver struct {
	operatorConfigClient operatorconfigclientv1alpha1.KubeschedulerV1alpha1Interface

	// queue only ever has one item, but it has nice error handling backoff/retry semantics
	queue workqueue.RateLimitingInterface

	// observers are called in an undefined order and their results are merged to
	// determine the observed configuration.
	// Not used
	observers []observeConfigFunc

	rateLimiter flowcontrol.RateLimiter
	// Not used
	listers Listers
	// Not used
	cachesSynced []cache.InformerSynced
}

func NewConfigObserver(
	operatorConfigInformer operatorconfiginformerv1alpha1.KubeSchedulerOperatorConfigInformer,
	kubeInformersForOpenShiftKubeSchedulerNamespace informers.SharedInformerFactory,
	operatorConfigClient operatorconfigclientv1alpha1.KubeschedulerV1alpha1Interface,
) *ConfigObserver {
	c := &ConfigObserver{
		operatorConfigClient: operatorConfigClient,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ConfigObserver"),
		rateLimiter:          flowcontrol.NewTokenBucketRateLimiter(0.05 /*3 per minute*/, 4),
		observers:            []observeConfigFunc{},
		listers:              Listers{},
		cachesSynced:         []cache.InformerSynced{},
	}

	operatorConfigInformer.Informer().AddEventHandler(c.eventHandler())
	kubeInformersForOpenShiftKubeSchedulerNamespace.Core().V1().ConfigMaps().Informer().AddEventHandler(c.eventHandler())
	return c
}

// sync reacts to a change in scheduler images.
func (c ConfigObserver) sync() error {
	operatorConfig, err := c.operatorConfigClient.KubeSchedulerOperatorConfigs().Get("instance", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// don't worry about errors
	currentConfig := map[string]interface{}{}
	json.NewDecoder(bytes.NewBuffer(operatorConfig.Spec.ObservedConfig.Raw)).Decode(&currentConfig)

	var (
		errs            []error
		observedConfigs []map[string]interface{}
	)
	for _, i := range rand.Perm(len(c.observers)) {
		observedConfig, currErrs := c.observers[i](c.listers, currentConfig)
		observedConfigs = append(observedConfigs, observedConfig)
		errs = append(errs, currErrs...)
	}

	mergedObservedConfig := map[string]interface{}{}
	for _, observedConfig := range observedConfigs {
		mergo.Merge(&mergedObservedConfig, observedConfig)
	}

	if !equality.Semantic.DeepEqual(currentConfig, mergedObservedConfig) {
		glog.Infof("writing updated observedConfig: %v", diff.ObjectDiff(operatorConfig.Spec.ObservedConfig.Object, mergedObservedConfig))
		operatorConfig.Spec.ObservedConfig = runtime.RawExtension{Object: &unstructured.Unstructured{Object: mergedObservedConfig}}
		updatedOperatorConfig, err := c.operatorConfigClient.KubeSchedulerOperatorConfigs().Update(operatorConfig)
		if err != nil {
			errs = append(errs, fmt.Errorf("kubescheduleroperatorconfigs/instance: error writing updated observed config: %v", err))
		} else {
			operatorConfig = updatedOperatorConfig
		}
	}

	status := operatorConfig.Status.DeepCopy()
	if len(errs) > 0 {
		var messages []string
		for _, currentError := range errs {
			messages = append(messages, currentError.Error())
		}
		v1helpers.SetOperatorCondition(&status.Conditions, operatorv1.OperatorCondition{
			Type:    operatorv1.OperatorStatusTypeFailing,
			Status:  operatorv1.ConditionTrue,
			Reason:  configObservationErrorConditionReason,
			Message: strings.Join(messages, "\n"),
		})
	} else {
		condition := v1helpers.FindOperatorCondition(status.Conditions, operatorv1.OperatorStatusTypeFailing)
		if condition != nil && condition.Status != operatorv1.ConditionFalse && condition.Reason == configObservationErrorConditionReason {
			condition.Status = operatorv1.ConditionFalse
			condition.Reason = ""
			condition.Message = ""
		}
	}

	if !equality.Semantic.DeepEqual(operatorConfig.Status, status) {
		operatorConfig.Status = *status
		_, err = c.operatorConfigClient.KubeSchedulerOperatorConfigs().UpdateStatus(operatorConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *ConfigObserver) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	glog.Infof("Starting ConfigObserver")
	defer glog.Infof("Shutting down ConfigObserver")

	// doesn't matter what workers say, only start one.
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *ConfigObserver) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *ConfigObserver) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)

	// before we call sync, we want to wait for token.  We do this to avoid hot looping.
	c.rateLimiter.Accept()

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
func (c *ConfigObserver) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(workQueueKey) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(workQueueKey) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(workQueueKey) },
	}
}
