package operator

import (
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/cluster-kube-scheduler-operator/pkg/apis/kubescheduler/v1alpha1"
	operatorconfigclient "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/cluster-kube-scheduler-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/configobservercontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"

	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/staticpod"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1alpha1helpers"
)

const (
	targetNamespaceName = "openshift-kube-scheduler"
	workQueueKey        = "key"
)

func RunOperator(ctx *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}
	operatorConfigClient, err := operatorconfigclient.NewForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}

	configClient, err := configv1client.NewForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}

	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)
	kubeInformersClusterScoped := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	kubeInformersNamespace := informers.NewFilteredSharedInformerFactory(kubeClient, 10*time.Minute, targetNamespaceName, nil)
	staticPodOperatorClient := &staticPodOperatorClient{
		informers: operatorConfigInformers,
		client:    operatorConfigClient.KubeschedulerV1alpha1(),
	}

	v1alpha1helpers.EnsureOperatorConfigExists(
		dynamicClient,
		v311_00_assets.MustAsset("v3.11.0/kube-scheduler/operator-config.yaml"),
		schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "kubescheduleroperatorconfigs"},
		v1alpha1helpers.GetImageEnv,
	)

	configObserver := configobservercontroller.NewConfigObserver(
		staticPodOperatorClient,
		operatorConfigInformers,
		kubeInformersNamespace,
		ctx.EventRecorder,
	)

	targetConfigReconciler := NewTargetConfigReconciler(
		os.Getenv("IMAGE"),
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespace,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient,
		ctx.EventRecorder,
	)

	staticPodControllers := staticpod.NewControllers(
		targetNamespaceName,
		"openshift-kube-scheduler",
		[]string{"cluster-kube-scheduler-operator", "installer"},
		deploymentConfigMaps,
		deploymentSecrets,
		staticPodOperatorClient,
		kubeClient,
		kubeInformersNamespace,
		kubeInformersClusterScoped,
		ctx.EventRecorder,
	)

	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"openshift-kube-scheduler-operator",
		configClient.ConfigV1(),
		staticPodOperatorClient,
		ctx.EventRecorder,
	)

	operatorConfigInformers.Start(ctx.StopCh)
	kubeInformersClusterScoped.Start(ctx.StopCh)
	kubeInformersNamespace.Start(ctx.StopCh)

	go staticPodControllers.Run(ctx.StopCh)
	go targetConfigReconciler.Run(1, ctx.StopCh)
	go configObserver.Run(1, ctx.StopCh)
	go clusterOperatorStatus.Run(1, ctx.StopCh)

	<-ctx.StopCh
	return fmt.Errorf("stopped")
}

// deploymentConfigMaps is a list of configmaps that are directly copied for the current values.  A different actor/controller modifies these.
// the first element should be the configmap that contains the static pod manifest
var deploymentConfigMaps = []string{
	"kube-scheduler-pod",
	"config",
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []string{
	"scheduler-kubeconfig",
}
