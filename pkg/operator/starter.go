package operator

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/apis/kubescheduler/v1alpha1"
	operatorconfigclient "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/library-go/pkg/operator/staticpod/staticpodcontroller"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1alpha1helpers"
)

const (
	targetNamespaceName = "openshift-kube-scheduler"
	workQueueKey        = "key"
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
	kubeInformersClusterScoped := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	kubeInformersNamespace := informers.NewFilteredSharedInformerFactory(kubeClient, 10*time.Minute, targetNamespaceName, nil)
	staticPodOperatorClient := &staticPodOperatorClient{
		informers: operatorConfigInformers,
		client:    operatorConfigClient.Kubescheduler(),
	}

	v1alpha1helpers.EnsureOperatorConfigExists(
		dynamicClient,
		v311_00_assets.MustAsset("v3.11.0/kube-scheduler/operator-config.yaml"),
		schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "kubescheduleroperatorconfigs"},
		v1alpha1helpers.GetImageEnv,
	)

	configObserver := NewConfigObserver(
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespace,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient,
		clientConfig,
	)
	targetConfigReconciler := NewTargetConfigReconciler(
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespace,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient,
	)

	deploymentController := staticpodcontroller.NewDeploymentController(
		targetNamespaceName,
		deploymentConfigMaps,
		deploymentSecrets,
		kubeInformersNamespace,
		staticPodOperatorClient,
		kubeClient,
	)
	installerController := staticpodcontroller.NewInstallerController(
		targetNamespaceName,
		deploymentConfigMaps,
		deploymentSecrets,
		[]string{"cluster-kube-scheduler-operator", "installer"},
		kubeInformersNamespace,
		staticPodOperatorClient,
		kubeClient,
	)
	nodeController := staticpodcontroller.NewNodeController(
		staticPodOperatorClient,
		kubeInformersClusterScoped,
	)
	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"openshift-kube-scheduler",
		"openshift-kube-scheduler",
		dynamicClient,
		&operatorStatusProvider{informers: operatorConfigInformers},
	)

	operatorConfigInformers.Start(stopCh)
	kubeInformersClusterScoped.Start(stopCh)
	kubeInformersNamespace.Start(stopCh)

	go targetConfigReconciler.Run(1, stopCh)
	go deploymentController.Run(1, stopCh)
	go installerController.Run(1, stopCh)
	go nodeController.Run(1, stopCh)
	go configObserver.Run(1, stopCh)
	go clusterOperatorStatus.Run(1, stopCh)

	<-stopCh
	return fmt.Errorf("stopped")
}

// deploymentConfigMaps is a list of configmaps that are directly copied for the current values.  A different actor/controller modifies these.
// the first element should be the configmap that contains the static pod manifest
var deploymentConfigMaps = []string{
	"kube-scheduler-pod",
	"deployment-kube-scheduler-config",
	"client-ca",
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []string{
	"serving-cert",
}
