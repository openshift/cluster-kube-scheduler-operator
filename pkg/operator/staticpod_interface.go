package operator

import (
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	operatorconfigclientv1alpha1 "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/clientset/versioned/typed/kubescheduler/v1alpha1"
	operatorclientinformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"k8s.io/client-go/tools/cache"
)

type staticPodOperatorClient struct {
	informers operatorclientinformers.SharedInformerFactory
	client    operatorconfigclientv1alpha1.KubeschedulerV1alpha1Interface
}

func (c *staticPodOperatorClient) Informer() cache.SharedIndexInformer {
	return c.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Informer()
}

func (c *staticPodOperatorClient) Get() (*operatorv1alpha1.OperatorSpec, *operatorv1alpha1.StaticPodOperatorStatus, string, error) {
	instance, err := c.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Lister().Get("instance")
	if err != nil {
		return nil, nil, "", err
	}

	return &instance.Spec.OperatorSpec, &instance.Status.StaticPodOperatorStatus, instance.ResourceVersion, nil
}

func (c *staticPodOperatorClient) UpdateStatus(resourceVersion string, status *operatorv1alpha1.StaticPodOperatorStatus) (*operatorv1alpha1.StaticPodOperatorStatus, error) {
	original, err := c.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Lister().Get("instance")
	if err != nil {
		return nil, err
	}
	copy := original.DeepCopy()
	copy.ResourceVersion = resourceVersion
	copy.Status.StaticPodOperatorStatus = *status

	ret, err := c.client.KubeSchedulerOperatorConfigs().UpdateStatus(copy)
	if err != nil {
		return nil, err
	}

	return &ret.Status.StaticPodOperatorStatus, nil
}

// TODO collapse this onto get
func (c *staticPodOperatorClient) CurrentStatus() (operatorv1alpha1.OperatorStatus, error) {
	instance, err := c.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Lister().Get("instance")
	if err != nil {
		return operatorv1alpha1.OperatorStatus{}, err
	}

	return instance.Status.OperatorStatus, nil
}
