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

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/configobservercontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/resourcesynccontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/staticpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/controller/revision"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
)

const (
	workQueueKey = "key"
)

func RunOperator(ctx *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(ctx.ProtoKubeConfig)
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
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		operatorclient.GlobalUserSpecifiedConfigNamespace,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.OperatorNamespace,
		operatorclient.TargetNamespace,
		"kube-system",
	)
	operatorClient := &operatorclient.OperatorClient{
		Informers: operatorConfigInformers,
		Client:    operatorConfigClient.KubeschedulerV1alpha1(),
	}

	kubeInformersClusterScoped := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	kubeInformersNamespace := informers.NewSharedInformerFactoryWithOptions(kubeClient, 10*time.Minute, informers.WithNamespace(operatorclient.TargetNamespace))

	v1helpers.EnsureOperatorConfigExists(
		dynamicClient,
		v311_00_assets.MustAsset("v3.11.0/kube-scheduler/operator-config.yaml"),
		schema.GroupVersionResource{Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "kubescheduleroperatorconfigs"},
	)

	resourceSyncController, err := resourcesynccontroller.NewResourceSyncController(
		operatorClient,
		kubeInformersForNamespaces,
		kubeClient,
		ctx.EventRecorder,
	)
	if err != nil {
		return err
	}

	configObserver := configobservercontroller.NewConfigObserver(
		operatorClient,
		operatorConfigInformers,
		kubeInformersNamespace,
		resourceSyncController,
		ctx.EventRecorder,
	)

	targetConfigReconciler := NewTargetConfigReconciler(
		os.Getenv("IMAGE"),
		operatorConfigInformers.Kubescheduler().V1alpha1().KubeSchedulerOperatorConfigs(),
		kubeInformersNamespace,
		kubeInformersForNamespaces,
		operatorConfigClient.KubeschedulerV1alpha1(),
		kubeClient,
		ctx.EventRecorder,
	)

	staticPodControllers := staticpod.NewControllers(
		operatorclient.TargetNamespace,
		"openshift-kube-scheduler",
		"kube-scheduler-pod",
		[]string{"cluster-kube-scheduler-operator", "installer"},
		[]string{"cluster-kube-scheduler-operator", "prune"},
		deploymentConfigMaps,
		deploymentSecrets,
		operatorClient,
		v1helpers.CachedConfigMapGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		v1helpers.CachedSecretGetter(kubeClient.CoreV1(), kubeInformersForNamespaces),
		kubeClient.CoreV1(),
		kubeClient,
		dynamicClient,
		kubeInformersNamespace,
		kubeInformersClusterScoped,
		ctx.EventRecorder,
	)

	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"kube-scheduler",
		[]configv1.ObjectReference{
			{Group: "kubescheduler.operator.openshift.io", Resource: "kubescheduleroperatorconfigs", Name: "instance"},
			{Resource: "namespaces", Name: operatorclient.GlobalUserSpecifiedConfigNamespace},
			{Resource: "namespaces", Name: operatorclient.TargetNamespace},
			{Resource: "namespaces", Name: "openshift-kube-scheduler-operator"},
		},
		configClient.ConfigV1(),
		operatorClient,
		status.NewVersionGetter(),
		ctx.EventRecorder,
	)

	operatorConfigInformers.Start(ctx.Done())
	kubeInformersClusterScoped.Start(ctx.Done())
	kubeInformersNamespace.Start(ctx.Done())
	kubeInformersForNamespaces.Start(ctx.Done())

	go staticPodControllers.Run(ctx.Done())
	go resourceSyncController.Run(1, ctx.Done())
	go targetConfigReconciler.Run(1, ctx.Done())
	go configObserver.Run(1, ctx.Done())
	go clusterOperatorStatus.Run(1, ctx.Done())

	<-ctx.Done()
	return fmt.Errorf("stopped")
}

// deploymentConfigMaps is a list of configmaps that are directly copied for the current values.  A different actor/controller modifies these.
// the first element should be the configmap that contains the static pod manifest
var deploymentConfigMaps = []revision.RevisionResource{
	{Name: "kube-scheduler-pod"},
	{Name: "config"},
	{Name: "policy-configmap", Optional: true},
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []revision.RevisionResource{
	{Name: "scheduler-kubeconfig"},
}
