package operator

import (
	"context"
	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/key"
	"os"
	"time"
	
	"go.opentelemetry.io/otel/exporters/trace/jaeger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	configv1informers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configmetrics"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/configobservation/configobservercontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/operatorclient"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/resourcesynccontroller"
	"github.com/openshift/cluster-kube-scheduler-operator/pkg/operator/v410_00_assets"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/genericoperatorclient"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/staleconditions"
	"github.com/openshift/library-go/pkg/operator/staticpod"
	"github.com/openshift/library-go/pkg/operator/staticpod/controller/revision"
	"github.com/openshift/library-go/pkg/operator/staticresourcecontroller"
	"github.com/openshift/library-go/pkg/operator/status"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	workQueueKey = "key"
)

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}
	configClient, err := configv1client.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	// Set up opentelemetry tracing exporter and provider
	traceProvider, flush, err := jaeger.NewExportPipeline(
		jaeger.WithCollectorEndpoint("http://localhost:14268/api/traces"),
		jaeger.WithProcess(jaeger.Process{
			ServiceName: "trace-demo",
			Tags: []core.KeyValue{
				key.String("exporter", "jaeger"),
				key.Float64("float", 312.23),
			},
		}),
		jaeger.RegisterAsGlobal(),
		jaeger.WithSDK(&sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
	)
	if err != nil {
		return err
	}
	defer flush()
	if err != nil {
		return err
	}

	configInformers := configv1informers.NewSharedInformerFactory(configClient, 10*time.Minute)
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		operatorclient.GlobalUserSpecifiedConfigNamespace,
		operatorclient.GlobalMachineSpecifiedConfigNamespace,
		operatorclient.OperatorNamespace,
		operatorclient.TargetNamespace,
		"kube-system",
	)
	operatorClient, dynamicInformers, err := genericoperatorclient.NewStaticPodOperatorClient(cc.KubeConfig, operatorv1.GroupVersion.WithResource("kubeschedulers"))
	if err != nil {
		return err
	}

	resourceSyncController, err := resourcesynccontroller.NewResourceSyncController(
		operatorClient,
		kubeInformersForNamespaces,
		configInformers,
		kubeClient,
		cc.EventRecorder,
	)
	if err != nil {
		return err
	}
	configObserver := configobservercontroller.NewConfigObserver(
		operatorClient,
		kubeInformersForNamespaces,
		configInformers,
		resourceSyncController,
		cc.EventRecorder,
	)

	staticResourceController := staticresourcecontroller.NewStaticResourceController(
		"KubeControllerManagerStaticResources",
		v410_00_assets.Asset,
		[]string{
			"v4.1.0/kube-scheduler/ns.yaml",
			"v4.1.0/kube-scheduler/kubeconfig-cm.yaml",
			"v4.1.0/kube-scheduler/kubeconfig-cert-syncer.yaml",
			"v4.1.0/kube-scheduler/leader-election-rolebinding.yaml",
			"v4.1.0/kube-scheduler/scheduler-clusterrolebinding.yaml",
			"v4.1.0/kube-scheduler/policyconfigmap-role.yaml",
			"v4.1.0/kube-scheduler/policyconfigmap-rolebinding.yaml",
			"v4.1.0/kube-scheduler/svc.yaml",
			"v4.1.0/kube-scheduler/sa.yaml",
			"v4.1.0/kube-scheduler/localhost-recovery-client-crb.yaml",
			"v4.1.0/kube-scheduler/localhost-recovery-sa.yaml",
			"v4.1.0/kube-scheduler/localhost-recovery-token.yaml",
		},
		(&resourceapply.ClientHolder{}).WithKubernetes(kubeClient),
		operatorClient,
		cc.EventRecorder,
	).AddKubeInformers(kubeInformersForNamespaces)

	targetConfigReconciler := NewTargetConfigReconciler(
		ctx,
		os.Getenv("IMAGE"),
		os.Getenv("OPERATOR_IMAGE"),
		operatorClient,
		kubeInformersForNamespaces.InformersFor(operatorclient.TargetNamespace),
		kubeInformersForNamespaces,
		configInformers,
		operatorClient,
		kubeClient,
		cc.EventRecorder,
		traceProvider.Tracer("target-config-reconciler"),
	)

	// don't change any versions until we sync
	versionRecorder := status.NewVersionGetter()
	clusterOperator, err := configClient.ConfigV1().ClusterOperators().Get(ctx, "kube-scheduler-operator", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	for _, version := range clusterOperator.Status.Versions {
		versionRecorder.SetVersion(version.Name, version.Version)
	}
	versionRecorder.SetVersion("raw-internal", status.VersionForOperatorFromEnv())

	staticPodControllers, err := staticpod.NewBuilder(operatorClient, kubeClient, kubeInformersForNamespaces).
		WithEvents(cc.EventRecorder).
		WithInstaller([]string{"cluster-kube-scheduler-operator", "installer"}).
		WithPruning([]string{"cluster-kube-scheduler-operator", "prune"}, "kube-scheduler-pod").
		WithResources(operatorclient.TargetNamespace, "openshift-kube-scheduler", deploymentConfigMaps, deploymentSecrets).
		WithCerts("kube-scheduler-certs", CertConfigMaps, CertSecrets).
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
		cc.EventRecorder,
	)

	staleConditions := staleconditions.NewRemoveStaleConditionsController(
		[]string{
			// the static pod operator used to directly set these. this removes those conditions since the static pod operator was updated.
			// these can be removed in 4.5
			"Available", "Progressing",
		},
		operatorClient,
		cc.EventRecorder,
	)

	configmetrics.Register(configInformers)

	kubeInformersForNamespaces.Start(ctx.Done())
	configInformers.Start(ctx.Done())
	dynamicInformers.Start(ctx.Done())

	go staticPodControllers.Start(ctx)
	go staticResourceController.Run(ctx, 1)
	go resourceSyncController.Run(ctx, 1)
	go targetConfigReconciler.Run(1, ctx.Done())
	go configObserver.Run(ctx, 1)
	go clusterOperatorStatus.Run(ctx, 1)
	go staleConditions.Run(ctx, 1)

	<-ctx.Done()
	return nil
}

// deploymentConfigMaps is a list of configmaps that are directly copied for the current values.  A different actor/controller modifies these.
// the first element should be the configmap that contains the static pod manifest
var deploymentConfigMaps = []revision.RevisionResource{
	{Name: "kube-scheduler-pod"},
	{Name: "config"},
	{Name: "serviceaccount-ca"},
	{Name: "policy-configmap", Optional: true},

	{Name: "scheduler-kubeconfig"},
	{Name: "kube-scheduler-cert-syncer-kubeconfig"},
}

// deploymentSecrets is a list of secrets that are directly copied for the current values.  A different actor/controller modifies these.
var deploymentSecrets = []revision.RevisionResource{
	{Name: "serving-cert", Optional: true},
	{Name: "localhost-recovery-client-token"},
}

var CertConfigMaps = []revision.RevisionResource{}

var CertSecrets = []revision.RevisionResource{
	{Name: "kube-scheduler-client-cert-key"},
}
