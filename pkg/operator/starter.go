package operator

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/apis/kubescheduler/v1alpha1"
	operatorconfigclient "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1alpha1helpers"
)

func RunOperator(clientConfig *rest.Config, stopCh <-chan struct{}) error {
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	operatorConfigClient, err := operatorconfigclient.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)
	kubeInformersNamespaced := informers.NewFilteredSharedInformerFactory(kubeClient, 10*time.Minute, targetNamespaceName, nil)

	v1alpha1helpers.EnsureOperatorConfigExists(
		dynamicClient,
		v311_00_assets.MustAsset("v3.11.0/kube-scheduler/operator-config.yaml"),
		schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "kubescheduleroperatorconfigs"},
		v1alpha1helpers.GetImageEnv,
	)

	operator := NewKubeSchedulerOperator(
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespaced,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient.AppsV1(),
		kubeClient.CoreV1(),
		kubeClient.RbacV1(),
	)

	configObserver := NewConfigObserver(
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespaced,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient,
		clientConfig,
	)

	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"openshift-kube-scheduler",
		"openshift-kube-scheduler",
		dynamicClient,
		&operatorStatusProvider{informers: operatorConfigInformers},
	)

	operatorConfigInformers.Start(stopCh)
	kubeInformersNamespaced.Start(stopCh)

	go operator.Run(1, stopCh)
	go configObserver.Run(1, stopCh)
	go clusterOperatorStatus.Run(1, stopCh)

	<-stopCh
	return fmt.Errorf("stopped")
}

type operatorStatusProvider struct {
	informers operatorclientinformers.SharedInformerFactory
}

func (p *operatorStatusProvider) Informer() cache.SharedIndexInformer {
	return p.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Informer()
}

func (p *operatorStatusProvider) CurrentStatus() (operatorv1alpha1.OperatorStatus, error) {
	instance, err := p.informers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs().Lister().Get("instance")
	if err != nil {
		return operatorv1alpha1.OperatorStatus{}, err
	}

	return instance.Status.OperatorStatus, nil
}
