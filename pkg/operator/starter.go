package operator

import (
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1 "github.com/openshift/api/operator/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	operatorversionedclient "github.com/openshift/client-go/operator/clientset/versioned"
	operatorv1informers "github.com/openshift/client-go/operator/informers/externalversions"
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
	operatorConfigClient, err := operatorversionedclient.NewForConfig(ctx.KubeConfig)
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
	configInformers := configv1informers.NewSharedInformerFactory(configClient, 10*time.Minute)
	operatorConfigInformers := operatorv1informers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)
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
		Client:    operatorConfigClient.OperatorV1(),
	}

	kubeInformersClusterScoped := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	kubeInformersNamespace := informers.NewSharedInformerFactoryWithOptions(kubeClient, 10*time.Minute, informers.WithNamespace(operatorclient.TargetNamespace))

	v1helpers.EnsureOperatorConfigExists(
		dynamicClient,
		v311_00_assets.MustAsset("v3.11.0/kube-scheduler/operator-config.yaml"),
		schema.GroupVersionResource{Group: operatorv1.GroupName, Version: "v1", Resource: "kubeschedulers"},
	)
	// Create scheduler config CR needed.
	schedulerConfig := &configv1.Scheduler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Scheduler",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.SchedulerSpec{
			Policy: configv1.ConfigMapNameReference{
				Name: "",
			},
		},
	}
	_, err = configClient.ConfigV1().Schedulers().Create(schedulerConfig)
	if err != nil && !errors.IsAlreadyExists(err) {
		// If the scheduler config type cluster already exists, let's proceed as usual.
		glog.Infof("creation of scheduler config custom resource failed let's not proceed further")
		return err
	}
	resourceSyncController, err := resourcesynccontroller.NewResourceSyncController(
		operatorClient,
		kubeInformersForNamespaces,
		configInformers,
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
		operatorConfigInformers.Operator().V1().KubeSchedulers(),
		kubeInformersNamespace,
		kubeInformersForNamespaces,
		configInformers,
		operatorConfigClient.OperatorV1(),
		kubeClient,
		ctx.EventRecorder,
	)

	// don't change any versions until we sync
	versionRecorder := status.NewVersionGetter()
	clusterOperator, err := configClient.ConfigV1().ClusterOperators().Get("kube-scheduler-operator", metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	for _, version := range clusterOperator.Status.Versions {
		versionRecorder.SetVersion(version.Name, version.Version)
	}
	versionRecorder.SetVersion("operator", os.Getenv("OPERATOR_IMAGE_VERSION"))

	staticPodControllers, err := staticpod.NewBuilder(operatorClient, kubeClient, kubeInformersForNamespaces).
		WithEvents(ctx.EventRecorder).
		WithInstaller([]string{"cluster-kube-scheduler-operator", "installer"}).
		WithPruning([]string{"cluster-kube-scheduler-operator", "prune"}, "kube-scheduler-pod").
		WithResources(operatorclient.TargetNamespace, "openshift-kube-scheduler", deploymentConfigMaps, deploymentSecrets).
		WithServiceMonitor(dynamicClient).
		WithVersioning(operatorclient.OperatorNamespace, "kube-scheduler", versionRecorder).
		ToControllers()
	if err != nil {
		return err
	}

	clusterOperatorStatus := status.NewClusterOperatorStatusController(
		"kube-scheduler",
		[]configv1.ObjectReference{
			{Group: "operator.openshift.io", Resource: "kubeschedulers", Name: "cluster"},
			{Resource: "namespaces", Name: operatorclient.GlobalUserSpecifiedConfigNamespace},
			{Resource: "namespaces", Name: operatorclient.TargetNamespace},
			{Resource: "namespaces", Name: "openshift-kube-scheduler-operator"},
		},
		configClient.ConfigV1(),
		configInformers.Config().V1().ClusterOperators(),
		operatorClient,
		versionRecorder,
		ctx.EventRecorder,
	)

	operatorConfigInformers.Start(ctx.Done())
	kubeInformersClusterScoped.Start(ctx.Done())
	kubeInformersNamespace.Start(ctx.Done())
	kubeInformersForNamespaces.Start(ctx.Done())
	configInformers.Start(ctx.Done())

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
	{Name: "scheduler-kubeconfig"},
	{Name: "serviceaccount-ca"},
	{Name: "policy-configmap", Optional: true},
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []revision.RevisionResource{
	{Name: "kube-scheduler-client-cert-key"},
}
